// Package mpris exposes subsd playback state and controls via the MPRIS
// D-Bus interface (org.mpris.MediaPlayer2 / org.mpris.MediaPlayer2.Player).
// It requires a running D-Bus session bus (DBUS_SESSION_BUS_ADDRESS).
package mpris

import (
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
	"github.com/rs/zerolog/log"
	"varakh.de/subsd/internal/player"
)

const (
	dbusServiceName  = "org.mpris.MediaPlayer2.subsd"
	dbusObjectPath   = "/org/mpris/MediaPlayer2"
	loopStatusNone   = "None"
	loopStatusRepeat = "Playlist"
	ifaceRoot        = "org.mpris.MediaPlayer2"
	ifacePlayer      = "org.mpris.MediaPlayer2.Player"
	trackIDPrefix    = "/de/varakh/subsd/Track/"
)

// Service registers and maintains the MPRIS D-Bus service.
type Service struct {
	conn  *dbus.Conn
	props *prop.Properties
	pl    *player.Player
	url   string
}

// New connects to the D-Bus session bus, registers org.mpris.MediaPlayer2.subsd,
// and subscribes to player state changes. daemonURL is the HTTP base URL used to
// build mpris:artUrl values; it may be empty, in which case cover art is omitted.
func New(p *player.Player, daemonURL string) (*Service, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("mpris: cannot connect to session bus: %w", err)
	}

	reply, err := conn.RequestName(dbusServiceName, dbus.NameFlagDoNotQueue)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("mpris: cannot request D-Bus name: %w", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		_ = conn.Close()
		return nil, fmt.Errorf("mpris: D-Bus name %s is already in use", dbusServiceName)
	}

	s := &Service{conn: conn, pl: p, url: strings.TrimRight(daemonURL, "/")}

	state := p.GetState()
	s.props, err = prop.Export(conn, dbus.ObjectPath(dbusObjectPath), s.buildPropsMap(state))
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("mpris: cannot export properties: %w", err)
	}

	if err := conn.Export(s, dbus.ObjectPath(dbusObjectPath), ifaceRoot); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("mpris: cannot export root interface: %w", err)
	}

	// Use ExportMethodTable for the player interface so the "Seek" D-Bus method
	// can map to a method with a non-conflicting Go name (govet stdmethods check
	// flags any method named Seek that does not match io.Seeker's signature).
	playerMethods := map[string]any{
		"Next":        s.next,
		"Previous":    s.previous,
		"Pause":       s.pause,
		"Play":        s.play,
		"PlayPause":   s.playPause,
		"Stop":        s.stop,
		"Seek":        s.seek,
		"SetPosition": s.setPosition,
		"OpenUri":     s.openURI,
	}
	if err := conn.ExportMethodTable(playerMethods, dbus.ObjectPath(dbusObjectPath), ifacePlayer); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("mpris: cannot export player interface: %w", err)
	}

	p.OnChange(s.onStateChange)

	log.Info().Str("name", dbusServiceName).Msg("mpris: registered on session bus")
	return s, nil
}

// Close releases the D-Bus name and closes the connection.
func (s *Service) Close() {
	_, _ = s.conn.ReleaseName(dbusServiceName)
	_ = s.conn.Close()
}

// ── org.mpris.MediaPlayer2 methods ───────────────────────────────────────────

func (s *Service) Raise() *dbus.Error { return nil }
func (s *Service) Quit() *dbus.Error  { return nil }

// ── org.mpris.MediaPlayer2.Player methods ────────────────────────────────────

func (s *Service) next() *dbus.Error {
	log.Debug().Msg("mpris: next")
	s.pl.Next()
	return nil
}

func (s *Service) previous() *dbus.Error {
	log.Debug().Msg("mpris: previous")
	s.pl.Prev()
	return nil
}

func (s *Service) pause() *dbus.Error {
	log.Debug().Msg("mpris: pause")
	s.pl.Pause()
	return nil
}

