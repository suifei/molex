package relay

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/suifei/molex/internal/protocol"
	"github.com/suifei/molex/internal/telemetry"
)

const defaultPairTimeout = 90 * time.Second

const maxRelayMetadataFrames = 8

const relayWriteTimeout = 30 * time.Second

// Keepalive bounds how long a dead connection can occupy its group slot.
// This matters most for the single-target-per-token rule: after a target
// host crashes, its stale sessions are cleared within keepAliveTimeout and
// the restarted target can join again.
const (
	keepAliveInterval = 20 * time.Second
	keepAliveTimeout  = 75 * time.Second
)

// Application close codes surfaced to clients for actionable errors.
const (
	CloseKicked          = 4401
	CloseTokenDisabled   = 4403
	CloseDuplicateTarget = 4409
)

type Options struct {
	Listen      string
	Path        string
	Tokens      []Credential
	PairTimeout time.Duration
	Logger      *slog.Logger
	Reporter    telemetry.Reporter
}

type Server struct {
	options  Options
	registry *registry
	tokens   *tokenStore
	groups   *groupSet
	upgrader websocket.Upgrader

	mu           sync.Mutex
	httpServer   *http.Server
	listener     net.Listener
	connections  map[*websocket.Conn]struct{}
	participants map[string]*participant
	closed       bool
	nextPeerID   atomic.Uint64
}

func New(options Options) *Server {
	if options.Path == "" {
		options.Path = "/ws/session"
	}
	if options.PairTimeout <= 0 {
		options.PairTimeout = defaultPairTimeout
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	s := &Server{
		options:      options,
		registry:     newRegistry(),
		tokens:       newTokenStore(options.Tokens),
		groups:       newGroupSet(),
		connections:  make(map[*websocket.Conn]struct{}),
		participants: make(map[string]*participant),
	}
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:    4096,
		WriteBufferSize:   4096,
		EnableCompression: false,
		CheckOrigin:       sameOriginOrNative,
	}
	return s
}

// UpdateTokens applies the latest token list without restarting the relay.
// Groups whose token was disabled or removed are disconnected immediately.
func (s *Server) UpdateTokens(credentials []Credential) {
	s.tokens.replace(credentials)
	active := s.tokens.activeIDs()
	for _, tokenID := range s.groups.tokenIDs() {
		if active[tokenID] {
			continue
		}
		members := s.groups.members(tokenID)
		for _, member := range members {
			member.armClose(CloseTokenDisabled, "token disabled by relay administrator")
		}
		for _, member := range members {
			member.closeWith(CloseTokenDisabled, "token disabled by relay administrator")
		}
		if len(members) > 0 {
			telemetry.Emit(s.options.Reporter, telemetry.Event{
				Type:    "relay_token_revoked",
				Level:   "warning",
				Message: fmt.Sprintf("Token %s was disabled or removed; %d connected client(s) were disconnected", tokenID, len(members)),
			})
		}
	}
}

// DisconnectPeer closes one connected participant by its peer id.
func (s *Server) DisconnectPeer(peerID string) bool {
	s.mu.Lock()
	target := s.participants[peerID]
	s.mu.Unlock()
	if target == nil {
		return false
	}
	target.armClose(CloseKicked, "disconnected by relay administrator")
	target.closeWith(CloseKicked, "disconnected by relay administrator")
	telemetry.Emit(s.options.Reporter, telemetry.Event{
		Type:    "relay_peer_kicked",
		Level:   "warning",
		Message: fmt.Sprintf("%s %s was disconnected by the relay administrator", displayRole(target.role), target.displayName()),
	})
	return true
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(s.options.Path, s.handleWebSocket)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	return securityHeaders(mux)
}

