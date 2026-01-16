// MIT License
// Copyright (c) 2025 pt-tools

package web

// SiteCategoryOption 分类选项
type SiteCategoryOption struct {
	Value any    `json:"value"`
	Name  string `json:"name"`
}

// SiteCategory 站点分类配置
type SiteCategory struct {
	Name    string               `json:"name"`
	Key     string               `json:"key"`
	Options []SiteCategoryOption `json:"options"`
	Cross   bool                 `json:"cross"` // 是否支持多选
	Notes   string               `json:"notes,omitempty"`
}

// SiteCategoriesConfig 站点分类配置集合
type SiteCategoriesConfig struct {
	SiteID     string         `json:"site_id"`
	SiteName   string         `json:"site_name"`
	Categories []SiteCategory `json:"categories"`
}

// 通用 NexusPHP 分类选项
var categoryIncldead = SiteCategory{
	Name: "显示断种/活种",
	Key:  "incldead",
	Options: []SiteCategoryOption{
		{Value: 0, Name: "全部"},
		{Value: 1, Name: "仅活种"},
		{Value: 2, Name: "仅断种"},
	},
	Cross: false,
}

var categorySpstate = SiteCategory{
	Name: "促销种子",
	Key:  "spstate",
	Options: []SiteCategoryOption{
		{Value: 0, Name: "全部"},
		{Value: 1, Name: "普通"},
		{Value: 2, Name: "免费"},
		{Value: 3, Name: "2X"},
		{Value: 4, Name: "2X免费"},
		{Value: 5, Name: "50%"},
		{Value: 6, Name: "2X 50%"},
		{Value: 7, Name: "30%"},
	},
	Cross: false,
}

var categoryInclbookmarked = SiteCategory{
	Name: "显示收藏",
	Key:  "inclbookmarked",
	Options: []SiteCategoryOption{
		{Value: 0, Name: "全部"},
		{Value: 1, Name: "仅收藏"},
		{Value: 2, Name: "仅未收藏"},
	},
	Cross: false,
}

