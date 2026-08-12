package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/suifei/molex/internal/config"
	"github.com/suifei/molex/internal/relay"
	"github.com/suifei/molex/internal/telemetry"
)

func TestThreeEndConcurrentTCPFlow(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()

	relayEvents := make(chan telemetry.Event, 16)
	relayServer := relay.New(relay.Options{
		Token:       "relay-token-0123456789",
		PairTimeout: 5 * time.Second,
		Reporter: telemetry.ReporterFunc(func(event telemetry.Event) {
			relayEvents <- event
		}),
	})
	httpServer := httptest.NewServer(relayServer.Handler())
	defer httpServer.Close()
	remote := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	secret := "mx1_integration-secret-with-ample-entropy"
	base := config.Config{
		Mode:   config.ModePunch,
		Secret: secret,
		Token:  "relay-token-0123456789",
		Remote: remote,
		Tunnel: config.TunnelConfig{Remote: "integration-channel"},
	}
	targetConfig := base
	targetConfig.Role = config.RoleTarget
	targetConfig.Tunnel.Local = echoAddress
	edgeConfig := base
	edgeConfig.Role = config.RoleEdge
	edgeConfig.Listen = "127.0.0.1:0"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 2)
	ready := make(chan string, 1)
	edgeReporter := telemetry.ReporterFunc(func(event telemetry.Event) {
		if event.Type == "edge_listening" {
			select {
			case ready <- event.Listen:
			default:
			}
		}
	})
	go func() { errors <- Run(ctx, targetConfig, nil) }()
	go func() { errors <- Run(ctx, edgeConfig, edgeReporter) }()

	var edgeAddress string
	select {
	case edgeAddress = <-ready:
	case err := <-errors:
		t.Fatalf("client stopped before ready: %v", err)
	case <-time.After(8 * time.Second):
		t.Fatal("timed out waiting for edge listener")
	}

	paired := waitForClientEvent(t, relayEvents, "relay_paired", 5*time.Second)
	if paired.PeerChange == nil || len(paired.PeerChange.Peers) != 2 {
		t.Fatalf("relay pair telemetry = %#v, want two peers", paired.PeerChange)
	}
	peerIDs := make(map[string]bool, 2)
	peerRoles := make(map[string]bool, 2)
	for _, peer := range paired.PeerChange.Peers {
		if peer.ID == "" || net.ParseIP(peer.IP) == nil || peer.Status != telemetry.PeerStatusPaired {
			t.Fatalf("relay peer = %#v, want a paired peer with a client IP", peer)
		}
		peerIDs[peer.ID] = true
		peerRoles[peer.Role] = true
	}
	if !peerRoles["edge"] || !peerRoles["target"] {
		t.Fatalf("relay roles = %#v, want Edge and Target", peerRoles)
	}

	const streams = 8
	var wg sync.WaitGroup
	resultErrors := make(chan error, streams)
	for i := range streams {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			payload := make([]byte, 48<<10)
			if _, err := rand.Read(payload); err != nil {
				resultErrors <- err
				return
			}
			copy(payload, []byte(fmt.Sprintf("stream-%02d", index)))
			conn, err := net.DialTimeout("tcp", edgeAddress, 3*time.Second)
			if err != nil {
				resultErrors <- err
				return
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
			if _, err := conn.Write(payload); err != nil {
				resultErrors <- err
				return
			}
			received := make([]byte, len(payload))
			if _, err := io.ReadFull(conn, received); err != nil {
				resultErrors <- err
				return
			}
			if !bytes.Equal(received, payload) {
				resultErrors <- fmt.Errorf("stream %d payload mismatch", index)
			}
		}(i)
	}
	wg.Wait()
	close(resultErrors)
	for err := range resultErrors {
		if err != nil {
			t.Error(err)
		}
	}

	cancel()
	for range 2 {
		select {
		case err := <-errors:
			if err != nil {
				t.Errorf("client shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("client did not stop after cancellation")
		}
	}

	removed := make(map[string]bool, 2)
	for len(removed) < 2 {
		event := waitForClientEvent(t, relayEvents, "relay_peer_disconnected", 5*time.Second)
		if event.PeerChange == nil || len(event.PeerChange.Peers) != 1 {
			t.Fatalf("relay disconnect telemetry = %#v", event.PeerChange)
		}
		removed[event.PeerChange.Peers[0].ID] = true
	}
	for peerID := range peerIDs {
		if !removed[peerID] {
			t.Fatalf("peer %q remained after client shutdown", peerID)
		}
	}
}

func TestMultipleEdgesAndTargetsShareOneRouteWithoutCrossTalk(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()

	remote, closeRelay := startTestRelay(t)
	defer closeRelay()
	targetConfig, edgeConfig := testClientConfigs(remote, echoAddress)
	targetConfig.Tunnel.Pool = 2
	targetConfig.Tunnel.Name = "shared-target"
	edgeConfig.Tunnel.Name = "shared-edge"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 4)
	ready := make(chan string, 2)
	reporter := telemetry.ReporterFunc(func(event telemetry.Event) {
		if event.Type == "edge_listening" {
			select {
			case ready <- event.Listen:
			default:
			}
		}
	})
	go func() { errors <- Run(ctx, targetConfig, nil) }()
	for range 2 {
		go func() { errors <- Run(ctx, edgeConfig, reporter) }()
	}

	edgeAddresses := make([]string, 0, 2)
	for len(edgeAddresses) < 2 {
		select {
		case address := <-ready:
			edgeAddresses = append(edgeAddresses, address)
		case err := <-errors:
			t.Fatalf("client stopped before both routes were ready: %v", err)
		case <-time.After(8 * time.Second):
			t.Fatal("timed out waiting for two Edge listeners")
		}
	}
	if edgeAddresses[0] == edgeAddresses[1] {
		t.Fatalf("two Edge sessions reported the same listener %q", edgeAddresses[0])
	}

	resultErrors := make(chan error, 2)
	var workers sync.WaitGroup
	for index, address := range edgeAddresses {
		workers.Add(1)
		go func(index int, address string) {
			defer workers.Done()
			payload := bytes.Repeat([]byte(fmt.Sprintf("route-%d/", index)), 4096)
			conn, err := net.DialTimeout("tcp", address, 3*time.Second)
			if err != nil {
				resultErrors <- err
				return
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
			if _, err := conn.Write(payload); err != nil {
				resultErrors <- err
				return
			}
			received := make([]byte, len(payload))
			if _, err := io.ReadFull(conn, received); err != nil {
				resultErrors <- err
				return
			}
			if !bytes.Equal(received, payload) {
				resultErrors <- fmt.Errorf("route %d payload crossed or changed", index)
			}
		}(index, address)
	}
	workers.Wait()
	close(resultErrors)
	for err := range resultErrors {
		t.Error(err)
	}

	cancel()
	for range 3 {
		select {
		case err := <-errors:
			if err != nil {
				t.Errorf("client shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("multi-client shutdown exceeded its bound")
		}
	}
}

func TestMultipleEdgesQueueForSingleTargetAndRecoverFIFO(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()

	remote, closeRelay := startTestRelay(t)
	defer closeRelay()
	targetConfig, edgeConfig := testClientConfigs(remote, echoAddress)
	targetConfig.Tunnel.Pool = config.DefaultTargetPool
	targetConfig.Tunnel.Name = "shared-target"
	edgeConfig.Tunnel.Name = "shared-edge"
	retry := integrationRetrySettings()

	targetCtx, stopTarget := context.WithCancel(context.Background())
	targetEvents := make(chan telemetry.Event, 64)
	targetErrors := make(chan error, 1)
	go func() {
		targetErrors <- run(targetCtx, targetConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			targetEvents <- event
		}), retry)
	}()

	edgeOneCtx, stopEdgeOne := context.WithCancel(context.Background())
	edgeOneEvents := make(chan telemetry.Event, 64)
	edgeOneErrors := make(chan error, 1)
	go func() {
		edgeOneErrors <- run(edgeOneCtx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeOneEvents <- event
		}), retry)
	}()

	firstReady := waitForClientEvent(t, edgeOneEvents, "edge_listening", 5*time.Second)
	assertEchoRoundTrip(t, firstReady.Listen, []byte("single-target-first-edge"))

	edgeTwoCtx, stopEdgeTwo := context.WithCancel(context.Background())
	edgeTwoEvents := make(chan telemetry.Event, 64)
	edgeTwoErrors := make(chan error, 1)
	go func() {
		edgeTwoErrors <- run(edgeTwoCtx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeTwoEvents <- event
		}), retry)
	}()

	// The only Target session is already paired, so the second Edge must stay
	// in connecting/waiting state and must not expose its local listener yet.
	select {
	case event := <-edgeTwoEvents:
		if event.Type == "edge_listening" {
			t.Fatalf("second Edge started listening while the only Target was occupied: %#v", event)
		}
	case <-time.After(250 * time.Millisecond):
	}

	stopEdgeOne()
	assertClientStopped(t, edgeOneErrors, "first edge")

	// Target reconnects after the first pair closes; FIFO pairing should now
	// promote the waiting second Edge and make its listener usable.
	secondReady := waitForClientEvent(t, edgeTwoEvents, "edge_listening", 5*time.Second)
	assertEchoRoundTrip(t, secondReady.Listen, []byte("single-target-second-edge"))

	stopEdgeTwo()
	stopTarget()
	assertClientStopped(t, edgeTwoErrors, "second edge")
	assertClientStopped(t, targetErrors, "target")
}

