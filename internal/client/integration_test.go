package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/suifei/molex/internal/config"
	"github.com/suifei/molex/internal/protocol"
	"github.com/suifei/molex/internal/relay"
	"github.com/suifei/molex/internal/telemetry"
)

const (
	integrationTokenA = "mx2_integration-token-alpha-0123456789"
	integrationTokenB = "mx2_integration-token-beta-9876543210"
)

func integrationCredentials() []relay.Credential {
	return []relay.Credential{
		{ID: "tok-alpha", Token: integrationTokenA},
		{ID: "tok-beta", Token: integrationTokenB},
	}
}

func TestThreeEndConcurrentTCPFlow(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()

	relayEvents := make(chan telemetry.Event, 64)
	relayServer := relay.New(relay.Options{
		Tokens:      integrationCredentials(),
		PairTimeout: 5 * time.Second,
		Reporter: telemetry.ReporterFunc(func(event telemetry.Event) {
			relayEvents <- event
		}),
	})
	httpServer := httptest.NewServer(relayServer.Handler())
	defer httpServer.Close()
	remote := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	targetConfig := targetConfigFor(remote, integrationTokenA,
		config.ServiceEntry{ID: "svc-echo", Name: "echo", Address: echoAddress})
	edgeConfig := edgeConfigFor(t, remote, integrationTokenA, "svc-echo")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 2)
	edgeEvents := make(chan telemetry.Event, 256)
	go func() { errors <- run(ctx, targetConfig, nil, integrationRetrySettings(), Updates{}) }()
	go func() {
		errors <- run(ctx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeEvents <- event
		}), integrationRetrySettings(), Updates{})
	}()

	mapping := waitForMappingState(t, edgeEvents, "svc-echo", telemetry.MappingStateListening, 8*time.Second)
	if mapping.Address != echoAddress || mapping.ServiceName != "echo" {
		t.Fatalf("mapping metadata = %#v", mapping)
	}

	paired := waitForClientEvent(t, relayEvents, "relay_paired", 5*time.Second)
	if paired.PeerChange == nil || len(paired.PeerChange.Peers) != 2 {
		t.Fatalf("relay pair telemetry = %#v, want two peers", paired.PeerChange)
	}
	peerIDs := make(map[string]bool, 2)
	for _, peer := range paired.PeerChange.Peers {
		if peer.TokenID != "tok-alpha" || peer.Status != telemetry.PeerStatusPaired {
			t.Fatalf("relay peer = %#v, want paired peer of tok-alpha", peer)
		}
		peerIDs[peer.ID] = true
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
			conn, err := net.DialTimeout("tcp", mapping.Listen, 3*time.Second)
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
	for len(removed) < len(peerIDs) {
		event := waitForClientEvent(t, relayEvents, "relay_peer_disconnected", 5*time.Second)
		if event.PeerChange == nil || len(event.PeerChange.Peers) != 1 {
			t.Fatalf("relay disconnect telemetry = %#v", event.PeerChange)
		}
		if peerIDs[event.PeerChange.Peers[0].ID] {
			removed[event.PeerChange.Peers[0].ID] = true
		}
	}
}

func TestEdgeMapsMultipleServicesWithoutCrossTalk(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()
	bannerAddress, closeBanner := startBannerServer(t, "banner-service-hello")
	defer closeBanner()

	remote, closeRelay, _ := startTestRelay(t)
	defer closeRelay()

	targetConfig := targetConfigFor(remote, integrationTokenA,
		config.ServiceEntry{ID: "svc-echo", Name: "echo", Address: echoAddress},
		config.ServiceEntry{ID: "svc-banner", Name: "banner", Address: bannerAddress})
	edgeConfig := edgeConfigFor(t, remote, integrationTokenA, "svc-echo", "svc-banner")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 2)
	edgeEvents := make(chan telemetry.Event, 256)
	go func() { errors <- run(ctx, targetConfig, nil, integrationRetrySettings(), Updates{}) }()
	go func() {
		errors <- run(ctx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeEvents <- event
		}), integrationRetrySettings(), Updates{})
	}()

	echoMapping := waitForMappingState(t, edgeEvents, "svc-echo", telemetry.MappingStateListening, 8*time.Second)
	bannerMapping := waitForMappingState(t, edgeEvents, "svc-banner", telemetry.MappingStateListening, 8*time.Second)
	if echoMapping.Listen == bannerMapping.Listen {
		t.Fatalf("both mappings share listener %q", echoMapping.Listen)
	}

	assertEchoRoundTrip(t, echoMapping.Listen, []byte("multi-service-echo"))

	conn, err := net.DialTimeout("tcp", bannerMapping.Listen, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
	banner, err := io.ReadAll(conn)
	if err != nil {
		t.Fatal(err)
	}
	if string(banner) != "banner-service-hello" {
		t.Fatalf("banner service returned %q; streams may have crossed services", banner)
	}

	cancel()
	assertClientStopped(t, errors, "multi-service client")
	assertClientStopped(t, errors, "multi-service client")
}

