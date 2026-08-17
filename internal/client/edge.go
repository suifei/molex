package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/suifei/molex/internal/config"
	"github.com/suifei/molex/internal/protocol"
	"github.com/suifei/molex/internal/telemetry"
)

const (
	tunnelPreambleTimeout = 15 * time.Second
	listenerRetryInterval = 3 * time.Second
)

func runEdgeProcess(ctx context.Context, cfg config.Config, reporter telemetry.Reporter, retry retrySettings, mappingUpdates <-chan []config.MappingEntry) error {
	groups := cfg.GroupTokens()
	rt := newEdgeRuntime(cfg, reporter)
	defer rt.shutdown()

	if mappingUpdates != nil {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case mappings, ok := <-mappingUpdates:
					if !ok {
						return
					}
					rt.setMappings(mappings)
				}
			}
		}()
	}
	go rt.housekeeping(ctx)

	aggregator := newGroupStateAggregator(reporter)
	var workers sync.WaitGroup
	for _, group := range groups {
		group := group
		groupReporter := reporter
		if len(groups) > 1 {
			groupReporter = aggregator.group(group.ID)
		}
		spec := sessionSpec{
			cfg:   cfg,
			role:  protocol.RoleEdge,
			token: group.Token,
			metadata: func() protocol.RelayMetadata {
				return rt.metadataFor(group.ID)
			},
			handler: func(ctx context.Context, conn net.Conn, sessionReporter telemetry.Reporter) error {
				return rt.runSession(ctx, conn, sessionReporter, group.ID)
			},
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			_ = runSessionLoop(ctx, spec, groupReporter, retry)
		}()
	}
	workers.Wait()
	return nil
}

type mappingCounter struct {
	connections atomic.Uint64
	bytes       atomic.Uint64
}

type edgeListener struct {
	group    string
	entry    config.MappingEntry
	listener net.Listener
	lastErr  error
}

type edgeRuntime struct {
	reporter   telemetry.Reporter
	streams    *streamGroup
	name       string
	remote     string
	groupNames []string

	statsDirty atomic.Bool

	mu          sync.Mutex
	mappings    []config.MappingEntry
	catalogs    map[string][]telemetry.CatalogService
	catalogIdx  map[string]map[string]telemetry.CatalogService
	online      map[string]bool
	sessions    map[string]*yamux.Session
	conns       map[string]*protocol.RecordConn
	listeners   map[string]*edgeListener
	counters    map[string]*mappingCounter
	activeConns map[net.Conn]string
}

func mappingKey(group, service string) string {
	return group + "\x00" + service
}

func newEdgeRuntime(cfg config.Config, reporter telemetry.Reporter) *edgeRuntime {
	groups := cfg.GroupTokens()
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.ID)
	}
	rt := &edgeRuntime{
		reporter:    reporter,
		streams:     newStreamGroup(maxConcurrentStreams),
		name:        nodeName(cfg, protocol.RoleEdge),
		remote:      relayDisplayEndpoint(cfg.Remote),
		groupNames:  names,
		catalogs:    make(map[string][]telemetry.CatalogService),
		catalogIdx:  make(map[string]map[string]telemetry.CatalogService),
		online:      make(map[string]bool),
		sessions:    make(map[string]*yamux.Session),
		conns:       make(map[string]*protocol.RecordConn),
		listeners:   make(map[string]*edgeListener),
		counters:    make(map[string]*mappingCounter),
		activeConns: make(map[net.Conn]string),
	}
	rt.mappings = rt.normalizeMappings(cfg.Mappings)
	return rt
}

// normalizeMappings pins every mapping to a concrete group name so lookups
// stay unambiguous when the edge joined only one group.
func (rt *edgeRuntime) normalizeMappings(mappings []config.MappingEntry) []config.MappingEntry {
	normalized := make([]config.MappingEntry, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.Group == "" && len(rt.groupNames) == 1 {
			mapping.Group = rt.groupNames[0]
		}
		normalized = append(normalized, mapping)
	}
	return normalized
}

// metadataFor reports the mapping count of one group for the relay console.
func (rt *edgeRuntime) metadataFor(group string) protocol.RelayMetadata {
	rt.mu.Lock()
	count := 0
	for _, mapping := range rt.mappings {
		if mapping.Group == group {
			count++
		}
	}
	rt.mu.Unlock()
	return protocol.RelayMetadata{
		Name:          rt.name,
		Endpoint:      fmt.Sprintf("%d mappings", count),
		RelayEndpoint: rt.remote,
		Platform:      platformLabel(),
	}
}