func (s *Service) play() *dbus.Error {
	log.Debug().Msg("mpris: play")
	s.pl.Play()
	return nil
}

func (s *Service) playPause() *dbus.Error {
	log.Debug().Msg("mpris: playpause")
	s.pl.PlayPause()
	return nil
}

func (s *Service) stop() *dbus.Error {
	log.Debug().Msg("mpris: stop")
	s.pl.ClearQueue()
	return nil
}

// seek moves the playback position by offset microseconds relative to the current position.
func (s *Service) seek(offset int64) *dbus.Error {
	st := s.pl.GetState()
	newPos := st.Position + float64(offset)/1e6
	if newPos < 0 {
		newPos = 0
	}
	log.Debug().Float64("position", newPos).Msg("mpris: seek")
	s.pl.Seek(newPos)
	return nil
}

// setPosition seeks to an absolute position (microseconds) for the given track.
func (s *Service) setPosition(_ dbus.ObjectPath, position int64) *dbus.Error {
	secs := float64(position) / 1e6
	log.Debug().Float64("position", secs).Msg("mpris: set position")
	s.pl.Seek(secs)
	return nil
}

// openURI is a no-op; subsd manages its queue through its own API.
func (s *Service) openURI(_ string) *dbus.Error { return nil }

// ── Internal ──────────────────────────────────────────────────────────────────

func (s *Service) onStateChange(state player.State) {
	status := playbackStatus(state)
	meta := s.buildMetadata(state)
	vol := float64(state.Volume) / 100.0
	pos := int64(state.Position * 1e6)
	hasTrack := state.CurrentIdx >= 0 && len(state.Queue) > 0
	canNext := hasTrack && (state.CurrentIdx < len(state.Queue)-1 || state.Repeat)
	canPrev := hasTrack && state.CurrentIdx > 0
	loopStatus := loopStatusNone
	if state.Repeat {
		loopStatus = loopStatusRepeat
	}

	s.props.SetMust(ifacePlayer, "PlaybackStatus", status)
	s.props.SetMust(ifacePlayer, "Metadata", meta)
	s.props.SetMust(ifacePlayer, "Volume", vol)
	s.props.SetMust(ifacePlayer, "Position", pos)
	s.props.SetMust(ifacePlayer, "CanGoNext", canNext)
	s.props.SetMust(ifacePlayer, "CanGoPrevious", canPrev)
	s.props.SetMust(ifacePlayer, "CanPlay", hasTrack)
	s.props.SetMust(ifacePlayer, "CanPause", hasTrack)
	s.props.SetMust(ifacePlayer, "CanSeek", hasTrack)
	s.props.SetMust(ifacePlayer, "Shuffle", state.Shuffle)
	s.props.SetMust(ifacePlayer, "LoopStatus", loopStatus)
}