func (s *Server) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.options.Listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.options.Listen, err)
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		listener.Close()
		return errors.New("relay server is closed")
	}
	s.listener = listener
	s.httpServer = &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	httpServer := s.httpServer
	s.mu.Unlock()

	telemetry.Emit(s.options.Reporter, telemetry.Event{
		Type:    "relay_listening",
		Level:   "info",
		State:   "running",
		Message: "Relay is accepting WebSocket sessions",
		Listen:  listener.Addr().String(),
	})

	go s.sweepExpiredLegacy(ctx)

	shutdownDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			s.closeConnections()
			timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = httpServer.Shutdown(timeout)
			cancel()
		case <-shutdownDone:
		}
	}()

	err = httpServer.Serve(listener)
	close(shutdownDone)
	if errors.Is(err, http.ErrServerClosed) || ctx.Err() != nil {
		return nil
	}
	return fmt.Errorf("serve relay: %w", err)
}

func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	httpServer := s.httpServer
	s.mu.Unlock()

	s.closeConnections()
	var err error
	if httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = httpServer.Shutdown(ctx)
		cancel()
	}
	return err
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token, status := s.authorize(r.Header.Get("Authorization"))
	if status != 0 {
		w.Header().Set("WWW-Authenticate", `Bearer realm="relay"`)
		if status == http.StatusForbidden {
			http.Error(w, "token disabled", status)
		} else {
			http.Error(w, "unauthorized", status)
		}
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	if !s.trackConnection(conn) {
		conn.Close()
		return
	}
	var peer *participant
	defer func() {
		s.untrackConnection(conn)
		if peer != nil {
			peer.close()
		} else {
			_ = conn.Close()
		}
	}()
	metadataFrames := make([][]byte, 0, 4)
	defaultPingHandler := conn.PingHandler()
	conn.SetPingHandler(func(data string) error {
		if len(data) == protocol.RelayMetadataFrameSize && len(metadataFrames) < maxRelayMetadataFrames {
			metadataFrames = append(metadataFrames, append([]byte(nil), data...))
		}
		return defaultPingHandler(data)
	})

	deadline := time.Now().Add(15 * time.Second)
	_ = conn.SetReadDeadline(deadline)
	conn.SetReadLimit(protocol.HelloSize)
	messageType, packet, err := conn.ReadMessage()
	if err != nil || messageType != websocket.BinaryMessage {
		return
	}
	hello, err := protocol.ParseHello(packet)
	if err != nil {
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "invalid session"),
			time.Now().Add(time.Second))
		return
	}
	if subtle.ConstantTimeCompare(hello.Route[:], token.route[:]) != 1 {
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "session route does not match the presented token"),
			time.Now().Add(time.Second))
		return
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(keepAliveTimeout))
	})
	_ = conn.SetReadDeadline(time.Now().Add(keepAliveTimeout))
	conn.SetReadLimit(65 << 10)
	ip, proxied := clientConnection(r)

	peer = &participant{
		id:          strconv.FormatUint(s.nextPeerID.Add(1), 10),
		ip:          ip,
		proxied:     proxied,
		tokenID:     token.id,
		legacyToken: token.legacy,
		routeID:     routeIdentifier(hello.Route),
		connectedAt: time.Now().UTC(),
		conn:        conn,
		hello:       append([]byte(nil), packet...),
		role:        hello.Role,
		route:       hello.Route,
		done:        make(chan struct{}),
		paired:      make(chan struct{}),
		reported:    make(chan struct{}),
		frames:      make(chan []byte),
	}
	initialMetadata := protocol.OpenRelayMetadata(hello, token.token, metadataFrames)
	peer.metadata.Store(&initialMetadata)
	// Clients may refresh their operational metadata (name, service and
	// mapping counts) at runtime through the same encrypted ping channel.
	refreshPeer := peer
	conn.SetPingHandler(func(data string) error {
		if len(data) == protocol.RelayMetadataFrameSize {
			s.queuePeerMetadata(refreshPeer, token.token, []byte(data))
		}
		return defaultPingHandler(data)
	})
	if err := s.groups.join(peer); err != nil {
		s.emitTargetRejected(peer)
		peer.closeWith(CloseDuplicateTarget, "another target is already connected for this token")
		return
	}
	if err := s.registry.join(peer); err != nil {
		s.groups.leave(peer)
		peer.closeWith(websocket.ClosePolicyViolation, "session unavailable")
		return
	}
	s.trackParticipant(peer)
	defer func() {
		s.untrackParticipant(peer)
		s.registry.remove(peer)
		s.groups.leave(peer)
		s.emitPeerDisconnected(peer)
	}()
	go s.keepAlive(peer)
	go s.readParticipant(peer)
	s.emitPeerConnected(peer)
	close(peer.reported)

	if peer.bridgeOwner {
		go s.bridge(peer, peer.peer.Load())
	}

	s.awaitParticipant(r.Context(), peer)
}

