// Package profiles manages named Digital Paper connection profiles.
package profiles

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/rsuzuki0/digitalpaper"
	"github.com/rsuzuki0/digitalpaper/credentials"
	"github.com/rsuzuki0/digitalpaper/internal/atomicfile"
	"github.com/rsuzuki0/digitalpaper/pairing"
)

const maxConfigSize = 1 << 20

// Manager stores profiles below one explicit configuration root.
type Manager struct{ root string }

// Summary is safe to display and excludes client IDs and key paths.
type Summary struct {
	Name       string                      `json:"name"`
	Address    string                      `json:"address"`
	Connection digitalpaper.ConnectionMode `json:"connection"`
	Current    bool                        `json:"current"`
}

type configuration struct {
	DefaultProfile string `json:"default_profile"`
}

// DefaultRoot returns the platform-standard private application directory.
func DefaultRoot() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "digitalpaper"), nil
}

// New constructs a manager rooted at directory.
func New(directory string) (*Manager, error) {
	if directory == "" || !filepath.IsAbs(directory) {
		return nil, errors.New("profiles: configuration directory must be absolute")
	}
	return &Manager{root: filepath.Clean(directory)}, nil
}

// ImportSony validates and copies one existing Sony credential pair into a
// newly named profile. Existing profiles are never overwritten.
func (m *Manager) ImportSony(name, address, fingerprint, credentialDirectory string) (digitalpaper.DeviceProfile, error) {
	if err := validateName(name); err != nil {
		return digitalpaper.DeviceProfile{}, err
	}
	creds, err := credentials.ImportSony(filepath.Join(credentialDirectory, "deviceid.dat"), filepath.Join(credentialDirectory, "privatekey.dat"))
	if err != nil {
		return digitalpaper.DeviceProfile{}, err
	}
	profile := digitalpaper.DeviceProfile{
		Name: name, Address: address, Connection: digitalpaper.InferConnectionMode(address), ClientID: creds.ClientID,
		PrivateKeyRef: "privatekey.pem", CertificateSHA256: fingerprint,
	}
	if _, err := digitalpaper.NewClient(profile, digitalpaper.WithCredentials(creds)); err != nil {
		return digitalpaper.DeviceProfile{}, fmt.Errorf("profiles: validate connection settings: %w", err)
	}
	if err := m.ensureRoot(); err != nil {
		return digitalpaper.DeviceProfile{}, err
	}
	profileDirectory := m.profileDirectory(name)
	if err := os.Mkdir(profileDirectory, 0o700); err != nil {
		return digitalpaper.DeviceProfile{}, fmt.Errorf("profiles: create %q: %w", name, err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(profileDirectory)
		}
	}()
	if err := atomicfile.WriteNew(filepath.Join(profileDirectory, "privatekey.pem"), creds.PrivateKeyPEM, 0o600); err != nil {
		return digitalpaper.DeviceProfile{}, err
	}
	if err := digitalpaper.SaveProfile(filepath.Join(profileDirectory, "profile.json"), profile); err != nil {
		return digitalpaper.DeviceProfile{}, err
	}
	if _, err := m.defaultName(); errors.Is(err, os.ErrNotExist) {
		if err := m.Use(name); err != nil {
			return digitalpaper.DeviceProfile{}, err
		}
	} else if err != nil {
		return digitalpaper.DeviceProfile{}, err
	}
	complete = true
	return m.Load(name)
}

