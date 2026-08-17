package config

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
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
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	ModeRelay  = "relay"
	ModeTarget = "target"
	ModeEdge   = "edge"

	DefaultWebSocketPath = "/ws/session"

	MinTokenLength = 16
	MaxServices    = 256
	MaxMappings    = 256
	MaxTokens      = 256
)

// Config keeps one small top-level surface for all three v2 roles. Every
// role reads only its own fields: relay uses listen and tokens, target uses
// remote, token, name, and services, edge uses remote, token, name, and
// mappings.
type Config struct {
	Mode     string         `json:"mode"`
	Listen   string         `json:"listen,omitempty"`
	Remote   string         `json:"remote,omitempty"`
	Token    string         `json:"token,omitempty"`
	Name     string         `json:"name,omitempty"`
	Tokens   []TokenEntry   `json:"tokens,omitempty"`
	Services []ServiceEntry `json:"services,omitempty"`
	Mappings []MappingEntry `json:"mappings,omitempty"`
}

// TokenEntry is one access-token record. On a relay, ID is the generated
// token id and the rotation fields keep the previous value valid through a
// grace window. On a target or edge, the same structure lists group
// memberships: ID is the local group name chosen by the operator and only
// ID plus Token are used. The token value doubles as the end-to-end key
// source for its group, so treat it like an SSH private key.
type TokenEntry struct {
	ID                string    `json:"id"`
	Token             string    `json:"token"`
	Note              string    `json:"note,omitempty"`
	Disabled          bool      `json:"disabled,omitempty"`
	CreatedAt         time.Time `json:"createdAt,omitempty"`
	PreviousToken     string    `json:"previousToken,omitempty"`
	PreviousExpiresAt time.Time `json:"previousExpiresAt,omitempty"`
}

// ServiceEntry is one forwardable backend address published by a target.
// Groups restricts which token groups may see and dial it; an empty list
// publishes the service to every group the target joined.
type ServiceEntry struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Address string   `json:"address"`
	Groups  []string `json:"groups,omitempty"`
}

// MappingEntry maps one published service to a local edge listener. Group
// selects which token group the service belongs to (optional while the
// edge only joined one group). LAN controls whether the listener binds all
// interfaces instead of loopback.
type MappingEntry struct {
	Service string `json:"service"`
	Group   string `json:"group,omitempty"`
	Port    int    `json:"port"`
	LAN     bool   `json:"lan,omitempty"`
}

// legacyFields marks the v1 (punch/secret/tunnel) layout so upgrades fail
// with migration guidance instead of an opaque unknown-field error.
var legacyFields = []string{"role", "secret", "tunnel"}

// ErrLegacyConfig is wrapped by Load when a v1 configuration file is found.
var ErrLegacyConfig = errors.New("legacy v1 configuration")

func LegacyConfigError() error {
	return fmt.Errorf("%w: this file uses the MoleX v1 layout (mode \"punch\" with role, secret, and tunnel). MoleX v2 uses mode \"relay\", \"target\", or \"edge\" with tokens, services, and mappings. Recreate it with `molex config init --mode <relay|target|edge>` and see the v2 migration notes in the README", ErrLegacyConfig)
}

func Default() Config {
	return Config{
		Mode:   ModeEdge,
		Remote: "wss://molex.example.com" + DefaultWebSocketPath,
	}
}

func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	raw, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	// Windows editors and PowerShell often prepend a UTF-8 BOM.
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	if err := detectLegacyConfig(raw); err != nil {
		return Config{}, err
	}

	var cfg Config
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Config{}, err
	}
	return cfg.Normalized(), nil
}

