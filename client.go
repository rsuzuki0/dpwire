package digitalpaper

import (
	"context"
	"crypto/rsa"
	"errors"
	"time"

	"github.com/rsuzuki0/digitalpaper/credentials"
	"github.com/rsuzuki0/digitalpaper/internal/wire/auth"
	"github.com/rsuzuki0/digitalpaper/internal/wire/transport"
)

// Client is a concurrency-safe, authenticated Digital Paper API client.
// Authenticate must succeed before any service request is made.
type Client struct {
	wire       *transport.Client
	clientID   string
	privateKey *rsa.PrivateKey

	Documents *DocumentsService
	Folders   *FoldersService
	Device    *DeviceService
}

type clientOptions struct {
	credentials *credentials.Credentials
	timeout     time.Duration
}

// Option customizes a Client.
type Option func(*clientOptions) error

// WithCredentials supplies credentials directly instead of reading
// DeviceProfile.PrivateKeyRef.
func WithCredentials(value credentials.Credentials) Option {
	return func(options *clientOptions) error {
		if value.ClientID == "" || len(value.PrivateKeyPEM) == 0 {
			return errors.New("digitalpaper: credentials are incomplete")
		}
		copy := value
		copy.PrivateKeyPEM = append([]byte(nil), value.PrivateKeyPEM...)
		options.credentials = &copy
		return nil
	}
}

// WithTimeout sets the complete HTTP request timeout.
func WithTimeout(value time.Duration) Option {
	return func(options *clientOptions) error {
		if value <= 0 {
			return errors.New("digitalpaper: timeout must be positive")
		}
		options.timeout = value
		return nil
	}
}

// NewClient creates a client with explicit per-device TLS trust.
func NewClient(profile DeviceProfile, options ...Option) (*Client, error) {
	if err := profile.validate(); err != nil {
		return nil, err
	}
	settings := clientOptions{timeout: 90 * time.Second}
	for _, option := range options {
		if option == nil {
			return nil, errors.New("digitalpaper: nil client option")
		}
		if err := option(&settings); err != nil {
			return nil, err
		}
	}
	wire, err := transport.New(profile.Address, transport.TrustConfig{
		CAPEM: profile.DeviceCAPEM, CertificateSHA256: profile.CertificateSHA256,
	}, settings.timeout)
	if err != nil {
		return nil, err
	}
	clientID := profile.ClientID
	var privateKey *rsa.PrivateKey
	if settings.credentials != nil {
		clientID = settings.credentials.ClientID
		privateKey, err = credentials.ParseRSAPrivateKey(settings.credentials.PrivateKeyPEM)
	} else if profile.PrivateKeyRef != "" {
		privateKey, err = credentials.LoadPrivateKey(profile.PrivateKeyRef)
	} else {
		err = errors.New("digitalpaper: profile has no private key reference")
	}
	if err != nil {
		return nil, err
	}
	client := &Client{wire: wire, clientID: clientID, privateKey: privateKey}
	client.Documents = &DocumentsService{client: client}
	client.Folders = &FoldersService{client: client}
	client.Device = &DeviceService{client: client}
	return client, nil
}

// Authenticate performs nonce-signature authentication for a fresh session.
func (c *Client) Authenticate(ctx context.Context) error {
	return publicError(auth.Authenticate(ctx, c.wire, c.clientID, c.privateKey))
}

// InspectedCertificate is untrusted first-contact certificate material.
type InspectedCertificate = transport.InspectedCertificate

// InspectUntrustedCertificate obtains a device certificate without sending
// credentials or an HTTP request. The fingerprint must be verified by a user.
func InspectUntrustedCertificate(ctx context.Context, address string) (InspectedCertificate, error) {
	return transport.InspectUntrustedCertificate(ctx, address)
}
