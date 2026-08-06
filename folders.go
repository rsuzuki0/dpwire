package digitalpaper

import (
	"context"
	"net/url"
)

// FoldersService exposes read-only folder operations.
type FoldersService struct{ client *Client }

// List retrieves all direct entries in one folder using bounded pagination.
func (s *FoldersService) List(ctx context.Context, folderID string, options ListOptions) ([]Entry, error) {
	if err := validateID(folderID); err != nil {
		return nil, err
	}
	endpoint := "/folders/" + url.PathEscape(folderID) + "/entries2"
	return s.client.listEntries(ctx, endpoint, "", options)
}