func detectLegacyConfig(raw []byte) error {
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil
	}
	if mode, ok := generic["mode"]; ok {
		var value string
		if err := json.Unmarshal(mode, &value); err == nil && strings.EqualFold(strings.TrimSpace(value), "punch") {
			return LegacyConfigError()
		}
	}
	for _, field := range legacyFields {
		if _, ok := generic[field]; ok {
			return LegacyConfigError()
		}
	}
	return nil
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
	c.Listen = strings.TrimSpace(c.Listen)
	c.Remote = strings.TrimSpace(c.Remote)
	c.Token = strings.TrimSpace(c.Token)
	c.Name = strings.TrimSpace(c.Name)

	for index := range c.Tokens {
		token := &c.Tokens[index]
		token.ID = strings.TrimSpace(token.ID)
		token.Token = strings.TrimSpace(token.Token)
		token.Note = strings.TrimSpace(token.Note)
		token.PreviousToken = strings.TrimSpace(token.PreviousToken)
	}
	for index := range c.Services {
		service := &c.Services[index]
		service.ID = strings.TrimSpace(service.ID)
		service.Name = strings.TrimSpace(service.Name)
		service.Address = strings.TrimSpace(service.Address)
		groups := service.Groups[:0]
		for _, group := range service.Groups {
			if trimmed := strings.TrimSpace(group); trimmed != "" {
				groups = append(groups, trimmed)
			}
		}
		service.Groups = groups
	}
	for index := range c.Mappings {
		mapping := &c.Mappings[index]
		mapping.Service = strings.TrimSpace(mapping.Service)
		mapping.Group = strings.TrimSpace(mapping.Group)
	}

	if c.Mode == "" {
		c.Mode = ModeEdge
	}
	if c.Mode == ModeRelay && c.Listen == "" {
		c.Listen = "127.0.0.1:8080"
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
		problems = append(problems, validateTokens(c.Tokens)...)
	case ModeTarget:
		problems = append(problems, validateClientCommon(c)...)
		problems = append(problems, validateServices(c.Services, c.groupNameSet())...)
	case ModeEdge:
		problems = append(problems, validateClientCommon(c)...)
		problems = append(problems, validateMappings(c.Mappings, c.groupNameSet(), len(c.GroupTokens()))...)
	default:
		problems = append(problems, "mode must be relay, target, or edge")
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

// GroupTokens returns the effective token-group memberships of a target or
// edge: either the multi-group `tokens` list or the single `token` value as
// one unnamed group.
func (c Config) GroupTokens() []TokenEntry {
	if len(c.Tokens) > 0 && c.Mode != ModeRelay {
		return c.Tokens
	}
	if c.Token != "" {
		return []TokenEntry{{ID: "", Token: c.Token}}
	}
	return nil
}

func (c Config) groupNameSet() map[string]bool {
	names := make(map[string]bool)
	for _, token := range c.GroupTokens() {
		names[token.ID] = true
	}
	return names
}

func validateClientCommon(c Config) []string {
	var problems []string
	if _, err := NormalizeRemote(c.Remote); err != nil {
		problems = append(problems, "remote: "+err.Error())
	} else if err := validateRemoteSecurity(c.Remote); err != nil {
		problems = append(problems, "remote: "+err.Error())
	}
	if err := validateNodeName(c.Name); err != nil {
		problems = append(problems, "name: "+err.Error())
	}

	if c.Token != "" && len(c.Tokens) > 0 {
		problems = append(problems, "use either the single token field or the tokens group list, not both")
		return problems
	}
	groups := c.GroupTokens()
	if len(groups) == 0 {
		problems = append(problems, fmt.Sprintf("token must contain at least %d characters", MinTokenLength))
		return problems
	}
	if len(groups) > MaxTokens {
		problems = append(problems, fmt.Sprintf("tokens: at most %d entries are supported", MaxTokens))
		return problems
	}
	seenNames := make(map[string]bool, len(groups))
	seenValues := make(map[string]bool, len(groups))
	for index, group := range groups {
		prefix := fmt.Sprintf("tokens[%d]", index)
		if len(group.Token) < MinTokenLength {
			problems = append(problems, fmt.Sprintf("%s.token must contain at least %d characters", prefix, MinTokenLength))
		} else if seenValues[group.Token] {
			problems = append(problems, prefix+".token: duplicate token value")
		}
		seenValues[group.Token] = true
		if group.ID == "" && len(groups) > 1 {
			problems = append(problems, prefix+".id: a group name is required when joining several groups")
		} else if group.ID != "" {
			if err := validateNodeName(group.ID); err != nil {
				problems = append(problems, prefix+".id: "+err.Error())
			}
		}
		if seenNames[group.ID] {
			problems = append(problems, prefix+".id: duplicate group name")
		}
		seenNames[group.ID] = true
	}
	return problems
}

func validateTokens(tokens []TokenEntry) []string {
	var problems []string
	if len(tokens) > MaxTokens {
		problems = append(problems, fmt.Sprintf("tokens: at most %d entries are supported", MaxTokens))
		return problems
	}
	seenIDs := make(map[string]bool, len(tokens))
	seenValues := make(map[string]bool, len(tokens))
	for index, token := range tokens {
		prefix := fmt.Sprintf("tokens[%d]", index)
		if token.ID == "" {
			problems = append(problems, prefix+".id is required")
		} else if seenIDs[token.ID] {
			problems = append(problems, prefix+".id: duplicate token id")
		}
		seenIDs[token.ID] = true
		if len(token.Token) < MinTokenLength {
			problems = append(problems, fmt.Sprintf("%s.token must contain at least %d characters", prefix, MinTokenLength))
		} else if seenValues[token.Token] {
			problems = append(problems, prefix+".token: duplicate token value")
		}
		seenValues[token.Token] = true
		if err := validateNodeName(token.Note); err != nil {
			problems = append(problems, prefix+".note: "+err.Error())
		}
		if token.PreviousToken != "" {
			if len(token.PreviousToken) < MinTokenLength {
				problems = append(problems, fmt.Sprintf("%s.previousToken must contain at least %d characters", prefix, MinTokenLength))
			}
			if token.PreviousToken == token.Token {
				problems = append(problems, prefix+".previousToken must differ from the current token")
			}
			if token.PreviousExpiresAt.IsZero() {
				problems = append(problems, prefix+".previousExpiresAt is required while a previous token is kept")
			}
		}
	}
	return problems
}

func validateServices(services []ServiceEntry, groupNames map[string]bool) []string {
	var problems []string
	if len(services) > MaxServices {
		problems = append(problems, fmt.Sprintf("services: at most %d entries are supported", MaxServices))
		return problems
	}
	seenIDs := make(map[string]bool, len(services))
	seenNames := make(map[string]bool, len(services))
	for index, service := range services {
		prefix := fmt.Sprintf("services[%d]", index)
		if service.ID == "" {
			problems = append(problems, prefix+".id is required")
		} else if seenIDs[service.ID] {
			problems = append(problems, prefix+".id: duplicate service id")
		}
		seenIDs[service.ID] = true
		if service.Name == "" {
			problems = append(problems, prefix+".name is required")
		} else if err := validateNodeName(service.Name); err != nil {
			problems = append(problems, prefix+".name: "+err.Error())
		} else if seenNames[service.Name] {
			problems = append(problems, prefix+".name: duplicate service name")
		}
		seenNames[service.Name] = true
		if err := validateDialAddress(service.Address); err != nil {
			problems = append(problems, prefix+".address: "+err.Error())
		}
		for _, group := range service.Groups {
			if !groupNames[group] {
				problems = append(problems, fmt.Sprintf("%s.groups: unknown group name %q", prefix, group))
			}
		}
	}
	return problems
}

func validateMappings(mappings []MappingEntry, groupNames map[string]bool, groupCount int) []string {
	var problems []string
	if len(mappings) > MaxMappings {
		problems = append(problems, fmt.Sprintf("mappings: at most %d entries are supported", MaxMappings))
		return problems
	}
	seenServices := make(map[string]bool, len(mappings))
	seenPorts := make(map[int]bool, len(mappings))
	for index, mapping := range mappings {
		prefix := fmt.Sprintf("mappings[%d]", index)
		key := mapping.Group + "\x00" + mapping.Service
		if mapping.Service == "" {
			problems = append(problems, prefix+".service is required")
		} else if seenServices[key] {
			problems = append(problems, prefix+".service: duplicate mapping for one service")
		}
		seenServices[key] = true
		if mapping.Group == "" {
			if groupCount > 1 {
				problems = append(problems, prefix+".group is required when the edge joined several groups")
			}
		} else if !groupNames[mapping.Group] {
			problems = append(problems, fmt.Sprintf("%s.group: unknown group name %q", prefix, mapping.Group))
		}
		if mapping.Port < 1 || mapping.Port > 65535 {
			problems = append(problems, prefix+".port must be between 1 and 65535")
		} else if seenPorts[mapping.Port] {
			problems = append(problems, prefix+".port: duplicate local port")
		}
		seenPorts[mapping.Port] = true
	}
	return problems
}

// VisibleTo reports whether one published service is visible to a group.
func (s ServiceEntry) VisibleTo(group string) bool {
	if len(s.Groups) == 0 {
		return true
	}
	for _, allowed := range s.Groups {
		if allowed == group {
			return true
		}
	}
	return false
}

// ListenAddress returns the local listener address for one edge mapping.
func (m MappingEntry) ListenAddress() string {
	host := "127.0.0.1"
	if m.LAN {
		host = "0.0.0.0"
	}
	return net.JoinHostPort(host, strconv.Itoa(m.Port))
}

// FindToken matches a bearer token value against the configured entries.
func (c Config) FindToken(value string) (TokenEntry, bool) {
	for _, token := range c.Tokens {
		if token.Token == value {
			return token, true
		}
	}
	return TokenEntry{}, false
}

// FindService returns the service entry with the given id.
func (c Config) FindService(id string) (ServiceEntry, bool) {
	for _, service := range c.Services {
		if service.ID == id {
			return service, true
		}
	}
	return ServiceEntry{}, false
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

// GenerateToken creates a relay access token with the v2 prefix.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return "mx2_" + base64.RawURLEncoding.EncodeToString(b), nil
}

// GenerateID creates a short random identifier for tokens and services.
func GenerateID(prefix string) (string, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return prefix + "-" + hex.EncodeToString(b), nil
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

// validateDialAddress requires a concrete host and a non-zero port because
// the target dials this address for every stream.
func validateDialAddress(address string) error {
	if address == "" {
		return errors.New("address is required")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || strings.TrimSpace(host) == "" {
		return errors.New("must use host:port form")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return errors.New("port must be between 1 and 65535")
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
