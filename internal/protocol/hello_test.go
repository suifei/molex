package protocol

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelloDerivesComplementaryKeys(t *testing.T) {
	secret := []byte(strings.Repeat("shared-secret-", 3))
	edge, err := NewHello(secret, "home-ssh", RoleEdge)
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewHello(secret, "home-ssh", RoleTarget)
	if err != nil {
		t.Fatal(err)
	}

	edgeKeys, err := edge.Derive(secret, "home-ssh", target.Packet[:])
	if err != nil {
		t.Fatal(err)
	}
	targetKeys, err := target.Derive(secret, "home-ssh", edge.Packet[:])
	if err != nil {
		t.Fatal(err)
	}
	if edgeKeys.TxKey != targetKeys.RxKey || edgeKeys.RxKey != targetKeys.TxKey {
		t.Fatal("directional keys are not complementary")
	}
	if edgeKeys.TxNoncePrefix != targetKeys.RxNoncePrefix || edgeKeys.Transcript != targetKeys.Transcript {
		t.Fatal("session metadata does not match")
	}
}

func TestHelloHidesLiteralInputsAndAuthenticatesPeer(t *testing.T) {
	secret := []byte("correct horse battery staple")
	channel := "visible-channel-canary"
	hello, err := NewHello(secret, channel, RoleEdge)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(hello.Packet[:], secret) || bytes.Contains(hello.Packet[:], []byte(channel)) || bytes.Contains(hello.Packet[:], []byte("molex")) {
		t.Fatal("hello packet leaked a literal protocol input")
	}

	peer, err := NewHello(secret, channel, RoleTarget)
	if err != nil {
		t.Fatal(err)
	}
	peer.Packet[proofStart] ^= 0x80
	if _, err := hello.Derive(secret, channel, peer.Packet[:]); err == nil {
		t.Fatal("tampered peer proof was accepted")
	}
}

func TestHelloRejectsWrongSecretAndSameRole(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	edge, _ := NewHello(secret, "channel", RoleEdge)
	target, _ := NewHello([]byte("different-secret-material-123456"), "channel", RoleTarget)
	if _, err := edge.Derive(secret, "channel", target.Packet[:]); err == nil {
		t.Fatal("peer with wrong secret was accepted")
	}

	otherEdge, _ := NewHello(secret, "channel", RoleEdge)
	if _, err := edge.Derive(secret, "channel", otherEdge.Packet[:]); err == nil {
		t.Fatal("peer with duplicate role was accepted")
	}
}
