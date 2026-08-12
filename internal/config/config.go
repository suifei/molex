package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	ModeRelay = "relay"
	ModePunch = "punch"

	RoleEdge   = "edge"
	RoleTarget = "target"

	DefaultWebSocketPath = "/ws/session"
	DefaultTargetPool    = 1
	MaxTargetPool        = 64
)

// Config deliberately keeps no more than seven top-level fields.
type Config struct {
	Mode   string       `json:"mode"`
	Role   string       `json:"role,omitempty"`
	Secret string       `json:"secret,omitempty"`
	Token  string       `json:"token,omitempty"`
	Listen string       `json:"listen,omitempty"`
	Remote string       `json:"remote,omitempty"`
	Tunnel TunnelConfig `json:"tunnel"`
}

type TunnelConfig struct {
	Local  string       `json:"local,omitempty"`
	Remote string       `json:"remote,omitempty"`
	Name   string       `json:"name,omitempty"`
	Pool   int          `json:"pool,omitempty"`
	Rules  []TunnelRule `json:"rules,omitempty"`
}

type TunnelRule struct {
	Name   string `json:"name,omitempty"`
	Listen string `json:"listen,omitempty"`
	Local  string `json:"local,omitempty"`
	Remote string `json:"remote"`
	Pool   int    `json:"pool,omitempty"`
}

func Default() Config {
	return Config{
		Mode:   ModePunch,
		Role:   RoleEdge,
		Listen: "127.0.0.1:2222",
		Remote: "wss://molex.example.com" + DefaultWebSocketPath,
		Tunnel: TunnelConfig{
			Local:  "127.0.0.1:22",
			Remote: "home-ssh",
		},
	}
}

func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	var cfg Config
	decoder := json.NewDecoder(io.LimitReader(f, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Config{}, err
	}
	return cfg.Normalized(), nil
}

