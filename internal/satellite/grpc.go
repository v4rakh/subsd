package satellite

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"varakh.de/subsd/internal/satellitepb"
)

// grpcTokenMetaKey is the gRPC metadata key used to carry the shared secret.
const grpcTokenMetaKey = "x-subsd-token" //nolint:gosec

// ── gRPC server (runs inside the daemon) ─────────────────────────────────────

// GRPCServer listens for satellite connections and registers them in the registry.
type GRPCServer struct {
	satellitepb.UnimplementedSatelliteServiceServer
	registry               *Registry
	srv                    *grpc.Server
	heartbeatTimeout       time.Duration
	heartbeatCheckInterval time.Duration
	tlsCert                string
	tlsKey                 string
	token                  string
}

// NewGRPCServer creates a GRPCServer backed by the given registry.
// heartbeatTimeout is how long a satellite may be silent before it is disconnected;
// heartbeatCheckInterval is how often that silence is checked.
// tlsCert and tlsKey enable TLS when both are non-empty.
// token, when non-empty, requires every connecting satellite to present it as x-subsd-token metadata.
func NewGRPCServer(reg *Registry, heartbeatTimeout, heartbeatCheckInterval time.Duration, tlsCert, tlsKey, token string) *GRPCServer {
	return &GRPCServer{
		registry:               reg,
		heartbeatTimeout:       heartbeatTimeout,
		heartbeatCheckInterval: heartbeatCheckInterval,
		tlsCert:                tlsCert,
		tlsKey:                 tlsKey,
		token:                  token,
	}
}

// Start begins listening on addr (e.g. ":9090") and blocks until stopped.
func (g *GRPCServer) Start(addr string) error {
	lc := net.ListenConfig{}
	lis, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return err
	}

	if g.token != "" && g.tlsCert == "" {
		log.Warn().Msg("satellite gRPC: token auth is set but TLS is not — token will be sent in plaintext")
	}

	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    30 * time.Second,
			Timeout: 10 * time.Second,
		}),
	}
	if g.tlsCert != "" && g.tlsKey != "" {
		creds, err := credentials.NewServerTLSFromFile(g.tlsCert, g.tlsKey)
		if err != nil {
			return fmt.Errorf("satellite gRPC: loading TLS credentials: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
		log.Info().Str("cert", g.tlsCert).Msg("satellite gRPC: TLS enabled")
	}
	if g.token != "" {
		opts = append(opts, grpc.StreamInterceptor(tokenStreamInterceptor(g.token)))
	}

	g.srv = grpc.NewServer(opts...)
	satellitepb.RegisterSatelliteServiceServer(g.srv, g)
	log.Info().Str("addr", addr).Msg("satellite gRPC: listening")
	return g.srv.Serve(lis)
}

// Stop gracefully shuts down the gRPC server.
func (g *GRPCServer) Stop() {
	if g.srv != nil {
		g.srv.GracefulStop()
	}
}

// tokenStreamInterceptor returns a stream interceptor that rejects any stream
// whose incoming metadata does not carry the expected x-subsd-token value.
func tokenStreamInterceptor(token string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		md, ok := metadata.FromIncomingContext(ss.Context())
		if !ok {
			return status.Error(codes.Unauthenticated, "missing metadata")
		}
		vals := md.Get(grpcTokenMetaKey)
		if len(vals) == 0 || vals[0] != token {
			return status.Error(codes.Unauthenticated, "invalid or missing "+grpcTokenMetaKey)
		}
		return handler(srv, ss)
	}
}

// Connect implements SatelliteServiceServer. It handles one satellite lifecycle.
func (g *GRPCServer) Connect(stream satellitepb.SatelliteService_ConnectServer) error {
	// First message must be a Registration.
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	reg, ok := first.GetPayload().(*satellitepb.SatelliteMessage_Registration)
	if !ok {
		return status.Error(codes.InvalidArgument, "first message must be Registration")
	}

	name := reg.Registration.GetName()
	if name == "" {
		return status.Error(codes.InvalidArgument, "satellite name must not be empty")
	}

	devices := pbToDevices(reg.Registration.GetDevices())
	rs := newRemoteSatellite(name, devices, stream, g.heartbeatTimeout, g.heartbeatCheckInterval)
	g.registry.Register(rs)
	defer g.registry.Unregister(name)

	log.Info().Str("name", name).Msg("satellite gRPC: connected")

	// Pump the command channel downstream in a separate goroutine.
	go rs.sendLoop()

	// Read upstream messages (state, heartbeat, devices, track-end).
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Debug().Str("name", name).Err(err).Msg("satellite gRPC: recv error")
			break
		}
		rs.handleUpstream(msg)
	}

	log.Info().Str("name", name).Msg("satellite gRPC: disconnected")
	return nil
}

