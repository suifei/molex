package protocol

import (
	"bytes"
	"testing"
)

func TestRelayMetadataRoundTripIsPaddedAndAuthenticated(t *testing.T) {
	hello, err := NewHello([]byte("metadata-secret-material-123456"), "private-channel", RoleEdge)
	if err != nil {
		t.Fatal(err)
	}
	want := RelayMetadata{
		Name:          "Beijing edge",
		Endpoint:      "127.0.0.1:2222",
		RelayEndpoint: "wss://relay.example.com/ws/session",
		Platform:      "windows/amd64",
	}
	frames, err := SealRelayMetadata(hello, "relay-token-0123456789", want)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 4 {
		t.Fatalf("metadata frames = %d, want 4", len(frames))
	}
	for _, frame := range frames {
		if len(frame) != RelayMetadataFrameSize {
			t.Fatalf("metadata frame length = %d", len(frame))
		}
		if bytes.Contains(frame, []byte(want.Name)) || bytes.Contains(frame, []byte(want.Endpoint)) || bytes.Contains(frame, []byte("MoleX")) {
			t.Fatal("relay metadata frame exposed a plaintext marker")
		}
	}

	got := OpenRelayMetadata(hello, "relay-token-0123456789", frames)
	if got != want {
		t.Fatalf("metadata = %#v, want %#v", got, want)
	}
	if wrongToken := OpenRelayMetadata(hello, "different-relay-token", frames); wrongToken != (RelayMetadata{}) {
		t.Fatalf("wrong token decoded metadata: %#v", wrongToken)
	}

	frames[0][10] ^= 0x80
	tampered := OpenRelayMetadata(hello, "relay-token-0123456789", frames)
	if tampered.Name != "" || tampered.Endpoint != want.Endpoint || tampered.RelayEndpoint != want.RelayEndpoint {
		t.Fatalf("tampered metadata was not isolated by field: %#v", tampered)
	}
}

func TestRelayMetadataTruncatesAtUTF8Boundary(t *testing.T) {
	hello, err := NewHello([]byte("metadata-secret-material-123456"), "channel", RoleTarget)
	if err != nil {
		t.Fatal(err)
	}
	longName := "上海节点-" + string(bytes.Repeat([]byte("a"), 120))
	frames, err := SealRelayMetadata(hello, "", RelayMetadata{Name: longName})
	if err != nil {
		t.Fatal(err)
	}
	got := OpenRelayMetadata(hello, "", frames)
	if got.Name == "" || len([]byte(got.Name)) > relayMetadataValueMax {
		t.Fatalf("truncated name = %q (%d bytes)", got.Name, len([]byte(got.Name)))
	}
}