func TestCatalogUpdatePropagatesToEdgeLive(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()
	secondAddress, closeSecond := startEchoServer(t)
	defer closeSecond()

	remote, closeRelay, _ := startTestRelay(t)
	defer closeRelay()

	firstService := config.ServiceEntry{ID: "svc-first", Name: "first", Address: echoAddress}
	secondService := config.ServiceEntry{ID: "svc-second", Name: "second", Address: secondAddress}
	targetConfig := targetConfigFor(remote, integrationTokenA, firstService)
	edgeConfig := edgeConfigFor(t, remote, integrationTokenA, "svc-first", "svc-second")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serviceUpdates := make(chan []config.ServiceEntry, 1)
	errors := make(chan error, 2)
	edgeEvents := make(chan telemetry.Event, 256)
	go func() {
		errors <- run(ctx, targetConfig, nil, integrationRetrySettings(), Updates{Services: serviceUpdates})
	}()
	go func() {
		errors <- run(ctx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeEvents <- event
		}), integrationRetrySettings(), Updates{})
	}()

	first := waitForMappingState(t, edgeEvents, "svc-first", telemetry.MappingStateListening, 8*time.Second)
	assertEchoRoundTrip(t, first.Listen, []byte("catalog-first"))
	pending := waitForMappingState(t, edgeEvents, "svc-second", telemetry.MappingStateWaiting, 4*time.Second)
	if !strings.Contains(pending.Message, "not publish") {
		t.Fatalf("unpublished mapping message = %q", pending.Message)
	}

	// Publish the second service at runtime; the edge must map it live.
	serviceUpdates <- []config.ServiceEntry{firstService, secondService}
	second := waitForMappingState(t, edgeEvents, "svc-second", telemetry.MappingStateListening, 8*time.Second)
	assertEchoRoundTrip(t, second.Listen, []byte("catalog-second"))

	// Remove the first service; its edge listener must close.
	serviceUpdates <- []config.ServiceEntry{secondService}
	waitForMappingState(t, edgeEvents, "svc-first", telemetry.MappingStateWaiting, 8*time.Second)
	if _, err := net.DialTimeout("tcp", first.Listen, 500*time.Millisecond); err == nil {
		t.Fatal("edge still accepts connections for a withdrawn service")
	}
	assertEchoRoundTrip(t, second.Listen, []byte("catalog-second-still-works"))

	cancel()
	assertClientStopped(t, errors, "catalog client")
	assertClientStopped(t, errors, "catalog client")
}

func TestEdgeMappingsApplyLive(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()
	remote, closeRelay, _ := startTestRelay(t)
	defer closeRelay()

	targetConfig := targetConfigFor(remote, integrationTokenA,
		config.ServiceEntry{ID: "svc-echo", Name: "echo", Address: echoAddress})
	edgeConfig := edgeConfigFor(t, remote, integrationTokenA)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mappingUpdates := make(chan []config.MappingEntry, 1)
	errors := make(chan error, 2)
	edgeEvents := make(chan telemetry.Event, 256)
	go func() { errors <- run(ctx, targetConfig, nil, integrationRetrySettings(), Updates{}) }()
	go func() {
		errors <- run(ctx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeEvents <- event
		}), integrationRetrySettings(), Updates{Mappings: mappingUpdates})
	}()

	waitForClientEvent(t, edgeEvents, "edge_catalog", 8*time.Second)

	port := freeLoopbackPort(t)
	mappingUpdates <- []config.MappingEntry{{Service: "svc-echo", Port: port}}
	mapping := waitForMappingState(t, edgeEvents, "svc-echo", telemetry.MappingStateListening, 8*time.Second)
	assertEchoRoundTrip(t, mapping.Listen, []byte("live-mapping"))

	mappingUpdates <- nil
	deadline := time.Now().Add(4 * time.Second)
	for {
		if _, err := net.DialTimeout("tcp", mapping.Listen, 250*time.Millisecond); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("edge kept listening after its mapping was removed")
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	assertClientStopped(t, errors, "live-mapping client")
	assertClientStopped(t, errors, "live-mapping client")
}

func TestMultipleEdgesShareOneTokenWithoutCrossTalk(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()
	remote, closeRelay, _ := startTestRelay(t)
	defer closeRelay()

	targetConfig := targetConfigFor(remote, integrationTokenA,
		config.ServiceEntry{ID: "svc-echo", Name: "echo", Address: echoAddress})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 4)
	go func() { errors <- run(ctx, targetConfig, nil, integrationRetrySettings(), Updates{}) }()

	const edgeCount = 3
	listens := make([]string, edgeCount)
	for index := range edgeCount {
		edgeConfig := edgeConfigFor(t, remote, integrationTokenA, "svc-echo")
		events := make(chan telemetry.Event, 256)
		go func() {
			errors <- run(ctx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
				events <- event
			}), integrationRetrySettings(), Updates{})
		}()
		mapping := waitForMappingState(t, events, "svc-echo", telemetry.MappingStateListening, 10*time.Second)
		listens[index] = mapping.Listen
	}
	if len(uniqueStrings(listens)) != edgeCount {
		t.Fatalf("edge listeners are not independent: %#v", listens)
	}

	var workers sync.WaitGroup
	results := make(chan error, edgeCount)
	for index, address := range listens {
		workers.Add(1)
		go func(index int, address string) {
			defer workers.Done()
			payload := bytes.Repeat([]byte(fmt.Sprintf("edge-%d/", index)), 8192)
			conn, err := net.DialTimeout("tcp", address, 3*time.Second)
			if err != nil {
				results <- err
				return
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
			if _, err := conn.Write(payload); err != nil {
				results <- err
				return
			}
			received := make([]byte, len(payload))
			if _, err := io.ReadFull(conn, received); err != nil {
				results <- err
				return
			}
			if !bytes.Equal(received, payload) {
				results <- fmt.Errorf("edge %d payload mismatch", index)
			}
		}(index, address)
	}
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Error(err)
		}
	}

	cancel()
	for range edgeCount + 1 {
		assertClientStopped(t, errors, "multi-edge client")
	}
}

