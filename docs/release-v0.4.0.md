# MoleX v0.4.0 Release Notes

**English** | [简体中文](release-v0.4.0.zh-CN.md)

`v0.4.0` is the v2 architecture: token groups, a published service catalog, and a clean break from the v1 punch model. Relay, Target, and Edge must be upgraded together. Old `punch` / `secret` / `channel` files fail at startup with migration guidance.

## Breaking change

v1 (`≤v0.3.1`) configurations are not read. Recreate every `molex.json` with `molex config init --mode relay|target|edge`. Mixed v1/v2 fleets do not interoperate. Cut over one token group at a time: Relay first, then that group's Target and Edges.

See the [upgrade guide](upgrade-guide.md).

## Required role upgrades

- **Relay: required.** Token registry, one Target plus N Edges per token, ciphertext-only forwarding, rotate/disable/delete, JSONL audit.
- **Target: required.** Publishes a live service catalog; dials only published addresses; one process may join several tokens.
- **Edge: required.** Maps published services to local ports; listeners exist only while the route is ready and the service is published.

## What changed

- Roles are `relay` / `target` / `edge`. One access token is one trust group: exactly one Target and any number of Edges. A second Target on the same token is rejected.
- The Target publishes `ip:port` services. Edges check the ones they need. Mappings default to `127.0.0.1`; each mapping can bind the LAN.
- A single Target or Edge process may join several tokens (`tokens[]`). Restrict visibility with `services[].groups`. Multi-group Edge mappings require `group`.
- Payload protection remains X25519 + HKDF-SHA256 + AES-256-GCM inside TLS 1.3. The PSK is derived from the token. The shipped Relay never decrypts payloads.
- Token rotation keeps the previous value valid for 1–30 days (default 3). Administrative actions write token ids only to a JSONL audit file.
- Relay console: password login, token manager, live peers, ciphertext counters. Target and Edge consoles: login-free, loopback-only, same-origin and CSRF.
- Linux keep-alive: `deploy/molex-relay.service` or `deploy/molex-keepalive.sh`.
- Catalog and mapping counts refresh over the encrypted metadata channel without reconnecting.

## Verification

- Full Go suite, race detector, `go vet`, frontend tests, and production frontend build.
- Real-socket client suite: concurrent multi-edge traffic, catalog publish/withdraw, duplicate-Target rejection, allowlist, token disable/kick recovery, cross-token isolation, Target restart, occupied mapping-port recovery, bounded shutdown.
- Token rotation grace expiry, metadata refresh merge, audit writer, multi-group Target/Edge.
- Windows, Linux, and macOS amd64/arm64 release archives.

## After you install

```bash
molex config init --mode relay --force
molex web --config molex.json --autostart
```

Create a token in the Relay console, then init Target and Edge with the same `wss://…/ws/session` URL and token. Do not reuse v1 `secret` or `channel` values.

Interactive use:

```bash
molex web --config molex.json --autostart
```

Server and reverse-proxy use:

```bash
molex web --config molex.json --autostart \
  --listen 127.0.0.1:9090 \
  --open-browser=false
```

See the [user guide](user-guide.md), [architecture](architecture.md), and [security model](security.md).
