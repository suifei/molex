package relay

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/suifei/molex/internal/protocol"
)

var errDuplicateRole = errors.New("a participant with this role is already waiting")

type participant struct {
	id          string
	ip          string
	proxied     bool
	metadata    protocol.RelayMetadata
	routeID     string
	connectedAt time.Time
	conn        *websocket.Conn
	hello       []byte
	role        protocol.Role
	route       [32]byte
	done        chan struct{}
	paired      chan struct{}
	reported    chan struct{}
	frames      chan []byte
	peer        atomic.Pointer[participant]
	bridgeOwner bool
	shutdown    sync.Once

	bytesReceived  atomic.Uint64
	bytesSent      atomic.Uint64
	framesReceived atomic.Uint64
	framesSent     atomic.Uint64
	lastActivity   atomic.Int64
	metricVersion  atomic.Uint64
}

type waitingPair struct {
	edge   *participant
	target *participant
}

type registry struct {
	mu      sync.Mutex
	waiting map[[32]byte]*waitingPair
}

func newRegistry() *registry {
	return &registry{waiting: make(map[[32]byte]*waitingPair)}
}

// join elects only the participant completing the pair to start the bridge.
func (r *registry) join(p *participant) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	pair := r.waiting[p.route]
	if pair == nil {
		pair = &waitingPair{}
		r.waiting[p.route] = pair
	}
	if pair.edge != nil && pair.edge.closed() {
		pair.edge = nil
	}
	if pair.target != nil && pair.target.closed() {
		pair.target = nil
	}
	if p.role == protocol.RoleEdge {
		if pair.edge != nil {
			return errDuplicateRole
		}
		pair.edge = p
	} else {
		if pair.target != nil {
			return errDuplicateRole
		}
		pair.target = p
	}
	if pair.edge != nil && pair.target != nil {
		delete(r.waiting, p.route)
		pair.edge.peer.Store(pair.target)
		pair.target.peer.Store(pair.edge)
		p.bridgeOwner = true
		close(pair.edge.paired)
		close(pair.target.paired)
	}
	return nil
}

func (p *participant) close() {
	p.closeWith(websocket.CloseNormalClosure, "")
}

func (p *participant) closeWith(code int, reason string) {
	p.shutdown.Do(func() {
		close(p.done)
		if code != 0 {
			_ = p.conn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(code, reason), time.Now().Add(time.Second))
		}
		_ = p.conn.Close()
	})
}

func (p *participant) abort() {
	p.closeWith(0, "")
}

func (p *participant) closed() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func (r *registry) remove(p *participant) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	pair := r.waiting[p.route]
	if pair == nil {
		return false
	}
	removed := false
	if pair.edge == p {
		pair.edge = nil
		removed = true
	}
	if pair.target == p {
		pair.target = nil
		removed = true
	}
	if pair.edge == nil && pair.target == nil {
		delete(r.waiting, p.route)
	}
	return removed
}
