package douban

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

var yearRegexp = regexp.MustCompile(`(19|20)\d{2}`)

func parseHTMLDetail(subjectID string, body io.Reader) (*htmlDetail, error) {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, fmt.Errorf("parse douban html: %w: %v", core.ErrParseFailed, err)
	}

	detail := &htmlDetail{
		ID:            subjectID,
		Title:         cleanText(doc.Find(`h1 span[property="v:itemreviewed"]`).First().Text()),
		Plot:          cleanText(doc.Find(`span[property="v:summary"]`).First().Text()),
		IMDBID:        extractIMDBID(doc),
		Directors:     collectTexts(doc, `a[rel="v:directedBy"]`),
		Actors:        collectTexts(doc, `a[rel="v:starring"]`),
		OriginalTitle: cleanText(doc.Find("meta[property='og:title']").AttrOr("content", "")),
	}

	ratingText := cleanText(doc.Find("strong.ll.rating_num").First().Text())
	if ratingText != "" {
		if score, parseErr := strconv.ParseFloat(ratingText, 64); parseErr == nil {
			detail.Rating = score
		}
	}

	yearText := cleanText(doc.Find("span.year").First().Text())
	if match := yearRegexp.FindString(yearText); match != "" {
		if year, parseErr := strconv.Atoi(match); parseErr == nil {
			detail.Year = year
		}
	}

	if detail.OriginalTitle == "" {
		detail.OriginalTitle = detail.Title
	}
	if detail.Title == "" {
		return nil, fmt.Errorf("douban html title missing: %w", core.ErrNotFound)
	}

	return detail, nil
}

func collectTexts(doc *goquery.Document, selector string) []string {
	items := make([]string, 0)
	doc.Find(selector).Each(func(_ int, sel *goquery.Selection) {
		text := cleanText(sel.Text())
		if text != "" {
			items = append(items, text)
		}
	})
	return items
}

func extractIMDBID(doc *goquery.Document) string {
	href, ok := doc.Find(`a[href*="imdb.com/title/"]`).First().Attr("href")
	if !ok {
		return ""
	}
	match := regexp.MustCompile(`tt\d+`).FindString(href)
	return match
}

func cleanText(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}
