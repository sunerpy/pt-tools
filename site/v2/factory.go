package v2

import (
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

func mergeSelectors(dst, src *SiteSelectors) {
	if src == nil || dst == nil {
		return
	}
	if src.TableRows != "" {
		dst.TableRows = src.TableRows
	}
	if src.Title != "" {
		dst.Title = src.Title
	}
	if src.TitleLink != "" {
		dst.TitleLink = src.TitleLink
	}
	if src.Size != "" {
		dst.Size = src.Size
	}
	if src.Seeders != "" {
		dst.Seeders = src.Seeders
	}
	if src.Leechers != "" {
		dst.Leechers = src.Leechers
	}
	if src.Snatched != "" {
		dst.Snatched = src.Snatched
	}
	if src.DiscountIcon != "" {
		dst.DiscountIcon = src.DiscountIcon
	}
	if src.DiscountEndTime != "" {
		dst.DiscountEndTime = src.DiscountEndTime
	}
	if src.DownloadLink != "" {
		dst.DownloadLink = src.DownloadLink
	}
	if src.Category != "" {
		dst.Category = src.Category
	}
	if src.UploadTime != "" {
		dst.UploadTime = src.UploadTime
	}
	if src.HRIcon != "" {
		dst.HRIcon = src.HRIcon
	}
	if src.Subtitle != "" {
		dst.Subtitle = src.Subtitle
	}
	if src.UserInfoUsername != "" {
		dst.UserInfoUsername = src.UserInfoUsername
	}
	if src.UserInfoUploaded != "" {
		dst.UserInfoUploaded = src.UserInfoUploaded
	}
	if src.UserInfoDownloaded != "" {
		dst.UserInfoDownloaded = src.UserInfoDownloaded
	}
	if src.UserInfoRatio != "" {
		dst.UserInfoRatio = src.UserInfoRatio
	}
	if src.UserInfoBonus != "" {
		dst.UserInfoBonus = src.UserInfoBonus
	}
	if src.UserInfoRank != "" {
		dst.UserInfoRank = src.UserInfoRank
	}
	if src.DetailDownloadLink != "" {
		dst.DetailDownloadLink = src.DetailDownloadLink
	}
	if src.DetailSubtitle != "" {
		dst.DetailSubtitle = src.DetailSubtitle
	}
}

type SiteConfig struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	BaseURL   string          `json:"baseUrl"`
	Options   json.RawMessage `json:"options"`
	RateLimit float64         `json:"rateLimit,omitempty"`
	RateBurst int             `json:"rateBurst,omitempty"`
}

type NexusPHPOptions struct {
	Cookie    string         `json:"cookie"`
	Selectors *SiteSelectors `json:"selectors,omitempty"`
}

type MTorrentOptions struct {
	APIKey string `json:"apiKey"`
}

type HDDolbyOptions struct {
	APIKey string `json:"apiKey"`
	Cookie string `json:"cookie"`
}

type Unit3DOptions struct {
	APIKey string `json:"apiKey"`
}

type GazelleOptions struct {
	APIKey string `json:"apiKey,omitempty"`
	Cookie string `json:"cookie,omitempty"`
}

type RousiOptions struct {
	Passkey string `json:"passkey"`
}

type SiteFactory struct {
	logger *zap.Logger
}

func NewSiteFactory(logger *zap.Logger) *SiteFactory {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SiteFactory{logger: logger}
}

var typeToSchemaMap = map[string]Schema{
	"nexusphp": SchemaNexusPHP,
	"mtorrent": SchemaMTorrent,
	"unit3d":   SchemaUnit3D,
	"gazelle":  SchemaGazelle,
	"hddolby":  SchemaHDDolby,
	"rousi":    SchemaRousi,
}

func typeToSchema(t string) Schema {
	if schema, ok := typeToSchemaMap[t]; ok {
		return schema
	}
	return Schema(t)
}

func (f *SiteFactory) CreateSite(config SiteConfig) (Site, error) {
	if config.ID == "" {
		return nil, fmt.Errorf("site ID is required")
	}
	if config.Name == "" {
		config.Name = config.ID
	}
	if config.BaseURL == "" {
		return nil, fmt.Errorf("site baseUrl is required")
	}

	siteDef := GetDefinitionRegistry().GetOrDefault(config.ID)
	if siteDef == nil {
		if config.Type == "" {
			return nil, fmt.Errorf("unknown site %q and no type specified", config.ID)
		}
		schema := typeToSchema(config.Type)
		factory, ok := GetDriverFactoryForSchema(schema.String())
		if !ok {
			return nil, fmt.Errorf("unsupported site type: %s", config.Type)
		}
		return factory(config, f.logger)
	}

	return CreateSiteFromDefinition(siteDef, config, f.logger)
}

func (f *SiteFactory) CreateSiteFromJSON(jsonData []byte) (Site, error) {
	var config SiteConfig
	if err := json.Unmarshal(jsonData, &config); err != nil {
		return nil, fmt.Errorf("parse site config: %w", err)
	}
	return f.CreateSite(config)
}

func (f *SiteFactory) CreateSitesFromJSON(jsonData []byte) ([]Site, error) {
	var configs []SiteConfig
	if err := json.Unmarshal(jsonData, &configs); err != nil {
		return nil, fmt.Errorf("parse site configs: %w", err)
	}

	sites := make([]Site, 0, len(configs))
	for _, config := range configs {
		site, err := f.CreateSite(config)
		if err != nil {
			f.logger.Warn("Failed to create site",
				zap.String("id", config.ID),
				zap.Error(err),
			)
			continue
		}
		sites = append(sites, site)
	}

	return sites, nil
}
