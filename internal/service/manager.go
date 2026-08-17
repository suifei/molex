package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/suifei/molex/internal/client"
	"github.com/suifei/molex/internal/config"
	"github.com/suifei/molex/internal/relay"
	"github.com/suifei/molex/internal/telemetry"
)

const maxEvents = 100

type Status struct {
	State     string                    `json:"state"`
	Mode      string                    `json:"mode,omitempty"`
	Listen    string                    `json:"listen,omitempty"`
	Message   string                    `json:"message,omitempty"`
	StartedAt time.Time                 `json:"startedAt,omitempty"`
	Peers     []telemetry.Peer          `json:"peers"`
	Catalog   *telemetry.CatalogUpdate  `json:"catalog,omitempty"`
	Mappings  []telemetry.MappingStatus `json:"mappings,omitempty"`
	Services  []telemetry.ServiceStatus `json:"services,omitempty"`
}

type Manager struct {
	mu             sync.RWMutex
	status         Status
	events         []telemetry.Event
	peers          map[string]telemetry.Peer
	cancel         context.CancelFunc
	done           chan struct{}
	generation     uint64
	relayServer    *relay.Server
	serviceUpdates chan []config.ServiceEntry
	mappingUpdates chan []config.MappingEntry
	external       telemetry.Reporter
	audit          *telemetry.AuditWriter
	logger         *slog.Logger
}

// SetAuditPath enables the durable relay audit log at the given location.
func (m *Manager) SetAuditPath(path string) {
	if path == "" {
		return
	}
	m.mu.Lock()
	m.audit = telemetry.NewAuditWriter(path)
	m.mu.Unlock()
}

// RecordAudit stores an administrative action (token create, rotate,
// disable, delete, kick) in the activity feed and the durable audit log.
func (m *Manager) RecordAudit(event telemetry.Event) {
	m.recordEvent(event)
}

func NewManager(reporter telemetry.Reporter, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		status:   Status{State: "idle", Message: "Ready"},
		peers:    make(map[string]telemetry.Peer),
		external: reporter,
		logger:   logger,
	}
}

