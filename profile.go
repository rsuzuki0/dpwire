package digitalpaper

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rsuzuki0/digitalpaper/internal/atomicfile"
)

const maxProfileSize = 1 << 20

// DeviceProfile contains non-session connection configuration. DeviceCAPEM is
// encoded as base64 by JSON; private key material remains in a separate file.
type DeviceProfile struct {
	Name              string `json:"name"`
	Address           string `json:"address"`
	Model             string `json:"model,omitempty"`
	Firmware          string `json:"firmware,omitempty"`
	ClientID          string `json:"client_id"`
	PrivateKeyRef     string `json:"private_key_ref"`
	DeviceCAPEM       []byte `json:"device_ca_pem,omitempty"`
	CertificateSHA256 string `json:"certificate_sha256,omitempty"`
}

// LoadProfile reads one strict JSON profile.
func LoadProfile(path string) (DeviceProfile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return DeviceProfile{}, err
	}
	if info.Size() > maxProfileSize {
		return DeviceProfile{}, errors.New("digitalpaper: profile exceeds size limit")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return DeviceProfile{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var profile DeviceProfile
	if err := decoder.Decode(&profile); err != nil {
		return DeviceProfile{}, fmt.Errorf("digitalpaper: decode profile: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return DeviceProfile{}, errors.New("digitalpaper: profile contains trailing data")
	}
	if err := profile.validate(); err != nil {
		return DeviceProfile{}, err
	}
	if profile.PrivateKeyRef != "" && !filepath.IsAbs(profile.PrivateKeyRef) {
		profile.PrivateKeyRef = filepath.Join(filepath.Dir(path), profile.PrivateKeyRef)
	}
	return profile, nil
}

// SaveProfile writes a profile with owner-only permissions and refuses to
// overwrite an existing file.
func SaveProfile(path string, profile DeviceProfile) error {
	if err := profile.validate(); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return atomicfile.WriteNew(path, encoded, 0o600)
}

func (p DeviceProfile) validate() error {
	if p.Name == "" || p.Address == "" || p.ClientID == "" {
		return errors.New("digitalpaper: profile name, address, and client_id are required")
	}
	if len(p.DeviceCAPEM) == 0 && p.CertificateSHA256 == "" {
		return errors.New("digitalpaper: profile requires a device CA or certificate fingerprint")
	}
	return nil
}
