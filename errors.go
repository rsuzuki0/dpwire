package dpwire

import (
	"errors"
	"fmt"

	"github.com/rsuzuki0/dpwire/internal/wire/transport"
)

// ErrUnsupported is the sentinel for a capability unavailable on a device or
// not implemented by this library version.
var ErrUnsupported = errors.New("dpwire: capability unsupported")

// ErrConflict is returned when a revision or destination-name precondition no
// longer matches device state.
var ErrConflict = errors.New("dpwire: write conflict")

// ErrNotEmpty is returned when an empty-only folder deletion encounters a
// child entry.
var ErrNotEmpty = errors.New("dpwire: folder not empty")

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
		return fmt.Sprintf("dpwire: HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("dpwire: HTTP %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

func publicError(err error) error {
	var wireError *transport.HTTPError
	if errors.As(err, &wireError) {
		apiError := &APIError{StatusCode: wireError.StatusCode, Code: wireError.Code, Message: wireError.Message}
		if wireError.Code == "40007" || wireError.Code == "40017" || wireError.StatusCode == 409 {
			return &ConflictError{Cause: apiError}
		}
		if wireError.Code == "40018" {
			return &FolderNotEmptyError{Cause: apiError}
		}
		return apiError
	}
	return err
}

// ConflictError preserves the device response while supporting errors.Is.
type ConflictError struct{ Cause *APIError }

func (e *ConflictError) Error() string        { return fmt.Sprintf("%v: %v", ErrConflict, e.Cause) }
func (e *ConflictError) Unwrap() error        { return e.Cause }
func (e *ConflictError) Is(target error) bool { return target == ErrConflict }

// FolderNotEmptyError preserves the device response while supporting
// errors.Is(err, ErrNotEmpty).
type FolderNotEmptyError struct{ Cause *APIError }

func (e *FolderNotEmptyError) Error() string        { return fmt.Sprintf("%v: %v", ErrNotEmpty, e.Cause) }
func (e *FolderNotEmptyError) Unwrap() error        { return e.Cause }
func (e *FolderNotEmptyError) Is(target error) bool { return target == ErrNotEmpty }

// PartialFailureError reports a multi-step write whose earlier step succeeded.
// EntryID identifies the metadata entry that may need later cleanup.
type PartialFailureError struct {
	Operation string
	EntryID   string
	Cause     error
}

func (e *PartialFailureError) Error() string {
	return fmt.Sprintf("dpwire: %s partially failed after creating %s: %v", e.Operation, e.EntryID, e.Cause)
}
func (e *PartialFailureError) Unwrap() error { return e.Cause }

// VerificationError reports a successful request whose observed state did not
// match the requested state.
type VerificationError struct {
	Field    string
	Expected string
	Actual   string
}

func (e *VerificationError) Error() string {
	return fmt.Sprintf("dpwire: verification failed for %s: expected %q, got %q", e.Field, e.Expected, e.Actual)
}
