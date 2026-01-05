// Package web provides HTTP server and API handlers
package web

import (
	"net/http"
)

// DeprecatedEndpoint represents a deprecated API endpoint
type DeprecatedEndpoint struct {
	Path        string // The deprecated endpoint path
	Replacement string // The new endpoint to use
	Version     string // Version when it will be removed
	Message     string // Custom deprecation message
}

// deprecatedEndpoints lists all deprecated API endpoints
var deprecatedEndpoints = map[string]DeprecatedEndpoint{
	// Currently no endpoints are deprecated, but this structure is ready for future use
	// Example:
	// "/api/old-endpoint": {
	//     Path:        "/api/old-endpoint",
	//     Replacement: "/api/v2/new-endpoint",
	//     Version:     "2.0.0",
	//     Message:     "This endpoint is deprecated",
	// },
}

// deprecationMiddleware adds deprecation headers to responses for deprecated endpoints
func deprecationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this endpoint is deprecated
		if dep, ok := deprecatedEndpoints[r.URL.Path]; ok {
			// Add deprecation headers
			w.Header().Set("Deprecation", "true")
			w.Header().Set("Sunset", dep.Version)
			if dep.Replacement != "" {
				w.Header().Set("Link", "<"+dep.Replacement+">; rel=\"successor-version\"")
			}
			if dep.Message != "" {
				w.Header().Set("X-Deprecation-Notice", dep.Message)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// AddDeprecatedEndpoint registers a deprecated endpoint
// This can be called during initialization to mark endpoints as deprecated
func AddDeprecatedEndpoint(path, replacement, version, message string) {
	deprecatedEndpoints[path] = DeprecatedEndpoint{
		Path:        path,
		Replacement: replacement,
		Version:     version,
		Message:     message,
	}
}

// GetDeprecatedEndpoints returns all deprecated endpoints
func GetDeprecatedEndpoints() map[string]DeprecatedEndpoint {
	result := make(map[string]DeprecatedEndpoint)
	for k, v := range deprecatedEndpoints {
		result[k] = v
	}
	return result
}

// IsDeprecated checks if an endpoint is deprecated
func IsDeprecated(path string) bool {
	_, ok := deprecatedEndpoints[path]
	return ok
}
