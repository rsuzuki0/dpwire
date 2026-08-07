package transport

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"net"
	"time"
)

// InspectedCertificate is untrusted first-contact material. A user must verify
// its fingerprint out of band before storing it in a profile.
type InspectedCertificate struct {
	PEM               []byte
	SHA256            string
	SubjectCommonName string
}

// InspectUntrustedCertificate performs a TLS handshake without sending an HTTP
// request or credentials. Its result is untrusted until explicitly confirmed.
func InspectUntrustedCertificate(ctx context.Context, address string) (InspectedCertificate, error) {
	base, err := baseURL(address)
	if err != nil {
		return InspectedCertificate{}, err
	}
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 15 * time.Second},
		Config: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true,
		},
	}
	connection, err := dialer.DialContext(ctx, "tcp", base.Host)
	if err != nil {
		return InspectedCertificate{}, err
	}
	defer connection.Close()
	tlsConnection := connection.(*tls.Conn)
	state := tlsConnection.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return InspectedCertificate{}, errors.New("dpwire: peer sent no certificate")
	}
	certificate := state.PeerCertificates[0]
	sum := sha256.Sum256(certificate.Raw)
	return InspectedCertificate{
		PEM:    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}),
		SHA256: hex.EncodeToString(sum[:]), SubjectCommonName: certificate.Subject.CommonName,
	}, nil
}
