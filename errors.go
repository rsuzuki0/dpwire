package digitalpaper

import (
	"errors"
	"fmt"

	"github.com/rsuzuki0/digitalpaper/internal/wire/transport"
)

// ErrUnsupported is the sentinel for a capability unavailable on a device or
// not implemented by this library version.
var ErrUnsupported = errors.New("digitalpaper: capability unsupported")

// UnsupportedError explains which capability could not be used.
type UnsupportedError struct {
	Capability Capability
	Model      string
	Firmware   string
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("%v: %s (model=%s firmware=%s)", ErrUnsupported, e.Capability, e.Model, e.Firmware)
}

func (e *UnsupportedError) Unwrap() error { return ErrUnsupported }

// APIError reports a non-success response from the device.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("digitalpaper: HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("digitalpaper: HTTP %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

func publicError(err error) error {
	var wireError *transport.HTTPError
	if errors.As(err, &wireError) {
		return &APIError{StatusCode: wireError.StatusCode, Code: wireError.Code, Message: wireError.Message}
	}
	return err
}
