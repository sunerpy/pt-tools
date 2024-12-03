package site

import (
	"github.com/gocolly/colly"
	"github.com/sunerpy/pt-tools/models"
)

type SiteParser interface {
	ParseTitleAndID(e *colly.HTMLElement, info *models.PHPTorrentInfo)
	ParseDiscount(e *colly.HTMLElement, info *models.PHPTorrentInfo)
	ParseHR(e *colly.HTMLElement, info *models.PHPTorrentInfo)
	ParseTorrentSizeMB(e *colly.HTMLElement, info *models.PHPTorrentInfo)
}
