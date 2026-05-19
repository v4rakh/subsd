package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
	"varakh.de/subsd/internal/cache"
	"varakh.de/subsd/internal/player"
	"varakh.de/subsd/internal/subsonic"
)

// SubsonicClient is the subset of *subsonic.Client used by the server.
// Tests may substitute a fake implementation.
type SubsonicClient interface {
	GetArtists(ctx context.Context) ([]subsonic.Artist, error)
	GetArtist(ctx context.Context, id string) (*subsonic.Artist, error)
	GetAlbum(ctx context.Context, id string) (*subsonic.Album, error)
	GetSong(ctx context.Context, id string) (*subsonic.Song, error)
	GetPlaylists(ctx context.Context) ([]subsonic.Playlist, error)
	GetPlaylist(ctx context.Context, id string) (*subsonic.Playlist, error)
	Search(ctx context.Context, query string) (*subsonic.SearchResult, error)
	Scrobble(ctx context.Context, id string) error
	StreamURL(id string) string
	CoverArtURL(id string, size int) string
}

// PlayerController is the subset of *player.Player used by the server.
// Tests may substitute a fake implementation.
type PlayerController interface {
	OnChange(fn func(player.State))
	OnTrackEnd(fn func(player.Track))
	GetState() player.State
	SetLastScrobble(status string)
	Play()
	Pause()
	PlayPause()
	Next()
	Prev()
	Seek(seconds float64)
	SetVolume(vol int)
	ToggleShuffle()
	ToggleRepeat()
	SetQueue(tracks []player.Track, startIdx int)
	AddToQueue(t player.Track)
	AddAllToQueue(tracks []player.Track)
	RemoveFromQueue(idx int)
	MoveInQueue(from, to int)
	ClearQueue()
	JumpTo(idx int)
	GetAudioDevices() ([]player.AudioDevice, error)
	GetAudioDevice() string
	SetAudioDevice(name string) error
}

// coverArtKey uniquely identifies a cover art request by Subsonic ID and
// pixel size so different sizes of the same image are cached separately.
type coverArtKey struct {
	id   string
	size int
}

// coverArtEntry is the in-memory representation of a cached cover art response.
type coverArtEntry struct {
	data        []byte
	contentType string
}

const (
	libraryCacheTTL  = 5 * time.Minute
	coverArtCacheTTL = 24 * time.Hour

	// artistsKey is the singleton cache key for the full artists list.
	artistsKey = "artists"
	// playlistsKey is the singleton cache key for the full playlists list.
	playlistsKey = "playlists"
)