func TestCrossTokenIsolation(t *testing.T) {
	alphaAddress, closeAlpha := startEchoServer(t)
	defer closeAlpha()
	betaAddress, closeBeta := startEchoServer(t)
	defer closeBeta()
	remote, closeRelay, _ := startTestRelay(t)
	defer closeRelay()

	targetAlpha := targetConfigFor(remote, integrationTokenA,
		config.ServiceEntry{ID: "svc-alpha", Name: "alpha", Address: alphaAddress})
	targetBeta := targetConfigFor(remote, integrationTokenB,
		config.ServiceEntry{ID: "svc-beta", Name: "beta", Address: betaAddress})
	edgeBeta := edgeConfigFor(t, remote, integrationTokenB, "svc-beta")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 3)
	edgeEvents := make(chan telemetry.Event, 256)
	go func() { errors <- run(ctx, targetAlpha, nil, integrationRetrySettings(), Updates{}) }()
	go func() { errors <- run(ctx, targetBeta, nil, integrationRetrySettings(), Updates{}) }()
	go func() {
		errors <- run(ctx, edgeBeta, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeEvents <- event
		}), integrationRetrySettings(), Updates{})
	}()

	catalog := waitForCatalogOnline(t, edgeEvents, 8*time.Second)
	if len(catalog.Services) != 1 || catalog.Services[0].ID != "svc-beta" {
		t.Fatalf("token-beta edge sees catalog %#v, want only svc-beta", catalog.Services)
	}
	mapping := waitForMappingState(t, edgeEvents, "svc-beta", telemetry.MappingStateListening, 8*time.Second)
	assertEchoRoundTrip(t, mapping.Listen, []byte("isolated-beta-traffic"))

	cancel()
	for range 3 {
		assertClientStopped(t, errors, "isolation client")
	}
}

func TestSecondTargetProcessIsRejected(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()
	remote, closeRelay, _ := startTestRelay(t)
	defer closeRelay()

	targetConfig := targetConfigFor(remote, integrationTokenA,
		config.ServiceEntry{ID: "svc-echo", Name: "echo", Address: echoAddress})
	edgeConfig := edgeConfigFor(t, remote, integrationTokenA, "svc-echo")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 3)
	edgeEvents := make(chan telemetry.Event, 256)
	go func() { errors <- run(ctx, targetConfig, nil, integrationRetrySettings(), Updates{}) }()
	go func() {
		errors <- run(ctx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeEvents <- event
		}), integrationRetrySettings(), Updates{})
	}()
	mapping := waitForMappingState(t, edgeEvents, "svc-echo", telemetry.MappingStateListening, 8*time.Second)
	assertEchoRoundTrip(t, mapping.Listen, []byte("before-duplicate-target"))

	// A second target process with the same token must be rejected with an
	// actionable message while the first target keeps serving.
	duplicateCtx, cancelDuplicate := context.WithCancel(context.Background())
	defer cancelDuplicate()
	duplicateEvents := make(chan telemetry.Event, 256)
	duplicateErrors := make(chan error, 1)
	go func() {
		duplicateErrors <- run(duplicateCtx, targetConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			duplicateEvents <- event
		}), integrationRetrySettings(), Updates{})
	}()

	rejected := waitForClientEvent(t, duplicateEvents, "client_rejected", 8*time.Second)
	if !strings.Contains(rejected.Message, "Another Target is already connected") {
		t.Fatalf("duplicate target message = %q", rejected.Message)
	}
	assertEchoRoundTrip(t, mapping.Listen, []byte("after-duplicate-target"))

	cancelDuplicate()
	assertClientStopped(t, duplicateErrors, "duplicate target")
	cancel()
	assertClientStopped(t, errors, "original client")
	assertClientStopped(t, errors, "original client")
}

func TestMaliciousEdgeCannotDialUnpublishedAddress(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()
	remote, closeRelay, _ := startTestRelay(t)
	defer closeRelay()

	targetConfig := targetConfigFor(remote, integrationTokenA,
		config.ServiceEntry{ID: "svc-echo", Name: "echo", Address: echoAddress})
	targetEvents := make(chan telemetry.Event, 256)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 1)
	go func() {
		errors <- run(ctx, targetConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			targetEvents <- event
		}), integrationRetrySettings(), Updates{})
	}()

	// Handcrafted edge that bypasses the catalog and asks for an arbitrary
	// address id. The hot-standby target pairs once this edge arrives, and
	// it must refuse the request instead of dialing.
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+integrationTokenA)
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	ws, _, err := dialer.Dial(remote, header)
	if err != nil {
		t.Fatal(err)
	}
	secret, channel := protocol.DeriveCredentials(integrationTokenA)
	secure, err := protocol.OpenSecureClient(ctx, ws, secret, channel, protocol.RoleEdge)
	if err != nil {
		t.Fatal(err)
	}
	defer secure.Close()
	session, err := yamux.Client(secure, yamuxConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	stream, err := session.OpenStream()
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	_ = stream.SetDeadline(time.Now().Add(5 * time.Second))
	if err := protocol.WriteDataPreamble(stream, "svc-not-published"); err != nil {
		t.Fatal(err)
	}
	status, err := protocol.ReadDialStatus(stream)
	if err != nil {
		t.Fatal(err)
	}
	if status != protocol.TunnelDialUnknown {
		t.Fatalf("unpublished service status = %d, want %d", status, protocol.TunnelDialUnknown)
	}
	refused := waitForClientEvent(t, targetEvents, "target_request_rejected", 5*time.Second)
	if !strings.Contains(refused.Message, "refused") {
		t.Fatalf("refusal message = %q", refused.Message)
	}

	cancel()
	assertClientStopped(t, errors, "allowlist target")
}

