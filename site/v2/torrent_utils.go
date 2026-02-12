package v2

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/sunerpy/requests"
	"github.com/zeebo/bencode"

	"github.com/sunerpy/pt-tools/utils/httpclient"
)

// Common errors for torrent operations
var (
	ErrInvalidTorrent  = errors.New("invalid torrent file")
	ErrInvalidMagnet   = errors.New("invalid magnet link")
	ErrTorrentNotFound = errors.New("torrent not found")
	ErrInvalidInfoHash = errors.New("invalid info hash")
)

// ParsedTorrent represents parsed torrent metadata
type ParsedTorrent struct {
	// Name is the torrent name
	Name string `json:"name"`
	// InfoHash is the 40-character hex info hash
	InfoHash string `json:"infoHash"`
	// Size is the total size in bytes
	Size int64 `json:"size"`
	// Files is the list of files in the torrent
	Files []TorrentFile `json:"files,omitempty"`
	// PieceLength is the piece size in bytes
	PieceLength int64 `json:"pieceLength"`
	// Comment is the torrent comment
	Comment string `json:"comment,omitempty"`
	// CreatedBy is the creator of the torrent
	CreatedBy string `json:"createdBy,omitempty"`
	// CreationDate is when the torrent was created
	CreationDate time.Time `json:"creationDate,omitempty"`
	// Announce is the primary tracker URL
	Announce string `json:"announce,omitempty"`
	// AnnounceList is the list of tracker URLs
	AnnounceList [][]string `json:"announceList,omitempty"`
	// RawMetadata is the raw torrent file bytes
	RawMetadata []byte `json:"-"`
}

// TorrentFile represents a file in a torrent
type TorrentFile struct {
	Path   string `json:"path"`
	Length int64  `json:"length"`
}

// torrentMetainfo represents the bencode structure of a torrent file
type torrentMetainfo struct {
	Announce     string      `bencode:"announce,omitempty"`
	AnnounceList [][]string  `bencode:"announce-list,omitempty"`
	Comment      string      `bencode:"comment,omitempty"`
	CreatedBy    string      `bencode:"created by,omitempty"`
	CreationDate int64       `bencode:"creation date,omitempty"`
	Info         torrentInfo `bencode:"info"`
}

type torrentInfo struct {
	Name        string            `bencode:"name"`
	PieceLength int64             `bencode:"piece length"`
	Pieces      string            `bencode:"pieces"`
	Length      int64             `bencode:"length,omitempty"`
	Files       []torrentFileInfo `bencode:"files,omitempty"`
	Private     int               `bencode:"private,omitempty"`
}

type torrentFileInfo struct {
	Length int64    `bencode:"length"`
	Path   []string `bencode:"path"`
}

// ComputeTorrentHash computes the info hash from torrent file bytes
func ComputeTorrentHash(data []byte) (string, error) {
	if len(data) == 0 {
		return "", ErrInvalidTorrent
	}

	// Decode the torrent to extract the info dictionary
	var metainfo map[string]any
	if err := bencode.DecodeBytes(data, &metainfo); err != nil {
		return "", fmt.Errorf("decode torrent: %w", err)
	}

	info, ok := metainfo["info"]
	if !ok {
		return "", ErrInvalidTorrent
	}

	// Re-encode the info dictionary
	infoBytes, err := bencode.EncodeBytes(info)
	if err != nil {
		return "", fmt.Errorf("encode info: %w", err)
	}

	// Compute SHA1 hash
	hash := sha1.Sum(infoBytes)
	return hex.EncodeToString(hash[:]), nil
}

// ComputeTorrentHashFromFile computes the info hash from a torrent file path
func ComputeTorrentHashFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return ComputeTorrentHash(data)
}

// ParseTorrent parses torrent file bytes into ParsedTorrent
func ParseTorrent(data []byte) (*ParsedTorrent, error) {
	if len(data) == 0 {
		return nil, ErrInvalidTorrent
	}

	var metainfo torrentMetainfo
	if err := bencode.DecodeBytes(data, &metainfo); err != nil {
		return nil, fmt.Errorf("decode torrent: %w", err)
	}

	// Compute info hash
	infoHash, err := ComputeTorrentHash(data)
	if err != nil {
		return nil, err
	}

	parsed := &ParsedTorrent{
		Name:         metainfo.Info.Name,
		InfoHash:     infoHash,
		PieceLength:  metainfo.Info.PieceLength,
		Comment:      metainfo.Comment,
		CreatedBy:    metainfo.CreatedBy,
		Announce:     metainfo.Announce,
		AnnounceList: metainfo.AnnounceList,
		RawMetadata:  data,
	}

	if metainfo.CreationDate > 0 {
		parsed.CreationDate = time.Unix(metainfo.CreationDate, 0)
	}

	// Calculate total size
	if metainfo.Info.Length > 0 {
		// Single file torrent
		parsed.Size = metainfo.Info.Length
		parsed.Files = []TorrentFile{{
			Path:   metainfo.Info.Name,
			Length: metainfo.Info.Length,
		}}
	} else {
		// Multi-file torrent
		for _, f := range metainfo.Info.Files {
			parsed.Size += f.Length
			parsed.Files = append(parsed.Files, TorrentFile{
				Path:   strings.Join(f.Path, "/"),
				Length: f.Length,
			})
		}
	}

	return parsed, nil
}

