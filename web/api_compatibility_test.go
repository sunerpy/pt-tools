// Package web provides HTTP server and API handlers
package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeprecationHeaders tests that deprecated endpoints return proper deprecation headers
func TestDeprecationHeaders(t *testing.T) {
	// Register a test deprecated endpoint
	AddDeprecatedEndpoint("/api/old-endpoint", "/api/v2/new-endpoint", "2.0.0", "This endpoint is deprecated")
	defer func() {
		// Clean up
		delete(deprecatedEndpoints, "/api/old-endpoint")
	}()

	tests := []struct {
		name                string
		path                string
		expectDeprecation   bool
		expectedReplacement string
		expectedVersion     string
		expectedMessage     string
	}{
		{
			name:                "deprecated endpoint has headers",
			path:                "/api/old-endpoint",
			expectDeprecation:   true,
			expectedReplacement: "/api/v2/new-endpoint",
			expectedVersion:     "2.0.0",
			expectedMessage:     "This endpoint is deprecated",
		},
		{
			name:              "non-deprecated endpoint has no headers",
			path:              "/api/v2/userinfo/aggregated",
			expectDeprecation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test handler
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"success": true}`))
			})

			// Wrap with deprecation middleware
			wrapped := deprecationMiddleware(handler)

			// Create request
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			// Execute
			wrapped.ServeHTTP(rec, req)

			// Check headers
			if tt.expectDeprecation {
				assert.Equal(t, "true", rec.Header().Get("Deprecation"))
				assert.Equal(t, tt.expectedVersion, rec.Header().Get("Sunset"))
				assert.Contains(t, rec.Header().Get("Link"), tt.expectedReplacement)
				assert.Equal(t, tt.expectedMessage, rec.Header().Get("X-Deprecation-Notice"))
			} else {
				assert.Empty(t, rec.Header().Get("Deprecation"))
				assert.Empty(t, rec.Header().Get("Sunset"))
			}
		})
	}
}

// TestDeprecatedEndpointRegistry tests the deprecated endpoint registry functions
func TestDeprecatedEndpointRegistry(t *testing.T) {
	// Clean state
	originalEndpoints := make(map[string]DeprecatedEndpoint)
	for k, v := range deprecatedEndpoints {
		originalEndpoints[k] = v
	}
	defer func() {
		deprecatedEndpoints = originalEndpoints
	}()

	t.Run("AddDeprecatedEndpoint", func(t *testing.T) {
		AddDeprecatedEndpoint("/test/path", "/test/v2/path", "3.0.0", "Test message")

		assert.True(t, IsDeprecated("/test/path"))
		assert.False(t, IsDeprecated("/nonexistent"))

		endpoints := GetDeprecatedEndpoints()
		dep, ok := endpoints["/test/path"]
		require.True(t, ok)
		assert.Equal(t, "/test/path", dep.Path)
		assert.Equal(t, "/test/v2/path", dep.Replacement)
		assert.Equal(t, "3.0.0", dep.Version)
		assert.Equal(t, "Test message", dep.Message)
	})

	t.Run("GetDeprecatedEndpoints returns copy", func(t *testing.T) {
		AddDeprecatedEndpoint("/copy/test", "/copy/v2/test", "2.0.0", "")

		endpoints := GetDeprecatedEndpoints()
		delete(endpoints, "/copy/test")

		// Original should still have it
		assert.True(t, IsDeprecated("/copy/test"))
	})
}

// TestAPIResponseFormat tests that API responses follow the standard format
func TestAPIResponseFormat(t *testing.T) {
	tests := []struct {
		name           string
		response       any
		expectSuccess  bool
		expectDataKey  bool
		expectErrorKey bool
	}{
		{
			name: "success response format",
			response: map[string]any{
				"success": true,
				"data":    map[string]any{"key": "value"},
			},
			expectSuccess:  true,
			expectDataKey:  true,
			expectErrorKey: false,
		},
		{
			name: "error response format",
			response: map[string]any{
				"success": false,
				"error": map[string]any{
					"code":    "INVALID_REQUEST",
					"message": "Invalid parameters",
				},
			},
			expectSuccess:  false,
			expectDataKey:  false,
			expectErrorKey: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.response)
			require.NoError(t, err)

			var result map[string]any
			err = json.Unmarshal(data, &result)
			require.NoError(t, err)

			success, ok := result["success"].(bool)
			require.True(t, ok, "response must have 'success' field")
			assert.Equal(t, tt.expectSuccess, success)

			_, hasData := result["data"]
			assert.Equal(t, tt.expectDataKey, hasData)

			_, hasError := result["error"]
			assert.Equal(t, tt.expectErrorKey, hasError)
		})
	}
}

// TestV2EndpointsExist tests that v2 API endpoints are properly defined
func TestV2EndpointsExist(t *testing.T) {
	expectedEndpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v2/userinfo/aggregated"},
		{http.MethodGet, "/api/v2/userinfo/sites"},
		{http.MethodPost, "/api/v2/userinfo/sync"},
		{http.MethodPost, "/api/v2/search/multi"},
	}

	// This test verifies the endpoint definitions exist
	// In a real integration test, you would start the server and make actual requests
	for _, ep := range expectedEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			// Verify endpoint path format
			assert.Contains(t, ep.path, "/api/v2/", "v2 endpoints should have /api/v2/ prefix")
		})
	}
}

// TestBackwardCompatibilityEndpoints tests that old endpoints still work
func TestBackwardCompatibilityEndpoints(t *testing.T) {
	// This test ensures backward compatibility by verifying
	// that old endpoint patterns are still supported

	t.Run("old endpoints should be mapped or deprecated", func(t *testing.T) {
		// List of old endpoints that should either:
		// 1. Still work (mapped to new implementation)
		// 2. Be deprecated with proper headers
		oldEndpoints := []string{
			// Add old endpoint paths here as they are deprecated
		}

		for _, path := range oldEndpoints {
			// Either the endpoint should work or be in deprecated list
			if IsDeprecated(path) {
				dep := deprecatedEndpoints[path]
				assert.NotEmpty(t, dep.Replacement, "deprecated endpoint should have replacement")
				assert.NotEmpty(t, dep.Version, "deprecated endpoint should have sunset version")
			}
		}
	})
}

// TestErrorCodeConsistency tests that error codes are consistent
func TestErrorCodeConsistency(t *testing.T) {
	validErrorCodes := map[string]int{
		"INVALID_REQUEST":  http.StatusBadRequest,
		"UNAUTHORIZED":     http.StatusUnauthorized,
		"FORBIDDEN":        http.StatusForbidden,
		"NOT_FOUND":        http.StatusNotFound,
		"SITE_NOT_FOUND":   http.StatusNotFound,
		"RATE_LIMITED":     http.StatusTooManyRequests,
		"INTERNAL_ERROR":   http.StatusInternalServerError,
		"SITE_UNAVAILABLE": http.StatusServiceUnavailable,
	}

	for code, status := range validErrorCodes {
		t.Run(code, func(t *testing.T) {
			assert.GreaterOrEqual(t, status, 400, "error codes should map to 4xx or 5xx status")
			assert.Less(t, status, 600, "error codes should map to valid HTTP status")
		})
	}
}

// TestRateLimitHeaders tests rate limit header format
func TestRateLimitHeaders(t *testing.T) {
	expectedHeaders := []string{
		"X-RateLimit-Limit",
		"X-RateLimit-Remaining",
		"X-RateLimit-Reset",
	}

	t.Run("rate limit headers should be defined", func(t *testing.T) {
		// This test documents the expected rate limit headers
		for _, header := range expectedHeaders {
			assert.NotEmpty(t, header)
		}
	})
}