func TestEdgeReconnectsAfterTargetRestart(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()

	remote, closeRelay := startTestRelay(t)
	defer closeRelay()
	targetConfig, edgeConfig := testClientConfigs(remote, echoAddress)
	retry := integrationRetrySettings()

	edgeCtx, stopEdge := context.WithCancel(context.Background())
	edgeErrors := make(chan error, 1)
	edgeEvents := make(chan telemetry.Event, 64)
	go func() {
		edgeErrors <- run(edgeCtx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeEvents <- event
		}), retry)
	}()

	firstTargetCtx, stopFirstTarget := context.WithCancel(context.Background())
	firstTargetErrors := make(chan error, 1)
	go func() { firstTargetErrors <- run(firstTargetCtx, targetConfig, nil, retry) }()

	firstReady := waitForClientEvent(t, edgeEvents, "edge_listening", 5*time.Second)
	assertEchoRoundTrip(t, firstReady.Listen, []byte("before-target-restart"))

	stopFirstTarget()
	assertClientStopped(t, firstTargetErrors, "first target")
	reconnecting := waitForClientEvent(t, edgeEvents, "client_reconnecting", 5*time.Second)
	if strings.Contains(reconnecting.Message, "<nil>") || !strings.Contains(reconnecting.Message, "retry") {
		t.Fatalf("reconnect message is not actionable: %q", reconnecting.Message)
	}

	secondTargetCtx, stopSecondTarget := context.WithCancel(context.Background())
	secondTargetErrors := make(chan error, 1)
	go func() { secondTargetErrors <- run(secondTargetCtx, targetConfig, nil, retry) }()

	secondReady := waitForClientEvent(t, edgeEvents, "edge_listening", 5*time.Second)
	assertEchoRoundTrip(t, secondReady.Listen, []byte("after-target-restart"))

	stopSecondTarget()
	stopEdge()
	assertClientStopped(t, secondTargetErrors, "second target")
	assertClientStopped(t, edgeErrors, "edge")
}

