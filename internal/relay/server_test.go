package relay

import (
	"context"
	"errors"
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

const (
	testTokenValue  = "mx2_relay-test-token-0123456789"
	otherTokenValue = "mx2_relay-other-token-9876543210"
)

func testCredentials() []Credential {
	return []Credential{
		{ID: "tok-test", Token: testTokenValue},
		{ID: "tok-other", Token: otherTokenValue},
	}
}

func TestRelayAdmissionByTokenState(t *testing.T) {
	server := New(Options{Tokens: []Credential{
		{ID: "tok-live", Token: testTokenValue},
		{ID: "tok-off", Token: otherTokenValue, Disabled: true},
	}})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	if _, response, err := websocket.DefaultDialer.Dial(url, nil); err == nil {
		t.Fatal("relay accepted a request without a token")
	} else if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing token response = %#v", response)
	}

	header := make(http.Header)
	header.Set("Authorization", "Bearer mx2_this-token-does-not-exist")
	if _, response, err := websocket.DefaultDialer.Dial(url, header); err == nil {
		t.Fatal("relay accepted an unknown token")
	} else if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown token response = %#v", response)
	}

	header.Set("Authorization", "Bearer "+otherTokenValue)
	if _, response, err := websocket.DefaultDialer.Dial(url, header); err == nil {
		t.Fatal("relay accepted a disabled token")
	} else if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("disabled token response = %#v", response)
	}

	header.Set("Authorization", "Bearer "+testTokenValue)
	conn, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		t.Fatalf("relay rejected a valid token: %v", err)
	}
	conn.Close()
}

func TestRelayWithoutTokensRejectsEveryone(t *testing.T) {
	server := New(Options{})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	header := make(http.Header)
	header.Set("Authorization", "Bearer "+testTokenValue)
	if _, response, err := websocket.DefaultDialer.Dial(url, header); err == nil {
		t.Fatal("relay without configured tokens accepted a client")
	} else if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("empty token store response = %#v", response)
	}
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

func TestRelayRejectsHelloRouteFromDifferentToken(t *testing.T) {
	server := New(Options{Tokens: testCredentials()})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	header := make(http.Header)
	header.Set("Authorization", "Bearer "+testTokenValue)
	conn, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Authenticate with one token but present a hello derived from another.
	secret, channel := protocol.DeriveCredentials(otherTokenValue)
	hello, err := protocol.NewHello(secret, channel, protocol.RoleEdge)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, hello.Packet[:]); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) || closeErr.Code != websocket.ClosePolicyViolation {
		t.Fatalf("route mismatch close = %v, want policy violation", err)
	}
	if !strings.Contains(closeErr.Text, "route does not match") {
		t.Fatalf("route mismatch reason = %q", closeErr.Text)
	}
}

