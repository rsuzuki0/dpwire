package auth

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/rsuzuki0/dpwire/internal/wire/transport"
)

// Authenticate signs a fresh device nonce and installs the returned session
// credential in the transport.
func Authenticate(ctx context.Context, client *transport.Client, clientID string, privateKey *rsa.PrivateKey) error {
	if clientID == "" || privateKey == nil {
		return errors.New("dpwire: authentication credentials are incomplete")
	}
	var nonceResponse struct {
		Nonce string `json:"nonce"`
	}
	endpoint := "/auth/nonce/" + url.PathEscape(clientID)
	if err := client.DoJSON(ctx, http.MethodGet, endpoint, nil, nil, &nonceResponse, false); err != nil {
		return err
	}
	if nonceResponse.Nonce == "" {
		return errors.New("dpwire: device returned an empty nonce")
	}
	digest := sha256.Sum256([]byte(nonceResponse.Nonce))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return fmt.Errorf("dpwire: sign nonce: %w", err)
	}
	payload, err := json.Marshal(map[string]string{
		"client_id": clientID, "nonce_signed": base64.StdEncoding.EncodeToString(signature),
	})
	if err != nil {
		return err
	}
	response, err := client.Do(ctx, http.MethodPut, "/auth", nil, bytes.NewReader(payload), false)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	credential, err := ParseCredentialsCookie(response.Header.Values("Set-Cookie"))
	if err != nil {
		return err
	}
	return client.SetCredential(credential)
}

// ParseCredentialsCookie handles the non-standard cookie form used by known
// devices while preserving base64 padding in the value.
func ParseCredentialsCookie(headers []string) (string, error) {
	var found string
	for _, header := range headers {
		pair := strings.TrimSpace(strings.SplitN(header, ";", 2)[0])
		name, value, ok := strings.Cut(pair, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "Credentials") {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" || strings.ContainsAny(value, ";\r\n\x00") {
			return "", errors.New("dpwire: malformed Credentials cookie")
		}
		if found != "" && found != value {
			return "", errors.New("dpwire: conflicting Credentials cookies")
		}
		found = value
	}
	if found == "" {
		return "", errors.New("dpwire: authentication response has no Credentials cookie")
	}
	return found, nil
}