// authorize resolves the bearer token. It returns a non-zero HTTP status
// when admission must be rejected.
func (s *Server) authorize(header string) (tokenState, int) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return tokenState{}, http.StatusUnauthorized
	}
	state, ok := s.tokens.lookup(strings.TrimPrefix(header, prefix))
	if !ok {
		return tokenState{}, http.StatusUnauthorized
	}
	if state.disabled {
		return tokenState{}, http.StatusForbidden
	}
	return state, 0
}

func (s *Server) keepAlive(p *participant) {
	ticker := time.NewTicker(keepAliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			if err := p.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
				p.abort()
				return
			}
		}
	}
}

// queuePeerMetadata buffers one encrypted metadata ping. A complete refresh
// is several frames (name, endpoint, platform, …); they are flushed together
// after a short burst window so the per-connection rate limit cannot drop
// the later fields.
func (s *Server) queuePeerMetadata(p *participant, tokenValue string, frame []byte) {
	p.metaMu.Lock()
	p.pendingMeta = append(p.pendingMeta, append([]byte(nil), frame...))
	if len(p.pendingMeta) > maxRelayMetadataFrames {
		p.pendingMeta = p.pendingMeta[len(p.pendingMeta)-maxRelayMetadataFrames:]
	}
	startFlush := !p.metaFlush
	if startFlush {
		p.metaFlush = true
	}
	p.metaMu.Unlock()
	if startFlush {
		go s.flushPeerMetadata(p, tokenValue)
	}
}

func (s *Server) flushPeerMetadata(p *participant, tokenValue string) {
	timer := time.NewTimer(40 * time.Millisecond)
	select {
	case <-p.done:
		timer.Stop()
	case <-timer.C:
	}
	p.metaMu.Lock()
	frames := p.pendingMeta
	p.pendingMeta = nil
	p.metaFlush = false
	p.metaMu.Unlock()
	if len(frames) == 0 {
		return
	}
	now := time.Now().UnixNano()
	last := p.lastMeta.Load()
	if last != 0 && now-last < int64(5*time.Second) {
		return
	}
	if !p.lastMeta.CompareAndSwap(last, now) {
		return
	}
	hello, err := protocol.ParseHello(p.hello)
	if err != nil {
		return
	}
	update := protocol.OpenRelayMetadata(hello, tokenValue, frames)
	if update == (protocol.RelayMetadata{}) {
		return
	}
	p.mergeMetadata(update)
	status := telemetry.PeerStatusWaiting
	select {
	case <-p.paired:
		status = telemetry.PeerStatusPaired
	default:
	}
	telemetry.Emit(s.options.Reporter, telemetry.Event{
		Type:      "relay_peer_stats",
		Level:     "info",
		Transient: true,
		PeerChange: &telemetry.PeerChange{
			Action: telemetry.PeerActionUpdate,
			Peers:  []telemetry.Peer{p.telemetryPeer(status)},
		},
	})
}

// sweepExpiredLegacy drops participants whose rotated-out token value fell
// past its grace window.
func (s *Server) sweepExpiredLegacy(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.dropExpiredLegacy()
		}
	}
}

