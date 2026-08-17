package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	auditMaxFileSize = 5 << 20
	auditFileMode    = 0o600
)

// auditRecord is one durable line in the relay audit log. It deliberately
// carries operator-relevant facts only: no token values, passwords, or
// payload data ever appear here.
type auditRecord struct {
	Time    time.Time   `json:"time"`
	Type    string      `json:"type"`
	Level   string      `json:"level"`
	Message string      `json:"message,omitempty"`
	Peers   []auditPeer `json:"peers,omitempty"`
}

type auditPeer struct {
	ID      string `json:"id"`
	IP      string `json:"ip,omitempty"`
	Name    string `json:"name,omitempty"`
	Role    string `json:"role,omitempty"`
	TokenID string `json:"tokenId,omitempty"`
	Status  string `json:"status,omitempty"`
}

// AuditWriter appends relay lifecycle events as JSON lines and rotates the
// file once it exceeds five megabytes (one previous generation is kept as
// <path>.1). Failures are swallowed: auditing must never break relaying.
type AuditWriter struct {
	mu   sync.Mutex
	path string
}

func NewAuditWriter(path string) *AuditWriter {
	return &AuditWriter{path: path}
}

func (w *AuditWriter) Path() string {
	return w.path
}

// auditableEvent selects durable, operator-actionable events: connection
// lifecycle, pairing, rejections, kicks, revocations, token management, and
// runtime errors. High-frequency transient statistics are excluded.
func auditableEvent(event Event) bool {
	if event.Transient {
		return false
	}
	switch {
	case event.Type == "relay_peer_stats":
		return false
	case strings.HasPrefix(event.Type, "relay_"),
		strings.HasPrefix(event.Type, "token_"),
		event.Type == "runtime_error":
		return true
	default:
		return false
	}
}

func (w *AuditWriter) Report(event Event) {
	if w == nil || !auditableEvent(event) {
		return
	}
	record := auditRecord{
		Time:    event.Time,
		Type:    event.Type,
		Level:   event.Level,
		Message: event.Message,
	}
	if record.Time.IsZero() {
		record.Time = time.Now().UTC()
	}
	if event.PeerChange != nil {
		for _, peer := range event.PeerChange.Peers {
			record.Peers = append(record.Peers, auditPeer{
				ID:      peer.ID,
				IP:      peer.IP,
				Name:    peer.Name,
				Role:    peer.Role,
				TokenID: peer.TokenID,
				Status:  peer.Status,
			})
		}
	}
	line, err := json.Marshal(record)
	if err != nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.rotateLocked()
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, auditFileMode)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(line, '\n'))
}

func (w *AuditWriter) rotateLocked() {
	info, err := os.Stat(w.path)
	if err != nil || info.Size() < auditMaxFileSize {
		return
	}
	previous := w.path + ".1"
	_ = os.Remove(previous)
	_ = os.Rename(w.path, previous)
}

// MultiReporter fans one event out to several reporters.
func MultiReporter(reporters ...Reporter) Reporter {
	return ReporterFunc(func(event Event) {
		for _, reporter := range reporters {
			if reporter != nil {
				reporter.Report(event)
			}
		}
	})
}

// DefaultAuditPath places the audit log beside the configuration file.
func DefaultAuditPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "molex-audit.jsonl")
}
