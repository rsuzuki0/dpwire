// Package pairing provides fresh Digital Paper client registration without a
// vendor application.
package pairing

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/rsuzuki0/digitalpaper/credentials"
	wireregistration "github.com/rsuzuki0/digitalpaper/internal/wire/registration"
)

// PINProvider obtains the PIN displayed by the device.
type PINProvider func(context.Context) (string, error)

// Result contains a new client identity, device trust anchor, and direct API
// address. No session cookie is retained.
type Result struct {
	Address           string
	DeviceCAPEM       []byte
	CertificateSHA256 string
	Credentials       credentials.Credentials
}

// Register performs fresh registration against a direct device address.
func Register(ctx context.Context, address string, providePIN PINProvider) (Result, error) {
	apiAddress, err := APIAddress(address)
	if err != nil {
		return Result{}, err
	}
	client, err := wireregistration.New(address, 90*time.Second)
	if err != nil {
		return Result{}, err
	}
	wireResult, err := client.Register(ctx, wireregistration.PINProvider(providePIN))
	if err != nil {
		return Result{}, err
	}
	fingerprint, err := certificateFingerprint(wireResult.DeviceCAPEM)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Address: apiAddress, DeviceCAPEM: wireResult.DeviceCAPEM, CertificateSHA256: fingerprint,
		Credentials: credentials.Credentials{ClientID: wireResult.ClientID, PrivateKeyPEM: wireResult.PrivateKeyPEM},
	}, nil
}

func certificateFingerprint(certificatePEM []byte) (string, error) {
	block, _ := pem.Decode(certificatePEM)
	if block == nil {
		return "", errors.New("pairing: registered device certificate is invalid")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", errors.New("pairing: registered device certificate is invalid")
	}
	fingerprint := sha256.Sum256(certificate.Raw)
	return hex.EncodeToString(fingerprint[:]), nil
}

// HTTPError is a device registration endpoint error.
type HTTPError = wireregistration.HTTPError

// APIAddress normalizes a host, registration URL, or API URL to the direct
// authenticated HTTPS endpoint on port 8443.
func APIAddress(address string) (string, error) {
	if !strings.Contains(address, "://") {
		address = "https://" + address
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", errors.New("pairing: invalid direct device address")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("pairing: address must use HTTP or HTTPS")
	}
	host := parsed.Hostname()
	if host == "localhost" || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()) {
		return "", errors.New("pairing: fresh pairing requires a direct, non-loopback device address")
	}
	parsed.Scheme = "https"
	parsed.Host = net.JoinHostPort(host, "8443")
	parsed.Path = ""
	return parsed.String(), nil
}
