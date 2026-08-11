package telemetry

import "time"

const (
	PeerActionUpsert = "upsert"
	PeerActionUpdate = "update"
	PeerActionRemove = "remove"

	PeerStatusWaiting = "waiting"
	PeerStatusPaired  = "paired"
)

type Peer struct {
	ID             string    `json:"id"`
	IP             string    `json:"ip"`
	Name           string    `json:"name,omitempty"`
	Role           string    `json:"role"`
	Status         string    `json:"status,omitempty"`
	Endpoint       string    `json:"endpoint,omitempty"`
	RelayEndpoint  string    `json:"relayEndpoint,omitempty"`
	Platform       string    `json:"platform,omitempty"`
	RouteID        string    `json:"routeId,omitempty"`
	PeerID         string    `json:"peerId,omitempty"`
	PeerName       string    `json:"peerName,omitempty"`
	Proxied        bool      `json:"proxied,omitempty"`
	ConnectedAt    time.Time `json:"connectedAt"`
	LastActivityAt time.Time `json:"lastActivityAt,omitempty,omitzero"`
	BytesReceived  uint64    `json:"bytesReceived,omitempty"`
	BytesSent      uint64    `json:"bytesSent,omitempty"`
	FramesReceived uint64    `json:"framesReceived,omitempty"`
	FramesSent     uint64    `json:"framesSent,omitempty"`
}

type PeerChange struct {
	Action string `json:"action"`
	Peers  []Peer `json:"peers"`
}

type Event struct {
	Type        string      `json:"type"`
	Level       string      `json:"level"`
	State       string      `json:"state,omitempty"`
	Message     string      `json:"message"`
	Listen      string      `json:"listen,omitempty"`
	Time        time.Time   `json:"time"`
	PeerChange  *PeerChange `json:"peerChange,omitempty"`
	Transient   bool        `json:"transient,omitempty"`
	ClearListen bool        `json:"-"`
}

type Reporter interface {
	Report(Event)
}

type ReporterFunc func(Event)

func (f ReporterFunc) Report(event Event) {
	if f != nil {
		f(event)
	}
}

func Emit(reporter Reporter, event Event) {
	if reporter == nil {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	reporter.Report(event)
}