func TestEdgeReconnectsAfterTargetRestart(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()
	remote, closeRelay, _ := startTestRelay(t)
	defer closeRelay()

	targetConfig := targetConfigFor(remote, integrationTokenA,
		config.ServiceEntry{ID: "svc-echo", Name: "echo", Address: echoAddress})
	edgeConfig := edgeConfigFor(t, remote, integrationTokenA, "svc-echo")
	retry := integrationRetrySettings()

	edgeCtx, stopEdge := context.WithCancel(context.Background())
	edgeErrors := make(chan error, 1)
	edgeEvents := make(chan telemetry.Event, 256)
	go func() {
		edgeErrors <- run(edgeCtx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeEvents <- event
		}), retry, Updates{})
	}()

	firstTargetCtx, stopFirstTarget := context.WithCancel(context.Background())
	firstTargetErrors := make(chan error, 1)
	go func() { firstTargetErrors <- run(firstTargetCtx, targetConfig, nil, retry, Updates{}) }()

	firstMapping := waitForMappingState(t, edgeEvents, "svc-echo", telemetry.MappingStateListening, 8*time.Second)
	assertEchoRoundTrip(t, firstMapping.Listen, []byte("before-target-restart"))

	stopFirstTarget()
	assertClientStopped(t, firstTargetErrors, "first target")
	reconnecting := waitForClientEvent(t, edgeEvents, "client_reconnecting", 8*time.Second)
	if strings.Contains(reconnecting.Message, "<nil>") || !strings.Contains(reconnecting.Message, "retry") {
		t.Fatalf("reconnect message is not actionable: %q", reconnecting.Message)
	}
	waitForMappingState(t, edgeEvents, "svc-echo", telemetry.MappingStateWaiting, 8*time.Second)

	secondTargetCtx, stopSecondTarget := context.WithCancel(context.Background())
	secondTargetErrors := make(chan error, 1)
	go func() { secondTargetErrors <- run(secondTargetCtx, targetConfig, nil, retry, Updates{}) }()

	secondMapping := waitForMappingState(t, edgeEvents, "svc-echo", telemetry.MappingStateListening, 10*time.Second)
	assertEchoRoundTrip(t, secondMapping.Listen, []byte("after-target-restart"))

	stopSecondTarget()
	stopEdge()
	assertClientStopped(t, secondTargetErrors, "second target")
	assertClientStopped(t, edgeErrors, "edge")
}

func TestEdgeMappingRecoversAfterPortIsReleased(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()
	remote, closeRelay, _ := startTestRelay(t)
	defer closeRelay()

	targetConfig := targetConfigFor(remote, integrationTokenA,
		config.ServiceEntry{ID: "svc-echo", Name: "echo", Address: echoAddress})
	edgeConfig := edgeConfigFor(t, remote, integrationTokenA, "svc-echo")

	occupied, err := net.Listen("tcp", edgeConfig.Mappings[0].ListenAddress())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 2)
	edgeEvents := make(chan telemetry.Event, 256)
	go func() { errors <- run(ctx, targetConfig, nil, integrationRetrySettings(), Updates{}) }()
	go func() {
		errors <- run(ctx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeEvents <- event
		}), integrationRetrySettings(), Updates{})
	}()

	problem := waitForClientEvent(t, edgeEvents, "edge_mapping_error", 8*time.Second)
	if !strings.Contains(problem.Message, "Stop the process using that address") {
		t.Fatalf("listener failure is not actionable: %q", problem.Message)
	}
	blocked := waitForMappingState(t, edgeEvents, "svc-echo", telemetry.MappingStateError, 8*time.Second)
	if blocked.Listen == "" {
		t.Fatalf("blocked mapping status = %#v", blocked)
	}

	if err := occupied.Close(); err != nil {
		t.Fatal(err)
	}
	recovered := waitForMappingState(t, edgeEvents, "svc-echo", telemetry.MappingStateListening, 10*time.Second)
	assertEchoRoundTrip(t, recovered.Listen, []byte("listener-recovered"))

	cancel()
	assertClientStopped(t, errors, "port-recovery client")
	assertClientStopped(t, errors, "port-recovery client")
}

func TestTokenDisableKicksAndReEnableRecovers(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()
	remote, closeRelay, relayServer := startTestRelay(t)
	defer closeRelay()

	targetConfig := targetConfigFor(remote, integrationTokenA,
		config.ServiceEntry{ID: "svc-echo", Name: "echo", Address: echoAddress})
	edgeConfig := edgeConfigFor(t, remote, integrationTokenA, "svc-echo")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 2)
	edgeEvents := make(chan telemetry.Event, 256)
	go func() { errors <- run(ctx, targetConfig, nil, integrationRetrySettings(), Updates{}) }()
	go func() {
		errors <- run(ctx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeEvents <- event
		}), integrationRetrySettings(), Updates{})
	}()
	mapping := waitForMappingState(t, edgeEvents, "svc-echo", telemetry.MappingStateListening, 8*time.Second)
	assertEchoRoundTrip(t, mapping.Listen, []byte("before-disable"))

	relayServer.UpdateTokens([]relay.Credential{
		{ID: "tok-alpha", Token: integrationTokenA, Disabled: true},
		{ID: "tok-beta", Token: integrationTokenB},
	})
	rejected := waitForClientEvent(t, edgeEvents, "client_rejected", 8*time.Second)
	if !strings.Contains(rejected.Message, "disabled") {
		t.Fatalf("disabled token message = %q", rejected.Message)
	}

	relayServer.UpdateTokens(integrationCredentials())
	recovered := waitForMappingState(t, edgeEvents, "svc-echo", telemetry.MappingStateListening, 10*time.Second)
	assertEchoRoundTrip(t, recovered.Listen, []byte("after-re-enable"))

	cancel()
	assertClientStopped(t, errors, "token-toggle client")
	assertClientStopped(t, errors, "token-toggle client")
}