// ── Remote satellite (represents one connected satellite on the server side) ─

type remoteSatellite struct {
	name    string
	stream  satellitepb.SatelliteService_ConnectServer
	cmdCh   chan *satellitepb.ServerMessage // buffered; sendLoop drains it
	closeCh chan struct{}
	once    sync.Once

	mu             sync.RWMutex
	devices        []AudioDevice
	activeDevice   string
	state          PlaybackState
	stateListeners []func(PlaybackState)
	endListeners   []func()

	lastHeartbeat          time.Time
	heartbeatTimeout       time.Duration
	heartbeatCheckInterval time.Duration
}

func newRemoteSatellite(name string, devices []AudioDevice, stream satellitepb.SatelliteService_ConnectServer, heartbeatTimeout, heartbeatCheckInterval time.Duration) *remoteSatellite {
	rs := &remoteSatellite{
		name:                   name,
		stream:                 stream,
		cmdCh:                  make(chan *satellitepb.ServerMessage, 32),
		closeCh:                make(chan struct{}),
		devices:                devices,
		state:                  PlaybackState{Status: StatusIdle},
		heartbeatTimeout:       heartbeatTimeout,
		heartbeatCheckInterval: heartbeatCheckInterval,
	}
	rs.lastHeartbeat = time.Now()

	// Start heartbeat watchdog.
	go rs.watchHeartbeat()
	return rs
}

func (rs *remoteSatellite) Name() string  { return rs.name }
func (rs *remoteSatellite) IsLocal() bool { return false }

func (rs *remoteSatellite) Devices() []AudioDevice {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.devices
}

func (rs *remoteSatellite) ActiveDevice() string {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.activeDevice
}

func (rs *remoteSatellite) Send(cmd Command) error {
	msg := cmdToPB(cmd)
	select {
	case rs.cmdCh <- msg:
		return nil
	case <-rs.closeCh:
		return io.ErrClosedPipe
	}
}

// RequestDevices enqueues a RequestDevices message to the satellite client,
// which will respond with an updated DeviceList.
func (rs *remoteSatellite) RequestDevices() {
	msg := &satellitepb.ServerMessage{
		Payload: &satellitepb.ServerMessage_RequestDevices{
			RequestDevices: &satellitepb.RequestDevices{},
		},
	}
	select {
	case rs.cmdCh <- msg:
	case <-rs.closeCh:
	}
}

func (rs *remoteSatellite) PlaybackState() PlaybackState {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.state
}

func (rs *remoteSatellite) OnPlaybackState(fn func(PlaybackState)) {
	rs.mu.Lock()
	rs.stateListeners = append(rs.stateListeners, fn)
	rs.mu.Unlock()
}

func (rs *remoteSatellite) OnTrackEnd(fn func()) {
	rs.mu.Lock()
	rs.endListeners = append(rs.endListeners, fn)
	rs.mu.Unlock()
}

func (rs *remoteSatellite) Close() {
	rs.once.Do(func() {
		close(rs.closeCh)
		// Immediately emit an idle state so callers (e.g. the registry's
		// onStateChange) can clear any stale "playing" UI state without
		// waiting for the gRPC stream to close and Unregister to fire.
		rs.mu.RLock()
		listeners := rs.stateListeners
		rs.mu.RUnlock()
		idle := PlaybackState{Status: StatusIdle}
		for _, fn := range listeners {
			go fn(idle)
		}
	})
}

// sendLoop drains cmdCh and writes ServerMessages to the gRPC stream.
func (rs *remoteSatellite) sendLoop() {
	for {
		select {
		case msg := <-rs.cmdCh:
			if err := rs.stream.Send(msg); err != nil {
				log.Debug().Str("name", rs.name).Err(err).Msg("satellite gRPC: send failed")
				rs.Close()
				return
			}
		case <-rs.closeCh:
			return
		}
	}
}

