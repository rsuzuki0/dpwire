// Package credentials imports and validates existing Digital Paper client
// credentials without performing device registration.
package credentials

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxCredentialFile = 1 << 20

// Credentials contains an existing client identity. It deliberately contains
// no session cookie.
type Credentials struct {
	ClientID      string
	PrivateKeyPEM []byte
}

// SonyCandidate identifies a colocated deviceid.dat/privatekey.dat pair.
type SonyCandidate struct {
	Directory    string
	ClientIDPath string
	KeyPath      string
}

// FindSonyCandidates finds unambiguous credential pairs below root. The caller
// must select a candidate; this function never silently chooses the first.
func FindSonyCandidates(root string) ([]SonyCandidate, error) {
	directories := make(map[string]uint8)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		switch entry.Name() {
		case "deviceid.dat":
			directories[filepath.Dir(path)] |= 1
		case "privatekey.dat":
			directories[filepath.Dir(path)] |= 2
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var candidates []SonyCandidate
	for directory, mask := range directories {
		if mask != 3 {
			continue
		}
		candidates = append(candidates, SonyCandidate{
			Directory: directory, ClientIDPath: filepath.Join(directory, "deviceid.dat"), KeyPath: filepath.Join(directory, "privatekey.dat"),
		})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Directory < candidates[j].Directory })
	return candidates, nil
}

// ImportSony reads and validates one explicitly selected credential pair.
func ImportSony(clientIDPath, keyPath string) (Credentials, error) {
	clientRaw, err := readSmallRegular(clientIDPath)
	if err != nil {
		return Credentials{}, fmt.Errorf("credentials: client ID: %w", err)
	}
	keyRaw, err := readSmallRegular(keyPath)
	if err != nil {
		return Credentials{}, fmt.Errorf("credentials: private key: %w", err)
	}
	clientID := strings.TrimSpace(string(clientRaw))
	if clientID == "" || strings.ContainsAny(clientID, "\r\n\x00") {
		return Credentials{}, errors.New("credentials: invalid client ID")
	}
	if _, err := ParseRSAPrivateKey(keyRaw); err != nil {
		return Credentials{}, err
	}
	return Credentials{ClientID: clientID, PrivateKeyPEM: append([]byte(nil), keyRaw...)}, nil
}

// LoadPrivateKey reads and validates one private key from a regular file.
func LoadPrivateKey(path string) (*rsa.PrivateKey, error) {
	raw, err := readSmallRegular(path)
	if err != nil {
		return nil, fmt.Errorf("credentials: private key: %w", err)
	}
	return ParseRSAPrivateKey(raw)
}

// ParseRSAPrivateKey accepts PKCS#1 and PKCS#8 PEM keys used by known clients.
func ParseRSAPrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, rest := pem.Decode(data)
	if block == nil || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, errors.New("credentials: invalid private key PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		if err := key.Validate(); err != nil {
			return nil, fmt.Errorf("credentials: invalid RSA key: %w", err)
		}
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("credentials: parse private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("credentials: private key is not RSA")
	}
	if err := key.Validate(); err != nil {
		return nil, fmt.Errorf("credentials: invalid RSA key: %w", err)
	}
	return key, nil
}

func readSmallRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxCredentialFile {
		return nil, errors.New("not a small regular file")
	}
	return os.ReadFile(path)
}