func (s *Server) dropExpiredLegacy() {
	s.mu.Lock()
	candidates := make([]*participant, 0)
	for _, participant := range s.participants {
		if participant.legacyToken {
			candidates = append(candidates, participant)
		}
	}
	s.mu.Unlock()
	const reason = "rotated token grace period ended; switch to the new token"
	expired := candidates[:0]
	for _, participant := range candidates {
		if s.tokens.legacyExpired(participant.tokenID) {
			expired = append(expired, participant)
		}
	}
	for _, participant := range expired {
		participant.armClose(CloseTokenDisabled, reason)
	}
	for _, participant := range expired {
		participant.closeWith(CloseTokenDisabled, reason)
	}
}

func (s *Server) awaitParticipant(ctx context.Context, participant *participant) {
	// Target sessions are long-lived hot-standby capacity in adaptive pool
	// mode. Keep an unmatched Target connected until it is paired; otherwise
	// the Target would hit the pairing timeout and reconnect continuously even
	// though the existing Edge route is healthy. Edge waiters still use the
	// timeout to release stale local clients.
	if participant.role == protocol.RoleTarget {
		select {
		case <-participant.paired:
			select {
			case <-participant.done:
			case <-ctx.Done():
			}
		case <-participant.done:
		case <-ctx.Done():
		}
		return
	}
	timer := time.NewTimer(s.options.PairTimeout)
	defer timer.Stop()
	select {
	case <-participant.paired:
		select {
		case <-participant.done:
		case <-ctx.Done():
		}
	case <-participant.done:
	case <-timer.C:
		if s.registry.remove(participant) {
			participant.closeWith(websocket.CloseTryAgainLater, "pair timeout")
			return
		}
		// Pairing won the registry race at the timeout boundary. Keep the
		// newly paired session instead of closing it as a timed-out waiter.
		select {
		case <-participant.done:
		case <-ctx.Done():
		}
	case <-ctx.Done():
		s.registry.remove(participant)
	}
}

// readParticipant is the only goroutine that reads from a participant's
// WebSocket. It detects waiting-peer disconnects and hands paired data to the
// bridge without changing frame order.
func (s *Server) readParticipant(participant *participant) {
	defer close(participant.frames)
	defer s.registry.remove(participant)

	for {
		messageType, payload, err := participant.conn.ReadMessage()
		if err != nil {
			participant.abort()
			return
		}
		_ = participant.conn.SetReadDeadline(time.Now().Add(keepAliveTimeout))
		if messageType != websocket.BinaryMessage {
			participant.closeWith(websocket.ClosePolicyViolation, "binary frames required")
			return
		}
		select {
		case <-participant.paired:
		default:
			participant.closeWith(websocket.ClosePolicyViolation, "wait for peer before sending data")
			return
		}
		select {
		case participant.frames <- payload:
		case <-participant.done:
			return
		}
	}
}

func (s *Server) bridge(a, b *participant) {
	edge, target := a, b
	if edge.role != protocol.RoleEdge {
		edge, target = target, edge
	}
	defer closeParticipants(edge, target)
	<-edge.reported
	<-target.reported

	deadline := time.Now().Add(10 * time.Second)
	_ = edge.conn.SetWriteDeadline(deadline)
	_ = target.conn.SetWriteDeadline(deadline)
	if err := edge.conn.WriteMessage(websocket.BinaryMessage, target.hello); err != nil {
		return
	}
	if err := target.conn.WriteMessage(websocket.BinaryMessage, edge.hello); err != nil {
		return
	}
	_ = edge.conn.SetWriteDeadline(time.Time{})
	_ = target.conn.SetWriteDeadline(time.Time{})

	s.emitPeersPaired(edge, target)

	errors := make(chan error, 2)
	go func() { errors <- s.relayFrames(target, edge) }()
	go func() { errors <- s.relayFrames(edge, target) }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastMetrics := peerMetricVersion(edge, target)
	for {
		select {
		case <-errors:
			closeParticipants(edge, target)
			<-errors
			if current := peerMetricVersion(edge, target); current != lastMetrics {
				s.emitPeerStats(edge, target)
			}
			return
		case <-ticker.C:
			if current := peerMetricVersion(edge, target); current != lastMetrics {
				lastMetrics = current
				s.emitPeerStats(edge, target)
			}
		}
	}
}

