package connector

import (
	"context"
	"fmt"
	"net/http"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

// NewConnector is a factory function that constructs the appropriate MediaServerConnector
// based on the product type. If product is "auto" or empty, it auto-detects via DetectServerType.
// Otherwise, it uses the explicit product type ("jellyfin" or "emby").
//
// If client is nil, http.DefaultClient is used.
func NewConnector(ctx context.Context, product, baseURL, apiKey string, client *http.Client) (core.MediaServerConnector, error) {
	if client == nil {
		client = http.DefaultClient
	}

	kind := product
	if kind == "" || kind == "auto" {
		detected, err := DetectServerType(ctx, baseURL, client)
		if err != nil {
			return nil, fmt.Errorf("auto-detect: %w", err)
		}
		kind = detected
	}

	switch kind {
	case "jellyfin":
		return NewJellyfinConnector(baseURL, apiKey, client), nil
	case "emby":
		return NewEmbyConnector(baseURL, apiKey, client), nil
	default:
		return nil, fmt.Errorf("%w: unknown connector type %q", core.ErrUnsupported, kind)
	}
}
