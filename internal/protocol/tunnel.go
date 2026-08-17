package protocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Tunnel streams run inside the encrypted yamux session between one edge
// and one target. The first byte of every stream selects its kind, so the
// relay never sees any of this and either side can reject unknown kinds.
const (
	TunnelStreamData    byte = 0x01
	TunnelStreamControl byte = 0x02
)

// Dial status codes the target returns after a data-stream preamble.
const (
	TunnelDialOK      byte = 0
	TunnelDialUnknown byte = 1
	TunnelDialFailed  byte = 2
)

const (
	maxServiceIDLength = 128
	maxCatalogPayload  = 1 << 20
)

// CatalogService describes one published forwardable backend.
type CatalogService struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

// CatalogMessage carries the full service catalog. The target sends the
// complete list on session start and again after every change.
type CatalogMessage struct {
	Services []CatalogService `json:"services"`
}

// ReadTunnelStreamKind consumes the stream-kind byte.
func ReadTunnelStreamKind(r io.Reader) (byte, error) {
	var kind [1]byte
	if _, err := io.ReadFull(r, kind[:]); err != nil {
		return 0, err
	}
	return kind[0], nil
}

// WriteDataPreamble opens a data stream that asks the target to dial the
// published service with the given id.
func WriteDataPreamble(w io.Writer, serviceID string) error {
	id := []byte(serviceID)
	if len(id) == 0 || len(id) > maxServiceIDLength {
		return errors.New("service id must be between 1 and 128 bytes")
	}
	frame := make([]byte, 0, 2+len(id))
	frame = append(frame, TunnelStreamData, byte(len(id)))
	frame = append(frame, id...)
	_, err := w.Write(frame)
	return err
}

// ReadDataPreamble reads the service id after the kind byte was consumed.
func ReadDataPreamble(r io.Reader) (string, error) {
	var length [1]byte
	if _, err := io.ReadFull(r, length[:]); err != nil {
		return "", err
	}
	if length[0] == 0 {
		return "", errors.New("empty service id")
	}
	id := make([]byte, int(length[0]))
	if _, err := io.ReadFull(r, id); err != nil {
		return "", err
	}
	return string(id), nil
}

// WriteDialStatus reports the dial outcome for one data stream.
func WriteDialStatus(w io.Writer, status byte) error {
	_, err := w.Write([]byte{status})
	return err
}

// ReadDialStatus waits for the target's dial outcome.
func ReadDialStatus(r io.Reader) (byte, error) {
	var status [1]byte
	if _, err := io.ReadFull(r, status[:]); err != nil {
		return 0, err
	}
	return status[0], nil
}

// WriteControlHeader marks a stream as the catalog control stream.
func WriteControlHeader(w io.Writer) error {
	_, err := w.Write([]byte{TunnelStreamControl})
	return err
}

// WriteCatalog sends one length-prefixed catalog snapshot.
func WriteCatalog(w io.Writer, message CatalogMessage) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode catalog: %w", err)
	}
	if len(payload) > maxCatalogPayload {
		return errors.New("catalog is too large")
	}
	frame := make([]byte, 4, 4+len(payload))
	binary.BigEndian.PutUint32(frame, uint32(len(payload)))
	frame = append(frame, payload...)
	_, err = w.Write(frame)
	return err
}

// ReadCatalog reads one length-prefixed catalog snapshot.
func ReadCatalog(r io.Reader) (CatalogMessage, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return CatalogMessage{}, err
	}
	length := binary.BigEndian.Uint32(header[:])
	if length == 0 || length > maxCatalogPayload {
		return CatalogMessage{}, errors.New("invalid catalog frame length")
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(r, payload); err != nil {
		return CatalogMessage{}, err
	}
	var message CatalogMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		return CatalogMessage{}, fmt.Errorf("decode catalog: %w", err)
	}
	return message, nil
}