func (rt *edgeRuntime) runSession(ctx context.Context, conn net.Conn, reporter telemetry.Reporter, group string) error {
	record, _ := conn.(*protocol.RecordConn)
	session, err := yamux.Client(conn, yamuxConfig())
	if err != nil {
		return fmt.Errorf("start edge multiplexer: %w", err)
	}
	catalog, statuses, problems := rt.attachSession(group, session, record)
	defer func() {
		// During shutdown the console reports "stopping"; only live route
		// drops should announce the connecting state.
		rt.detachSession(group, session, ctx.Err() == nil)
	}()

	telemetry.Emit(reporter, telemetry.Event{
		Type:     "edge_route_ready",
		Level:    "info",
		State:    "running",
		Message:  "Encrypted route is ready; waiting for the target service catalog",
		Catalog:  catalog,
		Mappings: statuses,
	})
	rt.emitListenProblems(problems)

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return errSessionClosed
		}
		go rt.handleInboundStream(group, stream)
	}
}

// handleInboundStream accepts the target's catalog control stream. Data
// streams never originate from the target, so anything else is closed.
func (rt *edgeRuntime) handleInboundStream(group string, stream *yamux.Stream) {
	_ = stream.SetReadDeadline(time.Now().Add(tunnelPreambleTimeout))
	kind, err := protocol.ReadTunnelStreamKind(stream)
	if err != nil || kind != protocol.TunnelStreamControl {
		stream.Close()
		return
	}
	_ = stream.SetReadDeadline(time.Time{})
	defer stream.Close()
	for {
		message, err := protocol.ReadCatalog(stream)
		if err != nil {
			return
		}
		rt.setCatalog(group, message.Services)
	}
}

func (rt *edgeRuntime) attachSession(group string, session *yamux.Session, record *protocol.RecordConn) (*telemetry.CatalogUpdate, []telemetry.MappingStatus, []string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.sessions[group] = session
	if record != nil {
		rt.conns[group] = record
	}
	rt.online[group] = true
	delete(rt.catalogs, group)
	delete(rt.catalogIdx, group)
	problems := rt.reconcileLocked()
	return rt.catalogUpdateLocked(), rt.mappingStatusesLocked(), problems
}

func (rt *edgeRuntime) detachSession(group string, session *yamux.Session, announce bool) {
	rt.mu.Lock()
	if rt.sessions[group] != session {
		rt.mu.Unlock()
		return
	}
	delete(rt.sessions, group)
	delete(rt.conns, group)
	rt.online[group] = false
	delete(rt.catalogs, group)
	delete(rt.catalogIdx, group)
	rt.reconcileLocked()
	statuses := rt.mappingStatusesLocked()
	catalog := rt.catalogUpdateLocked()
	active := make([]net.Conn, 0)
	for conn, connGroup := range rt.activeConns {
		if connGroup == group {
			active = append(active, conn)
		}
	}
	rt.mu.Unlock()
	_ = session.Close()
	for _, conn := range active {
		_ = conn.Close()
	}

	if !announce {
		return
	}
	telemetry.Emit(rt.reporter, telemetry.Event{
		Type:        "edge_catalog",
		Level:       "info",
		State:       "connecting",
		Message:     "Encrypted route is down; local mapping listeners are closed until it recovers",
		Catalog:     catalog,
		Mappings:    statuses,
		ClearListen: true,
	})
}

func (rt *edgeRuntime) setCatalog(group string, services []protocol.CatalogService) {
	catalog := make([]telemetry.CatalogService, 0, len(services))
	byID := make(map[string]telemetry.CatalogService, len(services))
	for _, service := range services {
		entry := telemetry.CatalogService{ID: service.ID, Name: service.Name, Address: service.Address, Group: group}
		if entry.ID == "" {
			continue
		}
		catalog = append(catalog, entry)
		byID[entry.ID] = entry
	}

	rt.mu.Lock()
	if !rt.online[group] {
		rt.mu.Unlock()
		return
	}
	same := equalCatalogs(rt.catalogs[group], catalog)
	rt.catalogs[group] = catalog
	rt.catalogIdx[group] = byID
	problems := rt.reconcileLocked()
	statuses := rt.mappingStatusesLocked()
	update := rt.catalogUpdateLocked()
	rt.mu.Unlock()

	if !same {
		telemetry.Emit(rt.reporter, telemetry.Event{
			Type:     "edge_catalog",
			Level:    "info",
			State:    "running",
			Message:  fmt.Sprintf("Target published %d service(s)", len(catalog)),
			Catalog:  update,
			Mappings: statuses,
		})
	}
	rt.emitListenProblems(problems)
}

