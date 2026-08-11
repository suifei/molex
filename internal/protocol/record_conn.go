package protocol

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const maxPlaintextRecord = 64 << 10

type RecordConn struct {
	ws       *websocket.Conn
	tx       cipher.AEAD
	rx       cipher.AEAD
	txPrefix [4]byte
	rxPrefix [4]byte

	writeMu sync.Mutex
	readMu  sync.Mutex
	txSeq   uint64
	rxSeq   uint64
	readBuf bytes.Reader
	close   sync.Once
}

func OpenSecureClient(ctx context.Context, ws *websocket.Conn, secret []byte, channel string, role Role) (*RecordConn, error) {
	return OpenSecureClientWithMetadata(ctx, ws, secret, channel, role, "", RelayMetadata{})
}

func OpenSecureClientWithMetadata(ctx context.Context, ws *websocket.Conn, secret []byte, channel string, role Role, relayToken string, metadata RelayMetadata) (*RecordConn, error) {
	hello, err := NewHello(secret, channel, role)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(15 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := ws.SetWriteDeadline(deadline); err != nil {
		return nil, err
	}
	metadataFrames, err := SealRelayMetadata(hello, relayToken, metadata)
	if err != nil {
		return nil, fmt.Errorf("prepare relay metadata: %w", err)
	}
	for _, frame := range metadataFrames {
		if err := ws.WriteControl(websocket.PingMessage, frame, deadline); err != nil {
			return nil, fmt.Errorf("send relay metadata: %w", err)
		}
	}
	if err := ws.WriteMessage(websocket.BinaryMessage, hello.Packet[:]); err != nil {
		return nil, fmt.Errorf("send client hello: %w", err)
	}
	if err := ws.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	messageType, peerPacket, err := ws.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("receive peer hello: %w", err)
	}
	if messageType != websocket.BinaryMessage {
		return nil, errors.New("peer hello was not a binary WebSocket frame")
	}
	keys, err := hello.Derive(secret, channel, peerPacket)
	if err != nil {
		return nil, err
	}
	conn, err := newRecordConn(ws, keys)
	if err != nil {
		return nil, err
	}
	if err := conn.exchangeFinished(keys); err != nil {
		conn.Close()
		return nil, err
	}
	conn.SetDeadline(time.Time{})
	return conn, nil
}

func newRecordConn(ws *websocket.Conn, keys SessionKeys) (*RecordConn, error) {
	txBlock, err := aes.NewCipher(keys.TxKey[:])
	if err != nil {
		return nil, err
	}
	rxBlock, err := aes.NewCipher(keys.RxKey[:])
	if err != nil {
		return nil, err
	}
	tx, err := cipher.NewGCM(txBlock)
	if err != nil {
		return nil, err
	}
	rx, err := cipher.NewGCM(rxBlock)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(maxPlaintextRecord + int64(rx.Overhead()))
	return &RecordConn{
		ws:       ws,
		tx:       tx,
		rx:       rx,
		txPrefix: keys.TxNoncePrefix,
		rxPrefix: keys.RxNoncePrefix,
	}, nil
}

func (c *RecordConn) exchangeFinished(keys SessionKeys) error {
	local := keys.Finished(keys.LocalRole)
	if _, err := c.Write(local[:]); err != nil {
		return fmt.Errorf("send key confirmation: %w", err)
	}
	peerBytes := make([]byte, 32)
	if _, err := io.ReadFull(c, peerBytes); err != nil {
		return fmt.Errorf("receive key confirmation: %w", err)
	}
	expected := keys.Finished(keys.LocalRole.Opposite())
	if subtle.ConstantTimeCompare(peerBytes, expected[:]) != 1 {
		return errors.New("session key confirmation failed")
	}
	return nil
}

func (c *RecordConn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	if c.readBuf.Len() > 0 {
		return c.readBuf.Read(p)
	}
	for {
		messageType, ciphertext, err := c.ws.ReadMessage()
		if err != nil {
			return 0, err
		}
		if messageType != websocket.BinaryMessage {
			return 0, errors.New("received non-binary data frame")
		}
		if c.rxSeq == math.MaxUint64 {
			return 0, errors.New("receive nonce space exhausted")
		}
		nonce, aad := recordNonce(c.rxPrefix, c.rxSeq)
		plaintext, err := c.rx.Open(nil, nonce, ciphertext, aad)
		if err != nil {
			return 0, errors.New("decrypt record: authentication failed")
		}
		c.rxSeq++
		if len(plaintext) == 0 {
			continue
		}
		c.readBuf.Reset(plaintext)
		return c.readBuf.Read(p)
	}
}

func (c *RecordConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	written := 0
	for len(p) > 0 {
		if c.txSeq == math.MaxUint64 {
			return written, errors.New("send nonce space exhausted")
		}
		chunkSize := min(len(p), maxPlaintextRecord)
		chunk := p[:chunkSize]
		nonce, aad := recordNonce(c.txPrefix, c.txSeq)
		ciphertext := c.tx.Seal(nil, nonce, chunk, aad)
		if err := c.ws.WriteMessage(websocket.BinaryMessage, ciphertext); err != nil {
			return written, err
		}
		c.txSeq++
		written += chunkSize
		p = p[chunkSize:]
	}
	return written, nil
}

func (c *RecordConn) Close() error {
	var err error
	c.close.Do(func() {
		err = c.ws.Close()
	})
	return err
}

func (c *RecordConn) LocalAddr() net.Addr {
	return c.ws.UnderlyingConn().LocalAddr()
}

func (c *RecordConn) RemoteAddr() net.Addr {
	return c.ws.UnderlyingConn().RemoteAddr()
}

func (c *RecordConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

func (c *RecordConn) SetReadDeadline(t time.Time) error {
	return c.ws.SetReadDeadline(t)
}

func (c *RecordConn) SetWriteDeadline(t time.Time) error {
	return c.ws.SetWriteDeadline(t)
}

func recordNonce(prefix [4]byte, sequence uint64) ([]byte, []byte) {
	nonce := make([]byte, 12)
	copy(nonce, prefix[:])
	binary.BigEndian.PutUint64(nonce[4:], sequence)
	aad := make([]byte, 8)
	binary.BigEndian.PutUint64(aad, sequence)
	return nonce, aad
}