func TestPairedSessionOutlivesPairTimeout(t *testing.T) {
	server := New(Options{PairTimeout: 80 * time.Millisecond, Tokens: testCredentials()})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	edgeConn, _ := dialParticipant(t, url, testTokenValue, protocol.RoleEdge, "", "")
	defer edgeConn.Close()
	targetConn, _ := dialParticipant(t, url, testTokenValue, protocol.RoleTarget, "", "instance-a")
	defer targetConn.Close()

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

func TestRelayReportsPeerLifecycleWithTokenGroups(t *testing.T) {
	events := make(chan telemetry.Event, 32)
	server := New(Options{
		PairTimeout: time.Second,
		Tokens:      testCredentials(),
		Reporter: telemetry.ReporterFunc(func(event telemetry.Event) {
			events <- event
		}),
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	edgeConn, _ := dialNamedParticipant(t, url, testTokenValue, protocol.RoleEdge, "office-edge", "")
	defer edgeConn.Close()
	targetConn, _ := dialNamedParticipant(t, url, testTokenValue, protocol.RoleTarget, "home-target", "instance-a")
	defer targetConn.Close()
	if _, _, err := edgeConn.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := targetConn.ReadMessage(); err != nil {
		t.Fatal(err)
	}

	connected := make(map[string]telemetry.Peer)
	for len(connected) < 2 {
		event := waitForRelayEvent(t, events, "relay_peer_connected")
		peer := event.PeerChange.Peers[0]
		if peer.TokenID != "tok-test" {
			t.Fatalf("peer token id = %q, want tok-test", peer.TokenID)
		}
		connected[peer.Role] = peer
	}
	if connected["edge"].Name != "office-edge" || connected["target"].Name != "home-target" {
		t.Fatalf("peer names = %#v", connected)
	}

	paired := waitForRelayEvent(t, events, "relay_paired")
	if paired.PeerChange == nil || len(paired.PeerChange.Peers) != 2 {
		t.Fatalf("paired event = %#v", paired)
	}
	for _, peer := range paired.PeerChange.Peers {
		if peer.Status != telemetry.PeerStatusPaired || peer.TokenID != "tok-test" || peer.PeerID == "" {
			t.Fatalf("paired peer = %#v", peer)
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
	if stats.PeerChange == nil || !stats.Transient {
		t.Fatalf("stats event = %#v", stats)
	}

	if err := edgeConn.Close(); err != nil {
		t.Fatal(err)
	}
	removed := make(map[string]bool)
	for len(removed) < 2 {
		event := waitForRelayEvent(t, events, "relay_peer_disconnected")
		removed[event.PeerChange.Peers[0].ID] = true
	}
}

func TestRelayRejectsSecondTargetInstancePerToken(t *testing.T) {
	events := make(chan telemetry.Event, 32)
	server := New(Options{
		PairTimeout: 5 * time.Second,
		Tokens:      testCredentials(),
		Reporter:    telemetry.ReporterFunc(func(event telemetry.Event) { events <- event }),
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	first, _ := dialParticipant(t, url, testTokenValue, protocol.RoleTarget, "", "instance-a")
	defer first.Close()
	waitForRelayEvent(t, events, "relay_peer_connected")

	// A second connection from the same process (same instance) joins the
	// adaptive pool and must be allowed.
	pool, _ := dialParticipant(t, url, testTokenValue, protocol.RoleTarget, "", "instance-a")
	defer pool.Close()
	waitForRelayEvent(t, events, "relay_peer_connected")

	// A different target process for the same token must be rejected.
	intruder, _ := dialParticipant(t, url, testTokenValue, protocol.RoleTarget, "", "instance-b")
	defer intruder.Close()
	_ = intruder.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := intruder.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) || closeErr.Code != CloseDuplicateTarget {
		t.Fatalf("duplicate target close = %v, want code %d", err, CloseDuplicateTarget)
	}
	if !strings.Contains(closeErr.Text, "another target") {
		t.Fatalf("duplicate target reason = %q", closeErr.Text)
	}
	waitForRelayEvent(t, events, "relay_target_rejected")

	// The same token still accepts targets under a different token id, and
	// the original instance can rejoin after its connections close.
	other, _ := dialParticipant(t, url, otherTokenValue, protocol.RoleTarget, "", "instance-b")
	defer other.Close()
	waitForRelayEvent(t, events, "relay_peer_connected")

	first.Close()
	pool.Close()
	waitForRelayEvent(t, events, "relay_peer_disconnected")
	waitForRelayEvent(t, events, "relay_peer_disconnected")

	replacement, _ := dialParticipant(t, url, testTokenValue, protocol.RoleTarget, "", "instance-b")
	defer replacement.Close()
	waitForRelayEvent(t, events, "relay_peer_connected")
}

func TestUpdateTokensDisconnectsDisabledGroups(t *testing.T) {
	events := make(chan telemetry.Event, 32)
	server := New(Options{
		PairTimeout: 5 * time.Second,
		Tokens:      testCredentials(),
		Reporter:    telemetry.ReporterFunc(func(event telemetry.Event) { events <- event }),
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	edgeConn, _ := dialParticipant(t, url, testTokenValue, protocol.RoleEdge, "", "")
	defer edgeConn.Close()
	targetConn, _ := dialParticipant(t, url, testTokenValue, protocol.RoleTarget, "", "instance-a")
	defer targetConn.Close()
	if _, _, err := edgeConn.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := targetConn.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	otherConn, _ := dialParticipant(t, url, otherTokenValue, protocol.RoleTarget, "", "instance-z")
	defer otherConn.Close()

	server.UpdateTokens([]Credential{
		{ID: "tok-test", Token: testTokenValue, Disabled: true},
		{ID: "tok-other", Token: otherTokenValue},
	})

	for _, conn := range []*websocket.Conn{edgeConn, targetConn} {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _, err := conn.ReadMessage()
		var closeErr *websocket.CloseError
		if !errors.As(err, &closeErr) || closeErr.Code != CloseTokenDisabled {
			t.Fatalf("disabled token close = %v, want code %d", err, CloseTokenDisabled)
		}
	}
	waitForRelayEvent(t, events, "relay_token_revoked")

	// The untouched token keeps its connection alive.
	_ = otherConn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second))

	// New connections with the disabled token are refused at admission.
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+testTokenValue)
	if _, response, err := websocket.DefaultDialer.Dial(url, header); err == nil {
		t.Fatal("disabled token reconnected")
	} else if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("disabled reconnect response = %#v", response)
	}
}

func TestExpiredTokenLifetimeRefusesAdmissionAndDropsSessions(t *testing.T) {
	events := make(chan telemetry.Event, 32)
	server := New(Options{
		PairTimeout: 5 * time.Second,
		Tokens: []Credential{{
			ID:        "tok-test",
			Token:     testTokenValue,
			ExpiresAt: time.Now().Add(time.Hour),
		}},
		Reporter: telemetry.ReporterFunc(func(event telemetry.Event) { events <- event }),
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	edgeConn, _ := dialParticipant(t, url, testTokenValue, protocol.RoleEdge, "", "")
	defer edgeConn.Close()
	targetConn, _ := dialParticipant(t, url, testTokenValue, protocol.RoleTarget, "", "instance-a")
	defer targetConn.Close()
	if _, _, err := edgeConn.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := targetConn.ReadMessage(); err != nil {
		t.Fatal(err)
	}

	server.UpdateTokens([]Credential{{
		ID:        "tok-test",
		Token:     testTokenValue,
		ExpiresAt: time.Now().Add(-time.Second),
	}})

	for _, conn := range []*websocket.Conn{edgeConn, targetConn} {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _, err := conn.ReadMessage()
		var closeErr *websocket.CloseError
		if !errors.As(err, &closeErr) || closeErr.Code != CloseTokenDisabled {
			t.Fatalf("expired token close = %v, want code %d", err, CloseTokenDisabled)
		}
		if !strings.Contains(closeErr.Text, "expired") {
			t.Fatalf("expired close reason = %q", closeErr.Text)
		}
	}
	waitForRelayEvent(t, events, "relay_token_revoked")

	header := make(http.Header)
	header.Set("Authorization", "Bearer "+testTokenValue)
	if _, response, err := websocket.DefaultDialer.Dial(url, header); err == nil {
		t.Fatal("expired token reconnected")
	} else if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("expired reconnect response = %#v", response)
	}
}

func TestDisconnectPeerClosesOneConnection(t *testing.T) {
	events := make(chan telemetry.Event, 32)
	server := New(Options{
		PairTimeout: 5 * time.Second,
		Tokens:      testCredentials(),
		Reporter:    telemetry.ReporterFunc(func(event telemetry.Event) { events <- event }),
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	edgeConn, _ := dialParticipant(t, url, testTokenValue, protocol.RoleEdge, "", "")
	defer edgeConn.Close()
	connected := waitForRelayEvent(t, events, "relay_peer_connected")
	peerID := connected.PeerChange.Peers[0].ID

	if server.DisconnectPeer("no-such-peer") {
		t.Fatal("unknown peer id reported as disconnected")
	}
	if !server.DisconnectPeer(peerID) {
		t.Fatal("existing peer could not be disconnected")
	}
	_ = edgeConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := edgeConn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) || closeErr.Code != CloseKicked {
		t.Fatalf("kick close = %v, want code %d", err, CloseKicked)
	}
	waitForRelayEvent(t, events, "relay_peer_kicked")
}

func TestRotatedTokenGraceAdmissionAndExpiry(t *testing.T) {
	const oldValue = "mx2_rotated-old-value-0123456789"
	const newValue = "mx2_rotated-new-value-9876543210"
	events := make(chan telemetry.Event, 32)
	server := New(Options{
		PairTimeout: 5 * time.Second,
		Tokens: []Credential{{
			ID:              "tok-rotate",
			Token:           newValue,
			Previous:        oldValue,
			PreviousExpires: time.Now().Add(time.Hour),
		}},
		Reporter: telemetry.ReporterFunc(func(event telemetry.Event) { events <- event }),
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	// Legacy-value clients pair with each other on the old route while the
	// grace window is open.
	oldEdge, oldEdgeHello := dialParticipant(t, url, oldValue, protocol.RoleEdge, "", "")
	defer oldEdge.Close()
	oldTarget, oldTargetHello := dialParticipant(t, url, oldValue, protocol.RoleTarget, "", "instance-r")
	defer oldTarget.Close()
	assertRelayedHello(t, oldEdge, oldTargetHello.Packet[:])
	assertRelayedHello(t, oldTarget, oldEdgeHello.Packet[:])

	// New-value clients pair independently on the new route, same group id.
	newEdge, newEdgeHello := dialParticipant(t, url, newValue, protocol.RoleEdge, "", "")
	defer newEdge.Close()
	newTarget, newTargetHello := dialParticipant(t, url, newValue, protocol.RoleTarget, "", "instance-r")
	defer newTarget.Close()
	assertRelayedHello(t, newEdge, newTargetHello.Packet[:])
	assertRelayedHello(t, newTarget, newEdgeHello.Packet[:])

	// Closing the grace window refuses new admissions with the old value
	// and drops the participants that still use it.
	server.UpdateTokens([]Credential{{
		ID:              "tok-rotate",
		Token:           newValue,
		Previous:        oldValue,
		PreviousExpires: time.Now().Add(-time.Second),
	}})
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+oldValue)
	if _, response, err := websocket.DefaultDialer.Dial(url, header); err == nil {
		t.Fatal("expired rotated value was re-admitted")
	} else if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired value response = %#v", response)
	}

	server.dropExpiredLegacy()
	_ = oldEdge.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		_, _, err := oldEdge.ReadMessage()
		if err == nil {
			continue
		}
		var closeErr *websocket.CloseError
		if !errors.As(err, &closeErr) || closeErr.Code != CloseTokenDisabled {
			t.Fatalf("legacy close = %v, want code %d", err, CloseTokenDisabled)
		}
		if !strings.Contains(closeErr.Text, "grace period ended") {
			t.Fatalf("legacy close reason = %q", closeErr.Text)
		}
		break
	}

	// New-value clients keep working.
	if err := newEdge.WriteMessage(websocket.BinaryMessage, []byte("still-alive")); err != nil {
		t.Fatal(err)
	}
	_ = newTarget.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, payload, err := newTarget.ReadMessage(); err != nil || string(payload) != "still-alive" {
		t.Fatalf("new-value relay failed: %q %v", payload, err)
	}
}

func TestPeerMetadataRefreshesThroughPingChannel(t *testing.T) {
	events := make(chan telemetry.Event, 64)
	server := New(Options{
		PairTimeout: 5 * time.Second,
		Tokens:      testCredentials(),
		Reporter:    telemetry.ReporterFunc(func(event telemetry.Event) { events <- event }),
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	conn, hello := dialNamedParticipant(t, url, testTokenValue, protocol.RoleTarget, "meta-target", "instance-m")
	defer conn.Close()
	connected := waitForRelayEvent(t, events, "relay_peer_connected")
	if connected.PeerChange.Peers[0].Endpoint != "" {
		t.Fatalf("initial endpoint = %q, want empty", connected.PeerChange.Peers[0].Endpoint)
	}

	// A full in-session refresh is several ping frames; they must merge
	// into one console update instead of being rate-limited per frame.
	writeRelayMetadata(t, conn, hello, testTokenValue, protocol.RelayMetadata{
		Name:     "meta-target",
		Endpoint: "5 services",
		Platform: "test/amd64",
		Instance: "instance-m",
	})
	deadline := time.Now().Add(3 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("metadata refresh never surfaced")
		}
		event := waitForRelayEvent(t, events, "relay_peer_stats")
		if len(event.PeerChange.Peers) == 1 && event.PeerChange.Peers[0].Endpoint == "5 services" {
			if event.PeerChange.Peers[0].Name != "meta-target" {
				t.Fatalf("partial refresh erased the name: %#v", event.PeerChange.Peers[0])
			}
			break
		}
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

func TestWaitingTargetIsKeptAsHotStandby(t *testing.T) {
	server := New(Options{PairTimeout: 10 * time.Millisecond})
	participant := &participant{
		role:   protocol.RoleTarget,
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
		t.Fatal("unpaired Target was closed by pairing timeout")
	case <-time.After(30 * time.Millisecond):
	}
	close(participant.done)
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("waiting Target did not finish after connection close")
	}
}

func TestWaitingParticipantDisconnectReleasesRoleImmediately(t *testing.T) {
	events := make(chan telemetry.Event, 16)
	server := New(Options{
		PairTimeout: 5 * time.Second,
		Tokens:      testCredentials(),
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

	staleEdge, _ := dialParticipant(t, url, testTokenValue, protocol.RoleEdge, "", "")
	waitForRelayEvent(t, events, "relay_peer_connected")
	if err := staleEdge.Close(); err != nil {
		t.Fatal(err)
	}
	disconnected := waitForRelayEvent(t, events, "relay_peer_disconnected")
	if disconnected.PeerChange == nil || len(disconnected.PeerChange.Peers) != 1 {
		t.Fatalf("unexpected disconnect event: %#v", disconnected)
	}

	replacementEdge, _ := dialParticipant(t, url, testTokenValue, protocol.RoleEdge, "", "")
	defer replacementEdge.Close()
	target, _ := dialParticipant(t, url, testTokenValue, protocol.RoleTarget, "", "instance-a")
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

func TestRelayPairsMultipleParticipantsFIFOAndAllowsDuplicateNames(t *testing.T) {
	events := make(chan telemetry.Event, 32)
	server := New(Options{
		PairTimeout: 5 * time.Second,
		Tokens:      testCredentials(),
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

	edgeOne, edgeOneHello := dialNamedParticipant(t, url, testTokenValue, protocol.RoleEdge, "shared-edge", "")
	defer edgeOne.Close()
	edgeTwo, edgeTwoHello := dialNamedParticipant(t, url, testTokenValue, protocol.RoleEdge, "shared-edge", "")
	defer edgeTwo.Close()
	// Both target sessions belong to one target process (same instance),
	// mirroring the adaptive pool in v2.
	targetOne, targetOneHello := dialParticipant(t, url, testTokenValue, protocol.RoleTarget, "", "instance-pool")
	defer targetOne.Close()
	targetTwo, targetTwoHello := dialParticipant(t, url, testTokenValue, protocol.RoleTarget, "", "instance-pool")
	defer targetTwo.Close()

	assertRelayedHello(t, edgeOne, targetOneHello.Packet[:])
	assertRelayedHello(t, edgeTwo, targetTwoHello.Packet[:])
	assertRelayedHello(t, targetOne, edgeOneHello.Packet[:])
	assertRelayedHello(t, targetTwo, edgeTwoHello.Packet[:])

	if edgeOneHello.Packet == edgeTwoHello.Packet || targetOneHello.Packet == targetTwoHello.Packet {
		t.Fatal("independent participants unexpectedly reused hello packets")
	}
	if got := server.registry.waitingCount(); got != 0 {
		t.Fatalf("waiting participants after FIFO pairing = %d, want 0", got)
	}

	edgeIDs := make(map[string]bool)
	for len(edgeIDs) < 2 {
		event := waitForRelayEvent(t, events, "relay_peer_connected")
		if event.PeerChange == nil || len(event.PeerChange.Peers) != 1 {
			continue
		}
		peer := event.PeerChange.Peers[0]
		if peer.Role == "edge" {
			if peer.Name != "shared-edge" {
				t.Fatalf("duplicate Edge name = %q, want shared-edge", peer.Name)
			}
			edgeIDs[peer.ID] = true
		}
	}
	if len(edgeIDs) != 2 {
		t.Fatalf("same-name Edges have %d distinct IDs, want 2", len(edgeIDs))
	}
}

func TestRelayQueuesAdditionalEdgeUntilTargetArrives(t *testing.T) {
	server := New(Options{PairTimeout: 5 * time.Second, Tokens: testCredentials()})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(func() {
		_ = server.Close()
		httpServer.Close()
	})
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	edgeOne, edgeOneHello := dialParticipant(t, url, testTokenValue, protocol.RoleEdge, "", "")
	defer edgeOne.Close()
	edgeTwo, edgeTwoHello := dialParticipant(t, url, testTokenValue, protocol.RoleEdge, "", "")
	defer edgeTwo.Close()
	targetOne, targetOneHello := dialParticipant(t, url, testTokenValue, protocol.RoleTarget, "", "instance-pool")
	defer targetOne.Close()

	assertRelayedHello(t, edgeOne, targetOneHello.Packet[:])
	assertRelayedHello(t, targetOne, edgeOneHello.Packet[:])
	if got := server.registry.waitingCount(); got != 1 {
		t.Fatalf("waiting participants = %d, want second Edge only", got)
	}

	targetTwo, targetTwoHello := dialParticipant(t, url, testTokenValue, protocol.RoleTarget, "", "instance-pool")
	defer targetTwo.Close()
	assertRelayedHello(t, edgeTwo, targetTwoHello.Packet[:])
	assertRelayedHello(t, targetTwo, edgeTwoHello.Packet[:])
}

func TestConcurrentWaitingConnectionChurnLeavesNoRegistryEntries(t *testing.T) {
	server := New(Options{PairTimeout: 5 * time.Second, Tokens: testCredentials()})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(func() {
		_ = server.Close()
		httpServer.Close()
	})
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	const connectionCount = 48
	connections := make([]*websocket.Conn, connectionCount)
	errors := make(chan error, connectionCount*2)
	var workers sync.WaitGroup
	workers.Add(connectionCount)
	for i := range connectionCount {
		go func(index int) {
			defer workers.Done()
			header := make(http.Header)
			header.Set("Authorization", "Bearer "+testTokenValue)
			conn, _, err := websocket.DefaultDialer.Dial(url, header)
			if err != nil {
				errors <- err
				return
			}
			secret, channel := protocol.DeriveCredentials(testTokenValue)
			hello, err := protocol.NewHello(secret, channel, protocol.RoleEdge)
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

func dialParticipant(t *testing.T, url, token string, role protocol.Role, name, instance string) (*websocket.Conn, *protocol.Hello) {
	return dialNamedParticipant(t, url, token, role, name, instance)
}

func dialNamedParticipant(t *testing.T, url, token string, role protocol.Role, name, instance string) (*websocket.Conn, *protocol.Hello) {
	t.Helper()
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+token)
	conn, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		t.Fatal(err)
	}
	secret, channel := protocol.DeriveCredentials(token)
	hello, err := protocol.NewHello(secret, channel, role)
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if name != "" || instance != "" {
		writeRelayMetadata(t, conn, hello, token, protocol.RelayMetadata{Name: name, Instance: instance})
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, hello.Packet[:]); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	return conn, hello
}

func assertRelayedHello(t *testing.T, conn *websocket.Conn, want []byte) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	kind, packet, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read paired hello: %v", err)
	}
	if kind != websocket.BinaryMessage {
		t.Fatalf("paired hello frame type = %d, want binary", kind)
	}
	if string(packet) != string(want) {
		t.Fatalf("paired hello does not match FIFO peer")
	}
	_ = conn.SetReadDeadline(time.Time{})
}

func waitForRegistrySize(t *testing.T, registry *registry, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := registry.waitingCount()
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("waiting registry size = %d, want %d", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func writeRelayMetadata(t *testing.T, conn *websocket.Conn, hello *protocol.Hello, token string, metadata protocol.RelayMetadata) {
	t.Helper()
	frames, err := protocol.SealRelayMetadata(hello, token, metadata)
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
