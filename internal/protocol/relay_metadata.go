package protocol

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"unicode"
	"unicode/utf8"
)

// RelayMetadataFrameSize is the maximum WebSocket control-frame payload. The
// fixed size keeps client metadata indistinguishable from padded binary data.
const RelayMetadataFrameSize = 125

const (
	relayMetadataVersion   = 1
	relayMetadataNonceSize = 12
	// version + length + value must fit the sealed plaintext after the
	// kind byte, the in-the-clear nonce, and the GCM tag.
	relayMetadataValueMax = RelayMetadataFrameSize - 1 - relayMetadataNonceSize - 16 - 2

	relayMetadataName byte = iota + 1
	relayMetadataEndpoint
	relayMetadataRelay
	relayMetadataPlatform
	relayMetadataInstance
)

// RelayMetadata contains operational labels that an authenticated Relay may
// display. It deliberately excludes the channel, route key, payload secret,
// and relay token. Instance identifies one target process so the relay can
// reject a second target joining the same token while the first is alive.
type RelayMetadata struct {
	Name          string
	Endpoint      string
	RelayEndpoint string
	Platform      string
	Instance      string
}

// SealRelayMetadata produces fixed-size encrypted WebSocket ping payloads.
// Sending these before the 128-byte hello remains compatible with older
// relays, whose standard ping handler simply acknowledges and ignores them.
func SealRelayMetadata(hello *Hello, relayToken string, metadata RelayMetadata) ([][]byte, error) {
	if hello == nil {
		return nil, nil
	}
	fields := []struct {
		kind  byte
		value string
	}{
		{relayMetadataName, metadata.Name},
		{relayMetadataEndpoint, metadata.Endpoint},
		{relayMetadataRelay, metadata.RelayEndpoint},
		{relayMetadataPlatform, metadata.Platform},
		{relayMetadataInstance, metadata.Instance},
	}
	key := relayMetadataKey(hello.Route, relayToken)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	frames := make([][]byte, 0, len(fields))
	for _, field := range fields {
		value := truncateRelayMetadataValue(field.value)
		if len(value) == 0 {
			continue
		}
		plaintext := make([]byte, RelayMetadataFrameSize-1-relayMetadataNonceSize-aead.Overhead())
		if _, err := rand.Read(plaintext); err != nil {
			return nil, err
		}
		plaintext[0] = relayMetadataVersion
		plaintext[1] = byte(len(value))
		copy(plaintext[2:], value)

		nonce := make([]byte, relayMetadataNonceSize)
		if _, err := rand.Read(nonce); err != nil {
			return nil, err
		}
		maskedKind := field.kind ^ key[0]
		frame := make([]byte, 1+relayMetadataNonceSize, RelayMetadataFrameSize)
		frame[0] = maskedKind
		copy(frame[1:], nonce)
		aad := relayMetadataAAD(hello, maskedKind)
		frame = aead.Seal(frame, nonce, plaintext, aad)
		frames = append(frames, frame)
	}
	return frames, nil
}

// OpenRelayMetadata ignores malformed or unrelated pings so ordinary
// WebSocket keepalives cannot prevent a client from joining.
func OpenRelayMetadata(hello *Hello, relayToken string, frames [][]byte) RelayMetadata {
	var metadata RelayMetadata
	if hello == nil {
		return metadata
	}
	key := relayMetadataKey(hello.Route, relayToken)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return metadata
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return metadata
	}

	for _, frame := range frames {
		if len(frame) != RelayMetadataFrameSize {
			continue
		}
		kind := frame[0] ^ key[0]
		if kind < relayMetadataName || kind > relayMetadataInstance {
			continue
		}
		nonce := frame[1 : 1+relayMetadataNonceSize]
		plaintext, err := aead.Open(nil, nonce, frame[1+relayMetadataNonceSize:], relayMetadataAAD(hello, frame[0]))
		if err != nil || len(plaintext) != RelayMetadataFrameSize-1-relayMetadataNonceSize-aead.Overhead() || plaintext[0] != relayMetadataVersion {
			continue
		}
		length := int(plaintext[1])
		if length > relayMetadataValueMax || 2+length > len(plaintext) {
			continue
		}
		value := string(plaintext[2 : 2+length])
		if !validRelayMetadataValue(value) {
			continue
		}
		switch kind {
		case relayMetadataName:
			metadata.Name = value
		case relayMetadataEndpoint:
			metadata.Endpoint = value
		case relayMetadataRelay:
			metadata.RelayEndpoint = value
		case relayMetadataPlatform:
			metadata.Platform = value
		case relayMetadataInstance:
			metadata.Instance = value
		}
	}
	return metadata
}

func relayMetadataKey(route [32]byte, relayToken string) [32]byte {
	mac := hmac.New(sha256.New, route[:])
	mac.Write([]byte("molex/relay-metadata-key/v1\x00"))
	mac.Write([]byte(relayToken))
	var key [32]byte
	copy(key[:], mac.Sum(nil))
	return key
}

func relayMetadataAAD(hello *Hello, maskedKind byte) []byte {
	aad := make([]byte, 0, HelloSize+1)
	aad = append(aad, hello.Packet[:]...)
	aad = append(aad, maskedKind)
	return aad
}

func truncateRelayMetadataValue(value string) []byte {
	if !validRelayMetadataValue(value) {
		return nil
	}
	encoded := []byte(value)
	if len(encoded) <= relayMetadataValueMax {
		return encoded
	}
	encoded = encoded[:relayMetadataValueMax]
	for !utf8.Valid(encoded) {
		encoded = encoded[:len(encoded)-1]
	}
	return encoded
}

func validRelayMetadataValue(value string) bool {
	if value == "" || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