// handleUpstream processes a message received from the satellite.
func (rs *remoteSatellite) handleUpstream(msg *satellitepb.SatelliteMessage) {
	switch p := msg.GetPayload().(type) {
	case *satellitepb.SatelliteMessage_Heartbeat:
		rs.mu.Lock()
		rs.lastHeartbeat = time.Now()
		rs.mu.Unlock()

	case *satellitepb.SatelliteMessage_State:
		ps := pbToPlaybackState(p.State)
		rs.mu.Lock()
		rs.state = ps
		if p.State.GetAudioDevice() != "" {
			rs.activeDevice = p.State.GetAudioDevice()
		}
		listeners := rs.stateListeners
		rs.mu.Unlock()
		for _, fn := range listeners {
			go fn(ps)
		}

	case *satellitepb.SatelliteMessage_Devices:
		devs := pbToDevices(p.Devices.GetDevices())
		rs.mu.Lock()
		rs.devices = devs
		rs.mu.Unlock()
		log.Debug().Str("name", rs.name).Int("count", len(devs)).Msg("satellite gRPC: device list updated")

	case *satellitepb.SatelliteMessage_TrackEnded:
		rs.mu.RLock()
		listeners := rs.endListeners
		rs.mu.RUnlock()
		for _, fn := range listeners {
			go fn()
		}
	}
}

// watchHeartbeat closes the satellite if no heartbeat is received within heartbeatTimeout.
func (rs *remoteSatellite) watchHeartbeat() {
	ticker := time.NewTicker(rs.heartbeatCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rs.mu.RLock()
			last := rs.lastHeartbeat
			rs.mu.RUnlock()
			if time.Since(last) > rs.heartbeatTimeout {
				log.Warn().Str("name", rs.name).Msg("satellite gRPC: heartbeat timeout — closing")
				rs.Close()
				return
			}
		case <-rs.closeCh:
			return
		}
	}
}

// ── Satellite-mode gRPC client (runs in a remote satellite binary) ────────────

// Client dials a remote subsd server, registers, and processes commands.
type Client struct {
	name              string
	addr              string
	handler           CommandHandler
	HeartbeatInterval time.Duration // how often to send heartbeats; defaults to DefaultHeartbeatInterval
	StatePushInterval time.Duration // how often to push playback state; defaults to DefaultStatePushInterval
	TLSEnabled        bool          // dial with TLS using system root CAs
	TLSCAFile         string        // path to CA cert file; implies TLS; used for self-signed server certs
	Token             string        // shared secret sent as x-subsd-token metadata; optional //nolint:gosec
}

// CommandHandler receives commands from the server and returns state updates.
type CommandHandler interface {
	// HandleCommand processes an inbound server command.
	HandleCommand(cmd Command)
	// StateSnapshot returns the current playback state to push upstream.
	StateSnapshot() PlaybackState
	// Devices returns available audio devices.
	Devices() []AudioDevice
}

// trackEndSetter is an optional extension of CommandHandler for handlers that
// can notify the client when a track ends naturally (so the client can forward
// TrackEnded upstream to the daemon).
type trackEndSetter interface {
	SetTrackEndCallback(fn func())
}

// NewClient creates a satellite gRPC client that will connect to addr.
func NewClient(name, addr string, handler CommandHandler) *Client {
	return &Client{
		name:              name,
		addr:              addr,
		handler:           handler,
		HeartbeatInterval: DefaultHeartbeatInterval,
		StatePushInterval: DefaultStatePushInterval,
	}
}

