package site

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/gocolly/colly"
	"github.com/sunerpy/pt-tools/models"
)

// mergeHeaders 合并 headers 和 customHeaders，customHeaders 优先覆盖
func mergeHeaders(headers, customHeaders map[string]string) map[string]string {
	// 创建一个新 map，包含 headers 的所有键值对
	merged := make(map[string]string, len(headers))
	for k, v := range headers {
		merged[k] = v
	}
	// 用 customHeaders 的键值对覆盖
	for k, v := range customHeaders {
		merged[k] = v
	}
	return merged
}

// CommonFetchTorrentInfo 获取种子信息
func CommonFetchTorrentInfo(ctx context.Context, c *colly.Collector, conf *SiteMapConfig, url string) (*models.PHPTorrentInfo, error) {
	// 设置请求头
	c.OnRequest(func(r *colly.Request) {
		select {
		case <-ctx.Done():
			r.Abort()
			return
		default:
		}
		r.Headers.Set("Cookie", conf.SharedConfig.Cookie)
		r.Headers.Set("Referer", conf.SharedConfig.SiteCfg.RefererConf.GetReferer())
		headers := mergeHeaders(conf.SharedConfig.Headers, conf.CustomHeaders)
		for k, v := range headers {
			r.Headers.Set(k, v)
		}
	})
	var torrentInfo models.PHPTorrentInfo
	var fetchErr error
	// 页面解析
	c.OnHTML("body", func(e *colly.HTMLElement) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if strings.Contains(e.Request.URL.String(), "login") {
			fetchErr = fmt.Errorf("无法访问站点，请检查您的 Cookie 是否有效")
			return
		}
		conf.Parser.ParseTitleAndID(e, &torrentInfo)
		conf.Parser.ParseDiscount(e, &torrentInfo)
		conf.Parser.ParseHR(e, &torrentInfo)
		conf.Parser.ParseTorrentSizeMB(e, &torrentInfo)
	})
	// 开始抓取
	err := c.Visit(url)
	if err != nil {
		return nil, err
	}
	// 检查上下文
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if fetchErr != nil {
		return nil, fetchErr
	}
	return &torrentInfo, nil
}

// CommonFetchMultiTorrents 并发抓取多个种子信息
// CommonFetchMultiTorrents 并发抓取多个种子信息，并返回结果和单一错误
func CommonFetchMultiTorrents(ctx context.Context, c *colly.Collector, conf *SiteMapConfig, urls []string) ([]*models.PHPTorrentInfo, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex                                       // 用于保护共享资源
	results := make([]*models.PHPTorrentInfo, 0, len(urls)) // 存储抓取结果
	var combinedErr error                                   // 单一错误
	for _, url := range urls {
		if ctx.Err() != nil {
			logger.Warn("fetchMultipleTorrents 循环已取消，跳过剩余任务")
			combinedErr = ctx.Err()
			break
		}
		wg.Add(1)
		go func(conf *SiteMapConfig) {
			defer wg.Done()
			info, err := CommonFetchTorrentInfo(ctx, c, conf, url)
			mu.Lock() // 锁定共享资源
			defer mu.Unlock()
			if err != nil {
				if combinedErr == nil {
					combinedErr = fmt.Errorf("Error fetching torrent info for %s: %v", url, err)
				} else {
					combinedErr = fmt.Errorf("%w; Error fetching torrent info for %s: %v", combinedErr, url, err)
				}
			} else {
				results = append(results, info)
			}
		}(conf)
	}
	wg.Wait()
	// 检查上下文是否已取消
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return results, combinedErr
}
