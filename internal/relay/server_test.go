package relay

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/suifei/molex/internal/protocol"
	"github.com/suifei/molex/internal/telemetry"
)

func TestRelayRequiresConfiguredToken(t *testing.T) {
	server := New(Options{Token: "0123456789abcdef"})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	_, response, err := websocket.DefaultDialer.Dial(url, nil)
	if err == nil {
		t.Fatal("relay accepted a request without its token")
	}
	if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %#v", response)
	}

	header := make(http.Header)
	header.Set("Authorization", "Bearer 0123456789abcdef")
	conn, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		t.Fatalf("relay rejected valid token: %v", err)
	}
	conn.Close()
}

func TestSecurityHeaders(t *testing.T) {
	server := New(Options{})
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unexpected health response: %d %#v", response.Code, response.Header())
	}
}

func TestPairedSessionOutlivesPairTimeout(t *testing.T) {
	server := New(Options{PairTimeout: 80 * time.Millisecond})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	edgeConn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer edgeConn.Close()
	targetConn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer targetConn.Close()

	secret := []byte("pair-timeout-regression-secret")
	edgeHello, _ := protocol.NewHello(secret, "channel", protocol.RoleEdge)
	targetHello, _ := protocol.NewHello(secret, "channel", protocol.RoleTarget)
	if err := edgeConn.WriteMessage(websocket.BinaryMessage, edgeHello.Packet[:]); err != nil {
		t.Fatal(err)
	}
	if err := targetConn.WriteMessage(websocket.BinaryMessage, targetHello.Packet[:]); err != nil {
		t.Fatal(err)
	}
	if _, _, err := edgeConn.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := targetConn.ReadMessage(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * 80 * time.Millisecond)
	payload := []byte("session-remains-open")
	if err := edgeConn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatalf("write after pair timeout: %v", err)
	}
	_ = targetConn.SetReadDeadline(time.Now().Add(time.Second))
	_, received, err := targetConn.ReadMessage()
	if err != nil {
		t.Fatalf("read after pair timeout: %v", err)
	}
	if string(received) != string(payload) {
		t.Fatalf("unexpected relayed payload %q", received)
	}
}

func TestClientIPTrustsForwardedHeadersOnlyFromLoopback(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    http.Header
		want       string
		proxied    bool
	}{
		{
			name:       "direct IPv4 ignores spoofed proxy headers",
			remoteAddr: "198.51.100.7:4567",
			headers: http.Header{
				"X-Forwarded-For": []string{"203.0.113.99"},
				"X-Real-Ip":       []string{"203.0.113.98"},
			},
			want: "198.51.100.7",
		},
		{
			name:       "loopback proxy uses forwarded IPv4",
			remoteAddr: "127.0.0.1:4567",
			headers:    http.Header{"X-Forwarded-For": []string{"203.0.113.10, 127.0.0.1"}},
			want:       "203.0.113.10",
			proxied:    true,
		},
		{
			name:       "loopback proxy falls back to real IPv6",
			remoteAddr: "[::1]:4567",
			headers:    http.Header{"X-Real-Ip": []string{"2001:db8::20"}},
			want:       "2001:db8::20",
			proxied:    true,
		},
		{
			name:       "direct IPv6 is normalized",
			remoteAddr: "[2001:0db8:0:0::8]:4567",
			want:       "2001:db8::8",
		},
		{
			name:       "IPv4 mapped address is unmapped",
			remoteAddr: "[::ffff:192.0.2.44]:4567",
			want:       "192.0.2.44",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/ws/session", nil)
			request.RemoteAddr = tt.remoteAddr
			request.Header = tt.headers
			if got := clientIP(request); got != tt.want {
				t.Fatalf("clientIP() = %q, want %q", got, tt.want)
			}
			if _, proxied := clientConnection(request); proxied != tt.proxied {
				t.Fatalf("clientConnection() proxied = %v, want %v", proxied, tt.proxied)
			}
		})
	}
}

