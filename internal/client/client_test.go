package client

import (
	"context"
	"errors"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/suifei/molex/internal/config"
	"github.com/suifei/molex/internal/telemetry"
)

func TestRetryPolicyGrowsCapsAndResetsAfterStableSession(t *testing.T) {
	policy := newRetryPolicy(retrySettings{
		initial:     time.Second,
		maximum:     15 * time.Second,
		stableAfter: 30 * time.Second,
		random:      func() float64 { return 0.5 },
	})

	for index, expected := range []time.Duration{
		time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		15 * time.Second,
		15 * time.Second,
	} {
		if actual := policy.nextDelay(0); actual != expected {
			t.Fatalf("delay %d = %s, want %s", index, actual, expected)
		}
	}

	if actual := policy.nextDelay(31 * time.Second); actual != time.Second {
		t.Fatalf("delay after stable session = %s, want 1s", actual)
	}
	if actual := policy.nextDelay(0); actual != 2*time.Second {
		t.Fatalf("delay after reset failure = %s, want 2s", actual)
	}
}

func TestRetryPolicyJitterStaysWithinConfiguredRange(t *testing.T) {
	settings := retrySettings{
		initial: time.Second,
		maximum: 2 * time.Second,
		jitter:  0.20,
	}
	settings.random = func() float64 { return 0 }
	lower := newRetryPolicy(settings).nextDelay(0)
	settings.random = func() float64 { return 1 }
	upper := newRetryPolicy(settings).nextDelay(0)

	if lower < 799*time.Millisecond || lower > 801*time.Millisecond {
		t.Fatalf("lower jitter delay = %s, want about 800ms", lower)
	}
	if upper < 1199*time.Millisecond || upper > 1201*time.Millisecond {
		t.Fatalf("upper jitter delay = %s, want about 1.2s", upper)
	}
}

func TestRetryPolicyJitterNeverExceedsMaximum(t *testing.T) {
	policy := newRetryPolicy(retrySettings{
		initial: 15 * time.Second,
		maximum: 15 * time.Second,
		jitter:  0.20,
		random:  func() float64 { return 1 },
	})

	if actual := policy.nextDelay(0); actual != 15*time.Second {
		t.Fatalf("maximum jitter delay = %s, want 15s", actual)
	}
}

func TestClientErrorGuidance(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"authentication", &relayHTTPError{statusCode: 401, status: "401 Unauthorized", err: errors.New("bad handshake")}, "relay token"},
		{"route", &relayHTTPError{statusCode: 404, status: "404 Not Found", err: errors.New("bad handshake")}, "/ws/session"},
		{"gateway", &relayHTTPError{statusCode: 502, status: "502 Bad Gateway", err: errors.New("bad handshake")}, "Caddy's upstream"},
		{"dns", &net.DNSError{Name: "relay.invalid", Err: "no such host"}, "DNS settings"},
		{"refused", syscall.ECONNREFUSED, "check the firewall"},
		{"tls", errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"), "system time"},
		{"timeout", context.DeadlineExceeded, "network reachability"},
		{"pair", errors.New("receive peer hello: websocket: close 1013: pair timeout"), "Start the other client"},
		{"session unavailable", errors.New("websocket: close 1008: session unavailable"), "relay route"},
		{"handshake", errors.New("peer authentication failed"), "same channel and secret"},
		{"listener", &localListenError{address: "127.0.0.1:2222", err: syscall.EADDRINUSE}, "Stop the process using that address"},
		{"closed", errSessionClosed, "Retry the local connection"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := guidanceForClientError(test.err); !strings.Contains(actual, test.want) {
				t.Fatalf("guidance = %q, want it to contain %q", actual, test.want)
			}
		})
	}
}

func TestReconnectMessageNeverContainsNilError(t *testing.T) {
	message := reconnectMessage(time.Second, nil)
	if strings.Contains(message, "<nil>") {
		t.Fatalf("reconnect message contains nil error: %q", message)
	}
	if !strings.Contains(message, "MoleX will keep retrying") {
		t.Fatalf("reconnect message is not actionable: %q", message)
	}
}

func TestAdaptiveTargetSlotExpandsOnlyOnceAcrossReconnects(t *testing.T) {
	pool := newTargetPoolReporter(4, nil)
	expansions := 0
	pool.onReady = func() { expansions++ }
	slot := pool.slot(0)

	for range 3 {
		telemetry.Emit(slot, telemetry.Event{Type: "target_ready"})
		telemetry.Emit(slot, telemetry.Event{Type: "client_reconnecting"})
	}
	if expansions != 1 {
		t.Fatalf("slot triggered %d adaptive expansions across reconnects, want 1", expansions)
	}
}

func TestRunEdgeReportsPeerSessionClosure(t *testing.T) {
	edgeConn, targetConn := net.Pipe()
	targetSession, err := yamux.Server(targetConn, yamuxConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer targetSession.Close()

	ready := make(chan struct{}, 1)
	reporter := telemetry.ReporterFunc(func(event telemetry.Event) {
		if event.Type == "edge_listening" {
			ready <- struct{}{}
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runEdge(ctx, config.Config{Listen: "127.0.0.1:0"}, edgeConn, reporter)
	}()

	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("edge stopped before listening: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for edge listener")
	}

	if err := targetSession.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, errSessionClosed) {
			t.Fatalf("edge close error = %v, want errSessionClosed", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("edge did not detect the closed peer session")
	}
}
