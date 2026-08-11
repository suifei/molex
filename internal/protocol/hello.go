package protocol

import (
	"bytes"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

const HelloSize = 128

const (
	routeStart = 0
	roleIndex  = 32
	pubStart   = 33
	nonceStart = 65
	proofStart = 81
	padStart   = 113
)

type Role byte

const (
	RoleEdge   Role = 1
	RoleTarget Role = 2
)

func (r Role) Valid() bool {
	return r == RoleEdge || r == RoleTarget
}

func (r Role) Opposite() Role {
	if r == RoleEdge {
		return RoleTarget
	}
	return RoleEdge
}

func (r Role) String() string {
	if r == RoleEdge {
		return "edge"
	}
	if r == RoleTarget {
		return "target"
	}
	return "unknown"
}

type Hello struct {
	Packet  [HelloSize]byte
	Role    Role
	Route   [32]byte
	Public  [32]byte
	Nonce   [16]byte
	private *ecdh.PrivateKey
}

type SessionKeys struct {
	TxKey         [32]byte
	RxKey         [32]byte
	TxNoncePrefix [4]byte
	RxNoncePrefix [4]byte
	Transcript    [32]byte
	LocalRole     Role
}

func NewHello(secret []byte, channel string, role Role) (*Hello, error) {
	if len(secret) < 16 {
		return nil, errors.New("secret must contain at least 16 bytes")
	}
	if channel == "" {
		return nil, errors.New("channel is required")
	}
	if !role.Valid() {
		return nil, errors.New("invalid role")
	}

	private, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ephemeral key: %w", err)
	}
	hello := &Hello{Role: role, private: private}
	copy(hello.Public[:], private.PublicKey().Bytes())
	if _, err := rand.Read(hello.Nonce[:]); err != nil {
		return nil, fmt.Errorf("generate hello nonce: %w", err)
	}
	hello.Route = routeFor(secret, channel)

	copy(hello.Packet[routeStart:roleIndex], hello.Route[:])
	copy(hello.Packet[pubStart:nonceStart], hello.Public[:])
	copy(hello.Packet[nonceStart:proofStart], hello.Nonce[:])
	hello.Packet[roleIndex] = byte(role) ^ roleMask(hello.Route, hello.Public, hello.Nonce)
	proof := helloProof(secret, hello.Route, role, hello.Public, hello.Nonce)
	copy(hello.Packet[proofStart:padStart], proof[:])
	if _, err := rand.Read(hello.Packet[padStart:]); err != nil {
		return nil, fmt.Errorf("generate hello padding: %w", err)
	}
	return hello, nil
}

func ParseHello(packet []byte) (*Hello, error) {
	if len(packet) != HelloSize {
		return nil, fmt.Errorf("unexpected hello length %d", len(packet))
	}
	hello := &Hello{}
	copy(hello.Packet[:], packet)
	copy(hello.Route[:], packet[routeStart:roleIndex])
	copy(hello.Public[:], packet[pubStart:nonceStart])
	copy(hello.Nonce[:], packet[nonceStart:proofStart])
	hello.Role = Role(packet[roleIndex] ^ roleMask(hello.Route, hello.Public, hello.Nonce))
	if !hello.Role.Valid() {
		return nil, errors.New("invalid hello role")
	}
	return hello, nil
}