// ParseTorrentFromFile parses a torrent file from path
func ParseTorrentFromFile(path string) (*ParsedTorrent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return ParseTorrent(data)
}

// ExtractMagnetHash extracts the info hash from a magnet link
func ExtractMagnetHash(magnetURI string) (string, error) {
	if !strings.HasPrefix(magnetURI, "magnet:?") {
		return "", ErrInvalidMagnet
	}

	// Parse the magnet URI
	u, err := url.Parse(magnetURI)
	if err != nil {
		return "", fmt.Errorf("parse magnet: %w", err)
	}

	// Get the xt (exact topic) parameter
	xt := u.Query().Get("xt")
	if xt == "" {
		return "", ErrInvalidMagnet
	}

	// Extract hash from urn:btih:HASH format
	if strings.HasPrefix(xt, "urn:btih:") {
		hash := strings.TrimPrefix(xt, "urn:btih:")
		hash = strings.ToLower(hash)

		// Handle base32 encoded hashes (32 chars)
		if len(hash) == 32 {
			decoded, err := base32Decode(hash)
			if err != nil {
				return "", fmt.Errorf("decode base32 hash: %w", err)
			}
			hash = hex.EncodeToString(decoded)
		}

		// Validate hex hash (40 chars)
		if len(hash) != 40 {
			return "", ErrInvalidInfoHash
		}

		// Validate hex characters
		if _, err := hex.DecodeString(hash); err != nil {
			return "", ErrInvalidInfoHash
		}

		return hash, nil
	}

	return "", ErrInvalidMagnet
}

// base32Decode decodes a base32 string (used in magnet links)
func base32Decode(s string) ([]byte, error) {
	s = strings.ToUpper(s)
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

	var bits uint64
	var bitCount int
	var result []byte

	for _, c := range s {
		idx := strings.IndexRune(alphabet, c)
		if idx < 0 {
			return nil, fmt.Errorf("invalid base32 character: %c", c)
		}

		bits = (bits << 5) | uint64(idx)
		bitCount += 5

		if bitCount >= 8 {
			bitCount -= 8
			result = append(result, byte(bits>>bitCount))
			bits &= (1 << bitCount) - 1
		}
	}

	return result, nil
}

// GetRemoteTorrent fetches a torrent file from a URL
func GetRemoteTorrent(torrentURL, cookie string) (*ParsedTorrent, error) {
	return GetRemoteTorrentWithRequests(torrentURL, cookie)
}

// GetRemoteTorrentWithRequests fetches a torrent file from a URL using requests library
func GetRemoteTorrentWithRequests(torrentURL, cookie string) (*ParsedTorrent, error) {
	session := requests.NewSession().WithTimeout(30 * time.Second)
	if proxyURL := httpclient.ResolveProxyFromEnvironment(torrentURL); proxyURL != "" {
		session = session.WithProxy(proxyURL)
	}
	defer func() { _ = session.Close() }()

	req, err := requests.NewGet(torrentURL).Build()
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.AddHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	if cookie != "" {
		req.AddHeader("Cookie", cookie)
	}

	resp, err := session.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch torrent: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	data := resp.Bytes()
	if len(data) == 0 {
		return nil, ErrTorrentNotFound
	}

	return ParseTorrent(data)
}

// isValidTorrentContentType checks if the content type is valid for a torrent file
func isValidTorrentContentType(contentType string) bool {
	validTypes := []string{
		"application/x-bittorrent",
		"application/octet-stream",
	}

	contentType = strings.ToLower(contentType)
	for _, valid := range validTypes {
		if strings.Contains(contentType, valid) {
			return true
		}
	}
	return false
}

// ValidateInfoHash validates an info hash string
func ValidateInfoHash(hash string) bool {
	if len(hash) != 40 {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}

// NormalizeInfoHash normalizes an info hash to lowercase
func NormalizeInfoHash(hash string) string {
	return strings.ToLower(strings.TrimSpace(hash))
}

// BuildMagnetLink builds a magnet link from an info hash and optional name
func BuildMagnetLink(infoHash, name string, trackers []string) string {
	var buf bytes.Buffer
	buf.WriteString("magnet:?xt=urn:btih:")
	buf.WriteString(strings.ToLower(infoHash))

	if name != "" {
		buf.WriteString("&dn=")
		buf.WriteString(url.QueryEscape(name))
	}

	for _, tracker := range trackers {
		buf.WriteString("&tr=")
		buf.WriteString(url.QueryEscape(tracker))
	}

	return buf.String()
}

// InfoHashFromMagnet is an alias for ExtractMagnetHash
var InfoHashFromMagnet = ExtractMagnetHash

// TorrentHashRegex matches a 40-character hex info hash
var TorrentHashRegex = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)

// IsTorrentHash checks if a string is a valid torrent info hash
func IsTorrentHash(s string) bool {
	return TorrentHashRegex.MatchString(s)
}