// wsClient wraps a WebSocket connection with its own write mutex.
// gorilla/websocket allows one concurrent reader and one concurrent writer;
// the mutex ensures broadcast and the initial sendTo never write in parallel.
type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *wsClient) send(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// Config holds the server's listen address and optional security settings.
type Config struct {
	Addr        string
	Token       string        // if non-empty, require cookie auth
	TLSCert     string        // path to TLS certificate file
	TLSKey      string        // path to TLS private key file
	ReadTimeout time.Duration // HTTP server read timeout
}

// Server wires the Subsonic client, player, and WebSocket hub together.
type Server struct {
	client      SubsonicClient
	httpClient  *http.Client
	player      PlayerController
	addr        string
	token       string
	tlsCert     string
	tlsKey      string
	readTimeout time.Duration
	staticFS    fs.FS

	artists   cache.Cache[string, []subsonic.Artist]
	artist    cache.Cache[string, *subsonic.Artist]
	album     cache.Cache[string, *subsonic.Album]
	coverArt  cache.Cache[coverArtKey, coverArtEntry]
	playlists cache.Cache[string, []subsonic.Playlist]
	playlist  cache.Cache[string, *subsonic.Playlist]

	sf singleflight.Group // deduplicates concurrent in-flight Subsonic fetches

	httpSrv *http.Server
	clients map[*websocket.Conn]*wsClient
	mu      sync.Mutex
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// New creates a Server and registers player callbacks for state changes and
// track completion (scrobbling).
func New(client SubsonicClient, p PlayerController, cfg Config, staticFS fs.FS) *Server {
	s := &Server{
		client:      client,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
		player:      p,
		addr:        cfg.Addr,
		token:       cfg.Token,
		tlsCert:     cfg.TLSCert,
		tlsKey:      cfg.TLSKey,
		readTimeout: cfg.ReadTimeout,
		staticFS:    staticFS,
		artists:     cache.NewTTL[string, []subsonic.Artist](libraryCacheTTL),
		artist:      cache.NewTTL[string, *subsonic.Artist](libraryCacheTTL),
		album:       cache.NewTTL[string, *subsonic.Album](libraryCacheTTL),
		coverArt:    cache.NewTTL[coverArtKey, coverArtEntry](coverArtCacheTTL),
		playlists:   cache.NewTTL[string, []subsonic.Playlist](libraryCacheTTL),
		playlist:    cache.NewTTL[string, *subsonic.Playlist](libraryCacheTTL),
		clients:     make(map[*websocket.Conn]*wsClient),
	}
	p.OnChange(func(state player.State) {
		s.broadcast(state)
	})
	p.OnTrackEnd(func(t player.Track) {
		if err := client.Scrobble(context.Background(), t.ID); err != nil {
			log.Error().Err(err).Str("id", t.ID).Str("title", t.Title).Msg("server: scrobble failed")
			p.SetLastScrobble("error")
		} else {
			log.Debug().Str("id", t.ID).Str("title", t.Title).Msg("server: scrobbled")
			p.SetLastScrobble("ok")
		}
	})
	return s
}

// Handler builds and returns the HTTP handler (router) for the server.
// It can be used directly in tests without starting a listener.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(requestLogger)
	r.Use(middleware.Recoverer)
	r.Use(s.authMiddleware)

	// ── Login (public when token auth is enabled) ──────────────────────────
	r.Post("/login", s.handleLoginPost)

	// ── WebSocket ──────────────────────────────────────────────────────────
	r.Get("/ws", s.handleWS)

	// ── Player controls ────────────────────────────────────────────────────
	r.Post("/api/play", s.handlePlay)
	r.Post("/api/pause", s.handlePause)
	r.Post("/api/playpause", s.handlePlayPause)
	r.Post("/api/next", s.handleNext)
	r.Post("/api/prev", s.handlePrev)
	r.Post("/api/seek", s.handleSeek)
	r.Post("/api/volume", s.handleVolume)
	r.Post("/api/shuffle", s.handleShuffle)
	r.Post("/api/repeat", s.handleRepeat)

	// ── Queue ──────────────────────────────────────────────────────────────
	r.Delete("/api/queue", s.handleClearQueue)
	r.Delete("/api/queue/{idx}", s.handleDequeue)
	r.Post("/api/queue/song/{id}", s.handleEnqueueSong)
	r.Post("/api/queue/album/{id}", s.handleEnqueueAlbum)
	r.Post("/api/queue/jump/{idx}", s.handleJump)
	r.Post("/api/queue/move", s.handleMove)
	r.Post("/api/play/song/{id}", s.handlePlaySong)
	r.Post("/api/play/album/{id}", s.handlePlayAlbum)

	// ── Library ────────────────────────────────────────────────────────────
	r.Get("/api/artists", s.handleArtists)
	r.Get("/api/artist/{id}", s.handleArtist)
	r.Get("/api/album/{id}", s.handleAlbum)
	r.Get("/api/search", s.handleSearch)
	r.Get("/api/coverart/{id}", s.handleCoverArt)

	// ── Playlists ──────────────────────────────────────────────────────────
	r.Get("/api/playlists", s.handlePlaylists)
	r.Get("/api/playlist/{id}", s.handlePlaylist)
	r.Post("/api/play/playlist/{id}", s.handlePlayPlaylist)
	r.Post("/api/queue/playlist/{id}", s.handleEnqueuePlaylist)

	// ── Audio devices ──────────────────────────────────────────────────────
	r.Get("/api/devices", s.handleDevices)
	r.Post("/api/device", s.handleDevice)

	// ── Cache ──────────────────────────────────────────────────────────────
	r.Delete("/api/cache", s.handleClearCache)

	// ── State snapshot ─────────────────────────────────────────────────────
	r.Get("/api/state", s.handleState)

	// ── Static frontend ────────────────────────────────────────────────────
	r.Handle("/*", http.FileServer(http.FS(s.staticFS)))

	return r
}

// Start builds the router, starts listening, and blocks until the server
// closes. Call Shutdown to trigger a graceful stop.
func (s *Server) Start() error {
	h := s.Handler()

	s.mu.Lock()
	s.httpSrv = &http.Server{Addr: s.addr, Handler: h, ReadTimeout: s.readTimeout}
	s.mu.Unlock()

	log.Info().Str("addr", s.addr).Msg("server: listening")
	if s.tlsCert != "" && s.tlsKey != "" {
		if err := s.httpSrv.ListenAndServeTLS(s.tlsCert, s.tlsKey); err != http.ErrServerClosed {
			return err
		}
	} else {
		if err := s.httpSrv.ListenAndServe(); err != http.ErrServerClosed {
			return err
		}
	}
	return nil
}

// Shutdown gracefully stops the HTTP server, waiting up to ctx's deadline for
// in-flight requests to complete.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	srv := s.httpSrv
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

// ── WebSocket ──────────────────────────────────────────────────────────────────

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("server: ws upgrade failed")
		return
	}

	wc := &wsClient{conn: conn}

	s.mu.Lock()
	s.clients[conn] = wc
	s.mu.Unlock()
	log.Debug().Str("remote", r.RemoteAddr).Msg("server: ws client connected")

	// Send full state immediately so the new client is in sync. This write
	// and any concurrent broadcast both go through wc.send, which holds
	// wc.mu, so they cannot interleave on the connection.
	if data, err := json.Marshal(s.player.GetState()); err == nil {
		if err := wc.send(data); err != nil {
			log.Debug().Err(err).Msg("server: ws initial send failed")
			s.mu.Lock()
			delete(s.clients, conn)
			s.mu.Unlock()
			conn.Close() //nolint:errcheck,gosec,gosec
			return
		}
	}

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	s.mu.Lock()
	delete(s.clients, conn)
	s.mu.Unlock()
	log.Debug().Str("remote", r.RemoteAddr).Msg("server: ws client disconnected")
	conn.Close() //nolint:errcheck,gosec,gosec
}

