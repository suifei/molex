package service

import (
	"testing"
	"time"

	"github.com/suifei/molex/internal/telemetry"
)

func TestRecordEventClearsInactiveListener(t *testing.T) {
	manager := NewManager(nil, nil)
	manager.status.Listen = "127.0.0.1:2222"

	manager.recordEvent(telemetry.Event{
		Type:        "client_reconnecting",
		State:       "connecting",
		Message:     "retrying",
		ClearListen: true,
	})

	status := manager.Status()
	if status.Listen != "" {
		t.Fatalf("listen = %q during reconnect, want empty", status.Listen)
	}
	if status.State != "connecting" {
		t.Fatalf("state = %q, want connecting", status.State)
	}
}

func TestRecordEventTracksRelayPeers(t *testing.T) {
	manager := NewManager(nil, nil)
	manager.status.Message = "Relay is accepting WebSocket sessions"
	connectedAt := time.Date(2026, time.August, 11, 12, 30, 0, 0, time.UTC)

	manager.recordEvent(telemetry.Event{
		Type:    "relay_peer_connected",
		Level:   "info",
		Message: "Target connected from 203.0.113.20",
		PeerChange: &telemetry.PeerChange{
			Action: telemetry.PeerActionUpsert,
			Peers: []telemetry.Peer{{
				ID:          "2",
				IP:          "203.0.113.20",
				Role:        "target",
				Status:      telemetry.PeerStatusWaiting,
				ConnectedAt: connectedAt.Add(time.Second),
			}},
		},
	})
	manager.recordEvent(telemetry.Event{
		Type:    "relay_peer_connected",
		Level:   "info",
		Message: "Edge connected from 198.51.100.10",
		PeerChange: &telemetry.PeerChange{
			Action: telemetry.PeerActionUpsert,
			Peers: []telemetry.Peer{{
				ID:          "1",
				IP:          "198.51.100.10",
				Role:        "edge",
				Status:      telemetry.PeerStatusWaiting,
				ConnectedAt: connectedAt,
			}},
		},
	})
	manager.recordEvent(telemetry.Event{
		Type:    "relay_paired",
		Level:   "info",
		Message: "Edge and target sessions paired",
		PeerChange: &telemetry.PeerChange{
			Action: telemetry.PeerActionUpsert,
			Peers: []telemetry.Peer{
				{ID: "2", IP: "203.0.113.20", Role: "target", Status: telemetry.PeerStatusPaired, ConnectedAt: connectedAt.Add(time.Second)},
				{ID: "1", IP: "198.51.100.10", Role: "edge", Status: telemetry.PeerStatusPaired, ConnectedAt: connectedAt},
			},
		},
	})

	status := manager.Status()
	if len(status.Peers) != 2 {
		t.Fatalf("peers = %#v, want two", status.Peers)
	}
	if status.Peers[0].ID != "1" || status.Peers[1].ID != "2" {
		t.Fatalf("peers are not sorted by connection time: %#v", status.Peers)
	}
	if status.Peers[0].Status != telemetry.PeerStatusPaired || status.Peers[1].Status != telemetry.PeerStatusPaired {
		t.Fatalf("paired state was not applied: %#v", status.Peers)
	}
	if status.Message != "Relay is accepting WebSocket sessions" {
		t.Fatalf("peer activity replaced runtime summary: %q", status.Message)
	}

	status.Peers[0].IP = "mutated"
	if manager.Status().Peers[0].IP != "198.51.100.10" {
		t.Fatal("Status returned mutable manager peer state")
	}

	manager.recordEvent(telemetry.Event{
		Type:  "relay_peer_disconnected",
		Level: "info",
		PeerChange: &telemetry.PeerChange{
			Action: telemetry.PeerActionRemove,
			Peers:  []telemetry.Peer{{ID: "1"}},
		},
	})
	remaining := manager.Status().Peers
	if len(remaining) != 1 || remaining[0].ID != "2" {
		t.Fatalf("peer removal = %#v, want only target", remaining)
	}
}