func (s *Service) buildPropsMap(state player.State) prop.Map {
	status := playbackStatus(state)
	meta := s.buildMetadata(state)
	vol := float64(state.Volume) / 100.0
	pos := int64(state.Position * 1e6)
	hasTrack := state.CurrentIdx >= 0 && len(state.Queue) > 0
	loopStatus := loopStatusNone
	if state.Repeat {
		loopStatus = loopStatusRepeat
	}

	return prop.Map{
		ifaceRoot: {
			"CanQuit":             &prop.Prop{Value: false, Writable: false, Emit: prop.EmitFalse},
			"CanRaise":            &prop.Prop{Value: false, Writable: false, Emit: prop.EmitFalse},
			"HasTrackList":        &prop.Prop{Value: false, Writable: false, Emit: prop.EmitFalse},
			"Identity":            &prop.Prop{Value: "subsd", Writable: false, Emit: prop.EmitFalse},
			"DesktopEntry":        &prop.Prop{Value: "subsd", Writable: false, Emit: prop.EmitFalse},
			"SupportedUriSchemes": &prop.Prop{Value: []string{}, Writable: false, Emit: prop.EmitFalse},
			"SupportedMimeTypes":  &prop.Prop{Value: []string{}, Writable: false, Emit: prop.EmitFalse},
		},
		ifacePlayer: {
			"PlaybackStatus": &prop.Prop{Value: status, Writable: false, Emit: prop.EmitTrue},
			"LoopStatus": &prop.Prop{
				Value: loopStatus, Writable: true, Emit: prop.EmitTrue,
				Callback: func(c *prop.Change) *dbus.Error {
					v := c.Value.(string)
					if v != loopStatusNone && v != loopStatusRepeat {
						// "Track" (repeat single) is not supported; reject per MPRIS spec.
						return dbus.MakeFailedError(fmt.Errorf("LoopStatus %q is not supported", v))
					}
					want := v != loopStatusNone
					if want != s.pl.GetState().Repeat {
						s.pl.ToggleRepeat()
					}
					return nil
				},
			},
			"Rate": &prop.Prop{Value: 1.0, Writable: false, Emit: prop.EmitFalse},
			"Shuffle": &prop.Prop{
				Value: state.Shuffle, Writable: true, Emit: prop.EmitTrue,
				Callback: func(c *prop.Change) *dbus.Error {
					want := c.Value.(bool)
					if want != s.pl.GetState().Shuffle {
						s.pl.ToggleShuffle()
					}
					return nil
				},
			},
			"Metadata":      &prop.Prop{Value: meta, Writable: false, Emit: prop.EmitTrue},
			"Volume":        &prop.Prop{Value: vol, Writable: false, Emit: prop.EmitTrue},
			"Position":      &prop.Prop{Value: pos, Writable: false, Emit: prop.EmitFalse},
			"MinimumRate":   &prop.Prop{Value: 1.0, Writable: false, Emit: prop.EmitFalse},
			"MaximumRate":   &prop.Prop{Value: 1.0, Writable: false, Emit: prop.EmitFalse},
			"CanGoNext":     &prop.Prop{Value: hasTrack, Writable: false, Emit: prop.EmitTrue},
			"CanGoPrevious": &prop.Prop{Value: hasTrack, Writable: false, Emit: prop.EmitTrue},
			"CanPlay":       &prop.Prop{Value: hasTrack, Writable: false, Emit: prop.EmitTrue},
			"CanPause":      &prop.Prop{Value: hasTrack, Writable: false, Emit: prop.EmitTrue},
			"CanSeek":       &prop.Prop{Value: hasTrack, Writable: false, Emit: prop.EmitTrue},
			"CanControl":    &prop.Prop{Value: true, Writable: false, Emit: prop.EmitFalse},
		},
	}
}

func (s *Service) buildMetadata(state player.State) map[string]dbus.Variant {
	if state.CurrentIdx < 0 || state.CurrentIdx >= len(state.Queue) {
		return map[string]dbus.Variant{
			"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath("/de/varakh/subsd/Track/none")),
		}
	}
	t := state.Queue[state.CurrentIdx]
	m := map[string]dbus.Variant{
		"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath(trackIDPrefix + sanitizeID(t.ID))),
	}
	if t.Title != "" {
		m["xesam:title"] = dbus.MakeVariant(t.Title)
	}
	if t.Artist != "" {
		m["xesam:artist"] = dbus.MakeVariant([]string{t.Artist})
	}
	if t.Album != "" {
		m["xesam:album"] = dbus.MakeVariant(t.Album)
	}
	if t.Duration > 0 {
		m["mpris:length"] = dbus.MakeVariant(int64(t.Duration) * 1_000_000)
	}
	if s.url != "" && t.CoverArt != "" {
		m["mpris:artUrl"] = dbus.MakeVariant(s.url + t.CoverArt)
	}
	return m
}

func playbackStatus(state player.State) string {
	if state.Playing {
		return "Playing"
	}
	if state.CurrentIdx >= 0 && len(state.Queue) > 0 {
		return "Paused"
	}
	return "Stopped"
}

// sanitizeID returns a D-Bus-safe path segment from an arbitrary track ID.
func sanitizeID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}