func (s *Server) broadcast(state player.State) {
	data, _ := json.Marshal(state)
	// Snapshot clients under lock, then write without holding it so a slow
	// or dead browser connection cannot block other broadcasts.
	s.mu.Lock()
	wcs := make([]*wsClient, 0, len(s.clients))
	for _, wc := range s.clients {
		wcs = append(wcs, wc)
	}
	s.mu.Unlock()
	for _, wc := range wcs {
		if err := wc.send(data); err != nil {
			log.Debug().Err(err).Msg("server: ws write failed — removing dead client")
			s.mu.Lock()
			delete(s.clients, wc.conn)
			s.mu.Unlock()
			wc.conn.Close() //nolint:errcheck,gosec,gosec,gosec
		}
	}
}

// ── Player controls ────────────────────────────────────────────────────────────

func (s *Server) handlePlayPause(w http.ResponseWriter, _ *http.Request) {
	s.player.PlayPause()
	s.ok(w)
}
func (s *Server) handlePlay(w http.ResponseWriter, _ *http.Request)  { s.player.Play(); s.ok(w) }
func (s *Server) handlePause(w http.ResponseWriter, _ *http.Request) { s.player.Pause(); s.ok(w) }
func (s *Server) handleNext(w http.ResponseWriter, _ *http.Request)  { s.player.Next(); s.ok(w) }
func (s *Server) handlePrev(w http.ResponseWriter, _ *http.Request)  { s.player.Prev(); s.ok(w) }
func (s *Server) handleShuffle(w http.ResponseWriter, _ *http.Request) {
	s.player.ToggleShuffle()
	s.ok(w)
}
func (s *Server) handleRepeat(w http.ResponseWriter, _ *http.Request) {
	s.player.ToggleRepeat()
	s.ok(w)
}
func (s *Server) handleClearQueue(w http.ResponseWriter, _ *http.Request) {
	s.player.ClearQueue()
	s.ok(w)
}
func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) { s.json(w, s.player.GetState()) }

