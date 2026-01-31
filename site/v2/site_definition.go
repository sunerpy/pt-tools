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

	// Unavailable marks the site as temporarily unavailable
	Unavailable bool `json:"unavailable,omitempty"`
	// UnavailableReason explains why the site is unavailable
	UnavailableReason string `json:"unavailableReason,omitempty"`

	// AuthMethod is "cookie" or "api_key" (inferred from Schema if empty)
	AuthMethod string `json:"authMethod,omitempty"`
	// RateLimit is requests per second (default: 2.0)
	RateLimit float64 `json:"rateLimit,omitempty"`
	// RateBurst is the burst size for rate limiting (default: 5)
	RateBurst int `json:"rateBurst,omitempty"`

	// TimezoneOffset is the site's timezone (e.g., "+0800")
	TimezoneOffset string `json:"timezoneOffset,omitempty"`

	// UserInfo contains user info fetching configuration
	UserInfo *UserInfoConfig `json:"userInfo,omitempty"`

	// LevelRequirements defines user level requirements
	LevelRequirements []SiteLevelRequirement `json:"levelRequirements,omitempty"`

	// Selectors contains custom CSS selectors (merged with schema defaults)
	Selectors *SiteSelectors `json:"selectors,omitempty"`

	// DetailParser contains detail page parsing configuration
	// If nil, uses DefaultDetailParserConfig()
	DetailParser *DetailParserConfig `json:"detailParser,omitempty"`
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

// DetailParserConfig defines how to parse torrent detail pages
// Used for RSS detail fetching to extract discount status, size, HR flag, etc.
type DetailParserConfig struct {
	TimeLayout       string                   `json:"timeLayout,omitempty"`
	DiscountMapping  map[string]DiscountLevel `json:"discountMapping,omitempty"`
	HRKeywords       []string                 `json:"hrKeywords,omitempty"`
	TitleSelector    string                   `json:"titleSelector,omitempty"`
	IDSelector       string                   `json:"idSelector,omitempty"`
	DiscountSelector string                   `json:"discountSelector,omitempty"`
	EndTimeSelector  string                   `json:"endTimeSelector,omitempty"`
	SizeSelector     string                   `json:"sizeSelector,omitempty"`
	SizeRegex        string                   `json:"sizeRegex,omitempty"`
}

// DefaultDetailParserConfig returns default config for standard NexusPHP sites
func DefaultDetailParserConfig() *DetailParserConfig {
	return &DetailParserConfig{
		TimeLayout: "2006-01-02 15:04:05",
		DiscountMapping: map[string]DiscountLevel{
			"free":          DiscountFree,
			"twoup":         Discount2xUp,
			"twoupfree":     Discount2xFree,
			"thirtypercent": DiscountPercent30,
			"halfdown":      DiscountPercent50,
			"twouphalfdown": Discount2x50,
			"pro_custom":    DiscountNone,
		},
		HRKeywords:       []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"},
		TitleSelector:    "input[name='torrent_name']",
		IDSelector:       "input[name='detail_torrent_id']",
		DiscountSelector: "h1 font",
		EndTimeSelector:  "h1 span[title]",
		SizeSelector:     "td.rowhead:contains('基本信息')",
		SizeRegex:        `大小：[^\d]*([\d.]+)\s*(GB|MB|KB|TB)`,
	}
}