func TestStaleRuntimePeerEventsAreIgnoredAfterRestart(t *testing.T) {
	manager := NewManager(nil, nil)
	manager.generation = 2

	manager.recordRuntimeEvent(1, telemetry.Event{
		Type:  "relay_peer_connected",
		Level: "info",
		PeerChange: &telemetry.PeerChange{
			Action: telemetry.PeerActionUpsert,
			Peers: []telemetry.Peer{{
				ID:          "1",
				IP:          "198.51.100.10",
				Role:        "edge",
				Status:      telemetry.PeerStatusPaired,
				ConnectedAt: time.Now().UTC(),
			}},
		},
	})

	if peers := manager.Status().Peers; len(peers) != 0 {
		t.Fatalf("stale runtime event restored peers: %#v", peers)
	}
	if events := manager.Events(); len(events) != 0 {
		t.Fatalf("stale runtime event reached activity log: %#v", events)
	}
}

func TestEventPayloadsReplaceRoleStatus(t *testing.T) {
	manager := NewManager(nil, nil)

	manager.recordEvent(telemetry.Event{
		Type: "edge_catalog",
		Catalog: &telemetry.CatalogUpdate{
			Online:   true,
			Services: []telemetry.CatalogService{{ID: "svc-1", Name: "web", Address: "10.0.0.5:80"}},
		},
		Mappings: []telemetry.MappingStatus{{
			Service: "svc-1",
			State:   telemetry.MappingStateListening,
			Listen:  "127.0.0.1:28080",
		}},
	})
	status := manager.Status()
	if status.Catalog == nil || !status.Catalog.Online || len(status.Catalog.Services) != 1 {
		t.Fatalf("catalog status = %#v", status.Catalog)
	}
	if len(status.Mappings) != 1 || status.Mappings[0].State != telemetry.MappingStateListening {
		t.Fatalf("mapping status = %#v", status.Mappings)
	}
	status.Catalog.Services[0].Name = "mutated"
	status.Mappings[0].State = "mutated"
	fresh := manager.Status()
	if fresh.Catalog.Services[0].Name != "web" || fresh.Mappings[0].State != telemetry.MappingStateListening {
		t.Fatal("Status returned mutable catalog or mapping state")
	}

	manager.recordEvent(telemetry.Event{
		Type:      "target_services",
		Transient: true,
		Services: []telemetry.ServiceStatus{{
			ID: "svc-1", Name: "web", Address: "10.0.0.5:80", Streams: 3,
		}},
	})
	status = manager.Status()
	if len(status.Services) != 1 || status.Services[0].Streams != 3 {
		t.Fatalf("service status = %#v", status.Services)
	}
	if events := manager.Events(); len(events) != 1 {
		t.Fatalf("transient service stats polluted the activity log: %d events", len(events))
	}
}

func TestLateTransientPeerStatsCannotResurrectDisconnectedPeer(t *testing.T) {
	manager := NewManager(nil, nil)
	peer := telemetry.Peer{
		ID:          "connection-1",
		IP:          "198.51.100.10",
		Role:        "edge",
		Status:      telemetry.PeerStatusPaired,
		ConnectedAt: time.Now().UTC(),
	}
	manager.recordEvent(telemetry.Event{
		Type: "relay_peer_connected",
		PeerChange: &telemetry.PeerChange{
			Action: telemetry.PeerActionUpsert,
			Peers:  []telemetry.Peer{peer},
		},
	})
	manager.recordEvent(telemetry.Event{
		Type: "relay_peer_disconnected",
		PeerChange: &telemetry.PeerChange{
			Action: telemetry.PeerActionRemove,
			Peers:  []telemetry.Peer{{ID: peer.ID}},
		},
	})
	peer.BytesReceived = 4096
	manager.recordEvent(telemetry.Event{
		Type:      "relay_peer_stats",
		Transient: true,
		PeerChange: &telemetry.PeerChange{
			Action: telemetry.PeerActionUpdate,
			Peers:  []telemetry.Peer{peer},
		},
	})

	if peers := manager.Status().Peers; len(peers) != 0 {
		t.Fatalf("late stats resurrected disconnected peer: %#v", peers)
	}
	if events := manager.Events(); len(events) != 2 {
		t.Fatalf("transient stats polluted activity history: %#v", events)
	}
}