func LoadOptional(path string) (Config, error) {
	cfg, err := Load(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	return cfg, err
}

func Save(path string, cfg Config) error {
	cfg = cfg.Normalized()
	if err := cfg.Validate(); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".molex-*.json")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("secure temporary config: %w", err)
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cfg); err != nil {
		tmp.Close()
		return fmt.Errorf("encode config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("flush config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

func (c Config) Normalized() Config {
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	c.Role = strings.ToLower(strings.TrimSpace(c.Role))
	c.Secret = strings.TrimSpace(c.Secret)
	c.Token = strings.TrimSpace(c.Token)
	c.Listen = strings.TrimSpace(c.Listen)
	c.Remote = strings.TrimSpace(c.Remote)
	c.Tunnel.Local = strings.TrimSpace(c.Tunnel.Local)
	c.Tunnel.Remote = strings.TrimSpace(c.Tunnel.Remote)
	c.Tunnel.Name = strings.TrimSpace(c.Tunnel.Name)
	for index := range c.Tunnel.Rules {
		rule := &c.Tunnel.Rules[index]
		rule.Name = strings.TrimSpace(rule.Name)
		rule.Listen = strings.TrimSpace(rule.Listen)
		rule.Local = strings.TrimSpace(rule.Local)
		rule.Remote = strings.TrimSpace(rule.Remote)
		if c.Role == RoleTarget && rule.Pool == 0 {
			rule.Pool = DefaultTargetPool
		}
	}

	if c.Mode == "" {
		c.Mode = ModePunch
	}
	if c.Mode == ModeRelay && c.Listen == "" {
		c.Listen = "127.0.0.1:8080"
	}
	if c.Mode == ModePunch && c.Role == RoleEdge && c.Listen == "" {
		c.Listen = "127.0.0.1:2222"
	}
	if c.Mode == ModePunch && c.Role == RoleTarget && c.Tunnel.Pool == 0 {
		c.Tunnel.Pool = DefaultTargetPool
	}
	if c.Remote != "" {
		if normalized, err := NormalizeRemote(c.Remote); err == nil {
			c.Remote = normalized
		}
	}
	return c
}

func (c Config) Validate() error {
	c = c.Normalized()
	var problems []string

	switch c.Mode {
	case ModeRelay:
		if err := validateAddress(c.Listen); err != nil {
			problems = append(problems, "listen: "+err.Error())
		}
	case ModePunch:
		if c.Role != RoleEdge && c.Role != RoleTarget {
			problems = append(problems, "role must be edge or target")
		}
		if len(c.Secret) < 16 {
			problems = append(problems, "secret must contain at least 16 characters")
		}
		if _, err := NormalizeRemote(c.Remote); err != nil {
			problems = append(problems, "remote: "+err.Error())
		} else if err := validateRemoteSecurity(c.Remote); err != nil {
			problems = append(problems, "remote: "+err.Error())
		}
		if len(c.Tunnel.Rules) == 0 {
			problems = append(problems, validateTunnelRoute(c.Role, c.Listen, c.Tunnel.Local, c.Tunnel.Remote, c.Tunnel.Name, c.Tunnel.Pool, "tunnel")...)
		} else {
			seenEdgeListeners := make(map[string]bool)
			for index, rule := range c.Tunnel.Rules {
				prefix := fmt.Sprintf("tunnel.rules[%d]", index)
				problems = append(problems, validateTunnelRoute(c.Role, rule.Listen, rule.Local, rule.Remote, rule.Name, rule.Pool, prefix)...)
				if c.Role == RoleEdge && rule.Listen != "" {
					if seenEdgeListeners[rule.Listen] {
						problems = append(problems, prefix+".listen: duplicate Edge listen address")
					}
					seenEdgeListeners[rule.Listen] = true
				}
			}
		}
	default:
		problems = append(problems, "mode must be relay or punch")
	}

	if c.Token != "" && len(c.Token) < 16 {
		problems = append(problems, "token must contain at least 16 characters when set")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func validateTunnelRoute(role, listen, local, remote, name string, pool int, prefix string) []string {
	var problems []string
	if remote == "" {
		problems = append(problems, prefix+".remote channel is required")
	} else if len(remote) > 128 {
		problems = append(problems, prefix+".remote channel must be at most 128 characters")
	}
	if err := validateNodeName(name); err != nil {
		problems = append(problems, prefix+".name: "+err.Error())
	}
	if role == RoleEdge {
		if err := validateAddress(listen); err != nil {
			problems = append(problems, prefix+".listen: "+err.Error())
		}
	}
	if role == RoleTarget {
		if err := validateAddress(local); err != nil {
			problems = append(problems, prefix+".local: "+err.Error())
		}
		if pool < 1 || pool > MaxTargetPool {
			problems = append(problems, fmt.Sprintf("%s.pool must be between 1 and %d", prefix, MaxTargetPool))
		}
	}
	return problems
}

func (c Config) ClientRoutes() []Config {
	c = c.Normalized()
	if c.Mode != ModePunch || len(c.Tunnel.Rules) == 0 {
		c.Tunnel.Rules = nil
		return []Config{c}
	}
	routes := make([]Config, 0, len(c.Tunnel.Rules))
	for _, rule := range c.Tunnel.Rules {
		route := c
		route.Tunnel = TunnelConfig{Local: rule.Local, Remote: rule.Remote, Name: rule.Name, Pool: rule.Pool}
		if c.Role == RoleEdge {
			route.Listen = rule.Listen
		}
		routes = append(routes, route)
	}
	return routes
}

func NormalizeRemote(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("WebSocket endpoint is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "wss://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("must be a valid ws:// or wss:// URL")
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return "", errors.New("scheme must be ws or wss")
	}
	if u.Host == "" || u.User != nil || u.Fragment != "" {
		return "", errors.New("must contain a host and no credentials or fragment")
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = DefaultWebSocketPath
	}
	return u.String(), nil
}

func GenerateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return "mx1_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode config: only one JSON object is allowed")
		}
		return fmt.Errorf("decode config: %w", err)
	}
	return nil
}

func validateAddress(address string) error {
	if address == "" {
		return errors.New("address is required")
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return errors.New("must use host:port form")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 0 || n > 65535 {
		return errors.New("port must be between 0 and 65535")
	}
	return nil
}

func validateNodeName(name string) error {
	if name == "" {
		return nil
	}
	if !utf8.ValidString(name) {
		return errors.New("must be valid UTF-8")
	}
	if len([]byte(name)) > 64 {
		return errors.New("must be at most 64 bytes")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return errors.New("must not contain control characters")
		}
	}
	return nil
}

func validateRemoteSecurity(raw string) error {
	normalized, err := NormalizeRemote(raw)
	if err != nil {
		return err
	}
	u, err := url.Parse(normalized)
	if err != nil || u.Scheme != "ws" {
		return nil
	}
	host := strings.TrimSpace(strings.ToLower(u.Hostname()))
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return errors.New("unencrypted ws is allowed only on loopback; use wss for remote relays")
}
