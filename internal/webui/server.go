package webui

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/suifei/molex/internal/config"
	"github.com/suifei/molex/internal/service"
	"github.com/suifei/molex/internal/telemetry"
)

const (
	defaultListen     = "127.0.0.1:9090"
	defaultConfigPath = "molex.json"
	defaultSessionTTL = 12 * time.Hour
)

//go:embed all:dist
var embeddedAssets embed.FS

type Options struct {
	Listen            string
	AutoListen        bool
	ConfigPath        string
	Password          string
	SetupPasswordPath string
	AutoStart         bool
	SessionTTL        time.Duration
	Logger            *slog.Logger
	Now               func() time.Time
	OnReady           func(string)
}

type Server struct {
	options      Options
	manager      *service.Manager
	handler      http.Handler
	assets       fs.FS
	authMu       sync.RWMutex
	passwordHash [32]byte
	setupPending bool
	sessions     *sessionStore
	loginLimiter *loginLimiter
	actionMu     sync.Mutex
	subscriberMu sync.RWMutex
	subscribers  map[chan telemetry.Event]struct{}
}

func New(options Options) (*Server, error) {
	if options.Listen == "" {
		options.Listen = defaultListen
	}
	if options.ConfigPath == "" {
		options.ConfigPath = defaultConfigPath
	}
	if options.SessionTTL <= 0 {
		options.SessionTTL = defaultSessionTTL
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if err := validateWebListen(options.Listen); err != nil {
		return nil, err
	}
	setupPending := options.Password == ""
	if setupPending {
		if options.SetupPasswordPath == "" {
			return nil, errors.New("web password is required unless first-run setup is enabled")
		}
	} else {
		if len(options.Password) < 12 {
			return nil, errors.New("web password must contain at least 12 characters")
		}
		if len(options.Password) > 1024 {
			return nil, errors.New("web password must contain at most 1024 characters")
		}
	}

	assets, err := fs.Sub(embeddedAssets, "dist")
	if err != nil {
		return nil, fmt.Errorf("open embedded web assets: %w", err)
	}

	server := &Server{
		options:      options,
		assets:       assets,
		passwordHash: hashToken(options.Password),
		setupPending: setupPending,
		sessions:     newSessionStore(options.Now),
		loginLimiter: newLoginLimiter(options.Now),
		subscribers:  make(map[chan telemetry.Event]struct{}),
	}
	server.manager = service.NewManager(telemetry.ReporterFunc(server.publish), options.Logger)
	server.handler = server.securityHeaders(server.routes())
	return server, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) Run(ctx context.Context) (runErr error) {
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.manager.Stop(stopCtx); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}()

	if s.options.AutoStart {
		cfg, err := config.LoadOptional(s.options.ConfigPath)
		if err != nil {
			return err
		}
		if err := s.manager.Start(cfg); err != nil {
			return fmt.Errorf("autostart runtime: %w", err)
		}
	}

	listener, err := listenWebConsole(s.options.Listen, s.options.AutoListen)
	if err != nil {
		return fmt.Errorf("listen for web console: %w", err)
	}
	httpServer := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	s.options.Logger.Info("Web console is ready", "listen", listener.Addr().String())
	if s.options.OnReady != nil {
		host := listener.Addr().String()
		if tcpAddress, ok := listener.Addr().(*net.TCPAddr); ok {
			host = net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", tcpAddress.Port))
		}
		s.options.OnReady("http://" + host + "/")
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.Serve(listener)
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve web console: %w", err)
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down web console: %w", err)
		}
		return nil
	}
}

func listenWebConsole(address string, auto bool) (net.Listener, error) {
	if !auto {
		return net.Listen("tcp", address)
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, err
	}
	if port == 0 {
		return net.Listen("tcp", address)
	}
	const attempts = 100
	var lastErr error
	for candidate := port; candidate < port+attempts && candidate <= 65535; candidate++ {
		candidateAddress := net.JoinHostPort(host, strconv.Itoa(candidate))
		listener, err := net.Listen("tcp", candidateAddress)
		if err == nil {
			return listener, nil
		}
		// AutoListen is only enabled for the validated loopback default. Try
		// the next candidate without connecting to the process that owns the
		// current port; probing it could trigger an unrelated local service.
		lastErr = err
	}
	return nil, fmt.Errorf("no available Web console port from %d to %d: %w", port, min(port+attempts-1, 65535), lastErr)
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/api/session", s.handleSession)
	mux.HandleFunc("/api/setup", s.handleSetup)
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.requireSession(s.handleLogout))
	mux.HandleFunc("/api/config", s.requireSession(s.handleConfig))
	mux.HandleFunc("/api/config/validate", s.requireSession(s.handleValidateConfig))
	mux.HandleFunc("/api/runtime/start", s.requireSession(s.handleStart))
	mux.HandleFunc("/api/runtime/stop", s.requireSession(s.handleStop))
	mux.HandleFunc("/api/runtime/status", s.requireSession(s.handleStatus))
	mux.HandleFunc("/api/events", s.requireSession(s.handleEvents))
	mux.HandleFunc("/api/events/stream", s.requireSession(s.handleEventStream))
	mux.HandleFunc("/api/secret", s.requireSession(s.handleGenerateSecret))
	mux.HandleFunc("/", s.handleStatic)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet, http.MethodHead)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write([]byte("ok\n"))
	}
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet, http.MethodHead)
		return
	}
	cleanPath := path.Clean("/" + r.URL.Path)
	if strings.Contains(cleanPath, "/.") {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(cleanPath, "/")
	if name == "" {
		name = "index.html"
	}
	info, err := fs.Stat(s.assets, name)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-store")
	} else if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.FileServer(http.FS(s.assets)).ServeHTTP(w, r)
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; connect-src 'self'; font-src 'self'; form-action 'self'; frame-ancestors 'none'; img-src 'self' data:; object-src 'none'; script-src 'self'; style-src 'self'")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		if requestIsSecure(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func validateWebListen(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errors.New("web listen must use loopback host:port form")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("web console must listen on loopback; use an HTTPS reverse proxy or SSH tunnel for remote access")
	}
	return nil
}

func (s *Server) publish(event telemetry.Event) {
	s.subscriberMu.RLock()
	defer s.subscriberMu.RUnlock()
	for subscriber := range s.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}

func (s *Server) subscribe() (<-chan telemetry.Event, func()) {
	channel := make(chan telemetry.Event, 16)
	s.subscriberMu.Lock()
	s.subscribers[channel] = struct{}{}
	s.subscriberMu.Unlock()
	return channel, func() {
		s.subscriberMu.Lock()
		delete(s.subscribers, channel)
		s.subscriberMu.Unlock()
	}
}
