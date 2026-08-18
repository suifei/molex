# MoleX 版本升级指南

[English](upgrade-guide.md) | **简体中文**

Relay、Edge、Target 是同一二进制的不同角色，按主机分别替换即可。**v0.4.0（v2）相对 `v0.3.1` 及更早版本是干净切换**：旧的 `punch` / `secret` / `channel` 配置会在启动时失败并给出迁移指引。

## 版本差异

| 版本 | 主要内容 | 对部署的影响 |
| --- | --- | --- |
| `v0.1.0`–`v0.3.1` | v1 punch 模型：全局 Relay token + 每条路由的 `secret` 与 `channel`。若仍停留在这些版本，见文末归档说明。 | v2 不会读取 v1 文件。 |
| `v0.4.0`（v2） | 角色改为 `relay` / `target` / `edge`。一个 Token = 一个 Target + N 个 Edge。Target 发布服务目录，Edge 勾选映射。Token 轮换宽限、JSONL 审计、单进程多组、元数据热刷新。 | 每份 `molex.json` 都要重建。Relay、Target、Edge 一起升级。 |

## 从 v1（`≤v0.3.1`）升到 v2

v2 **不会**自动迁移。`mode: "punch"` 以及 `role` / `secret` / `tunnel` 字段会导致启动错误，并指向本文。

1. 下载 v2 包并用 `SHA256SUMS` 校验。
2. 备份每份 `molex.json` 和 Relay 的 web-password。旧 JSON 不能原样继续用。
3. Relay：`molex config init --mode relay --force`。启动 `molex web`，设置管理密码，为每个信任组创建一个 Token。原来的一对 `secret` + `channel` 对应一个 Token。
4. 原 Target 机器：`molex config init --mode target --force`。`remote` 填同一 `wss://…/ws/session`，粘贴 Token，把旧的 `tunnel.local`（以及多规则里的各个 `local`）逐条加为已发布服务。
5. 原 Edge 机器：`molex config init --mode edge --force`。粘贴同一 Token，启动后把已发布服务映射到原来使用的本地端口。
6. 废弃 v1 的 `secret` 与 `channel`，它们不再是 v2 凭据。
7. 在 Relay 控制台确认：每个 Token 一个 Target、Edge 在线、密文计数在增长，并从每条 Edge 映射做一次 TCP 检查。

v1 与 v2 不能混跑。hello、凭据和目录协议都不同。按 Token 组整体切换（先 Relay，再该组的 Target 与 Edge）。

### v2 配置示例

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

## 已在 v2 上的后续操作

Token 轮换（`POST /api/tokens/:id/rotate` 或 Relay 控制台的「轮换」）会让旧值在 1–30 天内继续有效（默认 3 天）。请在到期前更新所有 Target 和 Edge。新建 Token 也可设置自身有效期（`never` / `1d` / `7d` / `30d` / `90d` / `365d`），之后用 `PUT /api/tokens/:id` 改期。审计只记录 token id。

单个 Target 或 Edge 进程可通过 `tokens[]` 加入多组。用 `services[].groups` 限制服务可见性。加入多组时，Edge 映射必须带 `group`。

Linux Relay 保活：`deploy/molex-relay.service`。没有 systemd 时：`deploy/molex-keepalive.sh`。

## 从 v2 回滚

1. 停止 v2 进程。
2. 恢复 v1 二进制和已备份的 punch 配置。
3. 把 Caddy 指回旧 Relay。v2 Token 不能用于 v1，v1 secret 也不能用于 v2。

## v2 切换验收

- [ ] `molex version` 显示 v2 构建。
- [ ] `molex config check` 接受新文件，并拒绝残留的 punch 文件。
- [ ] Relay 控制台：Token 列表、每 Token 一个 Target、Edge 在线。
- [ ] Target 目录与已发布服务一致；Edge 仅在运行中显示监听。
- [ ] 至少一条映射的 TCP 检查通过。
- [ ] 重复 Target 被拒绝；停用 Token 会断开整组。
- [ ] 数据面与管理面 `/healthz` 成功。

详见[使用手册](user-guide.zh-CN.md)、[架构与协议](architecture.md)和[测试说明](testing.md)。

## 归档：v0.1.0–v0.3.1（仅 v1）

仅在仍于 v1 各版本之间迁移、尚未切到 v2 时使用。

| 版本 | 说明 |
| --- | --- |
| `v0.1.0` | 每条不透明路由基本一对 Edge/Target。 |
| `v0.2.0` | FIFO 队列；多 Edge 同通道需要 Relay `>=v0.2.0`。 |
| `v0.3.0` | `tunnel.pool: 0` 按需 Target 会话。 |
| `v0.3.1` | 长期 Target 热备；升级 Relay 与 Target 以消除热备抖动。 |

v1 对端必须保持 `secret`、`token`、`remote`、`tunnel.remote` 一致。该布局在 v2 中已废弃。