// Pair performs fresh direct registration and stores the resulting identity in
// a new owner-private profile. An existing profile is never overwritten.
func (m *Manager) Pair(ctx context.Context, name, address string, providePIN pairing.PINProvider) (digitalpaper.DeviceProfile, error) {
	if err := validateName(name); err != nil {
		return digitalpaper.DeviceProfile{}, err
	}
	if err := m.ensureRoot(); err != nil {
		return digitalpaper.DeviceProfile{}, err
	}
	profileDirectory := m.profileDirectory(name)
	if err := os.Mkdir(profileDirectory, 0o700); err != nil {
		return digitalpaper.DeviceProfile{}, fmt.Errorf("profiles: create %q: %w", name, err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(profileDirectory)
		}
	}()
	result, err := pairing.Register(ctx, address, providePIN)
	if err != nil {
		return digitalpaper.DeviceProfile{}, err
	}
	profile := digitalpaper.DeviceProfile{
		Name: name, Address: result.Address, Connection: digitalpaper.ConnectionDirect,
		ClientID: result.Credentials.ClientID, PrivateKeyRef: "privatekey.pem", DeviceCAPEM: result.DeviceCAPEM,
		CertificateSHA256: result.CertificateSHA256,
	}
	if _, err := digitalpaper.NewClient(profile, digitalpaper.WithCredentials(result.Credentials)); err != nil {
		return digitalpaper.DeviceProfile{}, fmt.Errorf("profiles: validate paired identity: %w", err)
	}
	if err := atomicfile.WriteNew(filepath.Join(profileDirectory, "privatekey.pem"), result.Credentials.PrivateKeyPEM, 0o600); err != nil {
		return digitalpaper.DeviceProfile{}, err
	}
	if err := digitalpaper.SaveProfile(filepath.Join(profileDirectory, "profile.json"), profile); err != nil {
		return digitalpaper.DeviceProfile{}, err
	}
	if _, err := m.defaultName(); errors.Is(err, os.ErrNotExist) {
		if err := m.Use(name); err != nil {
			return digitalpaper.DeviceProfile{}, err
		}
	} else if err != nil {
		return digitalpaper.DeviceProfile{}, err
	}
	complete = true
	return m.Load(name)
}

// Load reads one named profile.
func (m *Manager) Load(name string) (digitalpaper.DeviceProfile, error) {
	if err := validateName(name); err != nil {
		return digitalpaper.DeviceProfile{}, err
	}
	return digitalpaper.LoadProfile(filepath.Join(m.profileDirectory(name), "profile.json"))
}

// Current loads the selected default profile.
func (m *Manager) Current() (string, digitalpaper.DeviceProfile, error) {
	name, err := m.defaultName()
	if err != nil {
		return "", digitalpaper.DeviceProfile{}, err
	}
	profile, err := m.Load(name)
	return name, profile, err
}

// Use selects an existing named profile as the default.
func (m *Manager) Use(name string) error {
	if _, err := m.Load(name); err != nil {
		return err
	}
	if err := m.ensureRoot(); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(configuration{DefaultProfile: name}, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Replace(filepath.Join(m.root, "config.json"), append(encoded, '\n'), 0o600)
}

// List returns display-safe profiles sorted by name.
func (m *Manager) List() ([]Summary, error) {
	current, currentErr := m.defaultName()
	if currentErr != nil && !errors.Is(currentErr, os.ErrNotExist) {
		return nil, currentErr
	}
	entries, err := os.ReadDir(filepath.Join(m.root, "profiles"))
	if errors.Is(err, os.ErrNotExist) {
		return []Summary{}, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || validateName(entry.Name()) != nil {
			continue
		}
		profile, err := m.Load(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("profiles: load %q: %w", entry.Name(), err)
		}
		items = append(items, Summary{Name: entry.Name(), Address: profile.Address, Connection: profile.EffectiveConnection(), Current: entry.Name() == current})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (m *Manager) ensureRoot() error {
	for _, directory := range []string{m.root, filepath.Join(m.root, "profiles")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return err
		}
		info, err := os.Lstat(directory)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("profiles: configuration path is not a directory")
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) defaultName() (string, error) {
	path := filepath.Join(m.root, "config.json")
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > maxConfigSize {
		return "", errors.New("profiles: configuration exceeds size limit")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var config configuration
	if err := decoder.Decode(&config); err != nil {
		return "", err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", errors.New("profiles: configuration contains trailing data")
	}
	if err := validateName(config.DefaultProfile); err != nil {
		return "", err
	}
	return config.DefaultProfile, nil
}

func (m *Manager) profileDirectory(name string) string {
	return filepath.Join(m.root, "profiles", name)
}

func validateName(name string) error {
	if name == "" || name == "." || name == ".." || len(name) > 64 {
		return errors.New("profiles: invalid profile name")
	}
	for _, value := range name {
		if !(unicode.IsLetter(value) || unicode.IsDigit(value) || strings.ContainsRune("._-", value)) {
			return errors.New("profiles: profile name may contain only letters, numbers, dot, underscore, and hyphen")
		}
	}
	return nil
}
