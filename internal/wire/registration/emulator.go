package registration

import (
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
)

// Emulator implements the registration server state machine for dptest. It is
// not used by production clients.
type Emulator struct {
	mu             sync.Mutex
	pin            string
	certificate    []byte
	random         io.Reader
	onRegistration func(string, *rsa.PublicKey)
	transcript     emulatorTranscript
}

type emulatorTranscript struct {
	dh                                          *dhExchange
	nonce1, mac, otherRaw, nonce2, clientPublic []byte
	authKey, wrapKey, m2hmac, eHash, m3hmac     []byte
	es, wrappedCertificate, m5hmac              []byte
}

// NewEmulator constructs a registration emulator with one device certificate.
func NewEmulator(pin string, certificate []byte, random io.Reader, onRegistration func(string, *rsa.PublicKey)) (*Emulator, error) {
	if pin == "" || strings.ContainsAny(pin, "\r\n\x00") || random == nil {
		return nil, errors.New("registration emulator: invalid configuration")
	}
	if err := validateCertificatePEM(certificate); err != nil {
		return nil, err
	}
	return &Emulator{pin: pin, certificate: append([]byte(nil), certificate...), random: random, onRegistration: onRegistration}, nil
}

// SetPIN changes the PIN used for subsequent sequences.
func (e *Emulator) SetPIN(pin string) error {
	if pin == "" || strings.ContainsAny(pin, "\r\n\x00") {
		return errors.New("registration emulator: invalid PIN")
	}
	e.mu.Lock()
	e.pin = pin
	e.transcript = emulatorTranscript{}
	e.mu.Unlock()
	return nil
}

// ServeHTTP handles only registration endpoints.
func (e *Emulator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	defer e.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch r.Method + " " + r.URL.Path {
	case "PUT /register/cleanup":
		e.transcript = emulatorTranscript{}
		w.WriteHeader(http.StatusNoContent)
	case "POST /register/pin":
		e.servePIN(w)
	case "POST /register/hash":
		e.serveHash(w, r)
	case "POST /register/ca":
		e.serveCA(w, r)
	case "POST /register":
		e.serveRegistration(w, r)
	default:
		writeEmulatorError(w, http.StatusNotFound, "40401", "registration endpoint not found")
	}
}

func (e *Emulator) servePIN(w http.ResponseWriter) {
	nonce1 := make([]byte, 16)
	mac := make([]byte, 6)
	es := make([]byte, 16)
	if _, err := io.ReadFull(e.random, nonce1); err != nil {
		writeEmulatorError(w, http.StatusInternalServerError, "RANDOM_FAILED", "randomness failed")
		return
	}
	if _, err := io.ReadFull(e.random, mac); err != nil {
		writeEmulatorError(w, http.StatusInternalServerError, "RANDOM_FAILED", "randomness failed")
		return
	}
	if _, err := io.ReadFull(e.random, es); err != nil {
		writeEmulatorError(w, http.StatusInternalServerError, "RANDOM_FAILED", "randomness failed")
		return
	}
	dh, err := newDH(e.random)
	if err != nil {
		writeEmulatorError(w, http.StatusInternalServerError, "RANDOM_FAILED", "randomness failed")
		return
	}
	e.transcript = emulatorTranscript{dh: dh, nonce1: nonce1, mac: mac, otherRaw: dh.publicBytes(), es: es}
	_ = json.NewEncoder(w).Encode(wireMessage{A: encode(nonce1), B: encode(mac), C: encode(e.transcript.otherRaw)})
}

func (e *Emulator) serveHash(w http.ResponseWriter, r *http.Request) {
	dh := e.transcript.dh
	if dh == nil || len(e.transcript.nonce1) == 0 {
		writeEmulatorError(w, http.StatusForbidden, "40305", "registration sequence expired")
		return
	}
	var m2 wireMessage
	if !decodeEmulatorMessage(w, r, &m2) {
		return
	}
	nonce1, err1 := decodeField("M2.a", m2.A, 16)
	nonce2, err2 := decodeField("M2.b", m2.B, 16)
	mac, err3 := decodeField("M2.c", m2.C, 6)
	clientPublic, err4 := decodeField("M2.d", m2.D, 257)
	m2hmac, err5 := decodeField("M2.e", m2.E, 32)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || subtle.ConstantTimeCompare(nonce1, e.transcript.nonce1) != 1 || subtle.ConstantTimeCompare(mac, e.transcript.mac) != 1 {
		writeEmulatorError(w, http.StatusBadRequest, "40003", "invalid registration message")
		return
	}
	shared, err := dh.sharedBytes(clientPublic)
	if err != nil {
		writeEmulatorError(w, http.StatusForbidden, "40301", "bad parameters")
		return
	}
	authKey, wrapKey := deriveKeys(shared, join(nonce1, mac, nonce2))
	wantM2 := hashMAC(authKey, nonce1, mac, e.transcript.otherRaw, nonce1, nonce2, mac, clientPublic)
	if subtle.ConstantTimeCompare(m2hmac, wantM2) != 1 {
		writeEmulatorError(w, http.StatusForbidden, "40301", "bad parameters")
		return
	}
	psk := hashMAC(authKey, []byte(e.pin))
	eHash := hashMAC(authKey, e.transcript.es, psk, e.transcript.otherRaw, clientPublic)
	m3hmac := hashMAC(authKey, nonce1, nonce2, mac, clientPublic, m2hmac, nonce2, eHash)
	e.transcript.nonce2, e.transcript.clientPublic = nonce2, clientPublic
	e.transcript.authKey, e.transcript.wrapKey, e.transcript.m2hmac = authKey, wrapKey, m2hmac
	e.transcript.eHash, e.transcript.m3hmac = eHash, m3hmac
	_ = json.NewEncoder(w).Encode(wireMessage{A: encode(nonce2), B: encode(eHash), E: encode(m3hmac)})
}

