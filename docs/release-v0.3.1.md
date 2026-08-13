# MoleX v0.3.1 Release Notes

**English** | [简体中文](release-v0.3.1.zh-CN.md)

`v0.3.1` is a stability and out-of-box usability patch for the adaptive Target pool introduced in `v0.3.0`. It removes standby connection churn and prevents spare sockets from accumulating after repeated reconnects.

## Required role upgrades

- **Relay: required.** It now retains unmatched Targets as long-lived standby instead of applying the ordinary Edge pairing timeout.
- **Target: required.** Standby handshake waits are long-lived but remain immediately cancellable, and each pool slot expands only once.
- **Edge: protocol compatible.** `v0.3.0` Edges may remain during a rolling deployment, then be unified on `v0.3.1`.

Upgrading only Relay or only Target does not fully remove the `v0.3.0` periodic connect/disconnect behavior. Upgrade Relay first, Target second, and Edges afterward as needed.

## Fixes and improvements

- Relay treats waiting Targets as long-lived standby while stale Edge waits remain bounded.
- A Target may wait beyond the old 15-second peer-hello deadline; key confirmation still receives a fresh 15-second security deadline after pairing.
- Context cancellation closes a WebSocket blocked in handshake reads, keeping shutdown bounded.
- Each adaptive pool slot triggers growth once, preventing network flaps from multiplying standby sockets.
- WebUI prefers `127.0.0.1:9090`, advances to another loopback port when occupied, prints the selected URL, and opens the default browser after readiness.
- Explicit `--listen` remains strict. Servers, SSH forwarding, and reverse proxies should add `--open-browser=false`.
- CLI Target pool help now documents `0` (adaptive) and `1-65535` correctly.

## Verification

- A Target standby waits beyond the old 15-second deadline, then uses the original connection for pairing and a real TCP echo round trip.
- Four Edge/Target pairs forward concurrently without cross-talk.
- Three Edges recover after latency, refused connection attempts, abrupt disconnects, and three repeated network-flap cycles.
- Target restart, occupied Edge-listener recovery, FIFO waiting, bounded shutdown, actionable errors, and handshake cancellation.
- Full Go suite, race detector, `go vet`, frontend tests, and production frontend build.
- Cross-builds for Windows, Linux, and macOS on amd64 and arm64.

## WebUI operation

Interactive use:

```bash
molex web --config molex.json --autostart
```

Server and fixed reverse-proxy use:

```bash
molex web --config molex.json --autostart \
  --listen 127.0.0.1:9090 \
  --open-browser=false
```

See the [upgrade guide](upgrade-guide.md) for complete rollout and rollback steps.
