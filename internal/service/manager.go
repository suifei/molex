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
	State     string           `json:"state"`
	Mode      string           `json:"mode,omitempty"`
	Role      string           `json:"role,omitempty"`
	Listen    string           `json:"listen,omitempty"`
	Message   string           `json:"message,omitempty"`
	StartedAt time.Time        `json:"startedAt,omitempty"`
	Peers     []telemetry.Peer `json:"peers"`
}

type Manager struct {
	mu         sync.RWMutex
	status     Status
	events     []telemetry.Event
	peers      map[string]telemetry.Peer
	cancel     context.CancelFunc
	done       chan struct{}
	generation uint64
	external   telemetry.Reporter
	logger     *slog.Logger
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
		Role:      cfg.Role,
		Message:   "Starting",
		StartedAt: time.Now().UTC(),
	}
	m.mu.Unlock()

	reporter := telemetry.ReporterFunc(func(event telemetry.Event) {
		m.recordRuntimeEvent(generation, event)
	})
	go func() {
		defer close(done)
		var err error
		if cfg.Mode == config.ModeRelay {
			server := relay.New(relay.Options{
				Listen:   cfg.Listen,
				Token:    cfg.Token,
				Logger:   m.logger,
				Reporter: reporter,
			})
			err = server.Run(ctx)
		} else {
			err = client.Run(ctx, cfg, reporter)
		}

		m.mu.Lock()
		m.cancel = nil
		m.done = nil
		if err != nil && ctx.Err() == nil {
			m.status.State = "error"
			m.status.Message = err.Error()
		} else {
			m.status.State = "idle"
			m.status.Message = "Stopped"
			m.status.Listen = ""
		}
		clear(m.peers)
		m.mu.Unlock()

		if err != nil && ctx.Err() == nil {
			m.recordRuntimeEvent(generation, telemetry.Event{Type: "runtime_error", Level: "error", State: "error", Message: err.Error()})
		}
	}()
	return nil
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
	return status
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
	m.mu.Unlock()
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