func (m *Manager) Start(cfg config.Config) error {
	cfg = cfg.Normalized()
	if err := cfg.Validate(); err != nil {
		return err
	}

	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return errors.New("MoleX is already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	m.generation++
	generation := m.generation
	m.cancel = cancel
	m.done = done
	clear(m.peers)
	m.status = Status{
		State:     "starting",
		Mode:      cfg.Mode,
		Message:   "Starting",
		StartedAt: time.Now().UTC(),
	}

	var relayServer *relay.Server
	var serviceUpdates chan []config.ServiceEntry
	var mappingUpdates chan []config.MappingEntry
	switch cfg.Mode {
	case config.ModeRelay:
		relayServer = relay.New(relay.Options{
			Listen:   cfg.Listen,
			Tokens:   relayCredentials(cfg.Tokens),
			Logger:   m.logger,
			Reporter: telemetry.ReporterFunc(func(event telemetry.Event) { m.recordRuntimeEvent(generation, event) }),
		})
		m.relayServer = relayServer
	case config.ModeTarget:
		serviceUpdates = make(chan []config.ServiceEntry, 1)
		m.serviceUpdates = serviceUpdates
		m.status.Services = initialServiceStatuses(cfg.Services)
	case config.ModeEdge:
		mappingUpdates = make(chan []config.MappingEntry, 1)
		m.mappingUpdates = mappingUpdates
		m.status.Mappings = initialMappingStatuses(cfg.Mappings)
		m.status.Catalog = &telemetry.CatalogUpdate{Online: false, Services: []telemetry.CatalogService{}}
	}
	m.mu.Unlock()

	reporter := telemetry.ReporterFunc(func(event telemetry.Event) {
		m.recordRuntimeEvent(generation, event)
	})
	go func() {
		defer close(done)
		var err error
		if cfg.Mode == config.ModeRelay {
			err = relayServer.Run(ctx)
		} else {
			err = client.RunWithUpdates(ctx, cfg, reporter, client.Updates{
				Services: serviceUpdates,
				Mappings: mappingUpdates,
			})
		}

		m.mu.Lock()
		m.cancel = nil
		m.done = nil
		m.relayServer = nil
		m.serviceUpdates = nil
		m.mappingUpdates = nil
		if err != nil && ctx.Err() == nil {
			m.status.State = "error"
			m.status.Message = err.Error()
		} else {
			m.status.State = "idle"
			m.status.Message = "Stopped"
			m.status.Listen = ""
		}
		m.status.Catalog = nil
		m.status.Mappings = nil
		m.status.Services = nil
		clear(m.peers)
		m.mu.Unlock()

		if err != nil && ctx.Err() == nil {
			m.recordRuntimeEvent(generation, telemetry.Event{Type: "runtime_error", Level: "error", State: "error", Message: err.Error()})
		}
	}()
	return nil
}

func relayCredentials(tokens []config.TokenEntry) []relay.Credential {
	credentials := make([]relay.Credential, 0, len(tokens))
	for _, token := range tokens {
		credentials = append(credentials, relay.Credential{
			ID:              token.ID,
			Token:           token.Token,
			Disabled:        token.Disabled,
			Previous:        token.PreviousToken,
			PreviousExpires: token.PreviousExpiresAt,
		})
	}
	return credentials
}

func initialServiceStatuses(services []config.ServiceEntry) []telemetry.ServiceStatus {
	statuses := make([]telemetry.ServiceStatus, 0, len(services))
	for _, service := range services {
		statuses = append(statuses, telemetry.ServiceStatus{
			ID:      service.ID,
			Name:    service.Name,
			Address: service.Address,
		})
	}
	return statuses
}

func initialMappingStatuses(mappings []config.MappingEntry) []telemetry.MappingStatus {
	statuses := make([]telemetry.MappingStatus, 0, len(mappings))
	for _, mapping := range mappings {
		statuses = append(statuses, telemetry.MappingStatus{
			Service: mapping.Service,
			LAN:     mapping.LAN,
			State:   telemetry.MappingStateWaiting,
			Message: "Waiting for the encrypted route",
		})
	}
	return statuses
}

// UpdateTokens pushes the latest token list into a running relay.
func (m *Manager) UpdateTokens(tokens []config.TokenEntry) {
	m.mu.RLock()
	relayServer := m.relayServer
	m.mu.RUnlock()
	if relayServer != nil {
		relayServer.UpdateTokens(relayCredentials(tokens))
	}
}

// UpdateServices pushes the latest service list into a running target.
func (m *Manager) UpdateServices(services []config.ServiceEntry) {
	m.mu.RLock()
	updates := m.serviceUpdates
	m.mu.RUnlock()
	if updates == nil {
		return
	}
	replaceChannelValue(updates, append([]config.ServiceEntry(nil), services...))
}

// UpdateMappings pushes the latest mapping list into a running edge.
func (m *Manager) UpdateMappings(mappings []config.MappingEntry) {
	m.mu.RLock()
	updates := m.mappingUpdates
	m.mu.RUnlock()
	if updates == nil {
		return
	}
	replaceChannelValue(updates, append([]config.MappingEntry(nil), mappings...))
}

// DisconnectPeer asks a running relay to close one connected client.
func (m *Manager) DisconnectPeer(peerID string) bool {
	m.mu.RLock()
	relayServer := m.relayServer
	m.mu.RUnlock()
	if relayServer == nil {
		return false
	}
	return relayServer.DisconnectPeer(peerID)
}

// replaceChannelValue keeps only the newest pending update in a buffered
// channel so a busy runtime never blocks configuration saves.
func replaceChannelValue[T any](channel chan T, value T) {
	for {
		select {
		case channel <- value:
			return
		default:
			select {
			case <-channel:
			default:
			}
		}
	}
}

func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	if m.cancel == nil {
		m.mu.Unlock()
		return nil
	}
	cancel := m.cancel
	done := m.done
	m.status.State = "stopping"
	m.status.Message = "Stopping"
	m.mu.Unlock()

	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop MoleX: %w", ctx.Err())
	}
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := m.status
	status.Peers = make([]telemetry.Peer, 0, len(m.peers))
	for _, peer := range m.peers {
		status.Peers = append(status.Peers, peer)
	}
	sort.Slice(status.Peers, func(i, j int) bool {
		left, right := status.Peers[i], status.Peers[j]
		if !left.ConnectedAt.Equal(right.ConnectedAt) {
			return left.ConnectedAt.Before(right.ConnectedAt)
		}
		if left.Role != right.Role {
			return left.Role < right.Role
		}
		if left.IP != right.IP {
			return left.IP < right.IP
		}
		return left.ID < right.ID
	})
	status.Catalog = cloneCatalog(m.status.Catalog)
	status.Mappings = append([]telemetry.MappingStatus(nil), m.status.Mappings...)
	status.Services = append([]telemetry.ServiceStatus(nil), m.status.Services...)
	return status
}

