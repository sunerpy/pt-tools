package site

import (
	"net/http"
	"time"

	"github.com/gocolly/colly"
)

func NewCollectorWithTransport() *colly.Collector {
	// 创建并配置 Collector
	// colly.AllowURLRevisit()
	c := colly.NewCollector()
	c.AllowURLRevisit = true
	c.SetRequestTimeout(30 * time.Second)
	c.WithTransport(&http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     30 * time.Second,
	})
	return c
}
