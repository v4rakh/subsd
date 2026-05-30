package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/urfave/cli/v3"
)

// cliConfig is loaded from ~/.config/subsd/cli.toml.
type cliConfig struct {
	URL   string `toml:"url"`
	Token string `toml:"token"`
}

func loadCLIConfig() cliConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return cliConfig{}
	}
	path := filepath.Join(home, ".config", "subsd", "cli.toml")
	var cfg cliConfig
	// Config file is optional; ignore any read/parse errors.
	_, _ = toml.DecodeFile(path, &cfg)
	return cfg
}

// remoteCLIClient handles HTTP requests to a running subsd instance.
type remoteCLIClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func newRemoteCLIClient(baseURL, token string) *remoteCLIClient {
	return &remoteCLIClient{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{},
	}
}

// clientFromCmd builds a remoteCLIClient by merging the TOML config with
// any --url / --token flags. urfave/cli/v3 resolves flags up the command
// lineage, so cmd.String("url") finds the flag on the "remote" parent even
// when called from a deeply nested subcommand.
func clientFromCmd(cmd *cli.Command) (*remoteCLIClient, error) {
	fileCfg := loadCLIConfig()

	serverURL := fileCfg.URL
	token := fileCfg.Token

	// Flags defined on the "remote" parent are visible from any subcommand.
	if v := cmd.String(flagURL); v != "" {
		serverURL = v
	}
	if v := cmd.String(flagToken); v != "" {
		token = v
	}

	if serverURL == "" {
		return nil, errors.New("subsd URL required: use --url, SUBSD_REMOTE_URL, or set url in ~/.config/subsd/cli.toml")
	}

	return newRemoteCLIClient(serverURL, token), nil
}

func (c *remoteCLIClient) request(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal: %w", err)
		}
		r = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r) //nolint:gosec
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.AddCookie(&http.Cookie{Name: "subsd_token", Value: c.token})
	}

	resp, err := c.http.Do(req) //nolint:gosec
	if err != nil {
		return nil, 0, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	return data, resp.StatusCode, nil
}

func (c *remoteCLIClient) post(ctx context.Context, path string, body any) error {
	data, status, err := c.request(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("server error %d: %s", status, string(data))
	}
	return nil
}

func (c *remoteCLIClient) put(ctx context.Context, path string, body any) error {
	data, status, err := c.request(ctx, http.MethodPut, path, body)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("server error %d: %s", status, string(data))
	}
	return nil
}

func (c *remoteCLIClient) delete(ctx context.Context, path string) error {
	data, status, err := c.request(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("server error %d: %s", status, string(data))
	}
	return nil
}

func (c *remoteCLIClient) get(ctx context.Context, path string) ([]byte, error) {
	data, status, err := c.request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("server error %d: %s", status, string(data))
	}
	return data, nil
}

func printPrettyJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		_, err = fmt.Println(string(data))
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func requireArg(cmd *cli.Command, name string) (string, error) {
	if cmd.Args().Len() == 0 {
		return "", fmt.Errorf("%s is required", name)
	}
	return cmd.Args().First(), nil
}

func requireIntArg(cmd *cli.Command, name string) (int, error) {
	s, err := requireArg(cmd, name)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", name, s, err)
	}
	return n, nil
}

