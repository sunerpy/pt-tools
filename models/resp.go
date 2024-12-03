package models

import (
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

var freeSet = []string{"free", "_2x_free"}

type ResType interface {
	MTTorrentDetail | PHPTorrentInfo
	FreeDownChecker
}
type FreeDownChecker interface {
	IsFree() bool
	CanbeFinished(logger *zap.Logger, enabled bool, speedLimit int) bool
	GetFreeEndTime() *time.Time
}
type APIResponse[T ResType] struct {
	Message string `json:"message"`
	Data    T      `json:"data"`
	Code    string `json:"code"`
}
type MTTorrentDetail struct {
	ID               string        `json:"id"`
	CreatedDate      string        `json:"createdDate"`
	LastModifiedDate string        `json:"lastModifiedDate"`
	Name             string        `json:"name"`
	SmallDescr       string        `json:"smallDescr"`
	IMDb             string        `json:"imdb"`
	IMDbRating       *string       `json:"imdbRating"`
	Douban           string        `json:"douban"`
	DoubanRating     *string       `json:"doubanRating"`
	DmmCode          string        `json:"dmmCode"`
	Author           interface{}   `json:"author"`
	Category         string        `json:"category"`
	Source           string        `json:"source"`
	Medium           interface{}   `json:"medium"`
	Standard         string        `json:"standard"`
	VideoCodec       string        `json:"videoCodec"`
	AudioCodec       string        `json:"audioCodec"`
	Team             string        `json:"team"`
	Processing       interface{}   `json:"processing"`
	Countries        []string      `json:"countries"`
	NumFiles         string        `json:"numfiles"`
	Size             string        `json:"size"`
	Tags             string        `json:"tags"`
	Labels           string        `json:"labels"`
	MsUp             string        `json:"msUp"`
	Anonymous        bool          `json:"anonymous"`
	InfoHash         interface{}   `json:"infoHash"`
	Status           *Status       `json:"status"`
	EditedBy         interface{}   `json:"editedBy"`
	EditDate         interface{}   `json:"editDate"`
	Collection       bool          `json:"collection"`
	InRss            bool          `json:"inRss"`
	CanVote          bool          `json:"canVote"`
	ImageList        interface{}   `json:"imageList"`
	ResetBox         interface{}   `json:"resetBox"`
	OriginFileName   string        `json:"originFileName"`
	Descr            string        `json:"descr"`
	Nfo              interface{}   `json:"nfo"`
	MediaInfo        string        `json:"mediainfo"`
	CIDs             interface{}   `json:"cids"`
	AIDs             interface{}   `json:"aids"`
	ShowcaseList     []Showcase    `json:"showcaseList"`
	TagList          []interface{} `json:"tagList"`
	Scope            string        `json:"scope"`
	ScopeTeams       []interface{} `json:"scopeTeams"`
	Thanked          bool          `json:"thanked"`
	Rewarded         bool          `json:"rewarded"`
}
type Status struct {
	ID               string         `json:"id"`
	CreatedDate      string         `json:"createdDate"`
	LastModifiedDate string         `json:"lastModifiedDate"`
	PickType         string         `json:"pickType"`
	ToppingLevel     string         `json:"toppingLevel"`
	ToppingEndTime   string         `json:"toppingEndTime"`
	Discount         string         `json:"discount"`
	DiscountEndTime  string         `json:"discountEndTime"`
	TimesCompleted   string         `json:"timesCompleted"`
	Comments         string         `json:"comments"`
	LastAction       string         `json:"lastAction"`
	LastSeederAction string         `json:"lastSeederAction"`
	Views            string         `json:"views"`
	Hits             string         `json:"hits"`
	Support          string         `json:"support"`
	Oppose           string         `json:"oppose"`
	Status           string         `json:"status"`
	Seeders          string         `json:"seeders"`
	Leechers         string         `json:"leechers"`
	Banned           bool           `json:"banned"`
	Visible          bool           `json:"visible"`
	PromotionRule    *PromotionRule `json:"promotionRule"`
	MallSingleFree   interface{}    `json:"mallSingleFree"`
}
type PromotionRule struct {
	Categories  []string `json:"categories"`
	CreatedDate string   `json:"createdDate"`
	Discount    string   `json:"discount"`
}
type Showcase struct {
	CreatedDate      string     `json:"createdDate"`
	LastModifiedDate string     `json:"lastModifiedDate"`
	ID               string     `json:"id"`
	Collection       bool       `json:"collection"`
	UserID           string     `json:"userid"`
	Username         string     `json:"username"`
	CnTitle          string     `json:"cntitle"`
	EnTitle          string     `json:"entitle"`
	Note             string     `json:"note"`
	Pic              string     `json:"pic"`
	Pic1             string     `json:"pic1"`
	Pic2             string     `json:"pic2"`
	Count            string     `json:"count"`
	Size             string     `json:"size"`
	View             string     `json:"view"`
	Statistics       Statistics `json:"statistics"`
}
type Statistics struct {
	CreatedDate      string `json:"createdDate"`
	LastModifiedDate string `json:"lastModifiedDate"`
	ID               string `json:"id"`
	Day              string `json:"day"`
	Week             string `json:"week"`
	Month            string `json:"month"`
	Year             string `json:"year"`
	DayClick         string `json:"dayClick"`
	WeekClick        string `json:"weekClick"`
	MonthClick       string `json:"monthClick"`
	YearClick        string `json:"yearClick"`
}

func isInLowerCaseSet(s string, set []string) bool {
	lowerStr := strings.ToLower(s)
	for _, item := range set {
		if lowerStr == strings.ToLower(item) {
			return true
		}
	}
	return false
}

func (t MTTorrentDetail) IsFree() bool {
	if t.Status != nil {
		if isInLowerCaseSet(t.Status.Discount, freeSet) || (t.Status.PromotionRule != nil && isInLowerCaseSet(t.Status.PromotionRule.Discount, freeSet)) {
			return true
		}
	}
	return false
}

func (t MTTorrentDetail) CanbeFinished(logger *zap.Logger, enabled bool, speedLimit int) bool {
	if !enabled {
		return true
	} else {
		timeEnd, err := time.Parse("2006-01-02 15:04:05", t.Status.DiscountEndTime)
		if err != nil {
			logger.Error("解析时间失败", zap.Error(err))
			return false
		}
		torrentSizeMB, err := strconv.Atoi(t.Size)
		if err != nil {
			logger.Error("解析种子大小失败", zap.Error(err))
			return false
		}
		duration := timeEnd.Sub(time.Now())
		secondsDiff := int(duration.Seconds())
		if secondsDiff*speedLimit < (torrentSizeMB / 1024 / 1024) {
			logger.Warn("种子免费时间不足以完成下载,跳过...")
			return false
		}
		return true
	}
}

func (t MTTorrentDetail) GetFreeEndTime() *time.Time {
	timeEnd, err := time.Parse("2006-01-02 15:04:05", t.Status.DiscountEndTime)
	if err != nil {
		return nil
	}
	return &timeEnd
}