func (s *Server) handleSeek(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Position float64 `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	s.player.Seek(body.Position)
	s.ok(w)
}

func (s *Server) handleVolume(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Volume int `json:"volume"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	s.player.SetVolume(body.Volume)
	s.ok(w)
}

// ── Queue ──────────────────────────────────────────────────────────────────────

func (s *Server) handleDequeue(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(chi.URLParam(r, "idx"))
	if err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid index: %v", err)
		return
	}
	s.player.RemoveFromQueue(idx)
	s.ok(w)
}

func (s *Server) handleJump(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(chi.URLParam(r, "idx"))
	if err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid index: %v", err)
		return
	}
	s.player.JumpTo(idx)
	s.ok(w)
}

func (s *Server) handleMove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From int `json:"from"`
		To   int `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	s.player.MoveInQueue(body.From, body.To)
	s.ok(w)
}

func (s *Server) handleEnqueueSong(w http.ResponseWriter, r *http.Request) {
	track, err := s.songToTrack(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.player.AddToQueue(*track)
	s.ok(w)
}

func (s *Server) handlePlaySong(w http.ResponseWriter, r *http.Request) {
	track, err := s.songToTrack(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.player.SetQueue([]player.Track{*track}, 0)
	s.ok(w)
}

func (s *Server) handleEnqueueAlbum(w http.ResponseWriter, r *http.Request) {
	tracks, err := s.albumToTracks(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.player.AddAllToQueue(tracks)
	s.ok(w)
}

func (s *Server) handlePlayAlbum(w http.ResponseWriter, r *http.Request) {
	tracks, err := s.albumToTracks(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.player.SetQueue(tracks, 0)
	s.ok(w)
}

// ── Library ────────────────────────────────────────────────────────────────────

func (s *Server) handleArtists(w http.ResponseWriter, r *http.Request) {
	artists, err := s.getArtists(r.Context())
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.json(w, artists)
}

func (s *Server) handleArtist(w http.ResponseWriter, r *http.Request) {
	artist, err := s.getArtist(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.json(w, artist)
}

func (s *Server) handleAlbum(w http.ResponseWriter, r *http.Request) {
	album, err := s.getAlbum(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.json(w, album)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		s.errorf(w, http.StatusBadRequest, "missing query parameter")
		return
	}
	result, err := s.client.Search(r.Context(), q)
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.json(w, result)
}

// ── Playlists ──────────────────────────────────────────────────────────────────

func (s *Server) handlePlaylists(w http.ResponseWriter, r *http.Request) {
	playlists, err := s.getPlaylists(r.Context())
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.json(w, playlists)
}

func (s *Server) handlePlaylist(w http.ResponseWriter, r *http.Request) {
	pl, err := s.getPlaylist(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.json(w, pl)
}

func (s *Server) handlePlayPlaylist(w http.ResponseWriter, r *http.Request) {
	tracks, err := s.playlistToTracks(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.player.SetQueue(tracks, 0)
	s.ok(w)
}

func (s *Server) handleEnqueuePlaylist(w http.ResponseWriter, r *http.Request) {
	tracks, err := s.playlistToTracks(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.player.AddAllToQueue(tracks)
	s.ok(w)
}

// handleCoverArt proxies cover art from Navidrome, keeping credentials
// server-side. Responses are cached in memory for coverArtCacheTTL.
func (s *Server) handleCoverArt(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	size := 300
	if n, err := strconv.Atoi(r.URL.Query().Get("size")); err == nil {
		size = n
	}

	key := coverArtKey{id: id, size: size}
	if cached, ok := s.coverArt.Get(key); ok {
		w.Header().Set("Content-Type", cached.contentType)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(cached.data) //nolint:errcheck,gosec,gosec
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, s.client.CoverArtURL(id, size), nil) //nolint:gosec
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "cover art: %v", err)
		return
	}
	resp, err := s.httpClient.Do(req) //nolint:gosec
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "cover art: %v", err)
		return
	}
	defer resp.Body.Close() //nolint:errcheck,gosec

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "cover art: read body: %v", err)
		return
	}

	ct := resp.Header.Get("Content-Type")
	// Only cache successful responses; don't persist 404s or upstream errors.
	if resp.StatusCode == http.StatusOK {
		s.coverArt.Set(key, coverArtEntry{data: data, contentType: ct})
	}

	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(resp.StatusCode)
	w.Write(data) //nolint:errcheck,gosec,gosec
}

// ── Audio devices ──────────────────────────────────────────────────────────────

type devicesResponse struct {
	Devices []player.AudioDevice `json:"devices"`
	Current string               `json:"current"`
}

func (s *Server) handleDevices(w http.ResponseWriter, _ *http.Request) {
	devices, err := s.player.GetAudioDevices()
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.json(w, devicesResponse{
		Devices: devices,
		Current: s.player.GetAudioDevice(),
	})
}

func (s *Server) handleDevice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	if err := s.player.SetAudioDevice(body.Name); err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.ok(w)
}

// handleClearCache evicts all entries from every in-memory cache so the next
// request re-fetches fresh data from the Subsonic server.
func (s *Server) handleClearCache(w http.ResponseWriter, _ *http.Request) {
	s.artists.Clear()
	s.artist.Clear()
	s.album.Clear()
	s.coverArt.Clear()
	s.playlists.Clear()
	s.playlist.Clear()
	log.Info().Msg("server: cache cleared")
	s.ok(w)
}

// ── Cache helpers ──────────────────────────────────────────────────────────────

func (s *Server) getArtists(ctx context.Context) ([]subsonic.Artist, error) {
	if v, ok := s.artists.Get(artistsKey); ok {
		log.Debug().Int("count", len(v)).Msg("cache: artists hit")
		return v, nil
	}
	v, err, _ := s.sf.Do(artistsKey, func() (any, error) {
		log.Debug().Msg("cache: artists miss — fetching from subsonic")
		start := time.Now()
		result, err := s.client.GetArtists(ctx)
		if err != nil {
			return nil, err
		}
		log.Debug().Dur("dur", time.Since(start)).Int("count", len(result)).Msg("cache: artists fetched")
		s.artists.Set(artistsKey, result)
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]subsonic.Artist), nil
}

func (s *Server) getArtist(ctx context.Context, id string) (*subsonic.Artist, error) {
	if v, ok := s.artist.Get(id); ok {
		log.Debug().Str("id", id).Msg("cache: artist hit")
		return v, nil
	}
	v, err, _ := s.sf.Do("artist:"+id, func() (any, error) {
		log.Debug().Str("id", id).Msg("cache: artist miss — fetching from subsonic")
		start := time.Now()
		result, err := s.client.GetArtist(ctx, id)
		if err != nil {
			return nil, err
		}
		log.Debug().Dur("dur", time.Since(start)).Str("id", id).Msg("cache: artist fetched")
		s.artist.Set(id, result)
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*subsonic.Artist), nil
}

func (s *Server) getAlbum(ctx context.Context, id string) (*subsonic.Album, error) {
	if v, ok := s.album.Get(id); ok {
		log.Debug().Str("id", id).Msg("cache: album hit")
		return v, nil
	}
	v, err, _ := s.sf.Do("album:"+id, func() (any, error) {
		log.Debug().Str("id", id).Msg("cache: album miss — fetching from subsonic")
		start := time.Now()
		result, err := s.client.GetAlbum(ctx, id)
		if err != nil {
			return nil, err
		}
		log.Debug().Dur("dur", time.Since(start)).Str("id", id).Msg("cache: album fetched")
		s.album.Set(id, result)
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*subsonic.Album), nil
}

func (s *Server) getPlaylists(ctx context.Context) ([]subsonic.Playlist, error) {
	if v, ok := s.playlists.Get(playlistsKey); ok {
		log.Debug().Int("count", len(v)).Msg("cache: playlists hit")
		return v, nil
	}
	v, err, _ := s.sf.Do(playlistsKey, func() (any, error) {
		log.Debug().Msg("cache: playlists miss — fetching from subsonic")
		start := time.Now()
		result, err := s.client.GetPlaylists(ctx)
		if err != nil {
			return nil, err
		}
		log.Debug().Dur("dur", time.Since(start)).Int("count", len(result)).Msg("cache: playlists fetched")
		s.playlists.Set(playlistsKey, result)
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]subsonic.Playlist), nil
}

func (s *Server) getPlaylist(ctx context.Context, id string) (*subsonic.Playlist, error) {
	if v, ok := s.playlist.Get(id); ok {
		log.Debug().Str("id", id).Msg("cache: playlist hit")
		return v, nil
	}
	v, err, _ := s.sf.Do("playlist:"+id, func() (any, error) {
		log.Debug().Str("id", id).Msg("cache: playlist miss — fetching from subsonic")
		start := time.Now()
		result, err := s.client.GetPlaylist(ctx, id)
		if err != nil {
			return nil, err
		}
		log.Debug().Dur("dur", time.Since(start)).Str("id", id).Msg("cache: playlist fetched")
		s.playlist.Set(id, result)
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*subsonic.Playlist), nil
}

// ── Conversion helpers ─────────────────────────────────────────────────────────

// toTrack converts a Subsonic song value to a player Track, routing the cover
// art through the local proxy and embedding the authenticated stream URL.
func (s *Server) toTrack(song subsonic.Song) player.Track {
	return player.Track{
		ID:           song.ID,
		Title:        song.Title,
		Artist:       song.Artist,
		Album:        song.Album,
		Duration:     song.Duration,
		CoverArt:     "/api/coverart/" + song.CoverArt,
		StreamURL:    s.client.StreamURL(song.ID),
		Suffix:       song.Suffix,
		BitRate:      song.BitRate,
		SamplingRate: song.SamplingRate,
		ChannelCount: song.ChannelCount,
	}
}

func (s *Server) songToTrack(ctx context.Context, id string) (*player.Track, error) {
	song, err := s.client.GetSong(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("songToTrack: %w", err)
	}
	t := s.toTrack(*song)
	return &t, nil
}

func (s *Server) albumToTracks(ctx context.Context, id string) ([]player.Track, error) {
	album, err := s.getAlbum(ctx, id)
	if err != nil {
		return nil, err
	}
	tracks := make([]player.Track, len(album.Songs))
	for i, song := range album.Songs {
		tracks[i] = s.toTrack(song)
	}
	return tracks, nil
}

func (s *Server) playlistToTracks(ctx context.Context, id string) ([]player.Track, error) {
	pl, err := s.getPlaylist(ctx, id)
	if err != nil {
		return nil, err
	}
	tracks := make([]player.Track, len(pl.Songs))
	for i, song := range pl.Songs {
		tracks[i] = s.toTrack(song)
	}
	return tracks, nil
}

// ── Response helpers ───────────────────────────────────────────────────────────

func (s *Server) ok(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`)) //nolint:errcheck,gosec,gosec
}