func (rt *edgeRuntime) setMappings(mappings []config.MappingEntry) {
	rt.mu.Lock()
	rt.mappings = rt.normalizeMappings(mappings)
	keep := make(map[string]bool, len(rt.mappings))
	for _, mapping := range rt.mappings {
		keep[mappingKey(mapping.Group, mapping.Service)] = true
	}
	for key := range rt.counters {
		if !keep[key] {
			delete(rt.counters, key)
		}
	}
	problems := rt.reconcileLocked()
	statuses := rt.mappingStatusesLocked()
	update := rt.catalogUpdateLocked()
	conns := make(map[string]*protocol.RecordConn, len(rt.conns))
	for group, conn := range rt.conns {
		conns[group] = conn
	}
	mappingCount := len(rt.mappings)
	rt.mu.Unlock()

	// Refresh relay-visible mapping counts on the live sessions.
	for group, conn := range conns {
		_ = conn.RefreshRelayMetadata(rt.metadataFor(group))
	}

	telemetry.Emit(rt.reporter, telemetry.Event{
		Type:     "edge_mappings",
		Level:    "info",
		Message:  fmt.Sprintf("Applied %d local mapping(s)", mappingCount),
		Catalog:  update,
		Mappings: statuses,
	})
	rt.emitListenProblems(problems)
}

// reconcileLocked opens and closes local listeners so they match the
// desired mappings, each group's published catalog, and its route state.
func (rt *edgeRuntime) reconcileLocked() []string {
	desired := make(map[string]config.MappingEntry)
	for _, mapping := range rt.mappings {
		if !rt.online[mapping.Group] {
			continue
		}
		if _, ok := rt.catalogIdx[mapping.Group][mapping.Service]; !ok {
			continue
		}
		desired[mappingKey(mapping.Group, mapping.Service)] = mapping
	}

	for key, listener := range rt.listeners {
		want, ok := desired[key]
		if ok && want == listener.entry && listener.listener != nil {
			continue
		}
		if !ok || want != listener.entry {
			listener.close()
			delete(rt.listeners, key)
		}
	}

	var problems []string
	for key, mapping := range desired {
		existing, ok := rt.listeners[key]
		if ok && existing.listener != nil {
			continue
		}
		if !ok {
			existing = &edgeListener{group: mapping.Group, entry: mapping}
			rt.listeners[key] = existing
		}
		if err := rt.openListenerLocked(existing); err != nil {
			problems = append(problems, fmt.Sprintf(
				"The local listener for %s could not start on %s. Stop the process using that address or pick another port; MoleX keeps retrying automatically. Details: %s",
				rt.describeServiceLocked(mapping.Group, mapping.Service), mapping.ListenAddress(), compactError(err)))
		}
	}
	return problems
}

func (rt *edgeRuntime) openListenerLocked(listener *edgeListener) error {
	ln, err := net.Listen("tcp", listener.entry.ListenAddress())
	if err != nil {
		listener.lastErr = err
		return err
	}
	listener.listener = ln
	listener.lastErr = nil
	go rt.serveListener(listener.group, listener.entry, ln)
	return nil
}

func (rt *edgeRuntime) serveListener(group string, entry config.MappingEntry, ln net.Listener) {
	for {
		local, err := ln.Accept()
		if err != nil {
			return
		}
		session := rt.currentSession(group)
		if session == nil {
			_ = local.Close()
			continue
		}
		counter := rt.counterFor(group, entry.Service)
		if !rt.streams.goIfAvailable(func() {
			rt.forward(local, group, entry, session, counter)
		}) {
			_ = local.Close()
			telemetry.Emit(rt.reporter, streamLimitEvent())
		}
	}
}

