package v2

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// Batch download errors
var (
	ErrNoFreeTorrents        = errors.New("no free torrents found")
	ErrArchiveCreationFailed = errors.New("failed to create archive")
	ErrTorrentDownloadFailed = errors.New("failed to download torrent")
)

// TorrentManifest represents metadata for a torrent in the batch download
type TorrentManifest struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	SizeBytes     int64         `json:"sizeBytes"`
	DiscountLevel DiscountLevel `json:"discountLevel"`
	DownloadURL   string        `json:"downloadUrl"`
	Category      string        `json:"category,omitempty"`
	Seeders       int           `json:"seeders,omitempty"`
	Leechers      int           `json:"leechers,omitempty"`
}

// FreeTorrentBatchResult represents the result of a batch download operation
type FreeTorrentBatchResult struct {
	// ArchivePath is the path to the created archive file
	ArchivePath string `json:"archivePath"`
	// ArchiveType is the type of archive (tar.gz or zip)
	ArchiveType string `json:"archiveType"`
	// TorrentCount is the number of torrents in the archive
	TorrentCount int `json:"torrentCount"`
	// TotalSize is the total size of all torrents (bytes)
	TotalSize int64 `json:"totalSize"`
	// Manifest contains metadata for all torrents
	Manifest []TorrentManifest `json:"manifest"`
}

// BatchDownloadService provides batch download functionality for free torrents
type BatchDownloadService struct {
	site   Site
	logger *zap.Logger
}

// NewBatchDownloadService creates a new batch download service
func NewBatchDownloadService(site Site, logger *zap.Logger) *BatchDownloadService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &BatchDownloadService{
		site:   site,
		logger: logger,
	}
}

// FetchFreeTorrents fetches all free torrents from the site
// This method only filters by free status, no other filter rules are applied
func (s *BatchDownloadService) FetchFreeTorrents(ctx context.Context) ([]TorrentItem, error) {
	s.logger.Info("Fetching free torrents", zap.String("site", s.site.ID()))

	// Search for free torrents only
	query := SearchQuery{
		FreeOnly: true,
		PageSize: 100, // Fetch up to 100 torrents
	}

	torrents, err := s.site.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("search free torrents: %w", err)
	}

	// Filter to ensure only free torrents are included
	freeTorrents := make([]TorrentItem, 0, len(torrents))
	for _, t := range torrents {
		if t.IsFree() {
			freeTorrents = append(freeTorrents, t)
		}
	}

	s.logger.Info("Found free torrents",
		zap.String("site", s.site.ID()),
		zap.Int("count", len(freeTorrents)),
	)

	return freeTorrents, nil
}

// DownloadFreeTorrents downloads all free torrents and packages them into an archive
func (s *BatchDownloadService) DownloadFreeTorrents(
	ctx context.Context,
	archiveType string, // "tar.gz" or "zip"
	outputDir string,
) (*FreeTorrentBatchResult, error) {
	// Fetch free torrents
	torrents, err := s.FetchFreeTorrents(ctx)
	if err != nil {
		return nil, err
	}

	if len(torrents) == 0 {
		return nil, ErrNoFreeTorrents
	}

	// Create temp directory for torrent files
	tempDir, err := os.MkdirTemp("", "batch_download_*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Download each torrent
	manifest := make([]TorrentManifest, 0, len(torrents))
	var totalSize int64
	downloadedFiles := make([]string, 0, len(torrents))

	for _, t := range torrents {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		s.logger.Debug("Downloading torrent",
			zap.String("id", t.ID),
			zap.String("title", t.Title),
		)

		// Download torrent file
		data, dlErr := s.site.Download(ctx, t.ID)
		if dlErr != nil {
			s.logger.Warn("Failed to download torrent",
				zap.String("id", t.ID),
				zap.Error(dlErr),
			)
			continue // Skip failed downloads
		}

		// Save to temp file
		filename := fmt.Sprintf("%s.torrent", sanitizeFilename(t.Title))
		filepath := filepath.Join(tempDir, filename)
		if writeErr := os.WriteFile(filepath, data, 0o644); writeErr != nil {
			s.logger.Warn("Failed to save torrent file",
				zap.String("id", t.ID),
				zap.Error(writeErr),
			)
			continue
		}

		downloadedFiles = append(downloadedFiles, filepath)
		totalSize += t.SizeBytes

		manifest = append(manifest, TorrentManifest{
			ID:            t.ID,
			Title:         t.Title,
			SizeBytes:     t.SizeBytes,
			DiscountLevel: t.DiscountLevel,
			DownloadURL:   t.DownloadURL,
			Category:      t.Category,
			Seeders:       t.Seeders,
			Leechers:      t.Leechers,
		})
	}

	if len(downloadedFiles) == 0 {
		return nil, fmt.Errorf("%w: all downloads failed", ErrTorrentDownloadFailed)
	}

	// Write manifest file
	manifestPath := filepath.Join(tempDir, "manifest.json")
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}
	downloadedFiles = append(downloadedFiles, manifestPath)

	// Create archive
	archiveName := fmt.Sprintf("%s_free_torrents_%s", s.site.ID(), time.Now().Format("20060102_150405"))
	var archivePath string

	switch archiveType {
	case "zip":
		archivePath = filepath.Join(outputDir, archiveName+".zip")
		if err := createZipArchive(archivePath, tempDir, downloadedFiles); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrArchiveCreationFailed, err)
		}
	default: // tar.gz
		archiveType = "tar.gz"
		archivePath = filepath.Join(outputDir, archiveName+".tar.gz")
		if err := createTarGzArchive(archivePath, tempDir, downloadedFiles); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrArchiveCreationFailed, err)
		}
	}

	s.logger.Info("Created archive",
		zap.String("path", archivePath),
		zap.Int("torrentCount", len(manifest)),
	)

	return &FreeTorrentBatchResult{
		ArchivePath:  archivePath,
		ArchiveType:  archiveType,
		TorrentCount: len(manifest),
		TotalSize:    totalSize,
		Manifest:     manifest,
	}, nil
}

