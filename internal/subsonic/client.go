// Package subsonic is adapted from github.com/wildeyedskies/stmps
// Original: MIT License, Copyright (c) stmps contributors
package subsonic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const apiVersion = "1.16.1"

// Client holds connection details for a Subsonic-compatible server.
type Client struct {
	baseURL    string
	username   string
	password   string
	clientName string
	httpClient *http.Client
}

// NewClient creates a new Client. baseURL should not have a trailing slash.
func NewClient(baseURL, username, password, clientName string) *Client {
	return &Client{
		baseURL:    baseURL,
		username:   username,
		password:   password,
		clientName: clientName,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// ── Domain types ─────────────────────────────────────────────────────────────

type Artist struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	AlbumCount int     `json:"albumCount"`
	CoverArt   string  `json:"coverArt"`
	Albums     []Album `json:"album,omitempty"`
}

type Album struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Artist     string `json:"artist"`
	ArtistID   string `json:"artistId"`
	CoverArt   string `json:"coverArt"`
	Year       int    `json:"year"`
	SongCount  int    `json:"songCount"`
	UserRating int    `json:"userRating,omitempty"`
	Songs      []Song `json:"song,omitempty"`
}

type Song struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Artist       string `json:"artist"`
	Album        string `json:"album"`
	AlbumID      string `json:"albumId"`
	ArtistID     string `json:"artistId"`
	Duration     int    `json:"duration"`
	Track        int    `json:"track"`
	CoverArt     string `json:"coverArt"`
	ContentType  string `json:"contentType"`
	Suffix       string `json:"suffix"`
	BitRate      int    `json:"bitRate"`
	SamplingRate int    `json:"samplingRate"`
	ChannelCount int    `json:"channelCount"`
	Year         int    `json:"year"`
	Genre        string `json:"genre"`
	Size         int64  `json:"size"`
	UserRating   int    `json:"userRating,omitempty"`
}

type SearchResult struct {
	Artists []Artist `json:"artist,omitempty"`
	Albums  []Album  `json:"album,omitempty"`
	Songs   []Song   `json:"song,omitempty"`
}

type LyricLine struct {
	Start int    `json:"start"`
	Value string `json:"value"`
}

type Lyrics struct {
	Synced bool        `json:"synced"`
	Lines  []LyricLine `json:"lines"`
}

type Playlist struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SongCount int    `json:"songCount"`
	CoverArt  string `json:"coverArt"`
	Comment   string `json:"comment"`
	Songs     []Song `json:"entry,omitempty"`
}

// ── Internal response envelope ────────────────────────────────────────────────

type response struct {
	SubsonicResponse subsonicResponse `json:"subsonic-response"`
}

type subsonicResponse struct {
	Status        string             `json:"status"`
	Error         *subsonicError     `json:"error,omitempty"`
	Artists       *artistsWrapper    `json:"artists,omitempty"`
	Artist        *Artist            `json:"artist,omitempty"`
	Album         *Album             `json:"album,omitempty"`
	Song          *Song              `json:"song,omitempty"`
	SearchResult2 *SearchResult      `json:"searchResult2,omitempty"`
	Playlists     *playlistsWrapper  `json:"playlists,omitempty"`
	Playlist      *Playlist          `json:"playlist,omitempty"`
	Lyrics        *lyricsWrapper     `json:"lyrics,omitempty"`
	LyricsList    *lyricsListWrapper `json:"lyricsList,omitempty"`
}

type subsonicError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type artistsWrapper struct {
	Index []struct {
		Artists []Artist `json:"artist"`
	} `json:"index"`
}

type playlistsWrapper struct {
	Playlists []Playlist `json:"playlist"`
}

type lyricsWrapper struct {
	Value string `json:"value"`
}

type lyricsListWrapper struct {
	StructuredLyrics []structuredLyrics `json:"structuredLyrics"`
}

type structuredLyrics struct {
	Synced bool         `json:"synced"`
	Lines  []lyricsLine `json:"line"`
}

type lyricsLine struct {
	Start int    `json:"start"`
	Value string `json:"value"`
}

// ── Public API ────────────────────────────────────────────────────────────────

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.get(ctx, "ping", nil)
	return err
}

