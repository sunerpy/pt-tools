package v2

type SiteDefinition struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Aka               []string               `json:"aka,omitempty"`
	Description       string                 `json:"description,omitempty"`
	Schema            Schema                 `json:"schema"`
	URLs              []string               `json:"urls"`
	LegacyURLs        []string               `json:"legacyUrls,omitempty"`
	FaviconURL        string                 `json:"faviconUrl,omitempty"`
	Unavailable       bool                   `json:"unavailable,omitempty"`
	UnavailableReason string                 `json:"unavailableReason,omitempty"`
	AuthMethod        AuthMethod             `json:"authMethod,omitempty"`
	RateLimit         float64                `json:"rateLimit,omitempty"`
	RateBurst         int                    `json:"rateBurst,omitempty"`
	TimezoneOffset    string                 `json:"timezoneOffset,omitempty"`
	UserInfo          *UserInfoConfig        `json:"userInfo,omitempty"`
	LevelRequirements []SiteLevelRequirement `json:"levelRequirements,omitempty"`
	Selectors         *SiteSelectors         `json:"selectors,omitempty"`
	DetailParser      *DetailParserConfig    `json:"detailParser,omitempty"`

	// CreateDriver is an optional custom driver factory for this site.
	// If nil, the driver is created based on Schema field.
	// This allows sites with unique APIs to provide custom driver logic.
	CreateDriver DriverFactory `json:"-"`
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
