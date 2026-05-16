package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

var (
	ErrTorrentNotFound    = errors.New("torrent not found")
	ErrDownloaderNotFound = errors.New("downloader not found")
)

type TorrentDTO struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	SiteName string    `json:"site_name"`
	State    string    `json:"state"`
	Size     int64     `json:"size"`
	Progress float64   `json:"progress"`
	Ratio    float64   `json:"ratio"`
	ETA      int64     `json:"eta"`
	AddedAt  time.Time `json:"added_at"`
}

type TorrentService interface {
	ListByDownloader(ctx context.Context, downloaderName string, page, pageSize int) (items []TorrentDTO, total int, err error)
	Get(ctx context.Context, downloaderName, torrentID string) (TorrentDTO, error)
	Pause(ctx context.Context, downloaderName, torrentID string) error
	Resume(ctx context.Context, downloaderName, torrentID string) error
	Delete(ctx context.Context, downloaderName, torrentID string, removeData bool) error
}

type downloaderResolver interface {
	GetDownloader(name string) (downloader.Downloader, error)
}

type torrentService struct {
	resolver downloaderResolver
}

func NewTorrentService(mgr *downloader.DownloaderManager) TorrentService {
	return &torrentService{resolver: mgr}
}

func newTorrentServiceWithResolver(r downloaderResolver) TorrentService {
	return &torrentService{resolver: r}
}

func (s *torrentService) resolve(name string) (downloader.Downloader, error) {
	dl, err := s.resolver.GetDownloader(name)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrDownloaderNotFound, name)
	}
	return dl, nil
}

func (s *torrentService) ListByDownloader(_ context.Context, name string, page, pageSize int) ([]TorrentDTO, int, error) {
	dl, err := s.resolve(name)
	if err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	all, err := dl.GetTorrentsBy(downloader.TorrentFilter{})
	if err != nil {
		return nil, 0, fmt.Errorf("list torrents: %w", err)
	}
	total := len(all)

	offset := (page - 1) * pageSize
	if offset >= total {
		return []TorrentDTO{}, total, nil
	}
	end := offset + pageSize
	if end > total {
		end = total
	}
	page2 := all[offset:end]

	items := make([]TorrentDTO, 0, len(page2))
	for i := range page2 {
		items = append(items, toDTO(&page2[i]))
	}
	return items, total, nil
}

func (s *torrentService) Get(_ context.Context, name, id string) (TorrentDTO, error) {
	dl, err := s.resolve(name)
	if err != nil {
		return TorrentDTO{}, err
	}
	tr, err := dl.GetTorrent(id)
	if err != nil {
		return TorrentDTO{}, mapTorrentErr(err)
	}
	return toDTO(&tr), nil
}

func (s *torrentService) Pause(_ context.Context, name, id string) error {
	dl, err := s.resolve(name)
	if err != nil {
		return err
	}
	if err := dl.PauseTorrent(id); err != nil {
		return mapTorrentErr(err)
	}
	return nil
}

func (s *torrentService) Resume(_ context.Context, name, id string) error {
	dl, err := s.resolve(name)
	if err != nil {
		return err
	}
	if err := dl.ResumeTorrent(id); err != nil {
		return mapTorrentErr(err)
	}
	return nil
}

func (s *torrentService) Delete(_ context.Context, name, id string, removeData bool) error {
	dl, err := s.resolve(name)
	if err != nil {
		return err
	}
	if err := dl.RemoveTorrent(id, removeData); err != nil {
		return mapTorrentErr(err)
	}
	return nil
}

func mapTorrentErr(err error) error {
	if errors.Is(err, downloader.ErrTorrentNotFound) {
		return fmt.Errorf("%w: %s", ErrTorrentNotFound, err.Error())
	}
	return err
}

func toDTO(t *downloader.Torrent) TorrentDTO {
	var added time.Time
	if t.DateAdded > 0 {
		added = time.Unix(t.DateAdded, 0)
	}
	return TorrentDTO{
		ID:       t.ID,
		Name:     t.Name,
		State:    string(t.State),
		Size:     t.TotalSize,
		Progress: t.Progress,
		Ratio:    t.Ratio,
		ETA:      t.ETA,
		AddedAt:  added,
	}
}