func (c *Client) GetArtists(ctx context.Context) ([]Artist, error) {
	resp, err := c.get(ctx, "getArtists", nil)
	if err != nil {
		return nil, err
	}
	if resp.Artists == nil {
		return nil, nil
	}
	var artists []Artist
	for _, idx := range resp.Artists.Index {
		artists = append(artists, idx.Artists...)
	}
	return artists, nil
}

func (c *Client) GetArtist(ctx context.Context, id string) (*Artist, error) {
	resp, err := c.get(ctx, "getArtist", url.Values{"id": {id}})
	if err != nil {
		return nil, err
	}
	return resp.Artist, nil
}

func (c *Client) GetAlbum(ctx context.Context, id string) (*Album, error) {
	resp, err := c.get(ctx, "getAlbum", url.Values{"id": {id}})
	if err != nil {
		return nil, err
	}
	return resp.Album, nil
}

func (c *Client) GetSong(ctx context.Context, id string) (*Song, error) {
	resp, err := c.get(ctx, "getSong", url.Values{"id": {id}})
	if err != nil {
		return nil, err
	}
	if resp.Song == nil {
		return nil, fmt.Errorf("subsonic: getSong returned no song for id %q", id)
	}
	return resp.Song, nil
}

func (c *Client) GetPlaylists(ctx context.Context) ([]Playlist, error) {
	resp, err := c.get(ctx, "getPlaylists", nil)
	if err != nil {
		return nil, err
	}
	if resp.Playlists == nil {
		return nil, nil
	}
	return resp.Playlists.Playlists, nil
}

func (c *Client) GetPlaylist(ctx context.Context, id string) (*Playlist, error) {
	resp, err := c.get(ctx, "getPlaylist", url.Values{"id": {id}})
	if err != nil {
		return nil, err
	}
	return resp.Playlist, nil
}

func (c *Client) GetLyrics(ctx context.Context, songID string) (*Lyrics, error) {
	// Try getLyricsBySongId first (OpenSubsonic API 1.22.0, returns synced lines).
	// Fall back to getLyrics (plain text) on any error or empty result.
	resp, err := c.get(ctx, "getLyricsBySongId", url.Values{"id": {songID}})
	if err == nil && resp.LyricsList != nil && len(resp.LyricsList.StructuredLyrics) > 0 {
		sl := resp.LyricsList.StructuredLyrics[0]
		if len(sl.Lines) > 0 {
			lines := make([]LyricLine, len(sl.Lines))
			for i, l := range sl.Lines {
				lines[i] = LyricLine(l)
			}
			return &Lyrics{Synced: sl.Synced, Lines: lines}, nil
		}
	}

	// Fall back to getLyrics (API 1.2.0). Needs artist+title, so fetch the song first.
	song, err := c.GetSong(ctx, songID)
	if err != nil {
		return nil, nil //nolint:nilnil,nilerr
	}
	resp2, err := c.get(ctx, "getLyrics", url.Values{
		"artist": {song.Artist},
		"title":  {song.Title},
	})
	if err != nil || resp2.Lyrics == nil || strings.TrimSpace(resp2.Lyrics.Value) == "" {
		return nil, nil //nolint:nilnil,nilerr
	}
	var lines []LyricLine
	for _, l := range strings.Split(resp2.Lyrics.Value, "\n") {
		lines = append(lines, LyricLine{Start: 0, Value: strings.TrimRight(l, "\r")})
	}
	return &Lyrics{Synced: false, Lines: lines}, nil
}

func (c *Client) Search(ctx context.Context, query string) (*SearchResult, error) {
	resp, err := c.get(ctx, "search2", url.Values{
		"query":       {query},
		"artistCount": {"5"},
		"albumCount":  {"5"},
		"songCount":   {"20"},
	})
	if err != nil {
		return nil, err
	}
	return resp.SearchResult2, nil
}

// StreamURL returns the authenticated stream URL for a song, to be passed directly to mpv.
func (c *Client) StreamURL(id string) string {
	p := c.authParams()
	p.Set("id", id)
	return fmt.Sprintf("%s/rest/stream?%s", c.baseURL, p.Encode())
}

