# Testing and release checks

## Local checks

```bash
gofmt -w .
go vet ./...
go test ./...
go test -race ./...

cd frontend
npm ci
npm run check
npm test
npm run build
```

## Three-end simulation

`internal/client/integration_test.go` creates all three MoleX roles in-process but uses real sockets and WebSocket framing:

1. Start a TCP echo service on a random loopback port.
2. Start the real relay HTTP handler behind an `httptest` TCP listener.
3. Start a target client that dials the echo service.
4. Start an edge client on a random loopback port.
5. Wait for the authenticated encrypted session and yamux listener.
6. Open eight concurrent TCP connections to the edge.
7. Send independent 48 KiB random payloads and verify exact echo responses.
8. Cancel both clients and verify bounded shutdown.

Additional recovery simulations use the same real sockets and protocol stack:

- stop Target after a successful flow, observe Edge leave the listening state, restart Target, and verify a new Edge listener forwards traffic;
- occupy the configured Edge address, confirm the retry event tells the operator how to release it, free the address, and verify Edge starts listening without a process restart.

These tests cover WebSocket upgrade, bearer admission, route pairing, X25519/PSK handshake, AES-GCM records, yamux multiplexing, target dialing, TCP copying, concurrent streams, cancellation, disconnect detection, listener teardown, and automatic route recovery.

The stream worker test verifies that the per-route concurrency guard never admits more than its bound and that shutdown waits for admitted work to drain.

Run only that simulation with:

```bash
go test -v ./internal/client -run TestThreeEndConcurrentTCPFlow
go test -v ./internal/client -run 'TestEdge(Reconnects|Recovers)'
```

## Protocol checks

The protocol suite verifies:

- complementary peers derive opposite directional keys;
- wrong secrets and duplicate roles fail;
- hello packets contain no literal secret, channel, or product marker;
- observed data frames do not contain a plaintext canary;
- a one-bit ciphertext change fails AES-GCM authentication;
- paired relay sessions remain open beyond the unpaired wait timeout;
- a participant claimed by pairing at the exact timeout boundary is not closed as a stale waiter;
- fixed-size operational metadata frames expose no plaintext label, reject the wrong token and tampering, and preserve the legacy 128-byte hello;
- Relay peer lifecycle events report trusted, normalized IPv4/IPv6 addresses and ignore spoofed forwarded headers from public clients;
- paired telemetry includes names, forwarding endpoints, counterpart IDs, platforms, pseudonymous route IDs, and ciphertext traffic counters;
- a late update-only statistics event cannot recreate a disconnected peer or pollute activity history;
- a real Relay/Edge/Target session adds two paired peers and removes both after shutdown.

## Web console checks

`internal/webui/server_test.go` exercises the authenticated management surface with a real HTTP server:

- unauthenticated API rejection and login/logout session handling;
- `HttpOnly`/`SameSite` cookie lifetime, CSRF enforcement, and cross-origin rejection;
- login rate limiting and non-loopback listen rejection;
- configuration validation and a real relay start/stop lifecycle;
- runtime cleanup when the Web listener cannot start;
- embedded static assets, cache policy, health checks, and security headers.

The frontend suite covers login failures, login/logout navigation, relay and client controls, validation, the detailed Relay connection table and empty state, dynamic peer activity, stale-update suppression, and full English/Chinese switching.

## Manual CLI simulation

Use three terminals and a local echo or SSH service.

Relay:

```bash
molex serve --config examples/relay.json
```

For local-only testing, change both client `remote` values to `ws://127.0.0.1:8080/ws/session`, then start:

```bash
molex connect --config examples/target.json
molex connect --config examples/edge.json
```

Connect to the edge address in a fourth terminal. Do not use plain `ws://` for a non-loopback endpoint.

## Manual Web simulation

Build the frontend and binary, create three configuration files and three password files, then start one console per role on different loopback management ports:

```bash
molex web --config relay.json  --listen 127.0.0.1:9090 --password-file relay.password  --autostart
molex web --config target.json --listen 127.0.0.1:9091 --password-file target.password --autostart
molex web --config edge.json   --listen 127.0.0.1:9092 --password-file edge.password   --autostart
```

Open all three consoles, verify their running state, and pass traffic through the edge listener. On separate hosts, the same default management port can be reused.

## Release checks

- Build the frontend before the Go binary so `go:embed` includes current assets.
- Build the same Go binary on Windows, macOS, and Linux runners.
- Launch `molex web` and test authenticated start/stop in relay, edge, and target configurations.
- Check the Web console at 1320px, 820px, 390px, and 320px viewport widths with no horizontal overflow.
- Verify English and Simplified Chinese, login persistence, logout, dark and light themes, keyboard focus, disabled fields, errors, empty activity, SSE updates, and reduced motion.
- Confirm the management listener rejects LAN/public addresses and remote access uses HTTPS or SSH forwarding.
- Verify the generated executable reports the intended version through `molex version`.
- Check the Caddy example against the current route path.
- Inspect dependency updates and rerun the race suite.