func (rt *edgeRuntime) forward(local net.Conn, group string, entry config.MappingEntry, session *yamux.Session, counter *mappingCounter) {
	defer local.Close()
	// Track the local connection so a dropped route closes it promptly
	// instead of leaving its bridge blocked on a local read.
	rt.mu.Lock()
	rt.activeConns[local] = group
	rt.mu.Unlock()
	defer func() {
		rt.mu.Lock()
		delete(rt.activeConns, local)
		rt.mu.Unlock()
	}()

	stream, err := session.OpenStream()
	if err != nil {
		event := telemetry.Event{
			Type:    "stream_error",
			Level:   "error",
			State:   "running",
			Message: "This local connection could not open an encrypted stream. Retry it; if this repeats, check peer health and reduce simultaneous connection attempts.",
		}
		if session.IsClosed() {
			event.State = "connecting"
			event.Message = "This local connection could not be forwarded because the encrypted route was interrupted. MoleX is reconnecting; retry after the route is ready."
			event.ClearListen = true
		}
		telemetry.Emit(rt.reporter, event)
		return
	}
	defer stream.Close()

	deadline := time.Now().Add(tunnelPreambleTimeout)
	_ = stream.SetDeadline(deadline)
	if err := protocol.WriteDataPreamble(stream, entry.Service); err != nil {
		return
	}
	status, err := protocol.ReadDialStatus(stream)
	if err != nil {
		telemetry.Emit(rt.reporter, telemetry.Event{
			Type:    "stream_error",
			Level:   "error",
			State:   "running",
			Message: "The target did not answer the forwarding request in time. Check the target's health and retry.",
		})
		return
	}
	switch status {
	case protocol.TunnelDialOK:
	case protocol.TunnelDialUnknown:
		telemetry.Emit(rt.reporter, telemetry.Event{
			Type:    "stream_error",
			Level:   "error",
			State:   "running",
			Message: fmt.Sprintf("The target no longer publishes %s. The catalog refreshes automatically; re-check the mapping afterwards.", rt.describeService(group, entry.Service)),
		})
		return
	case protocol.TunnelDialFailed:
		telemetry.Emit(rt.reporter, telemetry.Event{
			Type:    "stream_error",
			Level:   "error",
			State:   "running",
			Message: fmt.Sprintf("The target could not reach %s. Start the backend service or fix its address on the target console.", rt.describeService(group, entry.Service)),
		})
		return
	default:
		return
	}
	_ = stream.SetDeadline(time.Time{})

	counter.connections.Add(1)
	rt.statsDirty.Store(true)
	countingBridge(local, halfCloseStream{stream}, counter)
	rt.statsDirty.Store(true)
}

// housekeeping periodically retries failed listeners and refreshes mapping
// statuses so consoles converge on the live state even without traffic.
func (rt *edgeRuntime) housekeeping(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	ticks := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ticks++
			emit := rt.statsDirty.Swap(false)
			var recovered []string
			if ticks%3 == 0 {
				recovered = rt.retryFailedListeners()
				emit = true
			}
			if !emit {
				continue
			}
			rt.mu.Lock()
			statuses := rt.mappingStatusesLocked()
			update := rt.catalogUpdateLocked()
			rt.mu.Unlock()
			telemetry.Emit(rt.reporter, telemetry.Event{
				Type:      "edge_mappings",
				Level:     "info",
				Transient: true,
				Catalog:   update,
				Mappings:  statuses,
			})
			for _, message := range recovered {
				telemetry.Emit(rt.reporter, telemetry.Event{
					Type:    "edge_mapping_listening",
					Level:   "info",
					State:   "running",
					Message: message,
				})
			}
		}
	}
}

func (rt *edgeRuntime) retryFailedListeners() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	var recovered []string
	for _, listener := range rt.listeners {
		if listener.listener != nil || listener.lastErr == nil {
			continue
		}
		if err := rt.openListenerLocked(listener); err == nil {
			recovered = append(recovered, fmt.Sprintf(
				"Local mapping for %s recovered and is listening on %s",
				rt.describeServiceLocked(listener.group, listener.entry.Service), listener.listener.Addr().String()))
		}
	}
	return recovered
}

// catalogUpdateLocked aggregates every group's published services plus the
// per-group route state for the console.
func (rt *edgeRuntime) catalogUpdateLocked() *telemetry.CatalogUpdate {
	update := &telemetry.CatalogUpdate{Services: []telemetry.CatalogService{}}
	for _, group := range rt.groupNames {
		groupCatalog := telemetry.GroupCatalog{
			Group:    group,
			Online:   rt.online[group],
			Services: append([]telemetry.CatalogService{}, rt.catalogs[group]...),
		}
		if groupCatalog.Online {
			update.Online = true
		}
		update.Services = append(update.Services, groupCatalog.Services...)
		update.Groups = append(update.Groups, groupCatalog)
	}
	return update
}