func TestRelayKickTriggersAutomaticEdgeRecovery(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()

	relayEvents := make(chan telemetry.Event, 128)
	relayServer := relay.New(relay.Options{
		Tokens:      integrationCredentials(),
		PairTimeout: 5 * time.Second,
		Reporter:    telemetry.ReporterFunc(func(event telemetry.Event) { relayEvents <- event }),
	})
	httpServer := httptest.NewServer(relayServer.Handler())
	defer httpServer.Close()
	remote := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	targetConfig := targetConfigFor(remote, integrationTokenA,
		config.ServiceEntry{ID: "svc-echo", Name: "echo", Address: echoAddress})
	edgeConfig := edgeConfigFor(t, remote, integrationTokenA, "svc-echo")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 2)
	edgeEvents := make(chan telemetry.Event, 256)
	go func() { errors <- run(ctx, targetConfig, nil, integrationRetrySettings(), Updates{}) }()
	go func() {
		errors <- run(ctx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeEvents <- event
		}), integrationRetrySettings(), Updates{})
	}()
	mapping := waitForMappingState(t, edgeEvents, "svc-echo", telemetry.MappingStateListening, 8*time.Second)
	assertEchoRoundTrip(t, mapping.Listen, []byte("before-kick"))

	var edgePeerID string
	deadline := time.Now().Add(5 * time.Second)
	for edgePeerID == "" {
		if time.Now().After(deadline) {
			t.Fatal("relay never reported the edge peer")
		}
		event := waitForClientEvent(t, relayEvents, "relay_paired", 5*time.Second)
		for _, peer := range event.PeerChange.Peers {
			if peer.Role == "edge" {
				edgePeerID = peer.ID
			}
		}
	}

	if !relayServer.DisconnectPeer(edgePeerID) {
		t.Fatal("edge peer could not be kicked")
	}
	waitForClientEvent(t, edgeEvents, "client_reconnecting", 8*time.Second)
	recovered := waitForMappingState(t, edgeEvents, "svc-echo", telemetry.MappingStateListening, 10*time.Second)
	assertEchoRoundTrip(t, recovered.Listen, []byte("after-kick"))

	cancel()
	assertClientStopped(t, errors, "kick-recovery client")
	assertClientStopped(t, errors, "kick-recovery client")
}

func TestAdaptiveTargetHotStandbySurvivesPairTimeout(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()

	relayEvents := make(chan telemetry.Event, 64)
	relayServer := relay.New(relay.Options{
		Tokens:      integrationCredentials(),
		PairTimeout: 100 * time.Millisecond,
		Reporter:    telemetry.ReporterFunc(func(event telemetry.Event) { relayEvents <- event }),
	})
	httpServer := httptest.NewServer(relayServer.Handler())
	defer httpServer.Close()
	remote := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"
	targetConfig := targetConfigFor(remote, integrationTokenA,
		config.ServiceEntry{ID: "svc-echo", Name: "echo", Address: echoAddress})
	edgeConfig := edgeConfigFor(t, remote, integrationTokenA, "svc-echo")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	targetErrors := make(chan error, 1)
	edgeErrors := make(chan error, 1)
	edgeEvents := make(chan telemetry.Event, 256)
	go func() { targetErrors <- run(ctx, targetConfig, nil, integrationRetrySettings(), Updates{}) }()
	connected := waitForClientEvent(t, relayEvents, "relay_peer_connected", 5*time.Second)
	standbyID := connected.PeerChange.Peers[0].ID
	// Crossing the protocol's 15-second Edge handshake timeout proves the
	// standby Target survives both timeout mechanisms.
	time.Sleep(16 * time.Second)
	go func() {
		edgeErrors <- run(ctx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeEvents <- event
		}), integrationRetrySettings(), Updates{})
	}()

	paired := waitForClientEvent(t, relayEvents, "relay_paired", 5*time.Second)
	pairedStandby := false
	for _, peer := range paired.PeerChange.Peers {
		if peer.ID == standbyID && peer.Role == "target" {
			pairedStandby = true
		}
	}
	if !pairedStandby {
		t.Fatalf("original Target standby %q was replaced before pairing: %#v", standbyID, paired.PeerChange.Peers)
	}
	mapping := waitForMappingState(t, edgeEvents, "svc-echo", telemetry.MappingStateListening, 8*time.Second)
	assertEchoRoundTrip(t, mapping.Listen, []byte("standby-after-handshake-timeout"))
	cancel()
	assertClientStopped(t, targetErrors, "target")
	assertClientStopped(t, edgeErrors, "edge")
}

func TestMultipleEdgesRecoverThroughRepeatedNetworkFlaps(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()

	relayServer := relay.New(relay.Options{Tokens: integrationCredentials(), PairTimeout: 3 * time.Second})
	httpServer := httptest.NewServer(relayServer.Handler())
	defer httpServer.Close()
	relayURL, err := url.Parse(httpServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := startFaultProxy(t, relayURL.Host, 10*time.Millisecond)
	defer proxy.Close()
	remote := "ws://" + proxy.Addr() + "/ws/session"
	targetConfig := targetConfigFor(remote, integrationTokenA,
		config.ServiceEntry{ID: "svc-echo", Name: "echo", Address: echoAddress})
	retry := integrationRetrySettings()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 4)
	go func() { errors <- run(ctx, targetConfig, nil, retry, Updates{}) }()

	const edgeCount = 3
	edgeEvents := make([]chan telemetry.Event, edgeCount)
	for index := range edgeEvents {
		edgeEvents[index] = make(chan telemetry.Event, 512)
		events := edgeEvents[index]
		edgeConfig := edgeConfigFor(t, remote, integrationTokenA, "svc-echo")
		go func() {
			errors <- run(ctx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
				select {
				case events <- event:
				default:
				}
			}), retry, Updates{})
		}()
	}

	addresses := waitForEdgeGeneration(t, edgeEvents, "initial")
	assertEdgeSetRoundTrips(t, addresses, "initial")
	for cycle := 1; cycle <= 3; cycle++ {
		proxy.FailNextConnections(3)
		proxy.DropConnections()
		for index, events := range edgeEvents {
			event := waitForClientEvent(t, events, "client_reconnecting", 8*time.Second)
			if !strings.Contains(event.Message, "retry") {
				t.Fatalf("Edge %d cycle %d reconnect message is not actionable: %q", index, cycle, event.Message)
			}
		}
		addresses = waitForEdgeGeneration(t, edgeEvents, fmt.Sprintf("cycle-%d", cycle))
		assertEdgeSetRoundTrips(t, addresses, fmt.Sprintf("cycle-%d", cycle))
	}

	cancel()
	for range edgeCount + 1 {
		assertClientStopped(t, errors, "network-flap client")
	}
}

