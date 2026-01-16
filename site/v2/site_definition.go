package v2

// SiteDefinition contains site-specific metadata and configuration
type SiteDefinition struct {
	// ID is the unique site identifier (e.g., "hdsky")
	ID string `json:"id"`
	// Name is the human-readable site name (e.g., "HDSky")
	Name string `json:"name"`
	// Aka contains alternative names for the site
	Aka []string `json:"aka,omitempty"`
	// Description is a brief description of the site
	Description string `json:"description,omitempty"`
	// Schema is the base schema type (e.g., "NexusPHP", "mTorrent")
	Schema string `json:"schema"`

	// URLs are the primary site URLs
	URLs []string `json:"urls"`
	// LegacyURLs are alternative/legacy URLs
	LegacyURLs []string `json:"legacyUrls,omitempty"`
	// FaviconURL is the site's favicon URL for caching
	FaviconURL string `json:"faviconUrl,omitempty"`

	// TimezoneOffset is the site's timezone (e.g., "+0800")
	TimezoneOffset string `json:"timezoneOffset,omitempty"`

	// UserInfo contains user info fetching configuration
	UserInfo *UserInfoConfig `json:"userInfo,omitempty"`

	// LevelRequirements defines user level requirements
	LevelRequirements []SiteLevelRequirement `json:"levelRequirements,omitempty"`

	// Selectors contains custom CSS selectors (merged with schema defaults)
	Selectors *SiteSelectors `json:"selectors,omitempty"`
}

// UserInfoConfig defines how to fetch and parse user info
type UserInfoConfig struct {
	// PickLast specifies fields that should retain last known value
	PickLast []string `json:"pickLast,omitempty"`

	// RequestDelay between multi-step requests (milliseconds)
	RequestDelay int `json:"requestDelay,omitempty"`

	// Process defines multi-step user info fetching
	Process []UserInfoProcess `json:"process,omitempty"`

	// Selectors for parsing user info fields
	Selectors map[string]FieldSelector `json:"selectors,omitempty"`
}

// UserInfoProcess defines a single step in user info fetching
type UserInfoProcess struct {
	// RequestConfig for this step
	RequestConfig RequestConfig `json:"requestConfig"`

	// Assertion for parameter passing (e.g., {"id": "params.id"})
	Assertion map[string]string `json:"assertion,omitempty"`

	// Fields to extract in this step
	Fields []string `json:"fields"`
}

// RequestConfig for HTTP requests
type RequestConfig struct {
	// URL is the request path (e.g., "/index.php")
	URL string `json:"url"`
	// Method is the HTTP method (default: GET)
	Method string `json:"method,omitempty"`
	// Params are query parameters
	Params map[string]string `json:"params,omitempty"`
	// Data is the request body for POST requests
	Data map[string]any `json:"data,omitempty"`
	// ResponseType is "document" for HTML or "json" for JSON
	ResponseType string `json:"responseType,omitempty"`
	// Headers are additional HTTP headers
	Headers map[string]string `json:"headers,omitempty"`
}

// FieldSelector defines how to extract a field value
type FieldSelector struct {
	// Selector is CSS selector(s) for HTML or JSON path for API
	Selector []string `json:"selector,omitempty"`
	// Text is the default value if selector doesn't match
	Text string `json:"text,omitempty"`
	// Attr is the attribute to extract (for HTML elements)
	Attr string `json:"attr,omitempty"`
	// Filters to apply to extracted value
	Filters []Filter `json:"filters,omitempty"`
	// ElementProcess is a custom processing function name
	ElementProcess string `json:"elementProcess,omitempty"`
	// SwitchFilters for different selectors
	SwitchFilters map[string][]Filter `json:"switchFilters,omitempty"`
}

// Filter defines a value transformation
type Filter struct {
	// Name is the filter function name
	Name string `json:"name"`
	// Args are optional arguments
	Args []any `json:"args,omitempty"`
}
