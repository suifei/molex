package protocol

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type observedFrame struct {
	payload []byte
}

func TestRecordConnectionEncryptsAndAuthenticates(t *testing.T) {
	serverURL, frames, tamperNext, closeServer := startObservedRelay(t)
	defer closeServer()

	secret := []byte(strings.Repeat("record-secret-", 3))
	edge, target := openSecurePair(t, serverURL, secret)
	defer edge.Close()
	defer target.Close()
	drainObservedFrames(frames)

	canary := bytes.Repeat([]byte("plaintext-canary/"), 200)
	if _, err := edge.Write(canary); err != nil {
		t.Fatal(err)
	}
	received := make([]byte, len(canary))
	if _, err := io.ReadFull(target, received); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(received, canary) {
		t.Fatal("decrypted payload differs")
	}

	select {
	case frame := <-frames:
		if bytes.Contains(frame.payload, []byte("plaintext-canary")) {
			t.Fatal("encrypted WebSocket frame contains plaintext")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not observe encrypted frame")
	}

	drainObservedFrames(frames)
	tamperNext.Store(true)
	if _, err := edge.Write([]byte("tamper-test")); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 64)
	if _, err := target.Read(buffer); err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("expected authentication failure, got %v", err)
	}
}

func TestOpenSecureClientCancellationInterruptsPeerHello(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	connected := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		connected <- conn
	}))
	defer server.Close()
	defer server.CloseClientConnections()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := OpenSecureClient(ctx, ws, []byte("cancel-secret"), "cancel-channel", RoleTarget)
		result <- err
	}()

	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for WebSocket connection")
	}
	cancel()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected handshake cancellation error")
		}
	case <-time.After(time.Second):
		t.Fatal("handshake did not stop after context cancellation")
	}
}

func drainObservedFrames(frames <-chan observedFrame) {
	for {
		select {
		case <-frames:
		default:
			return
		}
	}
}

func openSecurePair(t *testing.T, serverURL string, secret []byte) (*RecordConn, *RecordConn) {
	t.Helper()
	type result struct {
		conn *RecordConn
		err  error
	}
	results := make(chan result, 2)
	open := func(role Role) {
		ws, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
		if err != nil {
			results <- result{err: err}
			return
		}
		conn, err := OpenSecureClient(context.Background(), ws, secret, "test-channel", role)
		results <- result{conn: conn, err: err}
	}
	go open(RoleEdge)
	go open(RoleTarget)
	a, b := <-results, <-results
	if a.err != nil {
		t.Fatal(a.err)
	}
	if b.err != nil {
		t.Fatal(b.err)
	}
	if a.conn == nil || b.conn == nil {
		t.Fatal("secure connection was nil")
	}
	// Results arrive in either order; directional behavior is symmetric for this test.
	return a.conn, b.conn
}

func startObservedRelay(t *testing.T) (string, <-chan observedFrame, *atomic.Bool, func()) {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	participants := make(chan *websocket.Conn, 2)
	frames := make(chan observedFrame, 16)
	tamperNext := &atomic.Bool{}
	done := make(chan struct{})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		participants <- conn
		<-done
	})
	server := httptest.NewServer(handler)

	go func() {
		a, b := <-participants, <-participants
		_, helloA, errA := a.ReadMessage()
		_, helloB, errB := b.ReadMessage()
		if errA != nil || errB != nil {
			close(done)
			return
		}
		_ = a.WriteMessage(websocket.BinaryMessage, helloB)
		_ = b.WriteMessage(websocket.BinaryMessage, helloA)
		copyFrames := func(dst, src *websocket.Conn) {
			for {
				kind, payload, err := src.ReadMessage()
				if err != nil {
					return
				}
				if len(payload) > 0 && tamperNext.CompareAndSwap(true, false) {
					payload = append([]byte(nil), payload...)
					payload[len(payload)-1] ^= 0x01
				}
				select {
				case frames <- observedFrame{payload: append([]byte(nil), payload...)}:
				default:
				}
				if err := dst.WriteMessage(kind, payload); err != nil {
					return
				}
			}
		}
		go copyFrames(b, a)
		go copyFrames(a, b)
	}()

	closeFn := func() {
		server.CloseClientConnections()
		select {
		case <-done:
		default:
			close(done)
		}
		server.Close()
	}
	return "ws" + strings.TrimPrefix(server.URL, "http"), frames, tamperNext, closeFn
}