func TestEdgeRecoversAfterListenAddressIsReleased(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()

	remote, closeRelay := startTestRelay(t)
	defer closeRelay()
	targetConfig, edgeConfig := testClientConfigs(remote, echoAddress)
	retry := integrationRetrySettings()

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	edgeConfig.Listen = occupied.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	targetErrors := make(chan error, 1)
	edgeErrors := make(chan error, 1)
	edgeEvents := make(chan telemetry.Event, 64)
	go func() { targetErrors <- run(ctx, targetConfig, nil, retry) }()
	go func() {
		edgeErrors <- run(ctx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeEvents <- event
		}), retry)
	}()

	reconnecting := waitForClientEvent(t, edgeEvents, "client_reconnecting", 5*time.Second)
	if !strings.Contains(reconnecting.Message, "Stop the process using that address") {
		t.Fatalf("listen failure is not actionable: %q", reconnecting.Message)
	}
	if err := occupied.Close(); err != nil {
		t.Fatal(err)
	}

	ready := waitForClientEvent(t, edgeEvents, "edge_listening", 5*time.Second)
	assertEchoRoundTrip(t, ready.Listen, []byte("listener-recovered"))

	cancel()
	assertClientStopped(t, targetErrors, "target")
	assertClientStopped(t, edgeErrors, "edge")
}

func startTestRelay(t *testing.T) (string, func()) {
	t.Helper()
	relayServer := relay.New(relay.Options{Token: "relay-token-0123456789", PairTimeout: 2 * time.Second})
	httpServer := httptest.NewServer(relayServer.Handler())
	return "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session", httpServer.Close
}

func testClientConfigs(remote, targetAddress string) (config.Config, config.Config) {
	base := config.Config{
		Mode:   config.ModePunch,
		Secret: "mx1_integration-secret-with-ample-entropy",
		Token:  "relay-token-0123456789",
		Remote: remote,
		Tunnel: config.TunnelConfig{Remote: "reconnect-channel"},
	}
	target := base
	target.Role = config.RoleTarget
	target.Tunnel.Local = targetAddress
	target.Tunnel.Pool = 1
	edge := base
	edge.Role = config.RoleEdge
	edge.Listen = "127.0.0.1:0"
	return target, edge
}

func integrationRetrySettings() retrySettings {
	return retrySettings{
		initial:     25 * time.Millisecond,
		maximum:     100 * time.Millisecond,
		stableAfter: 100 * time.Millisecond,
		random:      func() float64 { return 0.5 },
	}
}

func waitForClientEvent(t *testing.T, events <-chan telemetry.Event, eventType string, timeout time.Duration) telemetry.Event {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.Type == eventType {
				return event
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for client event %q", eventType)
		}
	}
}

func assertEchoRoundTrip(t *testing.T, address string, payload []byte) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		t.Fatalf("dial edge %s: %v", address, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write edge payload: %v", err)
	}
	received := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, received); err != nil {
		t.Fatalf("read edge payload: %v", err)
	}
	if !bytes.Equal(received, payload) {
		t.Fatalf("echo payload = %q, want %q", received, payload)
	}
}

func assertClientStopped(t *testing.T, errors <-chan error, name string) {
	t.Helper()
	select {
	case err := <-errors:
		if err != nil {
			t.Fatalf("%s shutdown: %v", name, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not stop", name)
	}
}

func startEchoServer(t *testing.T) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return listener.Addr().String(), func() {
		listener.Close()
		<-done
	}
}
