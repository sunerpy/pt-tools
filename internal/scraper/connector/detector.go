package connector

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

// Server type identifiers returned by DetectServerType.
const (
	ServerTypeJellyfin = "jellyfin"
	ServerTypeEmby     = "emby"
)

// DetectServerType probes baseURL via GET /System/Info/Public and classifies
// the server as Jellyfin or Emby based on ProductName. Returns core.ErrUnsupported
// when the ProductName is neither.
func DetectServerType(ctx context.Context, baseURL string, client *http.Client) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	bc := &baseClient{BaseURL: baseURL, Client: client}
	var info PublicInfo
	if err := bc.do(ctx, http.MethodGet, "/System/Info/Public", nil, &info); err != nil {
		return "", core.Wrap(err, "detect server type")
	}
	product := strings.ToLower(info.ProductName)
	switch {
	case strings.Contains(product, "jellyfin"):
		return ServerTypeJellyfin, nil
	case strings.Contains(product, "emby"):
		return ServerTypeEmby, nil
	default:
		return "", fmt.Errorf("%w: unknown ProductName %q", core.ErrUnsupported, info.ProductName)
	}
}