// createTarGzArchive creates a tar.gz archive from the given files
func createTarGzArchive(archivePath, baseDir string, files []string) error {
	// Create output file
	outFile, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("create archive file: %w", err)
	}
	defer outFile.Close()

	// Create gzip writer
	gzWriter := gzip.NewWriter(outFile)
	defer gzWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	for _, filePath := range files {
		if err := addFileToTar(tarWriter, baseDir, filePath); err != nil {
			return fmt.Errorf("add file to tar: %w", err)
		}
	}

	return nil
}

// addFileToTar adds a single file to a tar archive
func addFileToTar(tw *tar.Writer, baseDir, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	// Get relative path
	relPath, err := filepath.Rel(baseDir, filePath)
	if err != nil {
		relPath = filepath.Base(filePath)
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = relPath

	if writeErr := tw.WriteHeader(header); writeErr != nil {
		return writeErr
	}

	_, err = io.Copy(tw, file)
	return err
}

// createZipArchive creates a zip archive from the given files
func createZipArchive(archivePath, baseDir string, files []string) error {
	// Create output file
	outFile, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("create archive file: %w", err)
	}
	defer outFile.Close()

	// Create zip writer
	zipWriter := zip.NewWriter(outFile)
	defer zipWriter.Close()

	for _, filePath := range files {
		if err := addFileToZip(zipWriter, baseDir, filePath); err != nil {
			return fmt.Errorf("add file to zip: %w", err)
		}
	}

	return nil
}

// addFileToZip adds a single file to a zip archive
func addFileToZip(zw *zip.Writer, baseDir, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	// Get relative path
	relPath, err := filepath.Rel(baseDir, filePath)
	if err != nil {
		relPath = filepath.Base(filePath)
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = relPath
	header.Method = zip.Deflate

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, file)
	return err
}

// sanitizeFilename removes or replaces invalid characters in a filename
func sanitizeFilename(name string) string {
	// Replace invalid characters with underscore
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := name
	for _, char := range invalid {
		result = replaceAll(result, char, "_")
	}

	// Limit length
	if len(result) > 200 {
		result = result[:200]
	}

	// Remove leading/trailing spaces and dots
	result = trimChars(result, " .")

	if result == "" {
		result = "unnamed"
	}

	return result
}

// replaceAll replaces all occurrences of old with new in s
func replaceAll(s, old, new string) string {
	for {
		idx := indexOf(s, old)
		if idx == -1 {
			break
		}
		s = s[:idx] + new + s[idx+len(old):]
	}
	return s
}

// indexOf returns the index of substr in s, or -1 if not found
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// trimChars removes leading and trailing characters from s
func trimChars(s, chars string) string {
	for len(s) > 0 && containsChar(chars, s[0]) {
		s = s[1:]
	}
	for len(s) > 0 && containsChar(chars, s[len(s)-1]) {
		s = s[:len(s)-1]
	}
	return s
}

// containsChar checks if chars contains c
func containsChar(chars string, c byte) bool {
	for i := 0; i < len(chars); i++ {
		if chars[i] == c {
			return true
		}
	}
	return false
}
