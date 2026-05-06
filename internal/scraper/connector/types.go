package connector

// PublicInfo is the response from GET /System/Info/Public (no auth required).
type PublicInfo struct {
	ServerName      string `json:"ServerName"`
	Version         string `json:"Version"`
	ProductName     string `json:"ProductName"`
	OperatingSystem string `json:"OperatingSystem"`
	Id              string `json:"Id"`
}

// MediaFolder is one entry from GET /Library/MediaFolders.
type MediaFolder struct {
	Name           string   `json:"Name"`
	ServerId       string   `json:"ServerId"`
	Id             string   `json:"Id"`
	CollectionType string   `json:"CollectionType,omitempty"`
	LocationType   string   `json:"LocationType"`
	Path           string   `json:"Path,omitempty"`
	SubViews       []string `json:"SubViews,omitempty"`
}

// MediaFoldersResponse wraps the /Library/MediaFolders response.
type MediaFoldersResponse struct {
	Items            []MediaFolder `json:"Items"`
	TotalRecordCount int           `json:"TotalRecordCount"`
}

// ScheduledTask is one entry from GET /ScheduledTasks.
type ScheduledTask struct {
	Name                      string  `json:"Name"`
	Key                       string  `json:"Key"`
	Description               string  `json:"Description"`
	Category                  string  `json:"Category"`
	State                     string  `json:"State"`
	CurrentProgressPercentage float64 `json:"CurrentProgressPercentage,omitempty"`
	Id                        string  `json:"Id"`
}