func (s *Server) relayFrames(dst, src *participant) error {
	for {
		var payload []byte
		select {
		case frame, ok := <-src.frames:
			if !ok {
				return net.ErrClosed
			}
			payload = frame
		case <-src.done:
			return net.ErrClosed
		}
		now := time.Now().UTC()
		src.bytesReceived.Add(uint64(len(payload)))
		src.framesReceived.Add(1)
		src.lastActivity.Store(now.UnixNano())
		src.metricVersion.Add(1)
		_ = dst.conn.SetWriteDeadline(time.Now().Add(relayWriteTimeout))
		if err := dst.conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
			return err
		}
		_ = dst.conn.SetWriteDeadline(time.Time{})
		dst.bytesSent.Add(uint64(len(payload)))
		dst.framesSent.Add(1)
		dst.lastActivity.Store(now.UnixNano())
		dst.metricVersion.Add(1)
	}
}

func closeParticipants(a, b *participant) {
	a.close()
	b.close()
}

func (s *Server) trackConnection(conn *websocket.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.connections[conn] = struct{}{}
	return true
}

func (s *Server) untrackConnection(conn *websocket.Conn) {
	s.mu.Lock()
	delete(s.connections, conn)
	s.mu.Unlock()
}

func (s *Server) trackParticipant(p *participant) {
	s.mu.Lock()
	s.participants[p.id] = p
	s.mu.Unlock()
}

func (s *Server) untrackParticipant(p *participant) {
	s.mu.Lock()
	delete(s.participants, p.id)
	s.mu.Unlock()
}

func (s *Server) closeConnections() {
	s.mu.Lock()
	s.closed = true
	connections := make([]*websocket.Conn, 0, len(s.connections))
	for conn := range s.connections {
		connections = append(connections, conn)
	}
	clear(s.connections)
	s.mu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
}

func (s *Server) emitPeerConnected(participant *participant) {
	peer := participant.telemetryPeer(telemetry.PeerStatusWaiting)
	telemetry.Emit(s.options.Reporter, telemetry.Event{
		Type:    "relay_peer_connected",
		Level:   "info",
		Message: fmt.Sprintf("%s connected from %s", displayRole(participant.role), participant.ip),
		PeerChange: &telemetry.PeerChange{
			Action: telemetry.PeerActionUpsert,
			Peers:  []telemetry.Peer{peer},
		},
	})
}

func (s *Server) emitTargetRejected(participant *participant) {
	telemetry.Emit(s.options.Reporter, telemetry.Event{
		Type:    "relay_target_rejected",
		Level:   "warning",
		Message: fmt.Sprintf("A second Target from %s was rejected for token %s; each token accepts exactly one Target", participant.ip, participant.tokenID),
	})
}

func (s *Server) emitPeersPaired(edge, target *participant) {
	telemetry.Emit(s.options.Reporter, telemetry.Event{
		Type:    "relay_paired",
		Level:   "info",
		Message: "Edge and target sessions paired",
		PeerChange: &telemetry.PeerChange{
			Action: telemetry.PeerActionUpsert,
			Peers: []telemetry.Peer{
				edge.telemetryPeer(telemetry.PeerStatusPaired),
				target.telemetryPeer(telemetry.PeerStatusPaired),
			},
		},
	})
}