func cloneCatalog(catalog *telemetry.CatalogUpdate) *telemetry.CatalogUpdate {
	if catalog == nil {
		return nil
	}
	clone := &telemetry.CatalogUpdate{
		Online:   catalog.Online,
		Services: append([]telemetry.CatalogService(nil), catalog.Services...),
	}
	return clone
}

func (m *Manager) Running() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cancel != nil
}

func (m *Manager) Events() []telemetry.Event {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]telemetry.Event, len(m.events))
	copy(result, m.events)
	return result
}

func (m *Manager) recordEvent(event telemetry.Event) {
	m.recordEventForGeneration(0, false, event)
}

func (m *Manager) recordRuntimeEvent(generation uint64, event telemetry.Event) {
	m.recordEventForGeneration(generation, true, event)
}

func (m *Manager) recordEventForGeneration(generation uint64, enforceGeneration bool, event telemetry.Event) {
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	if event.PeerChange != nil {
		change := *event.PeerChange
		change.Peers = append([]telemetry.Peer(nil), event.PeerChange.Peers...)
		event.PeerChange = &change
	}
	m.mu.Lock()
	if enforceGeneration && generation != m.generation {
		m.mu.Unlock()
		return
	}
	if !event.Transient {
		m.events = append(m.events, event)
		if len(m.events) > maxEvents {
			copy(m.events, m.events[len(m.events)-maxEvents:])
			m.events = m.events[:maxEvents]
		}
	}
	if event.PeerChange != nil {
		m.applyPeerChange(*event.PeerChange)
	} else {
		if event.State != "" {
			m.status.State = event.State
		}
		if event.Message != "" {
			m.status.Message = event.Message
		}
		if event.ClearListen {
			m.status.Listen = ""
		} else if event.Listen != "" {
			m.status.Listen = event.Listen
		}
	}
	if event.Catalog != nil {
		m.status.Catalog = cloneCatalog(event.Catalog)
	}
	if event.Mappings != nil {
		m.status.Mappings = append([]telemetry.MappingStatus(nil), event.Mappings...)
	}
	if event.Services != nil {
		m.status.Services = append([]telemetry.ServiceStatus(nil), event.Services...)
	}
	audit := m.audit
	m.mu.Unlock()
	if audit != nil {
		audit.Report(event)
	}
	telemetry.Emit(m.external, event)
}

func (m *Manager) applyPeerChange(change telemetry.PeerChange) {
	for _, peer := range change.Peers {
		if peer.ID == "" {
			continue
		}
		switch change.Action {
		case telemetry.PeerActionUpsert:
			m.peers[peer.ID] = peer
		case telemetry.PeerActionUpdate:
			if _, ok := m.peers[peer.ID]; ok {
				m.peers[peer.ID] = peer
			}
		case telemetry.PeerActionRemove:
			delete(m.peers, peer.ID)
		}
	}
}
