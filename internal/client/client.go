package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/suifei/molex/internal/config"
	"github.com/suifei/molex/internal/protocol"
	"github.com/suifei/molex/internal/telemetry"
)

func Run(ctx context.Context, cfg config.Config, reporter telemetry.Reporter) error {
	return run(ctx, cfg, reporter, defaultRetrySettings())
}

func run(ctx context.Context, cfg config.Config, reporter telemetry.Reporter, retry retrySettings) error {
	cfg = cfg.Normalized()
	if err := cfg.Validate(); err != nil {
		return err
	}
	if cfg.Mode != config.ModePunch {
		return errors.New("client requires punch mode")
	}

	policy := newRetryPolicy(retry)
	for {
		telemetry.Emit(reporter, telemetry.Event{
			Type:        "client_connecting",
			Level:       "info",
			State:       "connecting",
			Message:     "Connecting to relay",
			ClearListen: true,
		})
		connectedFor, err := runAttempt(ctx, cfg, reporter)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			err = errSessionClosed
		}
		delay := policy.nextDelay(connectedFor)
		telemetry.Emit(reporter, telemetry.Event{
			Type:        "client_reconnecting",
			Level:       "warning",
			State:       "connecting",
			Message:     reconnectMessage(delay, err),
			ClearListen: true,
		})
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func RunOnce(ctx context.Context, cfg config.Config, reporter telemetry.Reporter) error {
	_, err := runAttempt(ctx, cfg, reporter)
	return err
}

func runAttempt(ctx context.Context, cfg config.Config, reporter telemetry.Reporter) (time.Duration, error) {
	role, err := protocolRole(cfg.Role)
	if err != nil {
		return 0, err
	}
	secureConn, err := dialSecure(ctx, cfg, role)
	if err != nil {
		return 0, err
	}
	defer secureConn.Close()
	connectedAt := time.Now()

	stopWatcher := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = secureConn.Close()
		case <-stopWatcher:
		}
	}()
	defer close(stopWatcher)

	if role == protocol.RoleEdge {
		err = runEdge(ctx, cfg, secureConn, reporter)
	} else {
		err = runTarget(ctx, cfg, secureConn, reporter)
	}
	return time.Since(connectedAt), err
}

func dialSecure(ctx context.Context, cfg config.Config, role protocol.Role) (net.Conn, error) {
	remote, err := config.NormalizeRemote(cfg.Remote)
	if err != nil {
		return nil, err
	}
	dialer := websocket.Dialer{
		Proxy:             http.ProxyFromEnvironment,
		HandshakeTimeout:  12 * time.Second,
		EnableCompression: false,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
		},
	}
	header := make(http.Header)
	if cfg.Token != "" {
		header.Set("Authorization", "Bearer "+cfg.Token)
	}
	ws, response, err := dialer.DialContext(ctx, remote, header)
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("dial relay: %w", &relayHTTPError{
				statusCode: response.StatusCode,
				status:     response.Status,
				err:        err,
			})
		}
		return nil, fmt.Errorf("dial relay: %w", err)
	}
	secure, err := protocol.OpenSecureClientWithMetadata(
		ctx,
		ws,
		[]byte(cfg.Secret),
		cfg.Tunnel.Remote,
		role,
		cfg.Token,
		relayMetadata(cfg, role),
	)
	if err != nil {
		ws.Close()
		return nil, fmt.Errorf("establish encrypted session: %w", err)
	}
	return secure, nil
}

func runEdge(ctx context.Context, cfg config.Config, conn net.Conn, reporter telemetry.Reporter) error {
	session, err := yamux.Client(conn, yamuxConfig())
	if err != nil {
		return fmt.Errorf("start edge multiplexer: %w", err)
	}
	streams := newStreamGroup(maxConcurrentStreams)

	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return &localListenError{address: cfg.Listen, err: err}
	}
	defer func() {
		_ = listener.Close()
		_ = session.Close()
		streams.waitForAll()
	}()
	telemetry.Emit(reporter, telemetry.Event{
		Type:    "edge_listening",
		Level:   "info",
		State:   "running",
		Message: "Encrypted route is ready",
		Listen:  listener.Addr().String(),
	})

	go func() {
		select {
		case <-ctx.Done():
		case <-session.CloseChan():
		}
		_ = listener.Close()
	}()

	for {
		local, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if session.IsClosed() {
				return errSessionClosed
			}
			return fmt.Errorf("accept local connection: %w", err)
		}
		if !streams.goIfAvailable(func() {
			defer local.Close()
			stream, err := session.OpenStream()
			if err != nil {
				event := telemetry.Event{
					Type:    "stream_error",
					Level:   "error",
					State:   "running",
					Message: "This local connection could not open an encrypted stream. Retry it; if this repeats, check peer health and reduce simultaneous connection attempts.",
				}
				if session.IsClosed() {
					event.State = "connecting"
					event.Message = "This local connection could not be forwarded because the encrypted route was interrupted. MoleX is reconnecting; retry after the route is ready."
					event.ClearListen = true
				}
				telemetry.Emit(reporter, event)
				return
			}
			telemetry.Emit(reporter, telemetry.Event{Type: "stream_opened", Level: "info", State: "running", Message: "Local connection routed"})
			bridge(local, stream)
		}) {
			_ = local.Close()
			telemetry.Emit(reporter, streamLimitEvent())
		}
	}
}