func TestMetadataPingsRemainCompatibleWithLegacyHelloReader(t *testing.T) {
	received := make(chan []byte, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, packet, err := conn.ReadMessage()
		if err == nil {
			received <- packet
		}
	}))
	defer httpServer.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	hello, err := protocol.NewHello([]byte("legacy-compatible-secret-123456"), "channel", protocol.RoleEdge)
	if err != nil {
		t.Fatal(err)
	}
	writeRelayMetadata(t, conn, hello, protocol.RelayMetadata{Name: "new-client", Endpoint: "127.0.0.1:2222"})
	if err := conn.WriteMessage(websocket.BinaryMessage, hello.Packet[:]); err != nil {
		t.Fatal(err)
	}
	select {
	case packet := <-received:
		if len(packet) != protocol.HelloSize {
			t.Fatalf("legacy reader received %d-byte data frame", len(packet))
		}
		if _, err := protocol.ParseHello(packet); err != nil {
			t.Fatalf("legacy reader received invalid hello: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("legacy reader did not receive the hello after metadata pings")
	}
}

func TestRelayReportsPeerLifecycle(t *testing.T) {
	events := make(chan telemetry.Event, 16)
	server := New(Options{
		PairTimeout: time.Second,
		Reporter: telemetry.ReporterFunc(func(event telemetry.Event) {
			events <- event
		}),
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	edgeHeader := http.Header{"X-Real-Ip": []string{"198.51.100.10"}}
	edgeConn, _, err := websocket.DefaultDialer.Dial(url, edgeHeader)
	if err != nil {
		t.Fatal(err)
	}
	defer edgeConn.Close()
	targetHeader := http.Header{"X-Forwarded-For": []string{"203.0.113.20"}}
	targetConn, _, err := websocket.DefaultDialer.Dial(url, targetHeader)
	if err != nil {
		t.Fatal(err)
	}
	defer targetConn.Close()

	secret := []byte("relay-peer-lifecycle-secret")
	edgeHello, _ := protocol.NewHello(secret, "peer-list", protocol.RoleEdge)
	targetHello, _ := protocol.NewHello(secret, "peer-list", protocol.RoleTarget)
	edgeMetadata := protocol.RelayMetadata{
		Name:          "office-edge",
		Endpoint:      "127.0.0.1:2222",
		RelayEndpoint: "wss://relay.example.com/ws/session",
		Platform:      "windows/amd64",
	}
	targetMetadata := protocol.RelayMetadata{
		Name:          "home-target",
		Endpoint:      "192.168.1.20:22",
		RelayEndpoint: "wss://relay.example.com/ws/session",
		Platform:      "linux/amd64",
	}
	writeRelayMetadata(t, edgeConn, edgeHello, edgeMetadata)
	writeRelayMetadata(t, targetConn, targetHello, targetMetadata)
	if err := edgeConn.WriteMessage(websocket.BinaryMessage, edgeHello.Packet[:]); err != nil {
		t.Fatal(err)
	}
	if err := targetConn.WriteMessage(websocket.BinaryMessage, targetHello.Packet[:]); err != nil {
		t.Fatal(err)
	}
	if _, _, err := edgeConn.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := targetConn.ReadMessage(); err != nil {
		t.Fatal(err)
	}

	connected := make(map[string]telemetry.Peer)
	for len(connected) < 2 {
		event := waitForRelayEvent(t, events, "relay_peer_connected")
		if event.PeerChange == nil || event.PeerChange.Action != telemetry.PeerActionUpsert || len(event.PeerChange.Peers) != 1 {
			t.Fatalf("unexpected connected event: %#v", event)
		}
		peer := event.PeerChange.Peers[0]
		if peer.Status != telemetry.PeerStatusWaiting || peer.ID == "" || peer.ConnectedAt.IsZero() {
			t.Fatalf("incomplete connected peer: %#v", peer)
		}
		connected[peer.Role] = peer
	}
	if connected["edge"].IP != "198.51.100.10" || connected["target"].IP != "203.0.113.20" {
		t.Fatalf("reported client IPs = %#v", connected)
	}

	paired := waitForRelayEvent(t, events, "relay_paired")
	if paired.PeerChange == nil || paired.PeerChange.Action != telemetry.PeerActionUpsert || len(paired.PeerChange.Peers) != 2 {
		t.Fatalf("unexpected paired event: %#v", paired)
	}
	for _, peer := range paired.PeerChange.Peers {
		if peer.Status != telemetry.PeerStatusPaired || peer.ID != connected[peer.Role].ID {
			t.Fatalf("paired peer does not match connection: %#v", peer)
		}
		if peer.RouteID == "" || peer.PeerID == "" {
			t.Fatalf("paired peer has no route or counterpart identity: %#v", peer)
		}
		if peer.Role == "edge" && (peer.Name != edgeMetadata.Name || peer.Endpoint != edgeMetadata.Endpoint || peer.PeerName != targetMetadata.Name) {
			t.Fatalf("edge metadata = %#v", peer)
		}
		if peer.Role == "target" && (peer.Name != targetMetadata.Name || peer.Endpoint != targetMetadata.Endpoint || peer.PeerName != edgeMetadata.Name) {
			t.Fatalf("target metadata = %#v", peer)
		}
	}

	payload := []byte("opaque-traffic-accounting")
	if err := edgeConn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatal(err)
	}
	_ = targetConn.SetReadDeadline(time.Now().Add(time.Second))
	if _, received, err := targetConn.ReadMessage(); err != nil || string(received) != string(payload) {
		t.Fatalf("relayed payload = %q, %v", received, err)
	}
	stats := waitForRelayEvent(t, events, "relay_peer_stats")
	if stats.PeerChange == nil || stats.PeerChange.Action != telemetry.PeerActionUpdate || !stats.Transient {
		t.Fatalf("unexpected peer stats event: %#v", stats)
	}
	var edgeStats, targetStats telemetry.Peer
	for _, peer := range stats.PeerChange.Peers {
		if peer.Role == "edge" {
			edgeStats = peer
		} else {
			targetStats = peer
		}
	}
	if edgeStats.BytesReceived < uint64(len(payload)) || targetStats.BytesSent < uint64(len(payload)) || edgeStats.LastActivityAt.IsZero() {
		t.Fatalf("traffic stats edge=%#v target=%#v", edgeStats, targetStats)
	}

	if err := edgeConn.Close(); err != nil {
		t.Fatal(err)
	}
	removed := make(map[string]bool)
	for len(removed) < 2 {
		event := waitForRelayEvent(t, events, "relay_peer_disconnected")
		if event.PeerChange == nil || event.PeerChange.Action != telemetry.PeerActionRemove || len(event.PeerChange.Peers) != 1 {
			t.Fatalf("unexpected disconnected event: %#v", event)
		}
		peer := event.PeerChange.Peers[0]
		removed[peer.ID] = true
	}
	if !removed[connected["edge"].ID] || !removed[connected["target"].ID] {
		t.Fatalf("removed peers = %#v, connected = %#v", removed, connected)
	}
}

func TestPairTimeoutKeepsParticipantClaimedByPair(t *testing.T) {
	server := New(Options{PairTimeout: 10 * time.Millisecond})
	participant := &participant{
		paired: make(chan struct{}),
		done:   make(chan struct{}),
	}
	returned := make(chan struct{})
	go func() {
		server.awaitParticipant(context.Background(), participant)
		close(returned)
	}()

	select {
	case <-returned:
		t.Fatal("participant returned after timeout even though the registry no longer considered it waiting")
	case <-time.After(30 * time.Millisecond):
	}
	close(participant.done)
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("participant did not finish after paired session closed")
	}
}

func TestWaitingParticipantDisconnectReleasesRoleImmediately(t *testing.T) {
	events := make(chan telemetry.Event, 16)
	server := New(Options{
		PairTimeout: 5 * time.Second,
		Reporter: telemetry.ReporterFunc(func(event telemetry.Event) {
			events <- event
		}),
	})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(func() {
		_ = server.Close()
		httpServer.Close()
	})
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"
	secret := []byte("waiting-disconnect-regression-secret")

	staleEdge := dialRelayParticipant(t, url, secret, "replacement-route", protocol.RoleEdge)
	waitForRelayEvent(t, events, "relay_peer_connected")
	if err := staleEdge.Close(); err != nil {
		t.Fatal(err)
	}
	disconnected := waitForRelayEvent(t, events, "relay_peer_disconnected")
	if disconnected.PeerChange == nil || len(disconnected.PeerChange.Peers) != 1 {
		t.Fatalf("unexpected disconnect event: %#v", disconnected)
	}

	replacementEdge := dialRelayParticipant(t, url, secret, "replacement-route", protocol.RoleEdge)
	defer replacementEdge.Close()
	target := dialRelayParticipant(t, url, secret, "replacement-route", protocol.RoleTarget)
	defer target.Close()
	_ = replacementEdge.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := replacementEdge.ReadMessage(); err != nil {
		t.Fatalf("replacement edge was not paired immediately: %v", err)
	}
	_ = target.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := target.ReadMessage(); err != nil {
		t.Fatalf("target was not paired with replacement edge: %v", err)
	}
}

func TestConcurrentWaitingConnectionChurnLeavesNoRegistryEntries(t *testing.T) {
	server := New(Options{PairTimeout: 5 * time.Second})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(func() {
		_ = server.Close()
		httpServer.Close()
	})
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"
	secret := []byte("concurrent-waiting-churn-secret")

	const connectionCount = 48
	connections := make([]*websocket.Conn, connectionCount)
	errors := make(chan error, connectionCount*2)
	var workers sync.WaitGroup
	workers.Add(connectionCount)
	for i := range connectionCount {
		go func(index int) {
			defer workers.Done()
			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				errors <- err
				return
			}
			hello, err := protocol.NewHello(secret, fmt.Sprintf("churn-%d", index), protocol.RoleEdge)
			if err == nil {
				err = conn.WriteMessage(websocket.BinaryMessage, hello.Packet[:])
			}
			if err != nil {
				_ = conn.Close()
				errors <- err
				return
			}
			connections[index] = conn
		}(i)
	}
	workers.Wait()
	if len(errors) > 0 {
		close(errors)
		for err := range errors {
			t.Errorf("connection setup failed: %v", err)
		}
		t.FailNow()
	}

	waitForRegistrySize(t, server.registry, connectionCount)
	workers.Add(connectionCount)
	for _, conn := range connections {
		go func(conn *websocket.Conn) {
			defer workers.Done()
			if err := conn.Close(); err != nil {
				errors <- err
			}
		}(conn)
	}
	workers.Wait()
	if len(errors) > 0 {
		close(errors)
		for err := range errors {
			t.Errorf("connection close failed: %v", err)
		}
	}
	waitForRegistrySize(t, server.registry, 0)
}

func dialRelayParticipant(t *testing.T, url string, secret []byte, channel string, role protocol.Role) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	hello, err := protocol.NewHello(secret, channel, role)
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, hello.Packet[:]); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	return conn
}

func waitForRegistrySize(t *testing.T, registry *registry, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		registry.mu.Lock()
		got := len(registry.waiting)
		registry.mu.Unlock()
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("waiting registry size = %d, want %d", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func writeRelayMetadata(t *testing.T, conn *websocket.Conn, hello *protocol.Hello, metadata protocol.RelayMetadata) {
	t.Helper()
	frames, err := protocol.SealRelayMetadata(hello, "", metadata)
	if err != nil {
		t.Fatal(err)
	}
	for _, frame := range frames {
		if err := conn.WriteControl(websocket.PingMessage, frame, time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
}

func waitForRelayEvent(t *testing.T, events <-chan telemetry.Event, eventType string) telemetry.Event {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.Type == eventType {
				return event
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for relay event %q", eventType)
		}
	}
}
