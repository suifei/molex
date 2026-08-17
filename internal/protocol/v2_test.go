package protocol

import (
	"bytes"
	"testing"
)

func TestDeriveCredentialsIsDeterministicPerToken(t *testing.T) {
	secretA, channelA := DeriveCredentials("mx2_token-alpha-0123456789")
	secretB, channelB := DeriveCredentials("mx2_token-alpha-0123456789")
	if !bytes.Equal(secretA, secretB) || channelA != channelB {
		t.Fatal("same token derived different credentials")
	}
	if len(secretA) != 32 {
		t.Fatalf("derived secret length = %d, want 32", len(secretA))
	}
	otherSecret, _ := DeriveCredentials("mx2_token-beta-0123456789")
	if bytes.Equal(secretA, otherSecret) {
		t.Fatal("different tokens derived the same secret")
	}
}

func TestDerivedSecretNeverEqualsToken(t *testing.T) {
	token := "mx2_token-alpha-0123456789-padded-to-32"
	secret, _ := DeriveCredentials(token)
	if bytes.Contains(secret, []byte(token[:8])) {
		t.Fatal("derived secret leaks token bytes")
	}
}

func TestRouteForTokenMatchesClientHelloRoute(t *testing.T) {
	token := "mx2_token-alpha-0123456789"
	secret, channel := DeriveCredentials(token)
	hello, err := NewHello(secret, channel, RoleEdge)
	if err != nil {
		t.Fatal(err)
	}
	expected := RouteForToken(token)
	if hello.Route != expected {
		t.Fatal("relay-side route differs from the client hello route")
	}
	other := RouteForToken("mx2_token-beta-0123456789")
	if other == expected {
		t.Fatal("two tokens produced the same route")
	}
}

func TestHellosFromDifferentTokensCannotDerive(t *testing.T) {
	secretA, channelA := DeriveCredentials("mx2_token-alpha-0123456789")
	secretB, channelB := DeriveCredentials("mx2_token-beta-0123456789")
	edge, err := NewHello(secretA, channelA, RoleEdge)
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewHello(secretB, channelB, RoleTarget)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := edge.Derive(secretA, channelA, target.Packet[:]); err == nil {
		t.Fatal("cross-token hellos derived a session")
	}
}
