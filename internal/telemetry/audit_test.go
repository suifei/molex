package telemetry

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuditWriterPersistsRelayAndTokenEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "molex-audit.jsonl")
	writer := NewAuditWriter(path)

	writer.Report(Event{
		Type:    "relay_peer_kicked",
		Level:   "warning",
		Message: "Edge office-edge was disconnected by the relay administrator",
		Time:    time.Date(2026, time.August, 17, 6, 0, 0, 0, time.UTC),
		PeerChange: &PeerChange{
			Action: PeerActionRemove,
			Peers:  []Peer{{ID: "7", IP: "198.51.100.10", Role: "edge", TokenID: "tok-alpha"}},
		},
	})
	writer.Report(Event{Type: "token_rotated", Level: "warning", Message: "Token tok-alpha was rotated"})
	// Excluded: transient statistics and non-relay client noise.
	writer.Report(Event{Type: "relay_peer_stats", Transient: true})
	writer.Report(Event{Type: "relay_peer_stats"})
	writer.Report(Event{Type: "edge_mappings", Message: "Applied 2 local mapping(s)"})

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) != 2 {
		t.Fatalf("audit lines = %d, want 2: %#v", len(lines), lines)
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if first["type"] != "relay_peer_kicked" {
		t.Fatalf("first audit record = %#v", first)
	}
	if !strings.Contains(lines[0], "tok-alpha") || strings.Contains(lines[0], "mx2_") {
		t.Fatalf("audit record should carry token ids but never token values: %s", lines[0])
	}
	if !strings.Contains(lines[1], "token_rotated") {
		t.Fatalf("second audit record = %s", lines[1])
	}
}

func TestAuditWriterRotatesLargeFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "molex-audit.jsonl")
	writer := NewAuditWriter(path)
	// Pre-create an oversized file so the next report rotates it away.
	if err := os.WriteFile(path, make([]byte, auditMaxFileSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	writer.Report(Event{Type: "relay_listening", Level: "info", Message: "Relay is accepting WebSocket sessions"})

	rotated, err := os.Stat(path + ".1")
	if err != nil || rotated.Size() <= auditMaxFileSize {
		t.Fatalf("previous generation missing: %v %v", rotated, err)
	}
	current, err := os.Stat(path)
	if err != nil || current.Size() == 0 || current.Size() > 4096 {
		t.Fatalf("fresh audit file state: %v %v", current, err)
	}
}