// HDSky 站点分类配置
var hdskyCategoriesConfig = SiteCategoriesConfig{
	SiteID:   "hdsky",
	SiteName: "HDSky",
	Categories: []SiteCategory{
		{
			Name: "类别",
			Key:  "cat",
			Options: []SiteCategoryOption{
				{Value: 401, Name: "Movies/电影"},
				{Value: 404, Name: "Documentaries/纪录片"},
				{Value: 410, Name: "iPad/iPad影视"},
				{Value: 405, Name: "Animations/动漫"},
				{Value: 402, Name: "TV Series/剧集(分集)"},
				{Value: 411, Name: "TV Series/剧集(合集)"},
				{Value: 403, Name: "TV Shows/综艺"},
				{Value: 406, Name: "Music Videos/音乐MV"},
				{Value: 407, Name: "Sports/体育"},
				{Value: 408, Name: "HQ Audio/无损音乐"},
				{Value: 409, Name: "Misc/其他"},
				{Value: 412, Name: "TV Series/海外剧集(分集)"},
				{Value: 413, Name: "TV Series/海外剧集(合集)"},
				{Value: 414, Name: "TV Shows/海外综艺(分集)"},
				{Value: 415, Name: "TV Shows/海外综艺(合集)"},
				{Value: 416, Name: "Shortplay/短剧"},
			},
			Cross: true,
		},
		{
			Name: "媒介",
			Key:  "medium",
			Options: []SiteCategoryOption{
				{Value: 1, Name: "UHD Blu-ray"},
				{Value: 14, Name: "UHD Blu-ray/DIY"},
				{Value: 12, Name: "Blu-ray/DIY"},
				{Value: 3, Name: "Remux"},
				{Value: 7, Name: "Encode"},
				{Value: 5, Name: "HDTV"},
				{Value: 6, Name: "DVDR"},
				{Value: 8, Name: "CD"},
				{Value: 4, Name: "MiniBD"},
				{Value: 9, Name: "Track"},
				{Value: 11, Name: "WEB-DL"},
				{Value: 15, Name: "SACD"},
				{Value: 2, Name: "HD DVD"},
				{Value: 16, Name: "3D Blu-ray"},
			},
			Cross: true,
		},
		{
			Name: "编码",
			Key:  "codec",
			Options: []SiteCategoryOption{
				{Value: 1, Name: "H.264/AVC"},
				{Value: 13, Name: "x265"},
				{Value: 10, Name: "x264"},
				{Value: 12, Name: "HEVC"},
				{Value: 2, Name: "VC-1"},
				{Value: 4, Name: "MPEG-2"},
				{Value: 3, Name: "Xvid"},
				{Value: 11, Name: "Other"},
				{Value: 14, Name: "MVC"},
				{Value: 15, Name: "ProRes"},
				{Value: 17, Name: "VP9"},
				{Value: 16, Name: "AV1"},
			},
			Cross: true,
		},
		{
			Name: "音频编码",
			Key:  "audiocodec",
			Options: []SiteCategoryOption{
				{Value: 10, Name: "DTS-HDMA"},
				{Value: 16, Name: "DTS-HDMA:X 7.1"},
				{Value: 17, Name: "TrueHD Atmos"},
				{Value: 19, Name: "PCM"},
				{Value: 11, Name: "TrueHD"},
				{Value: 3, Name: "DTS"},
				{Value: 13, Name: "LPCM"},
				{Value: 1, Name: "FLAC"},
				{Value: 2, Name: "APE"},
				{Value: 4, Name: "MP3"},
				{Value: 5, Name: "OGG"},
				{Value: 6, Name: "AAC"},
				{Value: 12, Name: "AC3/DD"},
				{Value: 7, Name: "Other"},
				{Value: 14, Name: "DTS-HD HR"},
				{Value: 15, Name: "WAV"},
				{Value: 18, Name: "DSD"},
				{Value: 22, Name: "Opus"},
				{Value: 20, Name: "E-AC3"},
				{Value: 21, Name: "DDP with Dolby Atmos"},
				{Value: 23, Name: "ALAC"},
			},
			Cross: true,
		},
		{
			Name: "分辨率",
			Key:  "standard",
			Options: []SiteCategoryOption{
				{Value: 5, Name: "4K/2160p"},
				{Value: 1, Name: "2K/1080p"},
				{Value: 2, Name: "1080i"},
				{Value: 3, Name: "720p"},
				{Value: 4, Name: "SD"},
				{Value: 6, Name: "8K/4320P"},
			},
			Cross: true,
		},
		{
			Name: "制作组",
			Key:  "team",
			Options: []SiteCategoryOption{
				{Value: 6, Name: "HDSky/原盘DIY小组"},
				{Value: 1, Name: "HDS/重编码及remux小组"},
				{Value: 28, Name: "HDS3D/3D重编码小组"},
				{Value: 9, Name: "HDSTV/电视录制小组"},
				{Value: 31, Name: "HDSWEB/网络视频小组"},
				{Value: 18, Name: "HDSPad/移动视频小组"},
				{Value: 22, Name: "HDSCD/无损音乐小组"},
				{Value: 34, Name: "HDSpecial|稀缺资源"},
				{Value: 24, Name: "Original/自制原创资源"},
				{Value: 27, Name: "Other/其他制作组或转发资源"},
				{Value: 26, Name: "Autoseed/自动发布机器人"},
				{Value: 30, Name: "BMDru小组"},
				{Value: 25, Name: "AREA11/韩剧合作小组"},
				{Value: 33, Name: "Request/应求发布资源"},
				{Value: 35, Name: "HDSWEB/(网络视频小组合集专用)"},
				{Value: 36, Name: "HDSAB/有声书小组"},
				{Value: 37, Name: "HDSWEB/(补档专用)"},
			},
			Cross: true,
		},
		categoryIncldead,
		categorySpstate,
		categoryInclbookmarked,
	},
}