func runTarget(ctx context.Context, cfg config.Config, conn net.Conn, reporter telemetry.Reporter) error {
	session, err := yamux.Server(conn, yamuxConfig())
	if err != nil {
		return fmt.Errorf("start target multiplexer: %w", err)
	}
	streams := newStreamGroup(maxConcurrentStreams)
	defer func() {
		_ = session.Close()
		streams.waitForAll()
	}()
	telemetry.Emit(reporter, telemetry.Event{
		Type:    "target_ready",
		Level:   "info",
		State:   "running",
		Message: "Target is ready to receive streams",
	})

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if session.IsClosed() {
				return errSessionClosed
			}
			return fmt.Errorf("accept encrypted stream: %w", err)
		}
		if !streams.goIfAvailable(func() {
			defer stream.Close()
			dialer := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
			target, err := dialer.DialContext(ctx, "tcp", cfg.Tunnel.Local)
			if err != nil {
				stream.Close()
				telemetry.Emit(reporter, telemetry.Event{
					Type:    "target_dial_error",
					Level:   "error",
					State:   "running",
					Message: targetServiceUnavailableMessage(cfg.Tunnel.Local, err),
				})
				return
			}
			telemetry.Emit(reporter, telemetry.Event{Type: "stream_opened", Level: "info", State: "running", Message: "Encrypted stream reached target service"})
			bridge(target, stream)
		}) {
			_ = stream.Close()
			telemetry.Emit(reporter, streamLimitEvent())
		}
	}
}

func protocolRole(role string) (protocol.Role, error) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case config.RoleEdge:
		return protocol.RoleEdge, nil
	case config.RoleTarget:
		return protocol.RoleTarget, nil
	default:
		return 0, errors.New("role must be edge or target")
	}
}

func yamuxConfig() *yamux.Config {
	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = 15 * time.Second
	cfg.ConnectionWriteTimeout = 15 * time.Second
	cfg.StreamOpenTimeout = 20 * time.Second
	cfg.StreamCloseTimeout = 10 * time.Second
	cfg.AcceptBacklog = maxConcurrentStreams
	return cfg
}

func relayMetadata(cfg config.Config, role protocol.Role) protocol.RelayMetadata {
	name := strings.TrimSpace(cfg.Tunnel.Name)
	if name == "" {
		name, _ = os.Hostname()
	}
	if name == "" {
		name = role.String()
	}
	endpoint := cfg.Tunnel.Local
	if role == protocol.RoleEdge {
		endpoint = cfg.Listen
	}
	return protocol.RelayMetadata{
		Name:          name,
		Endpoint:      endpoint,
		RelayEndpoint: relayDisplayEndpoint(cfg.Remote),
		Platform:      runtime.GOOS + "/" + runtime.GOARCH,
	}
}

func relayDisplayEndpoint(remote string) string {
	parsed, err := url.Parse(remote)
	if err != nil {
		return remote
	}
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}

func streamLimitEvent() telemetry.Event {
	return telemetry.Event{
		Type:    "stream_limit_reached",
		Level:   "warning",
		State:   "running",
		Message: "The route reached its 256 concurrent connection limit. Close idle connections or start another route, then retry.",
	}
}

func bridge(a, b net.Conn) {
	defer a.Close()
	defer b.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	copyOneWay := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if closer, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = closer.CloseWrite()
		}
	}
	go copyOneWay(a, b)
	go copyOneWay(b, a)
	wg.Wait()
}
