# MoleX Upgrade Guide

[English](upgrade-guide.md) | [简体中文](upgrade-guide.zh-CN.md)

Relay, Edge, and Target are roles of the same binary. Replace the binary on each host. **v0.4.0 (v2) is a clean break** from `v0.3.1` and earlier: old `punch` / `secret` / `channel` files fail at startup with migration guidance.

## Version differences

| Version | Main changes | Deployment impact |
| --- | --- | --- |
| `v0.1.0`–`v0.3.1` | v1 punch model: global Relay token plus per-route `secret` + `channel`. See the archived notes below if you are still on those builds. | v1 files are not read by v2. |
| `v0.4.0` (v2) | Roles are `relay` / `target` / `edge`. One token = one Target + N Edges. Target publishes a service catalog; Edges map published services. Token rotation with a grace window, JSONL audit, multi-group processes, live metadata refresh. | Recreate every `molex.json`. Upgrade Relay, Target, and Edge together. |

## Upgrade from v1 (`≤v0.3.1`) to v2

v2 does **not** auto-migrate. `mode: "punch"` and the `role` / `secret` / `tunnel` fields cause a startup error that points here.

1. Download the v2 archive and verify `SHA256SUMS`.
2. Back up every `molex.json` and the Relay web-password file. You will not reuse the JSON as-is.
3. Relay: `molex config init --mode relay --force`. Start `molex web`, set the management password, and create one token per trust group. Old `secret` + `channel` pairs become one token each.
4. Each former Target host: `molex config init --mode target --force`. Set `remote` to the same `wss://…/ws/session` URL, paste the token, and add every old `tunnel.local` (and each multi-rule `local`) as a published service.
5. Each former Edge host: `molex config init --mode edge --force`. Paste the same token, start, and map the published services to the ports you used before.
6. Discard v1 `secret` and `channel` values. They are not credentials in v2.
7. Confirm in the Relay console: one Target online per token, Edges listed, ciphertext counters moving, and a TCP check from each Edge mapping.

Rolling a mixed v1/v2 fleet does not work. The hello, credentials, and catalog protocol are different. Cut over a whole token group at once (Relay first, then that group’s Target and Edges).

### Example v2 files

```json
{
  "mode": "relay",
  "listen": "127.0.0.1:8080",
  "tokens": [{ "id": "tok-office", "token": "mx2_generated-value", "note": "office" }]
}
```

```json
{
  "mode": "target",
  "remote": "wss://molex.example.com/ws/session",
  "token": "mx2_generated-value",
  "services": [{ "id": "svc-ssh", "name": "ssh", "address": "127.0.0.1:22" }]
}
```

```json
{
  "mode": "edge",
  "remote": "wss://molex.example.com/ws/session",
  "token": "mx2_generated-value",
  "mappings": [{ "service": "svc-ssh", "port": 2222 }]
}
```

## Staying on v2

Token rotation (`POST /api/tokens/:id/rotate` or the Relay console **Rotate** button) keeps the previous value valid for 1–30 days (default 3). Update every Target and Edge before expiry. New tokens can also be given their own lifetime (`never`, `1d`, `7d`, `30d`, `90d`, `365d`); edit it later with `PUT /api/tokens/:id`. Audit records store token ids only.

A single Target or Edge process may join several tokens via `tokens[]`. Restrict Target services with `services[].groups`. Edge mappings need `group` when more than one group is joined.

Linux Relay keep-alive: `deploy/molex-relay.service`. Without systemd: `deploy/molex-keepalive.sh`.

## Rollback from v2

1. Stop the v2 processes.
2. Restore the v1 binaries and the backed-up punch configurations.
3. Point Caddy at the restored Relay. v2 tokens will not work on v1, and v1 secrets will not work on v2.

## Acceptance after a v2 cut-over

- [ ] `molex version` reports the v2 build.
- [ ] `molex config check` accepts the new files and rejects any leftover punch file.
- [ ] Relay console: token list, one Target per token, Edges online.
- [ ] Target catalog matches the published services; Edge mappings listen only when running.
- [ ] TCP check through at least one mapping.
- [ ] Duplicate Target is rejected; token disable disconnects the group.
- [ ] `/healthz` on the data and management listeners succeeds.

See the [user guide](user-guide.md), [architecture](architecture.md), and [testing](testing.md).

## Archived: v0.1.0–v0.3.1 (v1 only)

These notes apply only if you are still moving between v1 builds and have not cut over to v2.

| Version | Notes |
| --- | --- |
| `v0.1.0` | One Edge/Target pair per opaque route. |
| `v0.2.0` | FIFO queues; Relay must be `>=v0.2.0` for multiple Edges on one channel. |
| `v0.3.0` | `tunnel.pool: 0` demand-driven Target sessions. |
| `v0.3.1` | Long-lived Target standby; upgrade Relay and Target to stop standby churn. |

v1 peers must keep `secret`, `token`, `remote`, and `tunnel.remote` aligned. That layout is obsolete in v2.