// Run dials the server, registers, and processes the bidirectional stream until
// ctx is cancelled or the connection drops.
func (c *Client) Run(ctx context.Context) error {
	tlsActive := c.TLSEnabled || c.TLSCAFile != ""
	if c.Token != "" && !tlsActive {
		log.Warn().Msg("satellite gRPC: token auth is set but TLS is not — token will be sent in plaintext")
	}

	dialOpts := []grpc.DialOption{
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	}
	switch {
	case c.TLSCAFile != "":
		creds, err := credentials.NewClientTLSFromFile(c.TLSCAFile, "")
		if err != nil {
			return fmt.Errorf("satellite gRPC: loading CA cert: %w", err)
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
		log.Info().Str("ca", c.TLSCAFile).Msg("satellite gRPC: TLS enabled (custom CA)")
	case c.TLSEnabled:
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")))
		log.Info().Msg("satellite gRPC: TLS enabled (system roots)")
	default:
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	if c.Token != "" {
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(grpcTokenCreds{token: c.Token}))
	}

	conn, err := grpc.NewClient(c.addr, dialOpts...)
	if err != nil {
		return err
	}
	defer conn.Close() //nolint:errcheck

	stub := satellitepb.NewSatelliteServiceClient(conn)
	stream, err := stub.Connect(ctx)
	if err != nil {
		return err
	}

	// All upstream sends share a mutex so concurrent goroutines don't race on
	// the stream (gRPC streams are not safe for concurrent sends).
	var sendMu sync.Mutex
	send := func(msg *satellitepb.SatelliteMessage) {
		sendMu.Lock()
		_ = stream.Send(msg)
		sendMu.Unlock()
	}

	// Send registration.
	devices := c.handler.Devices()
	pbDevs := make([]*satellitepb.AudioDevice, len(devices))
	for i, d := range devices {
		pbDevs[i] = &satellitepb.AudioDevice{Id: d.ID, Name: d.Name}
	}
	sendMu.Lock()
	err = stream.Send(&satellitepb.SatelliteMessage{
		Payload: &satellitepb.SatelliteMessage_Registration{
			Registration: &satellitepb.Registration{
				Name:    c.name,
				Devices: pbDevs,
			},
		},
	})
	sendMu.Unlock()
	if err != nil {
		return err
	}
	log.Info().Str("name", c.name).Str("server", c.addr).Msg("satellite: registered with server")

	// Wire track-end forwarding: when the local player finishes a track
	// naturally, send TrackEnded upstream so the daemon advances its queue.
	if tes, ok := c.handler.(trackEndSetter); ok {
		tes.SetTrackEndCallback(func() {
			send(&satellitepb.SatelliteMessage{
				Payload: &satellitepb.SatelliteMessage_TrackEnded{
					TrackEnded: &satellitepb.TrackEnded{},
				},
			})
		})
		defer tes.SetTrackEndCallback(nil)
	}

	// Start heartbeat and state-push goroutines.
	go c.heartbeatLoop(ctx, send)
	go c.statePushLoop(ctx, send)

	// Process incoming server commands.
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch p := msg.GetPayload().(type) {
		case *satellitepb.ServerMessage_Command:
			c.handler.HandleCommand(pbToCommand(p.Command))
		case *satellitepb.ServerMessage_RequestDevices:
			devs := c.handler.Devices()
			pbDevs := make([]*satellitepb.AudioDevice, len(devs))
			for i, d := range devs {
				pbDevs[i] = &satellitepb.AudioDevice{Id: d.ID, Name: d.Name}
			}
			send(&satellitepb.SatelliteMessage{
				Payload: &satellitepb.SatelliteMessage_Devices{
					Devices: &satellitepb.DeviceList{Devices: pbDevs},
				},
			})
		}
	}
}

func (c *Client) heartbeatLoop(ctx context.Context, send func(*satellitepb.SatelliteMessage)) {
	ticker := time.NewTicker(c.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			send(&satellitepb.SatelliteMessage{
				Payload: &satellitepb.SatelliteMessage_Heartbeat{
					Heartbeat: &satellitepb.Heartbeat{TimestampMs: time.Now().UnixMilli()},
				},
			})
		case <-ctx.Done():
			return
		}
	}
}

func (c *Client) statePushLoop(ctx context.Context, send func(*satellitepb.SatelliteMessage)) {
	ticker := time.NewTicker(c.StatePushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			send(&satellitepb.SatelliteMessage{
				Payload: &satellitepb.SatelliteMessage_State{
					State: playbackStateToPB(c.handler.StateSnapshot()),
				},
			})
		case <-ctx.Done():
			return
		}
	}
}

// ── Per-RPC token credentials ─────────────────────────────────────────────────

// grpcTokenCreds implements credentials.PerRPCCredentials by attaching the
// shared secret as x-subsd-token metadata on every RPC call.
type grpcTokenCreds struct {
	token string
}

func (c grpcTokenCreds) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{grpcTokenMetaKey: c.token}, nil
}

// RequireTransportSecurity returns false so the token can also be used without
// TLS in fully-trusted local setups. Operators are warned at startup when a
// token is configured without TLS.
func (c grpcTokenCreds) RequireTransportSecurity() bool { return false }

// ── Proto conversion helpers ──────────────────────────────────────────────────

