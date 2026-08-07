package digitalpaper

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// Delete removes one document only if targetRevision is still current. The
// device response is followed by a metadata lookup to verify absence.
func (s *DocumentsService) Delete(ctx context.Context, id, targetRevision string) error {
	if err := validateID(id); err != nil {
		return err
	}
	if targetRevision == "" {
		return errors.New("digitalpaper: target revision is required for deletion")
	}
	payload := map[string]string{"target_revision": targetRevision}
	endpoint := "/documents/" + url.PathEscape(id)
	if err := s.client.wire.DoJSON(ctx, http.MethodDelete, endpoint, nil, payload, nil, true); err != nil {
		return publicError(err)
	}
	return verifyDeleted("document", id, func() error {
		_, err := s.Get(ctx, id)
		return err
	})
}

// DeleteEmpty removes one empty folder. It checks for children before the
// request and always sends force_delete_flag=false, so a concurrent new child
// also prevents deletion at the device.
func (s *FoldersService) DeleteEmpty(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	folder, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if folder.Path.String() == "Document" {
		return errors.New("digitalpaper: device root cannot be deleted")
	}
	entries, err := s.List(ctx, id, ListOptions{})
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("%w: %s contains %d entries", ErrNotEmpty, folder.Path.String(), len(entries))
	}
	payload := map[string]string{"force_delete_flag": "false"}
	endpoint := "/folders/" + url.PathEscape(id)
	if err := s.client.wire.DoJSON(ctx, http.MethodDelete, endpoint, nil, payload, nil, true); err != nil {
		return publicError(err)
	}
	return verifyDeleted("folder", id, func() error {
		_, err := s.Get(ctx, id)
		return err
	})
}

func verifyDeleted(kind, id string, lookup func() error) error {
	err := lookup()
	if err == nil {
		return &VerificationError{Field: kind + " deletion", Expected: "absent", Actual: id + " still exists"}
	}
	var apiError *APIError
	if errors.As(err, &apiError) && apiError.StatusCode == http.StatusNotFound {
		return nil
	}
	return fmt.Errorf("digitalpaper: %s deletion succeeded but absence could not be verified for %s: %w", kind, id, err)
}
