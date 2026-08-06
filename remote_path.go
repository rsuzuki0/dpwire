package digitalpaper

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// RemotePath is a normalized path in a Digital Paper document tree. It is not
// an operating-system path and must never be passed to filepath functions.
type RemotePath struct{ normalized string }

// ParseRemotePath validates and NFC-normalizes a device path.
func ParseRemotePath(value string) (RemotePath, error) {
	if strings.ContainsRune(value, 0) {
		return RemotePath{}, errors.New("digitalpaper: remote path contains NUL")
	}
	if strings.Contains(value, "\\") {
		return RemotePath{}, errors.New("digitalpaper: remote path contains backslash")
	}
	value = norm.NFC.String(value)
	if value == "" || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
		return RemotePath{}, fmt.Errorf("digitalpaper: invalid remote path %q", value)
	}
	segments := strings.Split(value, "/")
	if segments[0] != "Document" {
		return RemotePath{}, errors.New("digitalpaper: remote path must start with Document")
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return RemotePath{}, fmt.Errorf("digitalpaper: invalid remote path segment %q", segment)
		}
	}
	return RemotePath{normalized: value}, nil
}

// MustRemotePath parses value and panics on error. It is intended for static
// declarations and tests.
func MustRemotePath(value string) RemotePath {
	path, err := ParseRemotePath(value)
	if err != nil {
		panic(err)
	}
	return path
}

func (p RemotePath) String() string { return p.normalized }

// EscapedValue encodes the complete path for endpoints whose path variable
// represents a device path rather than one segment.
func (p RemotePath) EscapedValue() string { return url.PathEscape(p.normalized) }

func (p RemotePath) MarshalText() ([]byte, error) {
	if p.normalized == "" {
		return nil, errors.New("digitalpaper: zero remote path")
	}
	return []byte(p.normalized), nil
}

func (p *RemotePath) UnmarshalText(text []byte) error {
	parsed, err := ParseRemotePath(string(text))
	if err != nil {
		return err
	}
	*p = parsed
	return nil
}

func (p RemotePath) MarshalJSON() ([]byte, error) {
	text, err := p.MarshalText()
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(text))
}

func (p *RemotePath) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return p.UnmarshalText([]byte(value))
}
