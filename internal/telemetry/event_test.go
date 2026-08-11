package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPeerJSONOmitsZeroLastActivity(t *testing.T) {
	peer := Peer{
		ID:          "1",
		IP:          "127.0.0.1",
		Role:        "edge",
		ConnectedAt: time.Now().UTC(),
	}
	encoded, err := json.Marshal(peer)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "lastActivityAt") {
		t.Fatalf("zero last activity was serialized: %s", encoded)
	}
}
