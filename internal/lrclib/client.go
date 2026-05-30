// Package lrclib fetches lyrics from the LRCLIB free public API (lrclib.net).
package lrclib

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"varakh.de/subsd/internal/subsonic"
)

// Client fetches lyrics from lrclib.net.
type Client struct {
	http *http.Client
}

// New creates a Client with sensible timeouts.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 10 * time.Second}}
}

// GetLyrics queries LRCLIB for the best matching lyrics.
// Returns nil, nil when LRCLIB has no lyrics for the song.
func (c *Client) GetLyrics(ctx context.Context, artist, title, album string, duration int) (*subsonic.Lyrics, error) {
	q := url.Values{
		"artist_name": {artist},
		"track_name":  {title},
		"album_name":  {album},
		"duration":    {strconv.Itoa(duration)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://lrclib.net/api/get?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("lrclib: build request: %w", err)
	}
	req.Header.Set("User-Agent", "subsd")

	log.Debug().Str("artist", artist).Str("title", title).Msg("lrclib: fetching lyrics")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lrclib: request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		log.Debug().Str("artist", artist).Str("title", title).Msg("lrclib: no lyrics found")
		return nil, nil //nolint:nilnil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lrclib: unexpected status %d", resp.StatusCode)
	}

	var body struct {
		SyncedLyrics string `json:"syncedLyrics"`
		PlainLyrics  string `json:"plainLyrics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("lrclib: decode: %w", err)
	}

	if body.SyncedLyrics != "" {
		if lines := parseLRC(body.SyncedLyrics); len(lines) > 0 {
			log.Debug().Str("artist", artist).Str("title", title).Int("lines", len(lines)).Msg("lrclib: synced lyrics found")
			return &subsonic.Lyrics{Synced: true, Lines: lines}, nil
		}
	}
	if body.PlainLyrics != "" {
		var lines []subsonic.LyricLine
		for _, l := range strings.Split(body.PlainLyrics, "\n") {
			lines = append(lines, subsonic.LyricLine{Start: 0, Value: strings.TrimRight(l, "\r")})
		}
		if len(lines) > 0 {
			log.Debug().Str("artist", artist).Str("title", title).Int("lines", len(lines)).Msg("lrclib: plain lyrics found")
			return &subsonic.Lyrics{Synced: false, Lines: lines}, nil
		}
	}
	log.Debug().Str("artist", artist).Str("title", title).Msg("lrclib: response had no usable lyrics")
	return nil, nil //nolint:nilnil
}

// lrcLineRe matches LRC timestamp lines: [mm:ss.xx] or [mm:ss.xxx]
var lrcLineRe = regexp.MustCompile(`^\[(\d+):(\d+)\.(\d+)\]\s*(.*)$`)

// parseLRC parses an LRC-format string into LyricLines with millisecond timestamps.
// LRC format: [mm:ss.xx] line text  where xx = hundredths (2 digits) or xxx = ms (3 digits).
func parseLRC(s string) []subsonic.LyricLine {
	var lines []subsonic.LyricLine
	for _, raw := range strings.Split(s, "\n") {
		raw = strings.TrimRight(raw, "\r")
		m := lrcLineRe.FindStringSubmatch(raw)
		if m == nil {
			continue
		}
		mm, _ := strconv.Atoi(m[1])
		ss, _ := strconv.Atoi(m[2])
		// Normalize fractional part to milliseconds by right-padding to 3 digits.
		frac := m[3]
		for len(frac) < 3 {
			frac += "0"
		}
		fracMs, _ := strconv.Atoi(frac[:3])
		ms := mm*60000 + ss*1000 + fracMs
		lines = append(lines, subsonic.LyricLine{Start: ms, Value: m[4]})
	}
	return lines
}