func (h *Hello) Derive(secret []byte, channel string, peerPacket []byte) (SessionKeys, error) {
	if h.private == nil {
		return SessionKeys{}, errors.New("local hello has no private key")
	}
	peer, err := ParseHello(peerPacket)
	if err != nil {
		return SessionKeys{}, err
	}
	expectedRoute := routeFor(secret, channel)
	if subtle.ConstantTimeCompare(peer.Route[:], expectedRoute[:]) != 1 ||
		subtle.ConstantTimeCompare(h.Route[:], expectedRoute[:]) != 1 {
		return SessionKeys{}, errors.New("peer joined a different channel")
	}
	if peer.Role != h.Role.Opposite() {
		return SessionKeys{}, errors.New("peer role does not complement local role")
	}
	expectedProof := helloProof(secret, peer.Route, peer.Role, peer.Public, peer.Nonce)
	if subtle.ConstantTimeCompare(peer.Packet[proofStart:padStart], expectedProof[:]) != 1 {
		return SessionKeys{}, errors.New("peer authentication failed")
	}

	peerPublic, err := ecdh.X25519().NewPublicKey(peer.Public[:])
	if err != nil {
		return SessionKeys{}, fmt.Errorf("parse peer key: %w", err)
	}
	shared, err := h.private.ECDH(peerPublic)
	if err != nil {
		return SessionKeys{}, fmt.Errorf("derive shared key: %w", err)
	}

	transcript := buildTranscript(h, peer)
	transcriptHash := sha256.Sum256(transcript)
	saltMAC := hmac.New(sha256.New, secret)
	saltMAC.Write([]byte("molex/session-salt/v1\x00"))
	saltMAC.Write(transcript)
	salt := saltMAC.Sum(nil)
	reader := hkdf.New(sha256.New, shared, salt, []byte("molex/record-keys/v1"))
	material := make([]byte, 72)
	if _, err := io.ReadFull(reader, material); err != nil {
		return SessionKeys{}, fmt.Errorf("expand session keys: %w", err)
	}

	var edgeTx, targetTx [32]byte
	var edgePrefix, targetPrefix [4]byte
	copy(edgeTx[:], material[:32])
	copy(targetTx[:], material[32:64])
	copy(edgePrefix[:], material[64:68])
	copy(targetPrefix[:], material[68:72])

	keys := SessionKeys{Transcript: transcriptHash, LocalRole: h.Role}
	if h.Role == RoleEdge {
		keys.TxKey, keys.RxKey = edgeTx, targetTx
		keys.TxNoncePrefix, keys.RxNoncePrefix = edgePrefix, targetPrefix
	} else {
		keys.TxKey, keys.RxKey = targetTx, edgeTx
		keys.TxNoncePrefix, keys.RxNoncePrefix = targetPrefix, edgePrefix
	}
	return keys, nil
}

func (k SessionKeys) Finished(role Role) [32]byte {
	mac := hmac.New(sha256.New, k.TxKey[:])
	if role != k.LocalRole {
		mac = hmac.New(sha256.New, k.RxKey[:])
	}
	mac.Write([]byte("molex/finished/v1\x00"))
	mac.Write([]byte{byte(role)})
	mac.Write(k.Transcript[:])
	var result [32]byte
	copy(result[:], mac.Sum(nil))
	return result
}

func routeFor(secret []byte, channel string) [32]byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("molex/rendezvous/v1\x00"))
	mac.Write([]byte(channel))
	var route [32]byte
	copy(route[:], mac.Sum(nil))
	return route
}

func roleMask(route [32]byte, public [32]byte, nonce [16]byte) byte {
	mac := hmac.New(sha256.New, route[:])
	mac.Write([]byte("role\x00"))
	mac.Write(public[:])
	mac.Write(nonce[:])
	return mac.Sum(nil)[0]
}

func helloProof(secret []byte, route [32]byte, role Role, public [32]byte, nonce [16]byte) [32]byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("molex/hello-proof/v1\x00"))
	mac.Write(route[:])
	mac.Write([]byte{byte(role)})
	mac.Write(public[:])
	mac.Write(nonce[:])
	var proof [32]byte
	copy(proof[:], mac.Sum(nil))
	return proof
}

func buildTranscript(local, peer *Hello) []byte {
	edge, target := local, peer
	if local.Role == RoleTarget {
		edge, target = peer, local
	}
	var transcript bytes.Buffer
	transcript.WriteString("molex/transcript/v1\x00")
	transcript.Write(edge.Route[:])
	transcript.Write(edge.Public[:])
	transcript.Write(target.Public[:])
	transcript.Write(edge.Nonce[:])
	transcript.Write(target.Nonce[:])
	return transcript.Bytes()
}