// TestTargetServesTwoGroupsWithServiceVisibility runs ONE target process
// that joins two token groups and restricts one service to the alpha group.
func TestTargetServesTwoGroupsWithServiceVisibility(t *testing.T) {
	sharedAddress, closeShared := startEchoServer(t)
	defer closeShared()
	alphaAddress, closeAlpha := startBannerServer(t, "alpha-secret-service")
	defer closeAlpha()
	remote, closeRelay, _ := startTestRelay(t)
	defer closeRelay()

	targetConfig := config.Config{
		Mode:   config.ModeTarget,
		Remote: remote,
		Name:   "dual-group-target",
		Tokens: []config.TokenEntry{
			{ID: "alpha", Token: integrationTokenA},
			{ID: "beta", Token: integrationTokenB},
		},
		Services: []config.ServiceEntry{
			{ID: "svc-shared", Name: "shared-echo", Address: sharedAddress},
			{ID: "svc-alpha", Name: "alpha-only", Address: alphaAddress, Groups: []string{"alpha"}},
		},
	}
	edgeAlpha := edgeConfigFor(t, remote, integrationTokenA, "svc-shared", "svc-alpha")
	edgeBeta := edgeConfigFor(t, remote, integrationTokenB, "svc-shared")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 3)
	alphaEvents := make(chan telemetry.Event, 256)
	betaEvents := make(chan telemetry.Event, 256)
	go func() { errors <- run(ctx, targetConfig, nil, integrationRetrySettings(), Updates{}) }()
	go func() {
		errors <- run(ctx, edgeAlpha, telemetry.ReporterFunc(func(event telemetry.Event) {
			alphaEvents <- event
		}), integrationRetrySettings(), Updates{})
	}()
	go func() {
		errors <- run(ctx, edgeBeta, telemetry.ReporterFunc(func(event telemetry.Event) {
			betaEvents <- event
		}), integrationRetrySettings(), Updates{})
	}()

	// The alpha edge sees and reaches both services.
	sharedMapping := waitForMappingState(t, alphaEvents, "svc-shared", telemetry.MappingStateListening, 10*time.Second)
	alphaMapping := waitForMappingState(t, alphaEvents, "svc-alpha", telemetry.MappingStateListening, 10*time.Second)
	assertEchoRoundTrip(t, sharedMapping.Listen, []byte("alpha-shared"))
	conn, err := net.DialTimeout("tcp", alphaMapping.Listen, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
	banner, err := io.ReadAll(conn)
	conn.Close()
	if err != nil || string(banner) != "alpha-secret-service" {
		t.Fatalf("alpha-only service payload = %q %v", banner, err)
	}

	// The beta edge's catalog must not contain the alpha-only service.
	betaCatalog := waitForCatalogOnline(t, betaEvents, 10*time.Second)
	for _, service := range betaCatalog.Services {
		if service.ID == "svc-alpha" {
			t.Fatalf("beta group can see the alpha-only service: %#v", betaCatalog.Services)
		}
	}
	betaShared := waitForMappingState(t, betaEvents, "svc-shared", telemetry.MappingStateListening, 10*time.Second)
	assertEchoRoundTrip(t, betaShared.Listen, []byte("beta-shared"))

	// Even a handcrafted beta edge that knows the hidden id is refused.
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+integrationTokenB)
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	ws, _, err := dialer.Dial(remote, header)
	if err != nil {
		t.Fatal(err)
	}
	secret, channel := protocol.DeriveCredentials(integrationTokenB)
	secure, err := protocol.OpenSecureClient(ctx, ws, secret, channel, protocol.RoleEdge)
	if err != nil {
		t.Fatal(err)
	}
	defer secure.Close()
	session, err := yamux.Client(secure, yamuxConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	stream, err := session.OpenStream()
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	_ = stream.SetDeadline(time.Now().Add(5 * time.Second))
	if err := protocol.WriteDataPreamble(stream, "svc-alpha"); err != nil {
		t.Fatal(err)
	}
	status, err := protocol.ReadDialStatus(stream)
	if err != nil {
		t.Fatal(err)
	}
	if status != protocol.TunnelDialUnknown {
		t.Fatalf("cross-group dial status = %d, want %d (refused)", status, protocol.TunnelDialUnknown)
	}

	cancel()
	for range 3 {
		assertClientStopped(t, errors, "dual-group client")
	}
}