// CoverArtURL returns the authenticated cover art URL for a given coverArt ID.
func (c *Client) CoverArtURL(id string, size int) string {
	p := c.authParams()
	p.Set("id", id)
	p.Set("size", strconv.Itoa(size))
	return fmt.Sprintf("%s/rest/getCoverArt?%s", c.baseURL, p.Encode())
}

func (c *Client) Scrobble(ctx context.Context, id string) error {
	_, err := c.get(ctx, "scrobble", url.Values{"id": {id}, "submission": {"true"}})
	return err
}

func (c *Client) SetRating(ctx context.Context, id string, rating int) error {
	_, err := c.get(ctx, "setRating", url.Values{
		"id":     {id},
		"rating": {strconv.Itoa(rating)},
	})
	return err
}

// CreatePlaylist creates a new playlist with the given name and optional song IDs.
// Returns the created playlist (Subsonic returns it in the response envelope).
func (c *Client) CreatePlaylist(ctx context.Context, name string, songIDs []string) (*Playlist, error) {
	p := url.Values{"name": {name}}
	for _, id := range songIDs {
		p.Add("songId", id)
	}
	resp, err := c.get(ctx, "createPlaylist", p)
	if err != nil {
		return nil, err
	}
	return resp.Playlist, nil
}

// UpdatePlaylist updates an existing playlist's name and/or modifies its songs.
// name may be empty to leave the name unchanged.
// songIDsToAdd and songIndexesToRemove may be nil/empty.
func (c *Client) UpdatePlaylist(ctx context.Context, id, name string, songIDsToAdd []string, songIndexesToRemove []int) error {
	p := url.Values{"playlistId": {id}}
	if name != "" {
		p.Set("name", name)
	}
	for _, sid := range songIDsToAdd {
		p.Add("songIdToAdd", sid)
	}
	for _, idx := range songIndexesToRemove {
		p.Add("songIndexToRemove", strconv.Itoa(idx))
	}
	_, err := c.get(ctx, "updatePlaylist", p)
	return err
}

// ReplacePlaylistSongs replaces all songs in an existing playlist with the given
// ordered list of song IDs. This is the only way to reorder playlist entries via
// the Subsonic API (createPlaylist with an existing playlistId).
func (c *Client) ReplacePlaylistSongs(ctx context.Context, id string, songIDs []string) error {
	p := url.Values{"playlistId": {id}}
	for _, sid := range songIDs {
		p.Add("songId", sid)
	}
	_, err := c.get(ctx, "createPlaylist", p)
	return err
}

// DeletePlaylist deletes the playlist with the given ID.
func (c *Client) DeletePlaylist(ctx context.Context, id string) error {
	_, err := c.get(ctx, "deletePlaylist", url.Values{"id": {id}})
	return err
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (c *Client) authParams() url.Values {
	return url.Values{
		"u": {c.username},
		"p": {c.password},
		"v": {apiVersion},
		"c": {c.clientName},
		"f": {"json"},
	}
}

func (c *Client) get(ctx context.Context, endpoint string, params url.Values) (*subsonicResponse, error) {
	p := c.authParams()
	for k, v := range params {
		p[k] = v
	}
	reqURL := fmt.Sprintf("%s/rest/%s?%s", c.baseURL, endpoint, p.Encode())

	log.Debug().Str("endpoint", endpoint).Msg("subsonic: API call")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("subsonic: build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error().Err(err).Str("endpoint", endpoint).Msg("subsonic: request failed")
		return nil, fmt.Errorf("subsonic: request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var result response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Error().Err(err).Str("endpoint", endpoint).Msg("subsonic: decode failed")
		return nil, fmt.Errorf("subsonic: decode failed: %w", err)
	}

	sr := result.SubsonicResponse
	if sr.Status != "ok" {
		if sr.Error != nil {
			err := fmt.Errorf("subsonic error %d: %s", sr.Error.Code, sr.Error.Message)
			log.Error().Err(err).Str("endpoint", endpoint).Msg("subsonic: API error")
			return nil, err
		}
		err := fmt.Errorf("subsonic status: %s", sr.Status)
		log.Error().Err(err).Str("endpoint", endpoint).Msg("subsonic: unexpected status")
		return nil, err
	}
	return &sr, nil
}
