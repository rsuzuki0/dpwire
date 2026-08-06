package dptest

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

// Simulator is an in-process HTTPS Digital Paper protocol simulator.
type Simulator struct {
	State  *State
	server *httptest.Server
}

// Start starts a loopback TLS simulator.
func Start(state *State) *Simulator {
	if state == nil {
		state = NewState("DP-SIM", "0.0-p0")
	}
	sim := &Simulator{State: state}
	sim.server = httptest.NewTLSServer(http.HandlerFunc(sim.serveHTTP))
	return sim
}

// URL returns the simulator base URL.
func (s *Simulator) URL() string { return s.server.URL }

// Client returns an HTTP client that validates and trusts this simulator's
// certificate only. It never enables InsecureSkipVerify.
func (s *Simulator) Client() *http.Client {
	pool := x509.NewCertPool()
	pool.AddCert(s.server.Certificate())
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    pool,
		}},
	}
}

// DeviceCAPEM returns the simulator's ephemeral certificate as PEM.
func (s *Simulator) DeviceCAPEM() []byte {
	cert := s.server.Certificate()
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}

// CertificateSHA256 returns the TLS certificate fingerprint.
func (s *Simulator) CertificateSHA256() string {
	sum := sha256.Sum256(s.server.Certificate().Raw)
	return hex.EncodeToString(sum[:])
}

// Close stops the simulator.
func (s *Simulator) Close() { s.server.Close() }

func (s *Simulator) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if fault, ok := s.State.takeFault(r.Method + " " + r.URL.Path); ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(fault.Status)
		_, _ = w.Write([]byte(fault.Body))
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/ping":
		writeJSON(w, http.StatusOK, map[string]string{"value": "pong"})
	case r.Method == http.MethodGet && r.URL.Path == "/api_version":
		writeJSON(w, http.StatusOK, map[string]string{"value": "0.6.0"})
	case r.Method == http.MethodGet && r.URL.Path == "/system/status/firmware_version":
		model, firmware := s.State.Device()
		writeJSON(w, http.StatusOK, map[string]string{"value": firmware, "model_name": model})
	case r.Method == http.MethodGet && r.URL.Path == "/documents2":
		docs := s.State.Documents()
		encoded, _ := json.Marshal(docs)
		hash := sha256.Sum256(encoded)
		writeJSON(w, http.StatusOK, struct {
			Count int        `json:"count"`
			Hash  string     `json:"entry_list_hash"`
			Docs  []Document `json:"entry_list"`
		}{len(docs), hex.EncodeToString(hash[:]), docs})
	case strings.HasPrefix(r.URL.Path, "/register"):
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"code":    "P0_PAIRING_RESERVED",
			"message": "registration is reserved for phase P3",
		})
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"code": "NOT_FOUND"})
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