var remoteCommand = &cli.Command{
	Name:  "remote",
	Usage: "Control a running subsd instance via its HTTP API",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    flagURL,
			Usage:   "Base URL of the subsd server (e.g. http://localhost:8080)",
			Sources: cli.EnvVars(envRemoteURL),
		},
		&cli.StringFlag{
			Name:    flagToken,
			Usage:   "Authentication token (required when the server has token auth enabled)",
			Sources: cli.EnvVars(envRemoteToken),
		},
	},
	Commands: []*cli.Command{
		// ── Playback controls ─────────────────────────────────────────────
		{
			Name:  "play",
			Usage: "Resume playback",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/play", nil)
			},
		},
		{
			Name:  "pause",
			Usage: "Pause playback",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/pause", nil)
			},
		},
		{
			Name:  "play-pause",
			Usage: "Toggle play/pause",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/playpause", nil)
			},
		},
		{
			Name:  "next",
			Usage: "Skip to next track",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/next", nil)
			},
		},
		{
			Name:  "prev",
			Usage: "Go to previous track",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/prev", nil)
			},
		},
		{
			Name:      "seek",
			Usage:     "Seek to position in seconds",
			ArgsUsage: "<seconds>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				s, err := requireArg(cmd, "seconds")
				if err != nil {
					return err
				}
				pos, err := strconv.ParseFloat(s, 64)
				if err != nil {
					return fmt.Errorf("invalid position %q: %w", s, err)
				}
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/seek", map[string]float64{"position": pos})
			},
		},
		{
			Name:      "volume",
			Usage:     "Set volume (0-100)",
			ArgsUsage: "<level>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				s, err := requireArg(cmd, "level")
				if err != nil {
					return err
				}
				vol, err := strconv.Atoi(s)
				if err != nil {
					return fmt.Errorf("invalid volume %q: %w", s, err)
				}
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/volume", map[string]int{"volume": vol})
			},
		},
		{
			Name:  "shuffle",
			Usage: "Toggle shuffle mode",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/shuffle", nil)
			},
		},
		{
			Name:  "repeat",
			Usage: "Toggle repeat mode",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/repeat", nil)
			},
		},
		{
			Name:      "replaygain",
			Usage:     "Set ReplayGain mode (no|track|album)",
			ArgsUsage: "<mode>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				mode, err := requireArg(cmd, "mode")
				if err != nil {
					return err
				}
				if mode != "no" && mode != "track" && mode != "album" {
					return fmt.Errorf("invalid mode %q: must be no, track, or album", mode)
				}
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/replaygain", map[string]string{"mode": mode})
			},
		},
		{
			Name:  "state",
			Usage: "Print current player state as JSON",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				data, err := c.get(ctx, "/api/v1/state")
				if err != nil {
					return err
				}
				return printPrettyJSON(data)
			},
		},
		// ── Play something ────────────────────────────────────────────────
		{
			Name:      "play-song",
			Usage:     "Play a song immediately by Subsonic ID",
			ArgsUsage: "<id>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				id, err := requireArg(cmd, "song ID")
				if err != nil {
					return err
				}
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/play/song/"+id, nil)
			},
		},
		{
			Name:      "play-album",
			Usage:     "Play an album immediately by Subsonic ID",
			ArgsUsage: "<id>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				id, err := requireArg(cmd, "album ID")
				if err != nil {
					return err
				}
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/play/album/"+id, nil)
			},
		},
		{
			Name:      "play-playlist",
			Usage:     "Play a playlist immediately by Subsonic ID",
			ArgsUsage: "<id>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				id, err := requireArg(cmd, "playlist ID")
				if err != nil {
					return err
				}
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/play/playlist/"+id, nil)
			},
		},
		{
			Name:      "play-artist",
			Usage:     "Play all songs by an artist immediately by Subsonic ID",
			ArgsUsage: "<id>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				id, err := requireArg(cmd, "artist ID")
				if err != nil {
					return err
				}
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/play/artist/"+id, nil)
			},
		},
		// ── Ratings ───────────────────────────────────────────────────────
		{
			Name:      "rate-song",
			Usage:     "Rate a song by Subsonic ID (0 removes the rating)",
			ArgsUsage: "<id> <0-5>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				if cmd.Args().Len() != 2 {
					return errors.New("usage: rate-song <id> <0-5>")
				}
				id := cmd.Args().Get(0)
				rating, err := strconv.Atoi(cmd.Args().Get(1))
				if err != nil || rating < 0 || rating > 5 {
					return errors.New("rating must be an integer between 0 and 5")
				}
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/rating", map[string]any{"id": id, "rating": rating})
			},
		},
		{
			Name:      "rate-album",
			Usage:     "Rate an album by Subsonic ID (0 removes the rating)",
			ArgsUsage: "<id> <0-5>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				if cmd.Args().Len() != 2 {
					return errors.New("usage: rate-album <id> <0-5>")
				}
				id := cmd.Args().Get(0)
				rating, err := strconv.Atoi(cmd.Args().Get(1))
				if err != nil || rating < 0 || rating > 5 {
					return errors.New("rating must be an integer between 0 and 5")
				}
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/rating", map[string]any{"id": id, "rating": rating})
			},
		},
		// ── Queue management ──────────────────────────────────────────────
		{
			Name:  "queue",
			Usage: "Manage the playback queue",
			Commands: []*cli.Command{
				{
					Name:  "clear",
					Usage: "Clear the entire queue",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.delete(ctx, "/api/v1/queue")
					},
				},
				{
					Name:      "remove",
					Usage:     "Remove a track from the queue by index",
					ArgsUsage: "<index>",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						idx, err := requireIntArg(cmd, "index")
						if err != nil {
							return err
						}
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.delete(ctx, "/api/v1/queue/"+strconv.Itoa(idx))
					},
				},
				{
					Name:      "jump",
					Usage:     "Jump to a track in the queue by index",
					ArgsUsage: "<index>",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						idx, err := requireIntArg(cmd, "index")
						if err != nil {
							return err
						}
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.post(ctx, "/api/v1/queue/jump/"+strconv.Itoa(idx), nil)
					},
				},
				{
					Name:      "move",
					Usage:     "Move a track within the queue",
					ArgsUsage: "<from> <to>",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						if cmd.Args().Len() != 2 {
							return errors.New("usage: queue move <from> <to>")
						}
						from, err := strconv.Atoi(cmd.Args().Get(0))
						if err != nil {
							return fmt.Errorf("invalid <from> index: %w", err)
						}
						to, err := strconv.Atoi(cmd.Args().Get(1))
						if err != nil {
							return fmt.Errorf("invalid <to> index: %w", err)
						}
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.post(ctx, "/api/v1/queue/move", map[string]int{"from": from, "to": to})
					},
				},
				{
					Name:      "add-song",
					Usage:     "Add a song to the end of the queue by Subsonic ID",
					ArgsUsage: "<id>",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						id, err := requireArg(cmd, "song ID")
						if err != nil {
							return err
						}
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.post(ctx, "/api/v1/queue/song/"+id, nil)
					},
				},
				{
					Name:      "add-album",
					Usage:     "Append all songs of an album to the queue by Subsonic ID",
					ArgsUsage: "<id>",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						id, err := requireArg(cmd, "album ID")
						if err != nil {
							return err
						}
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.post(ctx, "/api/v1/queue/album/"+id, nil)
					},
				},
				{
					Name:      "add-playlist",
					Usage:     "Append all songs of a playlist to the queue by Subsonic ID",
					ArgsUsage: "<id>",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						id, err := requireArg(cmd, "playlist ID")
						if err != nil {
							return err
						}
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.post(ctx, "/api/v1/queue/playlist/"+id, nil)
					},
				},
				{
					Name:      "add-artist",
					Usage:     "Append all songs by an artist to the queue by Subsonic ID",
					ArgsUsage: "<id>",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						id, err := requireArg(cmd, "artist ID")
						if err != nil {
							return err
						}
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.post(ctx, "/api/v1/queue/artist/"+id, nil)
					},
				},
				{
					Name:      "save-as-playlist",
					Usage:     "Save the current queue as a new playlist",
					ArgsUsage: "<name>",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						name, err := requireArg(cmd, "playlist name")
						if err != nil {
							return err
						}
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.post(ctx, "/api/v1/playlist/from-queue", map[string]string{"name": name})
					},
				},
				{
					Name:      "append-to-playlist",
					Usage:     "Append the current queue to an existing playlist by Subsonic ID",
					ArgsUsage: "<playlist-id>",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						id, err := requireArg(cmd, "playlist ID")
						if err != nil {
							return err
						}
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.post(ctx, "/api/v1/playlist/"+id+"/add-queue", nil)
					},
				},
			},
		},
		// ── Playlist management ───────────────────────────────────────────
		{
			Name:  "playlist",
			Usage: "Manage playlists",
			Commands: []*cli.Command{
				{
					Name:      "create",
					Usage:     "Create a new empty playlist",
					ArgsUsage: "<name>",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						name, err := requireArg(cmd, "playlist name")
						if err != nil {
							return err
						}
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						data, status, err := c.request(ctx, http.MethodPost, "/api/v1/playlist", map[string]any{"name": name, "songIds": []string{}})
						if err != nil {
							return err
						}
						if status >= 300 {
							return fmt.Errorf("server error %d: %s", status, string(data))
						}
						return printPrettyJSON(data)
					},
				},
				{
					Name:      "delete",
					Usage:     "Delete a playlist by Subsonic ID",
					ArgsUsage: "<id>",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						id, err := requireArg(cmd, "playlist ID")
						if err != nil {
							return err
						}
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.delete(ctx, "/api/v1/playlist/"+id)
					},
				},
				{
					Name:      "rename",
					Usage:     "Rename a playlist",
					ArgsUsage: "<id> <new-name>",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						if cmd.Args().Len() != 2 {
							return errors.New("usage: playlist rename <id> <new-name>")
						}
						id := cmd.Args().Get(0)
						name := cmd.Args().Get(1)
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.put(ctx, "/api/v1/playlist/"+id, map[string]string{"name": name})
					},
				},
				{
					Name:      "add-song",
					Usage:     "Add a song to a playlist by Subsonic IDs",
					ArgsUsage: "<playlist-id> <song-id>",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						if cmd.Args().Len() != 2 {
							return errors.New("usage: playlist add-song <playlist-id> <song-id>")
						}
						playlistID := cmd.Args().Get(0)
						songID := cmd.Args().Get(1)
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.post(ctx, "/api/v1/playlist/"+playlistID+"/songs", map[string][]string{"songIds": {songID}})
					},
				},
				{
					Name:      "add-album",
					Usage:     "Add all songs of an album to a playlist by Subsonic IDs",
					ArgsUsage: "<playlist-id> <album-id>",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						if cmd.Args().Len() != 2 {
							return errors.New("usage: playlist add-album <playlist-id> <album-id>")
						}
						playlistID := cmd.Args().Get(0)
						albumID := cmd.Args().Get(1)
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.post(ctx, "/api/v1/playlist/"+playlistID+"/album/"+albumID, nil)
					},
				},
				{
					Name:      "remove-song",
					Usage:     "Remove a song from a playlist by its 0-based track index",
					ArgsUsage: "<playlist-id> <index>",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						if cmd.Args().Len() != 2 {
							return errors.New("usage: playlist remove-song <playlist-id> <index>")
						}
						playlistID := cmd.Args().Get(0)
						idx, err := strconv.Atoi(cmd.Args().Get(1))
						if err != nil {
							return fmt.Errorf("invalid index: %w", err)
						}
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.delete(ctx, fmt.Sprintf("/api/v1/playlist/%s/songs/%d", playlistID, idx))
					},
				},
				{
					Name:      "reorder",
					Usage:     "Move a track within a playlist (fetches current order, applies move, saves)",
					ArgsUsage: "<playlist-id> <from-index> <to-index>",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						if cmd.Args().Len() != 3 {
							return errors.New("usage: playlist reorder <playlist-id> <from-index> <to-index>")
						}
						playlistID := cmd.Args().Get(0)
						from, err := strconv.Atoi(cmd.Args().Get(1))
						if err != nil {
							return fmt.Errorf("invalid from-index: %w", err)
						}
						to, err := strconv.Atoi(cmd.Args().Get(2))
						if err != nil {
							return fmt.Errorf("invalid to-index: %w", err)
						}
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						// Fetch current playlist to get song IDs.
						data, err := c.get(ctx, "/api/v1/playlist/"+playlistID)
						if err != nil {
							return err
						}
						var pl struct {
							Songs []struct {
								ID string `json:"id"`
							} `json:"entry"`
						}
						if err := json.Unmarshal(data, &pl); err != nil {
							return fmt.Errorf("parse playlist: %w", err)
						}
						ids := make([]string, len(pl.Songs))
						for i, s := range pl.Songs {
							ids[i] = s.ID
						}
						if from < 0 || from >= len(ids) || to < 0 || to >= len(ids) {
							return fmt.Errorf("index out of range (playlist has %d tracks)", len(ids))
						}
						// Apply the move.
						moved := ids[from]
						newIDs := append(ids[:from], ids[from+1:]...)
						newIDs = append(newIDs[:to], append([]string{moved}, newIDs[to:]...)...)
						return c.put(ctx, "/api/v1/playlist/"+playlistID+"/songs", map[string][]string{"songIds": newIDs})
					},
				},
			},
		},
		// ── Library ───────────────────────────────────────────────────────
		{
			Name:  "songs",
			Usage: "List all songs (requires warm cache)",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				data, err := c.get(ctx, "/api/v1/songs")
				if err != nil {
					return err
				}
				return printPrettyJSON(data)
			},
		},
		{
			Name:  "artists",
			Usage: "List all artists",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				data, err := c.get(ctx, "/api/v1/artists")
				if err != nil {
					return err
				}
				return printPrettyJSON(data)
			},
		},
		{
			Name:      "artist",
			Usage:     "Get artist details by Subsonic ID",
			ArgsUsage: "<id>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				id, err := requireArg(cmd, "artist ID")
				if err != nil {
					return err
				}
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				data, err := c.get(ctx, "/api/v1/artist/"+id)
				if err != nil {
					return err
				}
				return printPrettyJSON(data)
			},
		},
		{
			Name:      "album",
			Usage:     "Get album details by Subsonic ID",
			ArgsUsage: "<id>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				id, err := requireArg(cmd, "album ID")
				if err != nil {
					return err
				}
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				data, err := c.get(ctx, "/api/v1/album/"+id)
				if err != nil {
					return err
				}
				return printPrettyJSON(data)
			},
		},
		{
			Name:      "search",
			Usage:     "Search the library (artists, albums, songs)",
			ArgsUsage: "<query>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				if cmd.Args().Len() == 0 {
					return errors.New("search query is required")
				}
				q := strings.Join(cmd.Args().Slice(), " ")
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				data, err := c.get(ctx, "/api/v1/search?q="+url.QueryEscape(q))
				if err != nil {
					return err
				}
				return printPrettyJSON(data)
			},
		},
		{
			Name:  "playlists",
			Usage: "List all playlists",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				data, err := c.get(ctx, "/api/v1/playlists")
				if err != nil {
					return err
				}
				return printPrettyJSON(data)
			},
		},
		{
			Name:      "playlist",
			Usage:     "Get playlist details by Subsonic ID",
			ArgsUsage: "<id>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				id, err := requireArg(cmd, "playlist ID")
				if err != nil {
					return err
				}
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				data, err := c.get(ctx, "/api/v1/playlist/"+id)
				if err != nil {
					return err
				}
				return printPrettyJSON(data)
			},
		},
		// ── Satellites ────────────────────────────────────────────────────
		{
			Name:  "satellites",
			Usage: "List connected satellites",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				data, err := c.get(ctx, "/api/v1/satellites")
				if err != nil {
					return err
				}
				return printPrettyJSON(data)
			},
		},
		{
			Name:      "satellite-use",
			Usage:     "Switch playback to a satellite by name",
			ArgsUsage: "<name>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				name, err := requireArg(cmd, "satellite name")
				if err != nil {
					return err
				}
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/satellites/active", map[string]string{"name": name})
			},
		},
		{
			Name:      "satellite-device",
			Usage:     "Set the audio output device for a satellite",
			ArgsUsage: "<satellite-name> <device-id>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				if cmd.Args().Len() != 2 {
					return errors.New("usage: satellite-device <satellite-name> <device-id>")
				}
				satName := cmd.Args().Get(0)
				device := cmd.Args().Get(1)
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/satellites/"+satName+"/device", map[string]string{"device": device})
			},
		},
		{
			Name:      "satellite-devices",
			Usage:     "List audio devices for a satellite",
			ArgsUsage: "<satellite-name>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				name, err := requireArg(cmd, "satellite name")
				if err != nil {
					return err
				}
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				data, err := c.get(ctx, "/api/v1/satellites")
				if err != nil {
					return err
				}
				var all []map[string]any
				if jsonErr := json.Unmarshal(data, &all); jsonErr != nil {
					return printPrettyJSON(data)
				}
				for _, s := range all {
					if s["name"] == name {
						out, _ := json.MarshalIndent(s, "", "  ")
						_, err = fmt.Println(string(out))
						return err
					}
				}
				return fmt.Errorf("satellite %q not found", name)
			},
		},
		// ── Audio devices ─────────────────────────────────────────────────
		{
			Name:  "devices",
			Usage: "List available audio output devices",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				data, err := c.get(ctx, "/api/v1/devices")
				if err != nil {
					return err
				}
				return printPrettyJSON(data)
			},
		},
		{
			Name:      "device",
			Usage:     "Set the active audio output device",
			ArgsUsage: "<name>",
			Action: func(ctx context.Context, cmd *cli.Command) error {
				name, err := requireArg(cmd, "device name")
				if err != nil {
					return err
				}
				c, err := clientFromCmd(cmd)
				if err != nil {
					return err
				}
				return c.post(ctx, "/api/v1/device", map[string]string{"name": name})
			},
		},
		// ── Cache ─────────────────────────────────────────────────────────
		{
			Name:  "cache",
			Usage: "Manage the server-side library cache",
			Commands: []*cli.Command{
				{
					Name:  "clear",
					Usage: "Clear all caches (including cover art) and trigger a library refresh",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.delete(ctx, "/api/v1/cache")
					},
				},
				{
					Name:  "refresh",
					Usage: "Refresh the library cache without clearing cover art",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						c, err := clientFromCmd(cmd)
						if err != nil {
							return err
						}
						return c.post(ctx, "/api/v1/cache", nil)
					},
				},
			},
		},
	},
}
