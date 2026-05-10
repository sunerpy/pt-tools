package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

type TorrentCompletedPayload struct {
	MediaPath string
	LibraryID *uint
	Type      string
}

type EventBridge struct {
	bus    core.EventBus
	scrape *ScrapeService
	log    Logger
}

func NewEventBridge(bus core.EventBus, scrape *ScrapeService, log Logger) *EventBridge {
	if bus == nil {
		bus = core.NopEventBus{}
	}
	if log == nil {
		log = noopLogger{}
	}
	return &EventBridge{bus: bus, scrape: scrape, log: log}
}

func (b *EventBridge) Start(ctx context.Context) error {
	if b.scrape == nil {
		return fmt.Errorf("event bridge: nil scrape service")
	}
	ch, cancel, err := b.bus.Subscribe(ctx, core.EventTypeTorrentCompleted)
	if err != nil {
		return fmt.Errorf("event bridge subscribe: %w", err)
	}
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				b.handleEvent(ctx, ev)
			}
		}
	}()
	return nil
}

func (b *EventBridge) handleEvent(ctx context.Context, ev core.Event) {
	if ev.Type != core.EventTypeTorrentCompleted {
		return
	}
	payload, ok := decodeTorrentCompletedPayload(ev.Payload)
	if !ok {
		b.log.Warnf("event bridge: unsupported payload %T", ev.Payload)
		return
	}
	if strings.TrimSpace(payload.MediaPath) == "" {
		b.log.Warnf("event bridge: empty media path")
		return
	}

	switch normalizeScrapeKind(payload.Type) {
	case "movie":
		if _, err := b.scrape.ScrapeMovie(ctx, ScrapeMovieRequest{
			LibraryID: payload.LibraryID,
			MediaPath: payload.MediaPath,
		}); err != nil {
			b.log.Errorf("event bridge scrape movie %s: %v", payload.MediaPath, err)
		}
	case "tv":
		if _, err := b.scrape.ScrapeTvShow(ctx, ScrapeTvShowRequest{
			LibraryID: payload.LibraryID,
			MediaPath: payload.MediaPath,
		}); err != nil {
			b.log.Errorf("event bridge scrape tv %s: %v", payload.MediaPath, err)
		}
	case "episode":
		if _, err := b.scrape.ScrapeEpisode(ctx, ScrapeEpisodeRequest{
			LibraryID: payload.LibraryID,
			MediaPath: payload.MediaPath,
		}); err != nil {
			b.log.Errorf("event bridge scrape episode %s: %v", payload.MediaPath, err)
		}
	default:
		b.log.Warnf("event bridge: unsupported media type %q", payload.Type)
	}
}

func decodeTorrentCompletedPayload(payload any) (TorrentCompletedPayload, bool) {
	switch v := payload.(type) {
	case TorrentCompletedPayload:
		return v, true
	case *TorrentCompletedPayload:
		if v == nil {
			return TorrentCompletedPayload{}, false
		}
		return *v, true
	case map[string]any:
		result := TorrentCompletedPayload{}
		mediaPath, ok := v["MediaPath"].(string)
		if !ok || strings.TrimSpace(mediaPath) == "" {
			mediaPath, _ = v["media_path"].(string)
		}
		result.MediaPath = mediaPath
		if kind, ok := v["Type"].(string); ok {
			result.Type = kind
		} else if kind, ok := v["type"].(string); ok {
			result.Type = kind
		}
		if id, ok := extractUint(v["LibraryID"]); ok {
			result.LibraryID = &id
		} else if id, ok := extractUint(v["library_id"]); ok {
			result.LibraryID = &id
		}
		return result, strings.TrimSpace(result.MediaPath) != ""
	default:
		return TorrentCompletedPayload{}, false
	}
}

func extractUint(v any) (uint, bool) {
	switch n := v.(type) {
	case uint:
		return n, true
	case uint8:
		return uint(n), true
	case uint16:
		return uint(n), true
	case uint32:
		return uint(n), true
	case uint64:
		return uint(n), true
	case int:
		if n >= 0 {
			return uint(n), true
		}
	case int8:
		if n >= 0 {
			return uint(n), true
		}
	case int16:
		if n >= 0 {
			return uint(n), true
		}
	case int32:
		if n >= 0 {
			return uint(n), true
		}
	case int64:
		if n >= 0 {
			return uint(n), true
		}
	case float64:
		if n >= 0 {
			return uint(n), true
		}
	}
	return 0, false
}

func normalizeScrapeKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "movie":
		return "movie"
	case "tv", "tvshow", "tv_show", "show", "series":
		return "tv"
	case "episode":
		return "episode"
	default:
		return ""
	}
}
