package subsonic_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"varakh.de/subsd/internal/subsonic"
)

// subsonicHandler is a minimal helper that wraps a response body in the
// Subsonic JSON envelope and serves it over a test HTTP server.
type subsonicHandler struct {
	t         *testing.T
	body      string // pre-built JSON response body
	status    int    // HTTP status code (default 200)
	lastPath  string // last request path seen (for assertion)
	lastQuery string
}

func (h *subsonicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.lastPath = r.URL.Path
	h.lastQuery = r.URL.RawQuery
	code := h.status
	if code == 0 {
		code = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write([]byte(h.body)) //nolint:errcheck
}

// wrap puts payload inside the standard Subsonic response envelope.
func wrap(payload string) string {
	return `{"subsonic-response":{"status":"ok",` + payload + `}}`
}

// errorResp produces a Subsonic error response.
func errorResp(code int, msg string) string {
	return `{"subsonic-response":{"status":"failed","error":{"code":` +
		intStr(code) + `,"message":"` + msg + `"}}}`
}

func intStr(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func newClient(t *testing.T, h http.Handler) *subsonic.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return subsonic.NewClient(srv.URL, "user", "pass", "test")
}

// ── GetArtists ────────────────────────────────────────────────────────────────

func TestGetArtists(t *testing.T) {
	h := &subsonicHandler{t: t, body: wrap(`"artists":{"index":[
		{"artist":[{"id":"1","name":"Artist One","albumCount":2},{"id":"2","name":"Artist Two","albumCount":1}]},
		{"artist":[{"id":"3","name":"Artist Three","albumCount":0}]}
	]}`)}
	c := newClient(t, h)
	artists, err := c.GetArtists(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artists) != 3 {
		t.Fatalf("got %d artists, want 3", len(artists))
	}
	if artists[0].ID != "1" || artists[0].Name != "Artist One" {
		t.Errorf("first artist: %+v", artists[0])
	}
	if artists[2].ID != "3" {
		t.Errorf("third artist: %+v", artists[2])
	}
}

func TestGetArtists_EmptyIndex(t *testing.T) {
	h := &subsonicHandler{t: t, body: wrap(`"artists":{"index":[]}`)}
	c := newClient(t, h)
	artists, err := c.GetArtists(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artists) != 0 {
		t.Errorf("expected empty slice, got %d", len(artists))
	}
}

func TestGetArtists_NilArtists(t *testing.T) {
	// Response with no "artists" key at all.
	h := &subsonicHandler{t: t, body: wrap(`"version":"1.16.1"`)}
	c := newClient(t, h)
	artists, err := c.GetArtists(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if artists != nil {
		t.Errorf("expected nil, got %v", artists)
	}
}

// ── GetArtist ────────────────────────────────────────────────────────────────

func TestGetArtist(t *testing.T) {
	body := wrap(`"artist":{"id":"42","name":"Test Artist","albumCount":1,"album":[{"id":"a1","name":"Album 1"}]}`)
	c := newClient(t, &subsonicHandler{t: t, body: body})
	a, err := c.GetArtist(context.Background(), "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a == nil || a.ID != "42" || a.Name != "Test Artist" {
		t.Errorf("unexpected artist: %+v", a)
	}
	if len(a.Albums) != 1 || a.Albums[0].ID != "a1" {
		t.Errorf("unexpected albums: %+v", a.Albums)
	}
}

// ── GetAlbum ─────────────────────────────────────────────────────────────────

func TestGetAlbum(t *testing.T) {
	body := wrap(`"album":{"id":"alb1","name":"Great Album","artist":"The Band","song":[{"id":"s1","title":"Track 1"},{"id":"s2","title":"Track 2"}]}`)
	c := newClient(t, &subsonicHandler{t: t, body: body})
	alb, err := c.GetAlbum(context.Background(), "alb1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alb.ID != "alb1" || alb.Name != "Great Album" {
		t.Errorf("unexpected album: %+v", alb)
	}
	if len(alb.Songs) != 2 {
		t.Errorf("expected 2 songs, got %d", len(alb.Songs))
	}
}

// ── GetSong ──────────────────────────────────────────────────────────────────

func TestGetSong(t *testing.T) {
	body := wrap(`"song":{"id":"s99","title":"My Song","artist":"Me","duration":200}`)
	c := newClient(t, &subsonicHandler{t: t, body: body})
	song, err := c.GetSong(context.Background(), "s99")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if song.ID != "s99" || song.Title != "My Song" {
		t.Errorf("unexpected song: %+v", song)
	}
}

func TestGetSong_NilResponse(t *testing.T) {
	// Server returns ok but no "song" field.
	body := wrap(`"version":"1.16.1"`)
	c := newClient(t, &subsonicHandler{t: t, body: body})
	_, err := c.GetSong(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error when song is absent")
	}
	if !strings.Contains(err.Error(), "no song") {
		t.Errorf("expected 'no song' in error, got: %v", err)
	}
}

// ── GetPlaylists ─────────────────────────────────────────────────────────────

func TestGetPlaylists(t *testing.T) {
	body := wrap(`"playlists":{"playlist":[{"id":"p1","name":"Playlist 1","songCount":5},{"id":"p2","name":"Playlist 2","songCount":3}]}`)
	c := newClient(t, &subsonicHandler{t: t, body: body})
	pls, err := c.GetPlaylists(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pls) != 2 {
		t.Fatalf("got %d playlists, want 2", len(pls))
	}
	if pls[0].ID != "p1" || pls[1].ID != "p2" {
		t.Errorf("unexpected playlists: %+v", pls)
	}
}

func TestGetPlaylists_Nil(t *testing.T) {
	body := wrap(`"version":"1.16.1"`)
	c := newClient(t, &subsonicHandler{t: t, body: body})
	pls, err := c.GetPlaylists(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pls != nil {
		t.Errorf("expected nil, got %v", pls)
	}
}

// ── GetPlaylist ───────────────────────────────────────────────────────────────

func TestGetPlaylist(t *testing.T) {
	body := wrap(`"playlist":{"id":"p1","name":"My Playlist","songCount":1,"entry":[{"id":"s1","title":"Track"}]}`)
	c := newClient(t, &subsonicHandler{t: t, body: body})
	pl, err := c.GetPlaylist(context.Background(), "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pl.ID != "p1" || len(pl.Songs) != 1 {
		t.Errorf("unexpected playlist: %+v", pl)
	}
}

// ── Search ───────────────────────────────────────────────────────────────────

func TestSearch(t *testing.T) {
	body := wrap(`"searchResult2":{"artist":[{"id":"a1","name":"Found Artist"}],"song":[{"id":"s1","title":"Found Song"}]}`)
	h := &subsonicHandler{t: t, body: body}
	c := newClient(t, h)
	result, err := c.Search(context.Background(), "found")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Artists) != 1 || result.Artists[0].ID != "a1" {
		t.Errorf("unexpected artists: %+v", result.Artists)
	}
	if len(result.Songs) != 1 || result.Songs[0].ID != "s1" {
		t.Errorf("unexpected songs: %+v", result.Songs)
	}
	// Verify query parameters are forwarded.
	if !strings.Contains(h.lastQuery, "query=found") {
		t.Errorf("expected query param in request, got: %s", h.lastQuery)
	}
}

// ── Scrobble ─────────────────────────────────────────────────────────────────

func TestScrobble(t *testing.T) {
	h := &subsonicHandler{t: t, body: wrap(`"version":"1.16.1"`)}
	c := newClient(t, h)
	if err := c.Scrobble(context.Background(), "s1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(h.lastQuery, "id=s1") {
		t.Errorf("expected id param in request, got: %s", h.lastQuery)
	}
	if !strings.Contains(h.lastQuery, "submission=true") {
		t.Errorf("expected submission=true in request, got: %s", h.lastQuery)
	}
}

// ── URL helpers ───────────────────────────────────────────────────────────────

func TestStreamURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	c := subsonic.NewClient(srv.URL, "user", "pass", "test")
	u := c.StreamURL("song123")
	if !strings.HasPrefix(u, srv.URL+"/rest/stream") {
		t.Errorf("unexpected StreamURL: %s", u)
	}
	if !strings.Contains(u, "id=song123") {
		t.Errorf("missing id param: %s", u)
	}
	if !strings.Contains(u, "u=user") {
		t.Errorf("missing u param: %s", u)
	}
}

func TestCoverArtURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	c := subsonic.NewClient(srv.URL, "user", "pass", "test")
	u := c.CoverArtURL("cover42", 300)
	if !strings.HasPrefix(u, srv.URL+"/rest/getCoverArt") {
		t.Errorf("unexpected CoverArtURL: %s", u)
	}
	if !strings.Contains(u, "id=cover42") {
		t.Errorf("missing id param: %s", u)
	}
	if !strings.Contains(u, "size=300") {
		t.Errorf("missing size param: %s", u)
	}
}