func (s *Server) json(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck,gosec
}

func (s *Server) errorf(w http.ResponseWriter, status int, format string, args ...any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf(format, args...)}) //nolint:errcheck,gosec
}

// ── Auth ───────────────────────────────────────────────────────────────────────

// authMiddleware enforces token authentication when s.token is non-empty.
// Authentication is carried by an HttpOnly session cookie set on successful login.
// The /login page is always accessible. API and WebSocket paths return 401 JSON
// when unauthenticated; everything else redirects to /login.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" || r.URL.Path == "/login" {
			next.ServeHTTP(w, r)
			return
		}
		if cookie, err := r.Cookie("subsd_token"); err == nil && cookie.Value == s.token {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/ws" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`)) //nolint:errcheck,gosec
			return
		}
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
	})
}

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if subtle.ConstantTimeCompare([]byte(r.FormValue("token")), []byte(s.token)) == 1 {
		http.SetCookie(w, &http.Cookie{
			Name:     "subsd_token",
			Value:    s.token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error":"wrong token"}`)) //nolint:errcheck,gosec
}

// requestLogger is a chi middleware that logs each HTTP request via zerolog.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		defer func() {
			log.Debug().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", ww.Status()).
				Dur("dur", time.Since(start)).
				Msg("server: request")
		}()
		next.ServeHTTP(ww, r)
	})
}