// TestEdgeJoinsTwoGroupsInOneProcess runs ONE edge process that maps
// services from two different token groups at the same time.
func TestEdgeJoinsTwoGroupsInOneProcess(t *testing.T) {
	alphaAddress, closeAlpha := startBannerServer(t, "from-group-alpha")
	defer closeAlpha()
	betaAddress, closeBeta := startBannerServer(t, "from-group-beta")
	defer closeBeta()
	remote, closeRelay, _ := startTestRelay(t)
	defer closeRelay()

	targetAlpha := targetConfigFor(remote, integrationTokenA,
		config.ServiceEntry{ID: "svc-a", Name: "alpha-svc", Address: alphaAddress})
	targetBeta := targetConfigFor(remote, integrationTokenB,
		config.ServiceEntry{ID: "svc-b", Name: "beta-svc", Address: betaAddress})
	edgeConfig := config.Config{
		Mode:   config.ModeEdge,
		Remote: remote,
		Name:   "dual-group-edge",
		Tokens: []config.TokenEntry{
			{ID: "alpha", Token: integrationTokenA},
			{ID: "beta", Token: integrationTokenB},
		},
		Mappings: []config.MappingEntry{
			{Service: "svc-a", Group: "alpha", Port: freeLoopbackPort(t)},
			{Service: "svc-b", Group: "beta", Port: freeLoopbackPort(t)},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 3)
	edgeEvents := make(chan telemetry.Event, 256)
	go func() { errors <- run(ctx, targetAlpha, nil, integrationRetrySettings(), Updates{}) }()
	go func() { errors <- run(ctx, targetBeta, nil, integrationRetrySettings(), Updates{}) }()
	go func() {
		errors <- run(ctx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeEvents <- event
		}), integrationRetrySettings(), Updates{})
	}()

	alphaMapping := waitForMappingState(t, edgeEvents, "svc-a", telemetry.MappingStateListening, 10*time.Second)
	betaMapping := waitForMappingState(t, edgeEvents, "svc-b", telemetry.MappingStateListening, 10*time.Second)
	if alphaMapping.Group != "alpha" || betaMapping.Group != "beta" {
		t.Fatalf("mapping groups = %q/%q", alphaMapping.Group, betaMapping.Group)
	}

	readBanner := func(address string) string {
		conn, err := net.DialTimeout("tcp", address, 3*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
		payload, err := io.ReadAll(conn)
		if err != nil {
			t.Fatal(err)
		}
		return string(payload)
	}
	if got := readBanner(alphaMapping.Listen); got != "from-group-alpha" {
		t.Fatalf("alpha mapping payload = %q", got)
	}
	if got := readBanner(betaMapping.Listen); got != "from-group-beta" {
		t.Fatalf("beta mapping payload = %q", got)
	}

	// The aggregated catalog reports both groups online with their entries.
	deadline := time.Now().Add(8 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("catalog never reported both groups online")
		}
		event := <-edgeEvents
		if event.Catalog == nil || len(event.Catalog.Groups) != 2 {
			continue
		}
		online := 0
		for _, group := range event.Catalog.Groups {
			if group.Online {
				online++
			}
		}
		if online == 2 {
			break
		}
	}

	cancel()
	for range 3 {
		assertClientStopped(t, errors, "dual-group edge client")
	}
}

// TestRelayMetadataFollowsServiceChanges verifies the in-session encrypted
// metadata refresh: the relay console's per-connection service count follows
// live catalog edits without a reconnect.
func TestRelayMetadataFollowsServiceChanges(t *testing.T) {
	echoAddress, closeEcho := startEchoServer(t)
	defer closeEcho()

	relayEvents := make(chan telemetry.Event, 256)
	relayServer := relay.New(relay.Options{
		Tokens:      integrationCredentials(),
		PairTimeout: 3 * time.Second,
		Reporter:    telemetry.ReporterFunc(func(event telemetry.Event) { relayEvents <- event }),
	})
	httpServer := httptest.NewServer(relayServer.Handler())
	defer httpServer.Close()
	remote := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session"

	firstService := config.ServiceEntry{ID: "svc-1", Name: "one", Address: echoAddress}
	targetConfig := targetConfigFor(remote, integrationTokenA, firstService)
	edgeConfig := edgeConfigFor(t, remote, integrationTokenA, "svc-1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serviceUpdates := make(chan []config.ServiceEntry, 1)
	errors := make(chan error, 2)
	edgeEvents := make(chan telemetry.Event, 256)
	go func() {
		errors <- run(ctx, targetConfig, nil, integrationRetrySettings(), Updates{Services: serviceUpdates})
	}()
	go func() {
		errors <- run(ctx, edgeConfig, telemetry.ReporterFunc(func(event telemetry.Event) {
			edgeEvents <- event
		}), integrationRetrySettings(), Updates{})
	}()

	// Wait for the target's own connection (the edge connects too).
	targetSeen := false
	targetDeadline := time.Now().Add(8 * time.Second)
	for !targetSeen {
		if time.Now().After(targetDeadline) {
			t.Fatal("target never connected")
		}
		event := waitForClientEvent(t, relayEvents, "relay_peer_connected", 8*time.Second)
		peer := event.PeerChange.Peers[0]
		if peer.Role == "target" {
			if peer.Endpoint != "1 services" {
				t.Fatalf("initial target endpoint = %q, want \"1 services\"", peer.Endpoint)
			}
			targetSeen = true
		}
	}
	// A paired session carries the encrypted refresh channel; the hot
	// standby session reports fresh counts when it connects.
	waitForMappingState(t, edgeEvents, "svc-1", telemetry.MappingStateListening, 10*time.Second)

	serviceUpdates <- []config.ServiceEntry{
		firstService,
		{ID: "svc-2", Name: "two", Address: echoAddress},
		{ID: "svc-3", Name: "three", Address: echoAddress},
	}
	deadline := time.Now().Add(8 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("relay never observed the refreshed service count")
		}
		event := waitForClientEvent(t, relayEvents, "relay_peer_stats", 8*time.Second)
		refreshed := false
		for _, peer := range event.PeerChange.Peers {
			if peer.Endpoint == "3 services" {
				refreshed = true
			}
		}
		if refreshed {
			break
		}
	}

	cancel()
	assertClientStopped(t, errors, "metadata-refresh target")
	assertClientStopped(t, errors, "metadata-refresh edge")
}

// --- helpers ---

func startTestRelay(t *testing.T) (string, func(), *relay.Server) {
	t.Helper()
	relayServer := relay.New(relay.Options{Tokens: integrationCredentials(), PairTimeout: 2 * time.Second})
	httpServer := httptest.NewServer(relayServer.Handler())
	return "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/session", httpServer.Close, relayServer
}

