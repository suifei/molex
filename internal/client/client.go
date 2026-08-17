package client

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
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

// Updates carries live configuration changes into a running client. A nil
// channel means the corresponding list never changes at runtime.
type Updates struct {
	Services <-chan []config.ServiceEntry
	Mappings <-chan []config.MappingEntry
}

func Run(ctx context.Context, cfg config.Config, reporter telemetry.Reporter) error {
	return RunWithUpdates(ctx, cfg, reporter, Updates{})
}

func RunWithUpdates(ctx context.Context, cfg config.Config, reporter telemetry.Reporter, updates Updates) error {
	return run(ctx, cfg, reporter, defaultRetrySettings(), updates)
}

func run(ctx context.Context, cfg config.Config, reporter telemetry.Reporter, retry retrySettings, updates Updates) error {
	cfg = cfg.Normalized()
	if err := cfg.Validate(); err != nil {
		return err
	}
	switch cfg.Mode {
	case config.ModeEdge:
		return runEdgeProcess(ctx, cfg, reporter, retry, updates.Mappings)
	case config.ModeTarget:
		return runTargetProcess(ctx, cfg, reporter, retry, updates.Services)
	default:
		return errors.New("client requires target or edge mode")
	}
}

// sessionHandler drives one authenticated encrypted session until it ends.
type sessionHandler func(ctx context.Context, conn net.Conn, reporter telemetry.Reporter) error

// sessionSpec bundles everything one reconnecting session loop needs. The
// metadata provider is evaluated per attempt so reconnects always report
// current service and mapping counts.
type sessionSpec struct {
	cfg      config.Config
	role     protocol.Role
	token    string
	metadata func() protocol.RelayMetadata
	handler  sessionHandler
}

func runSessionLoop(ctx context.Context, spec sessionSpec, reporter telemetry.Reporter, retry retrySettings) error {
	policy := newRetryPolicy(retry)
	for {
		telemetry.Emit(reporter, telemetry.Event{
			Type:        "client_connecting",
			Level:       "info",
			State:       "connecting",
			Message:     "Connecting to relay",
			ClearListen: true,
		})
		connectedFor, err := runAttempt(ctx, spec, reporter)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			err = errSessionClosed
		}
		if permanent := permanentRejection(err); permanent != "" {
			telemetry.Emit(reporter, telemetry.Event{
				Type:        "client_rejected",
				Level:       "error",
				State:       "connecting",
				Message:     permanent,
				ClearListen: true,
			})
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

func runAttempt(ctx context.Context, spec sessionSpec, reporter telemetry.Reporter) (time.Duration, error) {
	metadata := protocol.RelayMetadata{}
	if spec.metadata != nil {
		metadata = spec.metadata()
	}
	secureConn, err := dialSecure(ctx, spec.cfg, spec.role, spec.token, metadata)
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

	err = spec.handler(ctx, secureConn, reporter)
	return time.Since(connectedAt), err
}

func dialSecure(ctx context.Context, cfg config.Config, role protocol.Role, token string, metadata protocol.RelayMetadata) (net.Conn, error) {
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
	header.Set("Authorization", "Bearer "+token)
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
	secret, channel := protocol.DeriveCredentials(token)
	secure, err := protocol.OpenSecureClientWithMetadata(
		ctx,
		ws,
		secret,
		channel,
		role,
		token,
		metadata,
	)
	if err != nil {
		ws.Close()
		return nil, fmt.Errorf("establish encrypted session: %w", err)
	}
	return secure, nil
}

// groupStateAggregator merges the runtime states of several token-group
// session loops so the console state reflects the healthiest group instead
// of whichever loop reported last.
type groupStateAggregator struct {
	mu       sync.Mutex
	reporter telemetry.Reporter
	states   map[string]string
}

func newGroupStateAggregator(reporter telemetry.Reporter) *groupStateAggregator {
	return &groupStateAggregator{reporter: reporter, states: make(map[string]string)}
}

func (a *groupStateAggregator) group(name string) telemetry.Reporter {
	return telemetry.ReporterFunc(func(event telemetry.Event) {
		if event.State != "" {
			a.mu.Lock()
			a.states[name] = event.State
			running := false
			for _, state := range a.states {
				if state == "running" {
					running = true
					break
				}
			}
			a.mu.Unlock()
			if running {
				event.State = "running"
			}
		}
		telemetry.Emit(a.reporter, event)
	})
}

func protocolRole(mode string) (protocol.Role, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case config.ModeEdge:
		return protocol.RoleEdge, nil
	case config.ModeTarget:
		return protocol.RoleTarget, nil
	default:
		return 0, errors.New("mode must be target or edge")
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

func nodeName(cfg config.Config, role protocol.Role) string {
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		name, _ = os.Hostname()
	}
	if name == "" {
		name = role.String()
	}
	return name
}

func platformLabel() string {
	return runtime.GOOS + "/" + runtime.GOARCH
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

func newInstanceID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("pid-%d", os.Getpid())
	}
	return hex.EncodeToString(buffer)
}

// halfCloseStream adapts a yamux stream to the bridge's half-close
// protocol: yamux Close sends FIN while the read side stays usable, which
// matches CloseWrite semantics on a TCP connection. Without this, a backend
// that closes first would never propagate EOF to the other end.
type halfCloseStream struct {
	*yamux.Stream
}

func (s halfCloseStream) CloseWrite() error {
	return s.Stream.Close()
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
