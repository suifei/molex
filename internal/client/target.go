package client

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/suifei/molex/internal/config"
	"github.com/suifei/molex/internal/protocol"
	"github.com/suifei/molex/internal/telemetry"
)

// maxTargetSessions bounds each group's adaptive session pool: every paired
// edge causes the target to add one hot-standby session for the next edge.
const maxTargetSessions = 65535

func runTargetProcess(ctx context.Context, cfg config.Config, reporter telemetry.Reporter, retry retrySettings, serviceUpdates <-chan []config.ServiceEntry) error {
	groups := cfg.GroupTokens()
	rt := newTargetRuntime(cfg, reporter)

	if serviceUpdates != nil {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case services, ok := <-serviceUpdates:
					if !ok {
						return
					}
					rt.setServices(services)
				}
			}
		}()
	}
	go rt.housekeeping(ctx)

	telemetry.Emit(reporter, telemetry.Event{
		Type:        "client_connecting",
		Level:       "info",
		State:       "connecting",
		Message:     fmt.Sprintf("Connecting adaptive Target session pool (up to %d sessions)", maxTargetSessions),
		ClearListen: true,
		Services:    rt.serviceStatuses(),
	})

	aggregator := newGroupStateAggregator(reporter)
	var workers sync.WaitGroup
	for _, group := range groups {
		group := group
		groupReporter := reporter
		if len(groups) > 1 {
			groupReporter = aggregator.group(group.ID)
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			runTargetGroupPool(ctx, cfg, group, rt, groupReporter, retry)
		}()
	}
	workers.Wait()
	return nil
}

// runTargetGroupPool keeps one token group's adaptive hot-standby pool
// running: a new session slot is opened each time an existing slot pairs.
func runTargetGroupPool(ctx context.Context, cfg config.Config, group config.TokenEntry, rt *targetRuntime, reporter telemetry.Reporter, retry retrySettings) {
	pool := newTargetPoolReporter(maxTargetSessions, reporter)
	spec := sessionSpec{
		cfg:   cfg,
		role:  protocol.RoleTarget,
		token: group.Token,
		metadata: func() protocol.RelayMetadata {
			return rt.metadataFor(group.ID)
		},
		handler: func(ctx context.Context, conn net.Conn, sessionReporter telemetry.Reporter) error {
			return rt.runSession(ctx, conn, sessionReporter, group.ID)
		},
	}

	var workers sync.WaitGroup
	var launchMu sync.Mutex
	started := 0
	var launch func()
	launch = func() {
		launchMu.Lock()
		if started >= maxTargetSessions || ctx.Err() != nil {
			launchMu.Unlock()
			return
		}
		slot := started
		started++
		workers.Add(1)
		launchMu.Unlock()
		go func() {
			defer workers.Done()
			_ = runSessionLoop(ctx, spec, pool.slot(slot), retry)
		}()
	}
	pool.onReady = func() {
		launch()
	}
	launch()
	workers.Wait()
}

type targetPoolReporter struct {
	mu       sync.Mutex
	total    int
	ready    map[int]bool
	expanded map[int]bool
	reporter telemetry.Reporter
	onReady  func()
}

func newTargetPoolReporter(total int, reporter telemetry.Reporter) *targetPoolReporter {
	return &targetPoolReporter{
		total:    total,
		ready:    make(map[int]bool),
		expanded: make(map[int]bool),
		reporter: reporter,
	}
}

func (p *targetPoolReporter) slot(slot int) telemetry.Reporter {
	return telemetry.ReporterFunc(func(event telemetry.Event) {
		p.report(slot, event)
	})
}

func (p *targetPoolReporter) report(slot int, event telemetry.Event) {
	var readyCallback func()
	p.mu.Lock()
	switch event.Type {
	case "client_connecting":
		p.mu.Unlock()
		return
	case "target_ready":
		p.ready[slot] = true
		if !p.expanded[slot] {
			p.expanded[slot] = true
			readyCallback = p.onReady
		}
		connected := p.readyCount()
		event.Type = "target_pool_ready"
		event.State = "running"
		event.Message = fmt.Sprintf("Target is ready: %d live session(s); one hot-standby session is kept for the next edge", connected)
	case "client_reconnecting":
		if p.ready[slot] {
			p.ready[slot] = false
		}
		connected := p.readyCount()
		if connected > 0 {
			event.Type = "target_pool_degraded"
			event.State = "running"
			event.Message = fmt.Sprintf("Target session pool is degraded: %d session(s) still connected. %s", connected, event.Message)
		}
	}
	p.mu.Unlock()
	telemetry.Emit(p.reporter, event)
	if readyCallback != nil {
		readyCallback()
	}
}

func (p *targetPoolReporter) readyCount() int {
	ready := 0
	for _, connected := range p.ready {
		if connected {
			ready++
		}
	}
	return ready
}