// HDDolby 站点分类配置
var hddolbyCategoriesConfig = SiteCategoriesConfig{
	SiteID:   "hddolby",
	SiteName: "HD Dolby",
	Categories: []SiteCategory{
		{
			Name: "类别",
			Key:  "cat",
			Options: []SiteCategoryOption{
				{Value: 401, Name: "Movies电影"},
				{Value: 402, Name: "TV Series电视剧"},
				{Value: 404, Name: "Documentaries纪录片"},
				{Value: 405, Name: "Animations动漫"},
				{Value: 403, Name: "TV Shows综艺"},
				{Value: 406, Name: "Music Videos"},
				{Value: 407, Name: "Sports体育"},
				{Value: 408, Name: "HQ Audio音乐"},
				{Value: 410, Name: "Games游戏"},
				{Value: 411, Name: "Study学习"},
				{Value: 409, Name: "Others其他"},
			},
			Cross: true,
		},
		{
			Name: "媒介",
			Key:  "medium",
			Options: []SiteCategoryOption{
				{Value: 1, Name: "UHD"},
				{Value: 2, Name: "Blu-ray"},
				{Value: 3, Name: "Remux"},
				{Value: 10, Name: "Encode"},
				{Value: 6, Name: "WEB-DL"},
				{Value: 12, Name: "FEED"},
				{Value: 5, Name: "HDTV"},
				{Value: 7, Name: "Webrip"},
				{Value: 4, Name: "HD DVD"},
				{Value: 8, Name: "DVD"},
				{Value: 9, Name: "CD"},
				{Value: 11, Name: "Other"},
			},
			Cross: true,
		},
		{
			Name: "编码",
			Key:  "codec",
			Options: []SiteCategoryOption{
				{Value: 1, Name: "H.264/AVC"},
				{Value: 2, Name: "H.265/HEVC"},
				{Value: 13, Name: "H.266/VVC"},
				{Value: 11, Name: "AV1"},
				{Value: 12, Name: "VP9"},
				{Value: 14, Name: "AVS3"},
				{Value: 15, Name: "AVS+"},
				{Value: 16, Name: "AVS2"},
				{Value: 5, Name: "VC-1"},
				{Value: 6, Name: "MPEG-2"},
				{Value: 7, Name: "Other"},
			},
			Cross: true,
		},
		{
			Name: "音频编码",
			Key:  "audiocodec",
			Options: []SiteCategoryOption{
				{Value: 1, Name: "DTS-HD MA"},
				{Value: 2, Name: "TrueHD"},
				{Value: 15, Name: "DTS-X"},
				{Value: 3, Name: "LPCM"},
				{Value: 4, Name: "DTS"},
				{Value: 5, Name: "DD/AC3"},
				{Value: 14, Name: "DDP/EAC3"},
				{Value: 6, Name: "AAC"},
				{Value: 13, Name: "Opus"},
				{Value: 7, Name: "FLAC"},
				{Value: 8, Name: "APE"},
				{Value: 9, Name: "WAV"},
				{Value: 10, Name: "MP3"},
				{Value: 11, Name: "M4A"},
				{Value: 16, Name: "AV3A"},
				{Value: 17, Name: "AVSA"},
				{Value: 18, Name: "MPEG"},
				{Value: 12, Name: "Other"},
			},
			Cross: true,
		},
		{
			Name: "分辨率",
			Key:  "standard",
			Options: []SiteCategoryOption{
				{Value: 6, Name: "4320/8K"},
				{Value: 1, Name: "2160p/4K"},
				{Value: 2, Name: "1080p"},
				{Value: 3, Name: "1080i"},
				{Value: 4, Name: "720p"},
				{Value: 5, Name: "Others"},
			},
			Cross: true,
		},
		{
			Name: "制作组",
			Key:  "team",
			Options: []SiteCategoryOption{
				{Value: 1, Name: "Dream"},
				{Value: 10, Name: "DBTV"},
				{Value: 12, Name: "QHstudIo"},
				{Value: 13, Name: "CornerMV"},
				{Value: 14, Name: "Telesto"},
				{Value: 2, Name: "MTeam"},
				{Value: 4, Name: "WiKi"},
				{Value: 7, Name: "FRDS"},
				{Value: 9, Name: "HDo"},
				{Value: 11, Name: "beAst"},
				{Value: 5, Name: "CHD"},
				{Value: 6, Name: "CMCT"},
				{Value: 3, Name: "PTHome"},
				{Value: 8, Name: "Other"},
			},
			Cross: true,
		},
		categoryIncldead,
		categorySpstate,
		categoryInclbookmarked,
	},
}

