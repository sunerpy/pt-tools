// MIT License
// Copyright (c) 2025 pt-tools

package v2

// CategoryOption 分类选项
type CategoryOption struct {
	Value interface{} `json:"value"`
	Name  string      `json:"name"`
	Type  string      `json:"type,omitempty"` // e.g., "normal", "adult" for mteam
}

// CategoryDefinition 分类定义
type CategoryDefinition struct {
	Name    string           `json:"name"`
	Key     string           `json:"key"`
	Options []CategoryOption `json:"options"`
	Notes   string           `json:"notes,omitempty"`
}

// SiteCategoryConfig 站点分类配置
type SiteCategoryConfig struct {
	SiteID     string               `json:"siteId"`
	SiteName   string               `json:"siteName"`
	Categories []CategoryDefinition `json:"categories"`
}

// GetSiteCategoryConfig 获取站点分类配置
func GetSiteCategoryConfig(siteID string) *SiteCategoryConfig {
	config, ok := siteCategories[siteID]
	if !ok {
		return nil
	}
	return &config
}

// GetAllSiteCategoryConfigs 获取所有站点的分类配置
func GetAllSiteCategoryConfigs() map[string]SiteCategoryConfig {
	return siteCategories
}

// siteCategories 存储所有站点的分类配置
var siteCategories = map[string]SiteCategoryConfig{
	"mteam": {
		SiteID:   "mteam",
		SiteName: "M-Team",
		Categories: []CategoryDefinition{
			{
				Name: "分类入口",
				Key:  "mode",
				Options: []CategoryOption{
					{Value: "normal", Name: "综合"},
					{Value: "adult", Name: "成人"},
				},
			},
			{
				Name:  "类别（综合）",
				Key:   "categories",
				Notes: "请先设置分类入口为综合",
				Options: []CategoryOption{
					{Value: "401", Name: "电影/SD", Type: "normal"},
					{Value: "419", Name: "电影/HD", Type: "normal"},
					{Value: "420", Name: "电影/DVDiSo", Type: "normal"},
					{Value: "421", Name: "电影/Blu-Ray", Type: "normal"},
					{Value: "439", Name: "电影/Remux", Type: "normal"},
					{Value: "403", Name: "影剧/综艺/SD", Type: "normal"},
					{Value: "402", Name: "影剧/综艺/HD", Type: "normal"},
					{Value: "438", Name: "影剧/综艺/BD", Type: "normal"},
					{Value: "435", Name: "影剧/综艺/DVDiSo", Type: "normal"},
					{Value: "404", Name: "纪录", Type: "normal"},
					{Value: "434", Name: "Music(无损)", Type: "normal"},
					{Value: "406", Name: "演唱", Type: "normal"},
					{Value: "423", Name: "PC游戏", Type: "normal"},
					{Value: "448", Name: "TV游戏", Type: "normal"},
					{Value: "405", Name: "动画", Type: "normal"},
					{Value: "407", Name: "运动", Type: "normal"},
					{Value: "427", Name: "电子书", Type: "normal"},
					{Value: "422", Name: "软体", Type: "normal"},
					{Value: "442", Name: "有声书", Type: "normal"},
					{Value: "451", Name: "教育影片", Type: "normal"},
					{Value: "409", Name: "Misc(其他)", Type: "normal"},
				},
			},
			{
				Name:  "类别（成人）",
				Key:   "categories_adult",
				Notes: "请先设置分类入口为成人",
				Options: []CategoryOption{
					{Value: "410", Name: "AV(有码)/HD Censored", Type: "adult"},
					{Value: "424", Name: "AV(有码)/SD Censored", Type: "adult"},
					{Value: "437", Name: "AV(有码)/DVDiSo Censored", Type: "adult"},
					{Value: "431", Name: "AV(有码)/Blu-Ray Censored", Type: "adult"},
					{Value: "429", Name: "AV(无码)/HD Uncensored", Type: "adult"},
					{Value: "430", Name: "AV(无码)/SD Uncensored", Type: "adult"},
					{Value: "426", Name: "AV(无码)/DVDiSo Uncensored", Type: "adult"},
					{Value: "432", Name: "AV(无码)/Blu-Ray Uncensored", Type: "adult"},
					{Value: "436", Name: "AV(网站)/0Day", Type: "adult"},
					{Value: "440", Name: "AV(Gay)/HD", Type: "adult"},
					{Value: "425", Name: "IV(写真影集)", Type: "adult"},
					{Value: "433", Name: "IV(写真图集)", Type: "adult"},
					{Value: "411", Name: "H-游戏", Type: "adult"},
					{Value: "412", Name: "H-动画", Type: "adult"},
					{Value: "413", Name: "H-漫画", Type: "adult"},
				},
			},
			{
				Name: "视频编码",
				Key:  "videoCodecs",
				Options: []CategoryOption{
					{Value: "1", Name: "H.264(x264/AVC)"},
					{Value: "2", Name: "VC-1"},
					{Value: "3", Name: "Xvid"},
					{Value: "4", Name: "MPEG-2"},
					{Value: "16", Name: "H.265(x265/HEVC)"},
					{Value: "19", Name: "AV1"},
					{Value: "21", Name: "VP8/9"},
					{Value: "22", Name: "AVS"},
				},
			},
			{
				Name: "音频编码",
				Key:  "audioCodecs",
				Options: []CategoryOption{
					{Value: "1", Name: "FLAC"},
					{Value: "2", Name: "APE"},
					{Value: "3", Name: "DTS"},
					{Value: "4", Name: "MP2/3"},
					{Value: "5", Name: "OGG"},
					{Value: "6", Name: "AAC"},
					{Value: "7", Name: "Other"},
					{Value: "8", Name: "AC3(DD)"},
					{Value: "9", Name: "TrueHD"},
					{Value: "10", Name: "TrueHD Atmos"},
					{Value: "11", Name: "DTS-HD MA"},
					{Value: "12", Name: "E-AC3(DDP)"},
					{Value: "13", Name: "E-AC3 Atoms(DDP Atoms)"},
					{Value: "14", Name: "LPCM/PCM"},
					{Value: "15", Name: "WAV"},
				},
			},
			{
				Name: "解析度",
				Key:  "standards",
				Options: []CategoryOption{
					{Value: "1", Name: "1080p"},
					{Value: "2", Name: "1080i"},
					{Value: "3", Name: "720p"},
					{Value: "5", Name: "SD"},
					{Value: "6", Name: "4K"},
					{Value: "7", Name: "8K"},
				},
			},
			{
				Name: "促销",
				Key:  "discount",
				Options: []CategoryOption{
					{Value: "NORMAL", Name: "普通"},
					{Value: "PERCENT_70", Name: "30%"},
					{Value: "PERCENT_50", Name: "50%"},
					{Value: "FREE", Name: "免费"},
				},
			},
			{
				Name: "活/死种",
				Key:  "visible",
				Options: []CategoryOption{
					{Value: 1, Name: "仅活跃"},
					{Value: 2, Name: "仅死种"},
				},
			},
		},
	},
	"hdsky": {
		SiteID:   "hdsky",
		SiteName: "HDSky",
		Categories: []CategoryDefinition{
			{
				Name: "类别",
				Key:  "cat",
				Options: []CategoryOption{
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
			},
			{
				Name: "媒介",
				Key:  "medium",
				Options: []CategoryOption{
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
			},
			{
				Name: "编码",
				Key:  "codec",
				Options: []CategoryOption{
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
			},
			{
				Name: "音频编码",
				Key:  "audiocodec",
				Options: []CategoryOption{
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
			},
			{
				Name: "分辨率",
				Key:  "standard",
				Options: []CategoryOption{
					{Value: 5, Name: "4K/2160p"},
					{Value: 1, Name: "2K/1080p"},
					{Value: 2, Name: "1080i"},
					{Value: 3, Name: "720p"},
					{Value: 4, Name: "SD"},
					{Value: 6, Name: "8K/4320P"},
				},
			},
			{
				Name: "制作组",
				Key:  "team",
				Options: []CategoryOption{
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
				},
			},
			{
				Name: "显示断种/活种",
				Key:  "incldead",
				Options: []CategoryOption{
					{Value: 0, Name: "全部"},
					{Value: 1, Name: "仅活种"},
					{Value: 2, Name: "仅断种"},
				},
			},
			{
				Name: "促销种子",
				Key:  "spstate",
				Options: []CategoryOption{
					{Value: 0, Name: "全部"},
					{Value: 1, Name: "普通"},
					{Value: 2, Name: "免费"},
					{Value: 3, Name: "2X"},
					{Value: 4, Name: "2X免费"},
					{Value: 5, Name: "50%"},
					{Value: 6, Name: "2X 50%"},
					{Value: 7, Name: "30%"},
				},
			},
		},
	},
	"hddolby": {
		SiteID:   "hddolby",
		SiteName: "HD Dolby",
		Categories: []CategoryDefinition{
			{
				Name: "类别",
				Key:  "cat",
				Options: []CategoryOption{
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
			},
			{
				Name: "媒介",
				Key:  "medium",
				Options: []CategoryOption{
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
			},
			{
				Name: "编码",
				Key:  "codec",
				Options: []CategoryOption{
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
			},
			{
				Name: "音频编码",
				Key:  "audiocodec",
				Options: []CategoryOption{
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
					{Value: 12, Name: "Other"},
				},
			},
			{
				Name: "分辨率",
				Key:  "standard",
				Options: []CategoryOption{
					{Value: 6, Name: "4320/8K"},
					{Value: 1, Name: "2160p/4K"},
					{Value: 2, Name: "1080p"},
					{Value: 3, Name: "1080i"},
					{Value: 4, Name: "720p"},
					{Value: 5, Name: "Others"},
				},
			},
			{
				Name: "显示断种/活种",
				Key:  "incldead",
				Options: []CategoryOption{
					{Value: 0, Name: "全部"},
					{Value: 1, Name: "仅活种"},
					{Value: 2, Name: "仅断种"},
				},
			},
			{
				Name: "促销种子",
				Key:  "spstate",
				Options: []CategoryOption{
					{Value: 0, Name: "全部"},
					{Value: 1, Name: "普通"},
					{Value: 2, Name: "免费"},
					{Value: 3, Name: "2X"},
					{Value: 4, Name: "2X免费"},
					{Value: 5, Name: "50%"},
					{Value: 6, Name: "2X 50%"},
					{Value: 7, Name: "30%"},
				},
			},
		},
	},
	"springsunday": {
		SiteID:   "springsunday",
		SiteName: "SpringSunday",
		Categories: []CategoryDefinition{
			{
				Name: "类型",
				Key:  "cat",
				Options: []CategoryOption{
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
			},
			{
				Name: "地区",
				Key:  "source",
				Options: []CategoryOption{
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
			},
			{
				Name: "格式",
				Key:  "medium",
				Options: []CategoryOption{
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
			},
			{
				Name: "分辨率",
				Key:  "standard",
				Options: []CategoryOption{
					{Value: 1, Name: "2160p"},
					{Value: 2, Name: "1080p"},
					{Value: 3, Name: "1080i"},
					{Value: 4, Name: "720p"},
					{Value: 5, Name: "SD"},
					{Value: 99, Name: "Other"},
				},
			},
			{
				Name: "视频编码",
				Key:  "codec",
				Options: []CategoryOption{
					{Value: 1, Name: "H.265/HEVC"},
					{Value: 2, Name: "H.264/AVC"},
					{Value: 3, Name: "VC-1"},
					{Value: 4, Name: "MPEG-2"},
					{Value: 99, Name: "Other"},
				},
			},
			{
				Name: "音频编码",
				Key:  "audiocodec",
				Options: []CategoryOption{
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
			},
			{
				Name: "显示断种/活种",
				Key:  "incldead",
				Options: []CategoryOption{
					{Value: 0, Name: "全部"},
					{Value: 1, Name: "仅活种"},
					{Value: 2, Name: "仅断种"},
				},
			},
			{
				Name: "促销种子",
				Key:  "spstate",
				Options: []CategoryOption{
					{Value: 0, Name: "全部"},
					{Value: 1, Name: "普通"},
					{Value: 2, Name: "免费"},
					{Value: 3, Name: "2X"},
					{Value: 4, Name: "2X免费"},
					{Value: 5, Name: "50%"},
					{Value: 6, Name: "2X 50%"},
					{Value: 7, Name: "30%"},
				},
			},
		},
	},
}