type serviceStat struct {
	streams atomic.Uint64

	mu          sync.Mutex
	lastError   string
	lastErrorAt time.Time
}

type targetRuntime struct {
	reporter telemetry.Reporter
	instance string
	name     string
	remote   string

	statsDirty atomic.Bool

	mu       sync.Mutex
	services []config.ServiceEntry
	changed  chan struct{}
	stats    map[string]*serviceStat
	conns    map[*protocol.RecordConn]string
}

func newTargetRuntime(cfg config.Config, reporter telemetry.Reporter) *targetRuntime {
	rt := &targetRuntime{
		reporter: reporter,
		instance: newInstanceID(),
		name:     nodeName(cfg, protocol.RoleTarget),
		remote:   relayDisplayEndpoint(cfg.Remote),
		services: append([]config.ServiceEntry(nil), cfg.Services...),
		changed:  make(chan struct{}),
		stats:    make(map[string]*serviceStat),
		conns:    make(map[*protocol.RecordConn]string),
	}
	for _, service := range rt.services {
		rt.stats[service.ID] = &serviceStat{}
	}
	return rt
}

// metadataFor reports the visible-service count of one group so the relay
// console shows accurate per-connection facts.
func (rt *targetRuntime) metadataFor(group string) protocol.RelayMetadata {
	rt.mu.Lock()
	visible := 0
	for _, service := range rt.services {
		if service.VisibleTo(group) {
			visible++
		}
	}
	rt.mu.Unlock()
	return protocol.RelayMetadata{
		Name:          rt.name,
		Endpoint:      fmt.Sprintf("%d services", visible),
		RelayEndpoint: rt.remote,
		Platform:      platformLabel(),
		Instance:      rt.instance,
	}
}

func (rt *targetRuntime) setServices(services []config.ServiceEntry) {
	rt.mu.Lock()
	rt.services = append([]config.ServiceEntry(nil), services...)
	keep := make(map[string]bool, len(services))
	for _, service := range services {
		keep[service.ID] = true
		if _, ok := rt.stats[service.ID]; !ok {
			rt.stats[service.ID] = &serviceStat{}
		}
	}
	for id := range rt.stats {
		if !keep[id] {
			delete(rt.stats, id)
		}
	}
	changed := rt.changed
	rt.changed = make(chan struct{})
	conns := make(map[*protocol.RecordConn]string, len(rt.conns))
	for conn, group := range rt.conns {
		conns[conn] = group
	}
	rt.mu.Unlock()
	close(changed)

	// Live sessions republish their catalogs through the changed channel;
	// also refresh the relay-visible metadata so consoles show the new
	// per-group service counts without waiting for a reconnect.
	for conn, group := range conns {
		_ = conn.RefreshRelayMetadata(rt.metadataFor(group))
	}

	telemetry.Emit(rt.reporter, telemetry.Event{
		Type:     "target_catalog_published",
		Level:    "info",
		Message:  fmt.Sprintf("Published %d service(s) to connected edges", len(services)),
		Services: rt.serviceStatuses(),
	})
}

// snapshotFor returns the catalog one group is allowed to see.
func (rt *targetRuntime) snapshotFor(group string) ([]protocol.CatalogService, <-chan struct{}) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	services := make([]protocol.CatalogService, 0, len(rt.services))
	for _, service := range rt.services {
		if !service.VisibleTo(group) {
			continue
		}
		services = append(services, protocol.CatalogService{
			ID:      service.ID,
			Name:    service.Name,
			Address: service.Address,
		})
	}
	return services, rt.changed
}

// findServiceFor resolves a dial request against the current allowlist of
// one group: unknown ids and ids hidden from the group are both refused.
func (rt *targetRuntime) findServiceFor(id, group string) (config.ServiceEntry, bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for _, service := range rt.services {
		if service.ID == id {
			if !service.VisibleTo(group) {
				return config.ServiceEntry{}, false
			}
			return service, true
		}
	}
	return config.ServiceEntry{}, false
}

func (rt *targetRuntime) statFor(id string) *serviceStat {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	stat, ok := rt.stats[id]
	if !ok {
		stat = &serviceStat{}
		rt.stats[id] = stat
	}
	return stat
}

func (rt *targetRuntime) registerConn(conn *protocol.RecordConn, group string) {
	if conn == nil {
		return
	}
	rt.mu.Lock()
	rt.conns[conn] = group
	rt.mu.Unlock()
}

func (rt *targetRuntime) unregisterConn(conn *protocol.RecordConn) {
	if conn == nil {
		return
	}
	rt.mu.Lock()
	delete(rt.conns, conn)
	rt.mu.Unlock()
}