// SpringSunday (CMCT) 站点分类配置
var springsundayCategoriesConfig = SiteCategoriesConfig{
	SiteID:   "springsunday",
	SiteName: "SpringSunday",
	Categories: []SiteCategory{
		{
			Name: "类型",
			Key:  "cat",
			Options: []SiteCategoryOption{
				{Value: 501, Name: "Movies(电影)"},
				{Value: 502, Name: "TV Series(剧集)"},
				{Value: 503, Name: "Docs(纪录)"},
				{Value: 504, Name: "Animations(动画)"},
				{Value: 505, Name: "TV Shows(综艺)"},
				{Value: 506, Name: "Sports(体育)"},
				{Value: 507, Name: "MV(音乐视频)"},
				{Value: 508, Name: "Music(音乐)"},
				{Value: 509, Name: "Others(其他)"},
			},
			Cross: true,
		},
		{
			Name: "地区",
			Key:  "source",
			Options: []SiteCategoryOption{
				{Value: 1, Name: "Mainland(大陆)"},
				{Value: 2, Name: "Hongkong(香港)"},
				{Value: 3, Name: "Taiwan(台湾)"},
				{Value: 4, Name: "West(欧美)"},
				{Value: 5, Name: "Japan(日本)"},
				{Value: 6, Name: "Korea(韩国)"},
				{Value: 7, Name: "India(印度)"},
				{Value: 8, Name: "Russia(俄国)"},
				{Value: 9, Name: "Thailand(泰国)"},
				{Value: 99, Name: "Other(其他地区)"},
			},
			Cross: true,
		},
		{
			Name: "格式",
			Key:  "medium",
			Options: []SiteCategoryOption{
				{Value: 1, Name: "Blu-ray"},
				{Value: 4, Name: "Remux"},
				{Value: 2, Name: "MiniBD"},
				{Value: 6, Name: "BDRip"},
				{Value: 7, Name: "WEB-DL"},
				{Value: 8, Name: "WEBRip"},
				{Value: 5, Name: "HDTV"},
				{Value: 9, Name: "TVRip"},
				{Value: 3, Name: "DVD"},
				{Value: 10, Name: "DVDRip"},
				{Value: 11, Name: "CD"},
				{Value: 99, Name: "Other"},
			},
			Cross: true,
		},
		{
			Name: "分辨率",
			Key:  "standard",
			Options: []SiteCategoryOption{
				{Value: 1, Name: "2160p"},
				{Value: 2, Name: "1080p"},
				{Value: 3, Name: "1080i"},
				{Value: 4, Name: "720p"},
				{Value: 5, Name: "SD"},
				{Value: 99, Name: "Other"},
			},
			Cross: true,
		},
		{
			Name: "视频编码",
			Key:  "codec",
			Options: []SiteCategoryOption{
				{Value: 1, Name: "H.265/HEVC"},
				{Value: 2, Name: "H.264/AVC"},
				{Value: 3, Name: "VC-1"},
				{Value: 4, Name: "MPEG-2"},
				{Value: 99, Name: "Other"},
			},
			Cross: true,
		},
		{
			Name: "音频编码",
			Key:  "audiocodec",
			Options: []SiteCategoryOption{
				{Value: 1, Name: "DTS-HD"},
				{Value: 2, Name: "TrueHD"},
				{Value: 6, Name: "LPCM"},
				{Value: 3, Name: "DTS"},
				{Value: 11, Name: "E-AC-3"},
				{Value: 4, Name: "AC-3"},
				{Value: 5, Name: "AAC"},
				{Value: 7, Name: "FLAC"},
				{Value: 8, Name: "APE"},
				{Value: 9, Name: "WAV"},
				{Value: 10, Name: "MP3"},
				{Value: 99, Name: "Other"},
			},
			Cross: true,
		},
		{
			Name: "制作组",
			Key:  "team",
			Options: []SiteCategoryOption{
				{Value: 1, Name: "CMCT"},
				{Value: 8, Name: "CMCTA"},
				{Value: 9, Name: "CMCTV"},
				{Value: 2, Name: "Oldboys"},
				{Value: 12, Name: "GTR"},
				{Value: 13, Name: "CatEDU"},
				{Value: 14, Name: "Telesto"},
				{Value: 15, Name: "iFree"},
				{Value: 16, Name: "RO"},
				{Value: 17, Name: "XY"},
			},
			Cross: true,
		},
		categoryIncldead,
		categorySpstate,
	},
}