func (rt *edgeRuntime) mappingStatusesLocked() []telemetry.MappingStatus {
	now := time.Now().UTC()
	statuses := make([]telemetry.MappingStatus, 0, len(rt.mappings))
	for _, mapping := range rt.mappings {
		status := telemetry.MappingStatus{
			Service:   mapping.Service,
			Group:     mapping.Group,
			LAN:       mapping.LAN,
			UpdatedAt: now,
		}
		if service, ok := rt.catalogIdx[mapping.Group][mapping.Service]; ok {
			status.ServiceName = service.Name
			status.Address = service.Address
		}
		if counter, ok := rt.counters[mappingKey(mapping.Group, mapping.Service)]; ok {
			status.Connections = counter.connections.Load()
			status.Bytes = counter.bytes.Load()
		}
		listener := rt.listeners[mappingKey(mapping.Group, mapping.Service)]
		switch {
		case listener != nil && listener.listener != nil:
			status.State = telemetry.MappingStateListening
			status.Listen = listener.listener.Addr().String()
		case listener != nil && listener.lastErr != nil:
			status.State = telemetry.MappingStateError
			status.Listen = mapping.ListenAddress()
			status.Message = fmt.Sprintf("The local listener could not start on %s. Stop the process using that address or pick another port; MoleX keeps retrying automatically.", mapping.ListenAddress())
		case !rt.online[mapping.Group]:
			status.State = telemetry.MappingStateWaiting
			status.Message = "Waiting for the encrypted route"
		case rt.catalogIdx[mapping.Group] == nil:
			status.State = telemetry.MappingStateWaiting
			status.Message = "Waiting for the target service catalog"
		default:
			status.State = telemetry.MappingStateWaiting
			status.Message = "The target does not publish this service; it stays inactive until published again"
		}
		statuses = append(statuses, status)
	}
	return statuses
}

func (rt *edgeRuntime) emitListenProblems(problems []string) {
	for _, message := range problems {
		telemetry.Emit(rt.reporter, telemetry.Event{
			Type:    "edge_mapping_error",
			Level:   "error",
			State:   "running",
			Message: message,
		})
	}
}

func (rt *edgeRuntime) currentSession(group string) *yamux.Session {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.sessions[group]
}

func (rt *edgeRuntime) counterFor(group, serviceID string) *mappingCounter {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	key := mappingKey(group, serviceID)
	counter, ok := rt.counters[key]
	if !ok {
		counter = &mappingCounter{}
		rt.counters[key] = counter
	}
	return counter
}

func (rt *edgeRuntime) describeService(group, serviceID string) string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.describeServiceLocked(group, serviceID)
}

func (rt *edgeRuntime) describeServiceLocked(group, serviceID string) string {
	if service, ok := rt.catalogIdx[group][serviceID]; ok && service.Name != "" {
		return fmt.Sprintf("service %q", service.Name)
	}
	return fmt.Sprintf("service %s", serviceID)
}

func (rt *edgeRuntime) shutdown() {
	rt.mu.Lock()
	for key, listener := range rt.listeners {
		listener.close()
		delete(rt.listeners, key)
	}
	sessions := make([]*yamux.Session, 0, len(rt.sessions))
	for group, session := range rt.sessions {
		sessions = append(sessions, session)
		delete(rt.sessions, group)
	}
	active := make([]net.Conn, 0, len(rt.activeConns))
	for conn := range rt.activeConns {
		active = append(active, conn)
	}
	rt.mu.Unlock()
	for _, session := range sessions {
		_ = session.Close()
	}
	for _, conn := range active {
		_ = conn.Close()
	}
	rt.streams.waitForAll()
}

func (l *edgeListener) close() {
	if l.listener != nil {
		_ = l.listener.Close()
		l.listener = nil
	}
	l.lastErr = nil
}

func equalCatalogs(a, b []telemetry.CatalogService) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func countingBridge(a, b net.Conn, counter *mappingCounter) {
	defer a.Close()
	defer b.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	copyOneWay := func(dst, src net.Conn) {
		defer wg.Done()
		n, _ := io.Copy(dst, src)
		if n > 0 {
			counter.bytes.Add(uint64(n))
		}
		if closer, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = closer.CloseWrite()
		}
	}
	go copyOneWay(a, b)
	go copyOneWay(b, a)
	wg.Wait()
}
