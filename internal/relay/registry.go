package relay

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/suifei/molex/internal/protocol"
)

type participant struct {
	id          string
	ip          string
	proxied     bool
	metadata    atomic.Pointer[protocol.RelayMetadata]
	tokenID     string
	legacyToken bool
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
	armedCode   atomic.Int32
	armedText   atomic.Value
	lastMeta    atomic.Int64
	metaMu      sync.Mutex
	pendingMeta [][]byte
	metaFlush   bool

	bytesReceived  atomic.Uint64
	bytesSent      atomic.Uint64
	framesReceived atomic.Uint64
	framesSent     atomic.Uint64
	lastActivity   atomic.Int64
	metricVersion  atomic.Uint64
}

// waitingQueue contains unpaired participants for one opaque route key.
// Keeping each role in arrival order makes multi-client rendezvous
// deterministic while preserving one independent encrypted session per pair.
type waitingQueue struct {
	edges   []*participant
	targets []*participant
}

type registry struct {
	mu      sync.Mutex
	waiting map[[32]byte]*waitingQueue
}

func newRegistry() *registry {
	return &registry{waiting: make(map[[32]byte]*waitingQueue)}
}

// join elects only the participant completing a pair to start the bridge.
// Participants of the same role are queued instead of rejected. The route
// key remains opaque, and every pair still performs its own hello/key
// exchange, so sessions cannot share payload keys or yamux state.
func (r *registry) join(p *participant) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	queue := r.waiting[p.route]
	if queue == nil {
		queue = &waitingQueue{}
		r.waiting[p.route] = queue
	}
	queue.edges = activeParticipants(queue.edges)
	queue.targets = activeParticipants(queue.targets)

	var peer *participant
	if p.role == protocol.RoleEdge {
		peer, queue.targets = popParticipant(queue.targets)
		if peer == nil {
			queue.edges = append(queue.edges, p)
		}
	} else {
		peer, queue.edges = popParticipant(queue.edges)
		if peer == nil {
			queue.targets = append(queue.targets, p)
		}
	}

	if peer != nil {
		if len(queue.edges) == 0 && len(queue.targets) == 0 {
			delete(r.waiting, p.route)
		}
		p.peer.Store(peer)
		peer.peer.Store(p)
		p.bridgeOwner = true
		close(p.paired)
		close(peer.paired)
	}
	return nil
}

func (p *participant) close() {
	p.closeWith(websocket.CloseNormalClosure, "")
}

// armClose records the close code and reason that must win no matter which
// concurrent path (bridge teardown, read failure, shutdown) closes this
// participant first. Kicks and token revocations use it so clients always
// receive the actionable close reason.
func (p *participant) armClose(code int, reason string) {
	if p.armedCode.CompareAndSwap(0, int32(code)) {
		p.armedText.Store(reason)
	}
}

func (p *participant) closeWith(code int, reason string) {
	p.shutdown.Do(func() {
		if armed := p.armedCode.Load(); armed != 0 {
			code = int(armed)
			if text, ok := p.armedText.Load().(string); ok {
				reason = text
			}
		}
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

func (p *participant) currentMetadata() protocol.RelayMetadata {
	if metadata := p.metadata.Load(); metadata != nil {
		return *metadata
	}
	return protocol.RelayMetadata{}
}

// mergeMetadata overwrites only the fields present in the refresh so a
// partial update cannot erase previously reported labels.
func (p *participant) mergeMetadata(update protocol.RelayMetadata) {
	current := p.currentMetadata()
	if update.Name != "" {
		current.Name = update.Name
	}
	if update.Endpoint != "" {
		current.Endpoint = update.Endpoint
	}
	if update.RelayEndpoint != "" {
		current.RelayEndpoint = update.RelayEndpoint
	}
	if update.Platform != "" {
		current.Platform = update.Platform
	}
	if update.Instance != "" {
		current.Instance = update.Instance
	}
	p.metadata.Store(&current)
}

func (r *registry) remove(p *participant) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	queue := r.waiting[p.route]
	if queue == nil {
		return false
	}
	var removed bool
	queue.edges, removed = removeParticipant(queue.edges, p)
	var targetRemoved bool
	queue.targets, targetRemoved = removeParticipant(queue.targets, p)
	removed = removed || targetRemoved
	if len(queue.edges) == 0 && len(queue.targets) == 0 {
		delete(r.waiting, p.route)
	}
	return removed
}

func activeParticipants(participants []*participant) []*participant {
	active := participants[:0]
	for _, participant := range participants {
		if participant != nil && !participant.closed() {
			active = append(active, participant)
		}
	}
	return active
}

func popParticipant(participants []*participant) (*participant, []*participant) {
	for len(participants) > 0 {
		participant := participants[0]
		participants = participants[1:]
		if participant != nil && !participant.closed() {
			return participant, participants
		}
	}
	return nil, nil
}

func removeParticipant(participants []*participant, wanted *participant) ([]*participant, bool) {
	removed := false
	kept := participants[:0]
	for _, participant := range participants {
		if participant == wanted {
			removed = true
			continue
		}
		kept = append(kept, participant)
	}
	return kept, removed
}

func (r *registry) waitingCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for route, queue := range r.waiting {
		queue.edges = activeParticipants(queue.edges)
		queue.targets = activeParticipants(queue.targets)
		count += len(queue.edges) + len(queue.targets)
		if len(queue.edges) == 0 && len(queue.targets) == 0 {
			delete(r.waiting, route)
		}
	}
	return count
}