func targetConfigFor(remote, token string, services ...config.ServiceEntry) config.Config {
	return config.Config{
		Mode:     config.ModeTarget,
		Remote:   remote,
		Token:    token,
		Name:     "integration-target",
		Services: services,
	}
}

func edgeConfigFor(t *testing.T, remote, token string, serviceIDs ...string) config.Config {
	t.Helper()
	mappings := make([]config.MappingEntry, 0, len(serviceIDs))
	for _, id := range serviceIDs {
		mappings = append(mappings, config.MappingEntry{Service: id, Port: freeLoopbackPort(t)})
	}
	return config.Config{
		Mode:     config.ModeEdge,
		Remote:   remote,
		Token:    token,
		Name:     "integration-edge",
		Mappings: mappings,
	}
}

func freeLoopbackPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
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

// waitForMappingState scans every event carrying mapping statuses until the
// wanted service reaches the wanted state.
func waitForMappingState(t *testing.T, events <-chan telemetry.Event, serviceID, state string, timeout time.Duration) telemetry.MappingStatus {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			for _, mapping := range event.Mappings {
				if mapping.Service == serviceID && mapping.State == state {
					return mapping
				}
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for mapping %q to become %q", serviceID, state)
		}
	}
}

func waitForCatalogOnline(t *testing.T, events <-chan telemetry.Event, timeout time.Duration) telemetry.CatalogUpdate {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.Catalog != nil && event.Catalog.Online && len(event.Catalog.Services) > 0 {
				return *event.Catalog
			}
		case <-timer.C:
			t.Fatal("timed out waiting for an online catalog")
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

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func waitForEdgeGeneration(t *testing.T, events []chan telemetry.Event, generation string) []string {
	t.Helper()
	addresses := make([]string, len(events))
	for index, edgeEvents := range events {
		mapping := waitForMappingState(t, edgeEvents, "svc-echo", telemetry.MappingStateListening, 10*time.Second)
		addresses[index] = mapping.Listen
	}
	if len(uniqueStrings(addresses)) != len(addresses) {
		t.Fatalf("%s Edge listeners are not independent: %#v", generation, addresses)
	}
	return addresses
}

func assertEdgeSetRoundTrips(t *testing.T, addresses []string, generation string) {
	t.Helper()
	var workers sync.WaitGroup
	errors := make(chan error, len(addresses))
	for index, address := range addresses {
		workers.Add(1)
		go func(index int, address string) {
			defer workers.Done()
			payload := []byte(fmt.Sprintf("%s-edge-%d", generation, index))
			conn, err := net.DialTimeout("tcp", address, 2*time.Second)
			if err != nil {
				errors <- err
				return
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
			if _, err := conn.Write(payload); err != nil {
				errors <- err
				return
			}
			received := make([]byte, len(payload))
			if _, err := io.ReadFull(conn, received); err != nil {
				errors <- err
				return
			}
			if !bytes.Equal(received, payload) {
				errors <- fmt.Errorf("%s Edge %d payload mismatch", generation, index)
			}
		}(index, address)
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Error(err)
		}
	}
}

type faultProxy struct {
	listener net.Listener
	upstream string
	delay    time.Duration

	mu       sync.Mutex
	active   map[net.Conn]struct{}
	failNext int
	done     chan struct{}
}

func startFaultProxy(t *testing.T, upstream string, delay time.Duration) *faultProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxy := &faultProxy{
		listener: listener,
		upstream: upstream,
		delay:    delay,
		active:   make(map[net.Conn]struct{}),
		done:     make(chan struct{}),
	}
	go proxy.serve()
	return proxy
}

func (p *faultProxy) Addr() string {
	return p.listener.Addr().String()
}

func (p *faultProxy) FailNextConnections(count int) {
	p.mu.Lock()
	p.failNext += count
	p.mu.Unlock()
}

func (p *faultProxy) DropConnections() {
	p.mu.Lock()
	connections := make([]net.Conn, 0, len(p.active))
	for conn := range p.active {
		connections = append(connections, conn)
	}
	p.mu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
}

func (p *faultProxy) Close() {
	_ = p.listener.Close()
	p.DropConnections()
	<-p.done
}

func (p *faultProxy) serve() {
	defer close(p.done)
	for {
		downstream, err := p.listener.Accept()
		if err != nil {
			return
		}
		p.mu.Lock()
		fail := p.failNext > 0
		if fail {
			p.failNext--
		}
		p.mu.Unlock()
		if fail {
			_ = downstream.Close()
			continue
		}
		upstream, err := net.DialTimeout("tcp", p.upstream, time.Second)
		if err != nil {
			_ = downstream.Close()
			continue
		}
		p.track(downstream, upstream)
		go p.pipe(upstream, downstream)
		go p.pipe(downstream, upstream)
	}
}

func (p *faultProxy) track(connections ...net.Conn) {
	p.mu.Lock()
	for _, conn := range connections {
		p.active[conn] = struct{}{}
	}
	p.mu.Unlock()
}

func (p *faultProxy) pipe(dst, src net.Conn) {
	buffer := make([]byte, 16<<10)
	for {
		read, err := src.Read(buffer)
		if read > 0 {
			if p.delay > 0 {
				time.Sleep(p.delay)
			}
			if _, writeErr := dst.Write(buffer[:read]); writeErr != nil {
				err = writeErr
			}
		}
		if err != nil {
			_ = src.Close()
			_ = dst.Close()
			p.mu.Lock()
			delete(p.active, src)
			delete(p.active, dst)
			p.mu.Unlock()
			return
		}
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

// startBannerServer writes one fixed payload and closes, so tests can prove
// which backend a mapped stream actually reached.
func startBannerServer(t *testing.T, banner string) (string, func()) {
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
				_, _ = conn.Write([]byte(banner))
			}()
		}
	}()
	return listener.Addr().String(), func() {
		listener.Close()
		<-done
	}
}
