package dpwire

import (
	"errors"
	"testing"
)

func TestUnsupportedErrorWrapsSentinel(t *testing.T) {
	err := &UnsupportedError{Capability: CapabilityWhiteboard, Model: "test", Firmware: "0"}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("errors.Is(%v, ErrUnsupported) = false", err)
	}
}