// ── Error cases ───────────────────────────────────────────────────────────────

func TestSubsonicError_WithCode(t *testing.T) {
	h := &subsonicHandler{t: t, body: errorResp(40, "wrong credentials")}
	c := newClient(t, h)
	_, err := c.GetArtists(context.Background())
	if err == nil {
		t.Fatal("expected error from Subsonic error response")
	}
	if !strings.Contains(err.Error(), "40") || !strings.Contains(err.Error(), "wrong credentials") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSubsonicError_UnexpectedStatus(t *testing.T) {
	body := `{"subsonic-response":{"status":"unknown"}}`
	c := newClient(t, &subsonicHandler{t: t, body: body})
	_, err := c.GetArtists(context.Background())
	if err == nil {
		t.Fatal("expected error for unexpected status")
	}
}

func TestHTTPError(t *testing.T) {
	// Point client at a closed server to trigger a network error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // close immediately
	c := subsonic.NewClient(srv.URL, "user", "pass", "test")
	_, err := c.GetArtists(context.Background())
	if err == nil {
		t.Fatal("expected error for failed HTTP request")
	}
}

func TestDecodeError(t *testing.T) {
	h := &subsonicHandler{t: t, body: "not json at all {{{{"}
	c := newClient(t, h)
	_, err := c.GetArtists(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}