// MTeam 站点分类配置
var mteamCategoriesConfig = SiteCategoriesConfig{
	SiteID:   "mteam",
	SiteName: "MTeam",
	Categories: []SiteCategory{
		{
			Name: "类别",
			Key:  "cat",
			Options: []SiteCategoryOption{
				{Value: 401, Name: "Movie(电影)"},
				{Value: 419, Name: "Movie(电影)/Anime"},
				{Value: 420, Name: "Movie(电影)/3D"},
				{Value: 421, Name: "Movie(电影)/4K"},
				{Value: 439, Name: "Movie(电影)/iPhone"},
				{Value: 403, Name: "TV Series(电视剧)"},
				{Value: 402, Name: "TV Series(电视剧)/HK-TW"},
				{Value: 435, Name: "TV Series(电视剧)/Anime"},
				{Value: 438, Name: "TV Series(电视剧)/iPad"},
				{Value: 404, Name: "Documentaries(纪录片)"},
				{Value: 405, Name: "Animations(动漫)"},
				{Value: 407, Name: "Sports(体育)"},
				{Value: 422, Name: "Software(软件)"},
				{Value: 423, Name: "Game(游戏)"},
				{Value: 427, Name: "Ebook(电子书)"},
				{Value: 409, Name: "Misc(其他)"},
				{Value: 406, Name: "MV(音乐视频)"},
				{Value: 408, Name: "Music(音乐)"},
			},
			Cross: true,
		},
		{
			Name: "媒介",
			Key:  "medium",
			Options: []SiteCategoryOption{
				{Value: 1, Name: "Blu-ray"},
				{Value: 3, Name: "Remux"},
				{Value: 7, Name: "Encode"},
				{Value: 5, Name: "HDTV"},
				{Value: 11, Name: "WEB-DL"},
				{Value: 6, Name: "DVD"},
				{Value: 8, Name: "CD"},
			},
			Cross: true,
		},
		{
			Name: "编码",
			Key:  "codec",
			Options: []SiteCategoryOption{
				{Value: 1, Name: "H.264"},
				{Value: 16, Name: "H.265"},
				{Value: 2, Name: "VC-1"},
				{Value: 3, Name: "Xvid"},
				{Value: 4, Name: "MPEG-2"},
			},
			Cross: true,
		},
		{
			Name: "音频编码",
			Key:  "audiocodec",
			Options: []SiteCategoryOption{
				{Value: 1, Name: "FLAC"},
				{Value: 2, Name: "APE"},
				{Value: 3, Name: "DTS"},
				{Value: 4, Name: "MP3"},
				{Value: 6, Name: "AAC"},
				{Value: 10, Name: "DTS-HD MA"},
				{Value: 11, Name: "TrueHD"},
				{Value: 12, Name: "AC3"},
				{Value: 13, Name: "LPCM"},
				{Value: 14, Name: "DTS-HD HR"},
				{Value: 15, Name: "WAV"},
				{Value: 16, Name: "Atmos"},
				{Value: 17, Name: "DTS:X"},
			},
			Cross: true,
		},
		{
			Name: "分辨率",
			Key:  "standard",
			Options: []SiteCategoryOption{
				{Value: 1, Name: "1080p"},
				{Value: 2, Name: "1080i"},
				{Value: 3, Name: "720p"},
				{Value: 4, Name: "SD"},
				{Value: 5, Name: "4K"},
			},
			Cross: true,
		},
		categoryIncldead,
		categorySpstate,
	},
}

// siteCategoriesMap 站点分类配置映射表
var siteCategoriesMap = map[string]SiteCategoriesConfig{
	"hdsky":        hdskyCategoriesConfig,
	"HDSKY":        hdskyCategoriesConfig,
	"hddolby":      hddolbyCategoriesConfig,
	"HDDOLBY":      hddolbyCategoriesConfig,
	"springsunday": springsundayCategoriesConfig,
	"CMCT":         springsundayCategoriesConfig,
	"mteam":        mteamCategoriesConfig,
	"MTEAM":        mteamCategoriesConfig,
}

// GetSiteCategories 获取指定站点的分类配置
func GetSiteCategories(siteID string) *SiteCategoriesConfig {
	if config, ok := siteCategoriesMap[siteID]; ok {
		return &config
	}
	return nil
}

// GetAllSiteCategories 获取所有站点的分类配置
func GetAllSiteCategories() map[string]SiteCategoriesConfig {
	// 返回去重后的配置（只返回小写key）
	result := make(map[string]SiteCategoriesConfig)
	for key, config := range siteCategoriesMap {
		// 只添加小写的key以避免重复
		if key == config.SiteID {
			result[key] = config
		}
	}
	return result
}

// ListSupportedSites 列出支持分类筛选的站点
func ListSupportedSites() []string {
	seen := make(map[string]bool)
	var sites []string
	for _, config := range siteCategoriesMap {
		if !seen[config.SiteID] {
			seen[config.SiteID] = true
			sites = append(sites, config.SiteID)
		}
	}
	return sites
}
