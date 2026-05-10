// Package nfo implements NFO (metadata XML) writers compatible with Kodi,
// Jellyfin, and Emby media servers.
//
// # Dialects
//
// This package provides a base Kodi-compatible XML writer and Jellyfin/Emby
// dialects built as thin extensions. See writer.go (added in Task 10) for
// high-level WriteMovieNfo / WriteTvShowNfo / WriteEpisodeNfo APIs.
//
// # Design
//
// encoding/xml.Marshal is NOT used because:
//   - Struct tag ordering is not guaranteed (Kodi requires specific tag order)
//   - No support for selective self-closing empty elements
//   - No comment support
//
// Instead, xmlWriter wraps bufio.Writer with explicit openBlock/closeBlock/
// writeElement helpers that produce deterministic output for byte-level
// comparison with TMM reference NFOs.
package nfo