func pbToDevices(pbDevs []*satellitepb.AudioDevice) []AudioDevice {
	out := make([]AudioDevice, len(pbDevs))
	for i, d := range pbDevs {
		out[i] = AudioDevice{ID: d.GetId(), Name: d.GetName()}
	}
	return out
}

func pbToPlaybackState(s *satellitepb.PlayerState) PlaybackState {
	var status PlaybackStatus
	switch s.GetStatus() {
	case satellitepb.Status_PLAYING:
		status = StatusPlaying
	case satellitepb.Status_PAUSED:
		status = StatusPaused
	case satellitepb.Status_IDLE:
		status = StatusIdle
	}
	return PlaybackState{
		Status:      status,
		Position:    s.GetPosition(),
		Duration:    s.GetDuration(),
		CurrentURL:  s.GetCurrentUrl(),
		Volume:      int(s.GetVolume()),
		AudioDevice: s.GetAudioDevice(),
	}
}

func playbackStateToPB(ps PlaybackState) *satellitepb.PlayerState {
	var status satellitepb.Status
	switch ps.Status {
	case StatusPlaying:
		status = satellitepb.Status_PLAYING
	case StatusPaused:
		status = satellitepb.Status_PAUSED
	case StatusIdle:
		status = satellitepb.Status_IDLE
	}
	return &satellitepb.PlayerState{
		Status:      status,
		Position:    ps.Position,
		Duration:    ps.Duration,
		CurrentUrl:  ps.CurrentURL,
		Volume:      int32(ps.Volume), //nolint:gosec
		AudioDevice: ps.AudioDevice,
	}
}

func cmdToPB(cmd Command) *satellitepb.ServerMessage {
	pbCmd := &satellitepb.Command{
		Url:        cmd.URL,
		Position:   cmd.Position,
		Volume:     int32(cmd.Volume), //nolint:gosec
		Device:     cmd.Device,
		Id:         cmd.ID,
		Title:      cmd.Title,
		Artist:     cmd.Artist,
		Album:      cmd.Album,
		ReplayGain: cmd.ReplayGain,
	}
	switch cmd.Type {
	case CmdPlay:
		pbCmd.Type = satellitepb.CommandType_PLAY
	case CmdPause:
		pbCmd.Type = satellitepb.CommandType_PAUSE
	case CmdStop:
		pbCmd.Type = satellitepb.CommandType_STOP
	case CmdSeek:
		pbCmd.Type = satellitepb.CommandType_SEEK
	case CmdSetVolume:
		pbCmd.Type = satellitepb.CommandType_SET_VOLUME
	case CmdSetAudioDevice:
		pbCmd.Type = satellitepb.CommandType_SET_AUDIO_DEVICE
	case CmdResume:
		pbCmd.Type = satellitepb.CommandType_RESUME
	case CmdSetReplayGain:
		pbCmd.Type = satellitepb.CommandType_SET_REPLAY_GAIN
	}
	return &satellitepb.ServerMessage{
		Payload: &satellitepb.ServerMessage_Command{Command: pbCmd},
	}
}

func pbToCommand(c *satellitepb.Command) Command {
	cmd := Command{
		URL:        c.GetUrl(),
		Position:   c.GetPosition(),
		Volume:     int(c.GetVolume()),
		Device:     c.GetDevice(),
		ID:         c.GetId(),
		Title:      c.GetTitle(),
		Artist:     c.GetArtist(),
		Album:      c.GetAlbum(),
		ReplayGain: c.GetReplayGain(),
	}
	switch c.GetType() {
	case satellitepb.CommandType_PLAY:
		cmd.Type = CmdPlay
	case satellitepb.CommandType_PAUSE:
		cmd.Type = CmdPause
	case satellitepb.CommandType_STOP:
		cmd.Type = CmdStop
	case satellitepb.CommandType_SEEK:
		cmd.Type = CmdSeek
	case satellitepb.CommandType_SET_VOLUME:
		cmd.Type = CmdSetVolume
	case satellitepb.CommandType_SET_AUDIO_DEVICE:
		cmd.Type = CmdSetAudioDevice
	case satellitepb.CommandType_RESUME:
		cmd.Type = CmdResume
	case satellitepb.CommandType_SET_REPLAY_GAIN:
		cmd.Type = CmdSetReplayGain
	}
	return cmd
}
