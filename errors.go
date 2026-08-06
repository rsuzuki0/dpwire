package digitalpaper

import (
	"errors"
	"fmt"
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