func (s *Server) emitPeerStats(edge, target *participant) {
	telemetry.Emit(s.options.Reporter, telemetry.Event{
		Type:      "relay_peer_stats",
		Level:     "info",
		Transient: true,
		PeerChange: &telemetry.PeerChange{
			Action: telemetry.PeerActionUpdate,
			Peers: []telemetry.Peer{
				edge.telemetryPeer(telemetry.PeerStatusPaired),
				target.telemetryPeer(telemetry.PeerStatusPaired),
			},
		},
	})
}

func (s *Server) emitPeerDisconnected(participant *participant) {
	telemetry.Emit(s.options.Reporter, telemetry.Event{
		Type:    "relay_peer_disconnected",
		Level:   "info",
		Message: fmt.Sprintf("%s disconnected from %s", displayRole(participant.role), participant.ip),
		PeerChange: &telemetry.PeerChange{
			Action: telemetry.PeerActionRemove,
			Peers:  []telemetry.Peer{participant.telemetryPeer("")},
		},
	})
}

func (p *participant) telemetryPeer(status string) telemetry.Peer {
	metadata := p.currentMetadata()
	peer := telemetry.Peer{
		ID:             p.id,
		IP:             p.ip,
		Name:           metadata.Name,
		Role:           p.role.String(),
		Status:         status,
		TokenID:        p.tokenID,
		Endpoint:       metadata.Endpoint,
		RelayEndpoint:  metadata.RelayEndpoint,
		Platform:       metadata.Platform,
		RouteID:        p.routeID,
		Proxied:        p.proxied,
		ConnectedAt:    p.connectedAt,
		BytesReceived:  p.bytesReceived.Load(),
		BytesSent:      p.bytesSent.Load(),
		FramesReceived: p.framesReceived.Load(),
		FramesSent:     p.framesSent.Load(),
	}
	if lastActivity := p.lastActivity.Load(); lastActivity > 0 {
		peer.LastActivityAt = time.Unix(0, lastActivity).UTC()
	}
	if counterpart := p.peer.Load(); counterpart != nil {
		peer.PeerID = counterpart.id
		peer.PeerName = counterpart.currentMetadata().Name
	}
	return peer
}

func (p *participant) displayName() string {
	if name := p.currentMetadata().Name; name != "" {
		return name
	}
	return "#" + p.id
}

func peerMetricVersion(peers ...*participant) uint64 {
	var version uint64
	for _, peer := range peers {
		version += peer.metricVersion.Load()
	}
	return version
}

func routeIdentifier(route [32]byte) string {
	digest := sha256.Sum256(append([]byte("molex/route-display/v1\x00"), route[:]...))
	return hex.EncodeToString(digest[:6])
}

func displayRole(role protocol.Role) string {
	if role == protocol.RoleEdge {
		return "Edge"
	}
	return "Target"
}

func clientIP(r *http.Request) string {
	ip, _ := clientConnection(r)
	return ip
}

func clientConnection(r *http.Request) (string, bool) {
	direct, ok := parseIP(r.RemoteAddr)
	if !ok {
		return "unknown", false
	}
	if direct.IsLoopback() {
		if forwarded, ok := parseForwardedIP(r.Header.Get("X-Forwarded-For")); ok {
			return forwarded.String(), true
		}
		if forwarded, ok := parseIP(r.Header.Get("X-Real-IP")); ok {
			return forwarded.String(), true
		}
	}
	return direct.String(), false
}

func parseForwardedIP(value string) (netip.Addr, bool) {
	for address := range strings.SplitSeq(value, ",") {
		if parsed, ok := parseIP(address); ok {
			return parsed, true
		}
	}
	return netip.Addr{}, false
}

func parseIP(value string) (netip.Addr, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return netip.Addr{}, false
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	} else {
		value = strings.Trim(value, "[]")
	}
	if zone := strings.LastIndexByte(value, '%'); zone >= 0 {
		value = value[:zone]
	}
	address, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}, false
	}
	return address.Unmap(), true
}

func sameOriginOrNative(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
