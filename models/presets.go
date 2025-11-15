package models

import (
    "strings"
    "gorm.io/gorm"
)

type RSSPreset struct {
	Name            string
	URL             string
	Category        string
	Tag             string
	IntervalMinutes int32
	DownloadSubPath string
}
type SitePreset struct {
	Name       string
	AuthMethod string
	Enabled    bool
	RSS        []RSSPreset
}

var DefaultSitePresets = []SitePreset{
	{
		Name:       string(CMCT),
		AuthMethod: "cookie",
		Enabled:    false,
		RSS: []RSSPreset{{
			Name:            "CMCT",
			URL:             "https://springxxx.xxx",
			Category:        "Tv",
			Tag:             "CMCT",
			IntervalMinutes: 5,
			DownloadSubPath: "cmct/",
		}},
	},
	{
		Name:       string(HDSKY),
		AuthMethod: "cookie",
		Enabled:    false,
		RSS: []RSSPreset{{
			Name:            "HDSky",
			URL:             "https://hdsky.xxx/torrentrss.php?xxx",
			Category:        "Mv",
			Tag:             "HDSKY",
			IntervalMinutes: 5,
			DownloadSubPath: "hdsky/",
		}},
	},
	{
		Name:       string(MTEAM),
		AuthMethod: "api_key",
		Enabled:    false,
		RSS: []RSSPreset{{
			Name:            "TMP2",
			URL:             "https://rss.m-team.xxx/api/rss/xxx",
			Category:        "Tv",
			Tag:             "MT",
			IntervalMinutes: 10,
			DownloadSubPath: "mteam/tvs",
		}},
	},
}

func SeedDefaultSites(db *gorm.DB) error {
    var cnt int64
    if err := db.Model(&SiteSetting{}).Count(&cnt).Error; err != nil { return err }
    if cnt > 0 { return nil }
    for _, p := range DefaultSitePresets {
        name := strings.ToLower(p.Name)
        site := SiteSetting{Name: name, AuthMethod: p.AuthMethod, Enabled: p.Enabled}
        if err := db.Create(&site).Error; err != nil { return err }
        for _, r := range p.RSS {
            rr := RSSSubscription{SiteID: site.ID, Name: r.Name, URL: r.URL, Category: r.Category, Tag: r.Tag, IntervalMinutes: r.IntervalMinutes, DownloadSubPath: r.DownloadSubPath}
            if err := db.Create(&rr).Error; err != nil { return err }
        }
    }
    return nil
}