func (rt *targetRuntime) serviceStatuses() []telemetry.ServiceStatus {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	statuses := make([]telemetry.ServiceStatus, 0, len(rt.services))
	for _, service := range rt.services {
		status := telemetry.ServiceStatus{
			ID:      service.ID,
			Name:    service.Name,
			Address: service.Address,
			Groups:  append([]string(nil), service.Groups...),
		}
		if stat, ok := rt.stats[service.ID]; ok {
			status.Streams = stat.streams.Load()
			stat.mu.Lock()
			status.LastError = stat.lastError
			status.LastErrorAt = stat.lastErrorAt
			stat.mu.Unlock()
		}
		statuses = append(statuses, status)
	}
	return statuses
}

// housekeeping refreshes service statuses on changes and every few seconds
// so consoles converge on the live state even without traffic.
func (rt *targetRuntime) housekeeping(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	ticks := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ticks++
			if !rt.statsDirty.Swap(false) && ticks%3 != 0 {
				continue
			}
			telemetry.Emit(rt.reporter, telemetry.Event{
				Type:      "target_services",
				Level:     "info",
				Transient: true,
				Services:  rt.serviceStatuses(),
			})
		}
	}
}

func (rt *targetRuntime) runSession(ctx context.Context, conn net.Conn, reporter telemetry.Reporter, group string) error {
	if record, ok := conn.(*protocol.RecordConn); ok {
		rt.registerConn(record, group)
		defer rt.unregisterConn(record)
	}
	session, err := yamux.Server(conn, yamuxConfig())
	if err != nil {
		return fmt.Errorf("start target multiplexer: %w", err)
	}
	streams := newStreamGroup(maxConcurrentStreams)
	defer func() {
		_ = session.Close()
		streams.waitForAll()
	}()
	telemetry.Emit(reporter, telemetry.Event{
		Type:     "target_ready",
		Level:    "info",
		State:    "running",
		Message:  "Target is ready to receive streams",
		Services: rt.serviceStatuses(),
	})

	go func() {
		_ = rt.publishLoop(ctx, session, group)
	}()

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if session.IsClosed() {
				return errSessionClosed
			}
			return fmt.Errorf("accept encrypted stream: %w", err)
		}
		if !streams.goIfAvailable(func() {
			rt.handleStream(ctx, stream, group)
		}) {
			_ = stream.Close()
			telemetry.Emit(reporter, streamLimitEvent())
		}
	}
}

// publishLoop owns this session's catalog control stream: it sends the
// group-filtered catalog immediately and again after every service edit.
func (rt *targetRuntime) publishLoop(ctx context.Context, session *yamux.Session, group string) error {
	stream, err := session.OpenStream()
	if err != nil {
		return err
	}
	defer stream.Close()
	if err := protocol.WriteControlHeader(stream); err != nil {
		return err
	}
	for {
		services, changed := rt.snapshotFor(group)
		if err := protocol.WriteCatalog(stream, protocol.CatalogMessage{Services: services}); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-session.CloseChan():
			return nil
		case <-changed:
		}
	}
}

func (rt *targetRuntime) handleStream(ctx context.Context, stream *yamux.Stream, group string) {
	defer stream.Close()
	_ = stream.SetDeadline(time.Now().Add(tunnelPreambleTimeout))
	kind, err := protocol.ReadTunnelStreamKind(stream)
	if err != nil || kind != protocol.TunnelStreamData {
		return
	}
	serviceID, err := protocol.ReadDataPreamble(stream)
	if err != nil {
		return
	}
	service, ok := rt.findServiceFor(serviceID, group)
	if !ok {
		_ = protocol.WriteDialStatus(stream, protocol.TunnelDialUnknown)
		telemetry.Emit(rt.reporter, telemetry.Event{
			Type:    "target_request_rejected",
			Level:   "warning",
			State:   "running",
			Message: "An edge requested an address that is not in the published service list; the request was refused",
		})
		return
	}
	dialer := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	backend, err := dialer.DialContext(ctx, "tcp", service.Address)
	if err != nil {
		_ = protocol.WriteDialStatus(stream, protocol.TunnelDialFailed)
		stat := rt.statFor(service.ID)
		stat.mu.Lock()
		stat.lastError = compactError(err)
		stat.lastErrorAt = time.Now().UTC()
		stat.mu.Unlock()
		rt.statsDirty.Store(true)
		telemetry.Emit(rt.reporter, telemetry.Event{
			Type:    "target_dial_error",
			Level:   "error",
			State:   "running",
			Message: targetServiceUnavailableMessage(service.Name, service.Address, err),
		})
		return
	}
	if err := protocol.WriteDialStatus(stream, protocol.TunnelDialOK); err != nil {
		_ = backend.Close()
		return
	}
	_ = stream.SetDeadline(time.Time{})
	stat := rt.statFor(service.ID)
	stat.streams.Add(1)
	rt.statsDirty.Store(true)
	telemetry.Emit(rt.reporter, telemetry.Event{
		Type:    "stream_opened",
		Level:   "info",
		State:   "running",
		Message: fmt.Sprintf("Encrypted stream reached service %q", service.Name),
	})
	bridge(backend, halfCloseStream{stream})
	rt.statsDirty.Store(true)
}
