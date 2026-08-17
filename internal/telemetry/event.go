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
	TokenID        string    `json:"tokenId,omitempty"`
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

// CatalogService mirrors one published target service as seen by an edge.
// Group names the token group the entry was published through.
type CatalogService struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Group   string `json:"group,omitempty"`
}

// GroupCatalog is one token group's view of the published services.
type GroupCatalog struct {
	Group    string           `json:"group"`
	Online   bool             `json:"online"`
	Services []CatalogService `json:"services"`
}

// CatalogUpdate replaces the edge's view of the published service catalogs.
// Online reports whether at least one group route is ready; Services
// aggregates every group's entries and Groups carries the per-group state.
type CatalogUpdate struct {
	Online   bool             `json:"online"`
	Services []CatalogService `json:"services"`
	Groups   []GroupCatalog   `json:"groups,omitempty"`
}

// Mapping state values shown by the edge console.
const (
	MappingStateListening = "listening"
	MappingStateWaiting   = "waiting"
	MappingStateError     = "error"
)

// MappingStatus reports one edge mapping's listener state and counters.
type MappingStatus struct {
	Service     string    `json:"service"`
	Group       string    `json:"group,omitempty"`
	ServiceName string    `json:"serviceName,omitempty"`
	Address     string    `json:"address,omitempty"`
	Listen      string    `json:"listen,omitempty"`
	LAN         bool      `json:"lan,omitempty"`
	State       string    `json:"state"`
	Message     string    `json:"message,omitempty"`
	Connections uint64    `json:"connections,omitempty"`
	Bytes       uint64    `json:"bytes,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty,omitzero"`
}

// ServiceStatus reports one target service's publish state and counters.
// Groups repeats the configured visibility restriction (empty = all groups).
type ServiceStatus struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Address     string    `json:"address"`
	Groups      []string  `json:"groups,omitempty"`
	Streams     uint64    `json:"streams,omitempty"`
	LastError   string    `json:"lastError,omitempty"`
	LastErrorAt time.Time `json:"lastErrorAt,omitempty,omitzero"`
}

type PeerChange struct {
	Action string `json:"action"`
	Peers  []Peer `json:"peers"`
}

type Event struct {
	Type        string          `json:"type"`
	Level       string          `json:"level"`
	State       string          `json:"state,omitempty"`
	Message     string          `json:"message"`
	Listen      string          `json:"listen,omitempty"`
	Time        time.Time       `json:"time"`
	PeerChange  *PeerChange     `json:"peerChange,omitempty"`
	Catalog     *CatalogUpdate  `json:"catalog,omitempty"`
	Mappings    []MappingStatus `json:"mappings,omitempty"`
	Services    []ServiceStatus `json:"services,omitempty"`
	Transient   bool            `json:"transient,omitempty"`
	ClearListen bool            `json:"-"`
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
