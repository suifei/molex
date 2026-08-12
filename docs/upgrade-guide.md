# MoleX Upgrade Guide

[English](upgrade-guide.md) | [简体中文](upgrade-guide.zh-CN.md)

This guide compares `v0.1.0`, `v0.2.0`, and `v0.3.0`, explains component compatibility, and provides a production upgrade and rollback path. Relay, Edge, and Target are roles of the same binary; replace the binary on each role's host as needed.

## Version differences

| Version | Main changes | Deployment impact |
| --- | --- | --- |
| `v0.1.0` | Initial public release with basic WSS/TCP transit, WebUI login, and automated releases. | A route effectively served one Edge/Target pair. |
| `v0.2.0` | Per-route FIFO Edge/Target queues, same-role waiting, reusable node names, first-run WebUI password setup, fixed Target pools, multi-rule CRUD, Caddy helper, richer peer metadata, and actionable errors. | Relay must be at least `v0.2.0` for multiple Edges to queue on one route. Target pool still defaults to `1`. |
| `v0.3.0` | Demand-driven one-Target/many-Edge sessions. `tunnel.pool: 0` opens the next independent WSS session after each pairing, up to 65,535; fixed pools `1–65535` remain supported. | Upgrade Target to `v0.3.0` and use `pool: 0` for the recommended topology. |

## Which roles must be upgraded

```text
Edge 1 ─┐
Edge 2 ─┼──> Relay 1 ───> Target 1
Edge N ─┘
```

| Current state | Relay | Target | Edge | Result |
| --- | --- | --- | --- | --- |
| `v0.1.0` | Must upgrade to `>=v0.2.0` | Recommended | Optional | Old Relay cannot queue multiple Edges on one route. |
| `v0.2.0` | Can remain | Upgrade to `v0.3.0` | Can remain | Target with `pool: 0` can accept multiple Edges; the protocol remains compatible. |
| Mixed rollout | `v0.3.0` | `v0.3.0` | `v0.2.0` or `v0.3.0` | Suitable for rolling upgrades; unify versions afterward. |

The Relay does not need to know the Target pool size and never decrypts payloads. Keep `secret`, `token`, `remote`, and `tunnel.remote` aligned between peers.

## Configuration migration

For the recommended Target configuration, set:

```json
{
  "tunnel": {
    "pool": 0
  }
}
```

`0` is demand-driven mode, `1` is one fixed session, and `N` pre-opens N independent sessions (`1–65535`). Each multi-rule Target route can set its own pool.

## Recommended upgrade

1. Download the `v0.3.0` archive for the platform and verify it with `SHA256SUMS`.
2. Back up each `molex.json` and Web password file.
3. Upgrade Relay first, then restart and check `/healthz`.
4. Upgrade Target, set `tunnel.pool` to `0`, and confirm the WebUI reports a running adaptive pool.
5. Upgrade Edges one at a time. An Edge only reports its local listener after a secure route is ready.
6. Confirm all Edges are `paired` in the Relay console and use peer IDs to distinguish duplicate names.
7. Run one TCP check from every Edge local endpoint.

Rolling upgrades are supported. Do not stop the only Relay and only Target simultaneously.

## Rollback and acceptance

Stop the `v0.3.0` Target, restore the previous binary/configuration, and set `pool: 1` for legacy single-session behavior. Keep the route secret, token, channel, and WSS URL unchanged. Verify version output, paired peers, TCP traffic, Target restart recovery, `/healthz`, and system resource usage.

See the [complete user guide](user-guide.md), [architecture](architecture.md), and [testing checks](testing.md).