func (e *Emulator) serveCA(w http.ResponseWriter, r *http.Request) {
	t := &e.transcript
	if len(t.authKey) == 0 {
		writeEmulatorError(w, http.StatusForbidden, "40305", "registration sequence expired")
		return
	}
	var m4 wireMessage
	if !decodeEmulatorMessage(w, r, &m4) {
		return
	}
	nonce1, err1 := decodeField("M4.a", m4.A, 16)
	rHash, err2 := decodeField("M4.b", m4.B, 32)
	wrappedRS, err3 := decodeFieldRange("M4.d", m4.D, 32, 128)
	m4hmac, err4 := decodeField("M4.e", m4.E, 32)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || subtle.ConstantTimeCompare(nonce1, t.nonce1) != 1 {
		writeEmulatorError(w, http.StatusBadRequest, "40003", "invalid registration message")
		return
	}
	wantM4 := hashMAC(t.authKey, t.nonce2, t.eHash, t.m3hmac, t.nonce1, rHash, wrappedRS)
	rs, unwrapErr := unwrap(wrappedRS, t.authKey, t.wrapKey)
	psk := hashMAC(t.authKey, []byte(e.pin))
	wantRHash := hashMAC(t.authKey, rs, psk, t.otherRaw, t.clientPublic)
	if unwrapErr != nil || subtle.ConstantTimeCompare(m4hmac, wantM4) != 1 || subtle.ConstantTimeCompare(rHash, wantRHash) != 1 {
		writeEmulatorError(w, http.StatusForbidden, "40301", "PIN or parameters rejected")
		return
	}
	wrappedCertificate, err := wrap(append(append([]byte(nil), t.es...), e.certificate...), t.authKey, t.wrapKey, e.random)
	if err != nil {
		writeEmulatorError(w, http.StatusInternalServerError, "RANDOM_FAILED", "randomness failed")
		return
	}
	m5hmac := hashMAC(t.authKey, t.nonce1, rHash, wrappedRS, m4hmac, t.nonce2, wrappedCertificate)
	t.wrappedCertificate, t.m5hmac = wrappedCertificate, m5hmac
	_ = json.NewEncoder(w).Encode(wireMessage{A: encode(t.nonce2), D: encode(wrappedCertificate), E: encode(m5hmac)})
}

func (e *Emulator) serveRegistration(w http.ResponseWriter, r *http.Request) {
	t := &e.transcript
	if len(t.wrappedCertificate) == 0 {
		writeEmulatorError(w, http.StatusForbidden, "40305", "registration sequence expired")
		return
	}
	var m6 wireMessage
	if !decodeEmulatorMessage(w, r, &m6) {
		return
	}
	nonce1, err1 := decodeField("M6.a", m6.A, 16)
	wrappedIdentity, err2 := decodeFieldRange("M6.d", m6.D, 64, maxResponseSize/2)
	m6hmac, err3 := decodeField("M6.e", m6.E, 32)
	wantM6 := hashMAC(t.authKey, t.nonce2, t.wrappedCertificate, t.m5hmac, t.nonce1, wrappedIdentity)
	if err1 != nil || err2 != nil || err3 != nil || subtle.ConstantTimeCompare(nonce1, t.nonce1) != 1 || subtle.ConstantTimeCompare(m6hmac, wantM6) != 1 {
		writeEmulatorError(w, http.StatusForbidden, "40301", "bad parameters")
		return
	}
	identity, err := unwrap(wrappedIdentity, t.authKey, t.wrapKey)
	if err != nil || len(identity) <= 36 {
		writeEmulatorError(w, http.StatusForbidden, "40301", "invalid client identity")
		return
	}
	clientID := string(identity[:36])
	block, rest := pem.Decode(identity[36:])
	if block == nil || block.Type != "PUBLIC KEY" || len(strings.TrimSpace(string(rest))) != 0 {
		writeEmulatorError(w, http.StatusForbidden, "40301", "invalid client public key")
		return
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	publicKey, ok := parsed.(*rsa.PublicKey)
	if err != nil || !ok || publicKey.N.BitLen() != 2048 || publicKey.E != 65537 {
		writeEmulatorError(w, http.StatusForbidden, "40301", "invalid client public key")
		return
	}
	if e.onRegistration != nil {
		e.onRegistration(clientID, publicKey)
	}
	t.dh = nil
	w.WriteHeader(http.StatusNoContent)
}

func decodeEmulatorMessage(w http.ResponseWriter, r *http.Request, target *wireMessage) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxResponseSize+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeEmulatorError(w, http.StatusBadRequest, "40000", "invalid JSON")
		return false
	}
	return true
}

func writeEmulatorError(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error_code": code, "message": message})
}
