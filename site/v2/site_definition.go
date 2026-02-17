package v2

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// idPattern enforces lowercase alphanumeric IDs with hyphens/underscores
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// timezonePattern validates timezone offset format like "+0800" or "-0500"
var timezonePattern = regexp.MustCompile(`^[+-]\d{4}$`)

// sizePattern validates size strings like "200GB", "1.5TB", "1TB"
var sizePattern = regexp.MustCompile(`(?i)^[\d.]+\s*[KMGTP]?i?B$`)

// intervalPattern validates ISO 8601 duration like "P5W", "P10W"
var intervalPattern = regexp.MustCompile(`^P\d+[DWMY]$`)

// ValidationError contains structured validation error information
type ValidationError struct {
	SiteID string
	Schema string
	Field  string
	Rule   string
	Detail string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("site=%q schema=%s field=%s rule=%s: %s", e.SiteID, e.Schema, e.Field, e.Rule, e.Detail)
}

// ValidationErrors collects multiple validation errors
type ValidationErrors []*ValidationError

func (errs ValidationErrors) Error() string {
	if len(errs) == 0 {
		return ""
	}
	var b strings.Builder
	for i, e := range errs {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(e.Error())
	}
	return b.String()
}

// Validate checks the SiteDefinition for completeness and correctness.
// Returns nil if valid, or ValidationErrors containing all issues found.
func (d *SiteDefinition) Validate() error {
	var errs ValidationErrors

	addErr := func(field, rule, detail string) {
		errs = append(errs, &ValidationError{
			SiteID: d.ID,
			Schema: string(d.Schema),
			Field:  field,
			Rule:   rule,
			Detail: detail,
		})
	}

	// === Universal rules ===

	if d.ID == "" {
		addErr("ID", "Required", "must not be empty")
	} else {
		if len(d.ID) > 50 {
			addErr("ID", "MaxLength", "must be ≤ 50 characters")
		}
		if !idPattern.MatchString(d.ID) {
			addErr("ID", "Format", "must be lowercase alphanumeric with hyphens/underscores, starting with a letter (e.g., \"hdsky\", \"my-site\")")
		}
	}

	if d.Name == "" {
		addErr("Name", "Required", "must not be empty")
	}

	if !d.Schema.IsValid() {
		addErr("Schema", "InvalidValue", fmt.Sprintf("%q is not a valid schema; valid values: NexusPHP, mTorrent, Unit3D, Gazelle, HDDolby, Rousi", d.Schema))
	}

	if len(d.URLs) == 0 {
		addErr("URLs", "Required", "must have at least one URL")
	} else {
		for i, u := range d.URLs {
			if err := validateURL(u); err != nil {
				addErr(fmt.Sprintf("URLs[%d]", i), "InvalidURL", fmt.Sprintf("%q: %s", u, err))
			}
		}
	}

	for i, u := range d.LegacyURLs {
		if err := validateURL(u); err != nil {
			addErr(fmt.Sprintf("LegacyURLs[%d]", i), "InvalidURL", fmt.Sprintf("%q: %s", u, err))
		}
	}

	if d.FaviconURL != "" {
		if err := validateURL(d.FaviconURL); err != nil {
			addErr("FaviconURL", "InvalidURL", fmt.Sprintf("%q: %s", d.FaviconURL, err))
		}
	}

	if d.AuthMethod != "" && !d.AuthMethod.IsValid() {
		addErr("AuthMethod", "InvalidValue", fmt.Sprintf("%q is not a valid auth method", d.AuthMethod))
	}

	if d.RateLimit < 0 {
		addErr("RateLimit", "InvalidValue", "must be ≥ 0")
	}

	if d.RateBurst < 0 {
		addErr("RateBurst", "InvalidValue", "must be ≥ 0")
	}

	if d.TimezoneOffset != "" && !timezonePattern.MatchString(d.TimezoneOffset) {
		addErr("TimezoneOffset", "Format", fmt.Sprintf("%q must match format like \"+0800\" or \"-0500\"", d.TimezoneOffset))
	}

	if d.Unavailable && d.UnavailableReason == "" {
		addErr("UnavailableReason", "Required", "must provide a reason when site is marked unavailable")
	}

	// === Schema-specific rules ===
	if d.Schema.IsValid() {
		d.validateSchemaSpecific(addErr)
	}

	// === UserInfo internal consistency ===
	if d.UserInfo != nil {
		d.validateUserInfo(addErr)
	}

	// === LevelRequirements validation ===
	if len(d.LevelRequirements) > 0 {
		d.validateLevelRequirements(addErr)
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// validateSchemaSpecific validates schema-dependent requirements
func (d *SiteDefinition) validateSchemaSpecific(addErr func(field, rule, detail string)) {
	switch d.Schema {
	case SchemaNexusPHP:
		if d.Selectors == nil {
			addErr("Selectors", "SchemaRequired", "NexusPHP schema requires Selectors for search page parsing")
		} else {
			if d.Selectors.TableRows == "" {
				addErr("Selectors.TableRows", "SchemaRequired", "NexusPHP schema requires TableRows selector")
			}
			if d.Selectors.Title == "" {
				addErr("Selectors.Title", "SchemaRequired", "NexusPHP schema requires Title selector")
			}
			if d.Selectors.TitleLink == "" {
				addErr("Selectors.TitleLink", "SchemaRequired", "NexusPHP schema requires TitleLink selector")
			}
		}
		if d.UserInfo == nil {
			addErr("UserInfo", "SchemaRequired", "NexusPHP schema requires UserInfo config for user data fetching")
		}

	case SchemaMTorrent:
		if d.UserInfo == nil {
			addErr("UserInfo", "SchemaRequired", "mTorrent schema requires UserInfo config with API process steps")
		}

	case SchemaHDDolby:
		if d.AuthMethod != "" && d.AuthMethod != AuthMethodCookieAndAPIKey {
			addErr("AuthMethod", "SchemaRequired", fmt.Sprintf("HDDolby schema requires AuthMethod=%q, got %q", AuthMethodCookieAndAPIKey, d.AuthMethod))
		}

	case SchemaRousi:
		if d.CreateDriver == nil {
			addErr("CreateDriver", "SchemaRequired", "Rousi schema requires a custom CreateDriver function")
		}
	}
}

// validateUserInfo checks UserInfo internal consistency
func (d *SiteDefinition) validateUserInfo(addErr func(field, rule, detail string)) {
	ui := d.UserInfo

	if len(ui.Process) == 0 {
		addErr("UserInfo.Process", "Required", "must have at least one process step")
		return
	}

	allProcessFields := make(map[string]bool)
	collectedFields := make(map[string]bool)

	for i, proc := range ui.Process {
		prefix := fmt.Sprintf("UserInfo.Process[%d]", i)

		if proc.RequestConfig.URL == "" {
			addErr(prefix+".RequestConfig.URL", "Required", "must not be empty")
		}

		if len(proc.Fields) == 0 {
			addErr(prefix+".Fields", "Required", "must have at least one field")
		}

		for assertField, assertRef := range proc.Assertion {
			// assertRef format: "params.<fieldName>" → field must be collected in an earlier step
			if _, ok := strings.CutPrefix(assertRef, "params."); ok {
				refField := strings.TrimPrefix(assertRef, "params.")
				if !collectedFields[refField] {
					addErr(prefix+".Assertion", "InvalidReference",
						fmt.Sprintf("assertion %q references field %q via %q, but it has not been collected in a previous process step",
							assertField, refField, assertRef))
				}
			}
		}

		for _, field := range proc.Fields {
			allProcessFields[field] = true
			collectedFields[field] = true
		}
	}

	// Check that every field referenced in Process has a corresponding Selector
	if len(ui.Selectors) > 0 {
		for field := range allProcessFields {
			if _, ok := ui.Selectors[field]; !ok {
				addErr(fmt.Sprintf("UserInfo.Selectors[%q]", field), "Missing",
					fmt.Sprintf("field %q is referenced in Process but has no selector defined", field))
			}
		}
	} else if len(allProcessFields) > 0 {
		addErr("UserInfo.Selectors", "Required", "Process references fields but no Selectors are defined")
	}

	// Check that each FieldSelector has at least one selector path or a default Text
	for name, sel := range ui.Selectors {
		if len(sel.Selector) == 0 && sel.Text == "" {
			addErr(fmt.Sprintf("UserInfo.Selectors[%q]", name), "NoSelector",
				"must have at least one CSS/JSON selector or a default Text value")
		}
	}
}

// validateLevelRequirements checks LevelRequirements data validity
func (d *SiteDefinition) validateLevelRequirements(addErr func(field, rule, detail string)) {
	seenIDs := make(map[int]string)
	seenNames := make(map[string]bool)

	for i, req := range d.LevelRequirements {
		prefix := fmt.Sprintf("LevelRequirements[%d]", i)

		if req.Name == "" {
			addErr(prefix+".Name", "Required", "level name must not be empty")
		}

		if existing, ok := seenIDs[req.ID]; ok {
			addErr(prefix+".ID", "Duplicate", fmt.Sprintf("level ID %d is already used by %q", req.ID, existing))
		}
		seenIDs[req.ID] = req.Name

		if seenNames[req.Name] {
			addErr(prefix+".Name", "Duplicate", fmt.Sprintf("level name %q is already used", req.Name))
		}
		seenNames[req.Name] = true

		if req.Downloaded != "" && !sizePattern.MatchString(req.Downloaded) {
			addErr(prefix+".Downloaded", "Format", fmt.Sprintf("%q is not a valid size string (expected format: \"200GB\", \"1.5TB\")", req.Downloaded))
		}
		if req.Uploaded != "" && !sizePattern.MatchString(req.Uploaded) {
			addErr(prefix+".Uploaded", "Format", fmt.Sprintf("%q is not a valid size string (expected format: \"200GB\", \"1.5TB\")", req.Uploaded))
		}
		if req.SeedingSize != "" && !sizePattern.MatchString(req.SeedingSize) {
			addErr(prefix+".SeedingSize", "Format", fmt.Sprintf("%q is not a valid size string (expected format: \"200GB\", \"1.5TB\")", req.SeedingSize))
		}

		if req.Interval != "" && !intervalPattern.MatchString(req.Interval) {
			addErr(prefix+".Interval", "Format", fmt.Sprintf("%q is not a valid ISO 8601 duration (expected format: \"P5W\", \"P10W\")", req.Interval))
		}

		if req.Ratio < 0 {
			addErr(prefix+".Ratio", "InvalidValue", "ratio must be ≥ 0")
		}

		for j, alt := range req.Alternative {
			altPrefix := fmt.Sprintf("%s.Alternative[%d]", prefix, j)
			if alt.Downloaded != "" && !sizePattern.MatchString(alt.Downloaded) {
				addErr(altPrefix+".Downloaded", "Format", fmt.Sprintf("%q is not a valid size string", alt.Downloaded))
			}
		}
	}
}

// validateURL checks that a URL is well-formed with http(s) scheme and host
func validateURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("failed to parse: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("must have scheme and host (e.g., \"https://example.com/\")")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https, got %q", parsed.Scheme)
	}
	return nil
}

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
	RateWindow        time.Duration          `json:"-"`
	RateWindowLimit   int                    `json:"rateWindowLimit,omitempty"`
	HREnabled         bool                   `json:"hrEnabled,omitempty"`
	HRSeedTimeHours   int                    `json:"hrSeedTimeHours,omitempty"`
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
