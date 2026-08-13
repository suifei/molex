# MoleX 版本升级指南

[English](upgrade-guide.md) | **简体中文**

本文说明 `v0.1.0` 至 `v0.3.1` 的主要差异、兼容关系以及生产环境升级步骤。MoleX 的 Relay、Edge、Target 都是同一个二进制的不同运行角色；升级时请根据角色分别替换对应主机上的二进制。

## 版本差异

| 版本 | 主要内容 | 对部署的影响 |
| --- | --- | --- |
| `v0.1.0` | 首次公开版本；Relay、Edge、Target 的基本 WSS/TCP 转发、WebUI 登录和自动发布流程。 | 同一不透明路由默认按一对 Edge/Target 会合，不能满足多个 Edge 共享一个 Target。 |
| `v0.2.0` | Relay 为每个路由维护 Edge/Target FIFO 等待队列；允许同角色客户端排队和同名 Node name；首次运行 WebUI 密码设置；Target 固定会话池；多转发规则 CRUD；Caddy 配置辅助；更完整的连接元数据和错误指导。 | Relay 至少升级到 `v0.2.0`，才能让多个 Edge 在同一通道排队并按 FIFO 配对。Target 的 `pool` 默认仍为 `1`。 |
| `v0.3.0` | 单 Target 多 Edge 的按需会话扩容。`tunnel.pool: 0` 时，Target 每完成一个配对就建立下一条独立 WSS 会话，最多 65,535 条；保留固定池 `1–65535`。 | 推荐升级 Target 到 `v0.3.0` 并把 `pool` 设为 `0`。Relay/Edge 可保持 `v0.2.0`，但完整的运行状态和文案建议三端最终统一到 `v0.3.0`。 |
| `v0.3.1` | Relay 将未配对 Target 保留为长期热备，消除自动池备用会话因配对/握手超时产生的断开重连假告警；Target 热备等待不再受短握手期限影响，取消时仍立即退出；每个池槽只扩容一次，避免重连累积 Socket；WebUI 默认从 `9090` 自动避让端口并在就绪后打开浏览器；补充多 Edge 与网络故障测试。 | **必须升级 Relay 和 Target** 才能完整消除备用 Target 的接入/断开循环及长期 Socket 增殖风险。Edge 协议兼容，可滚动升级。服务器应固定 WebUI 端口并使用 `--open-browser=false`。 |

## 升级哪些端

目标拓扑为：

```text
Edge 1 ─┐
Edge 2 ─┼──> Relay 1 ───> Target 1
Edge N ─┘
```

| 当前版本 | Relay | Target | Edge | 结果 |
| --- | --- | --- | --- | --- |
| `v0.1.0` | 必须升级到 `>=v0.2.0` | 建议升级 | 可暂不升级 | 旧 Relay 不支持同一路由多 Edge 队列。 |
| `v0.2.0` | 可暂不升级 | 升级到 `v0.3.0` | 可暂不升级 | Target 使用 `pool: 0` 后可按需接入多个 Edge；协议仍兼容。 |
| 混合运行 | `v0.3.0` | `v0.3.0` | `v0.2.0` 或 `v0.3.0` | 适合滚动升级；建议最终三端统一版本。 |
| 修复 `v0.3.0` 热备重连 | 升级到 `v0.3.1` | 升级到 `v0.3.1` | 可保持 `v0.3.0` | Relay 与 Target 分别修复配对等待、握手期限和池槽扩容。 |

Relay 不需要知道 Target 的会话池数量，也不解密端到端载荷；因此 `v0.2.0` Relay 可以转发 `v0.3.0` Target 新建立的独立会话。Edge 与 Target 必须继续保持相同的 `secret`、`token`、`remote` 和 `tunnel.remote`，角色必须互补。

## 配置迁移

### 从 `v0.2.0` 升级 Target

备份原配置后，将 Target 的池大小改为 `0`：

```json
{
  "mode": "punch",
  "role": "target",
  "secret": "与 Edge 相同的端到端密钥",
  "token": "与 Relay 相同的令牌",
  "remote": "wss://molex.example.com/ws/session",
  "tunnel": {
    "local": "127.0.0.1:22",
    "remote": "home-ssh",
    "name": "home-target",
    "pool": 0
  }
}
```

含义：

- `0`：按需扩容，推荐用于一个 Target 服务多个 Edge。
- `1`：固定一条会话，适合单 Edge 或保守迁移。
- `N`：预先建立 N 条独立会话，范围 `1–65535`。

多规则配置中，每条 Target 规则也可以单独设置 `pool`；未填写时使用自动模式。

### 从 `v0.1.0` 升级

`molex.json` 保持向后兼容。先升级 Relay，再升级客户端；不要把 `secret`、`token` 或 Web 管理密码写入公开脚本。首次运行的 WebUI 会引导创建管理密码，不再要求手工创建 `web-password`。

## 推荐升级步骤

1. 下载对应平台的 `v0.3.1` 包，并用 `SHA256SUMS` 校验完整性。
2. 备份 Relay、Target、Edge 的 `molex.json` 和 Web 管理密码文件。
3. 先升级 Relay：停止旧进程，替换二进制，启动并检查本机 `/healthz`。
4. 升级 Target：替换二进制，将 `tunnel.pool` 改为 `0`，启动 WebUI 并确认状态为运行中。
5. 升级 Edge：逐台替换二进制并启动。Edge 只有在安全路由就绪后才会显示本地监听。
6. 在 Relay WebUI 中确认多个 Edge 均显示为 `paired`，名称相同的节点由不同 peer ID 区分。
7. 从每个 Edge 的本地端口发起一次 TCP 测试，并确认目标服务收到请求。

生产环境可以滚动升级。升级期间短暂断线会触发客户端退避重连；不要同时停止唯一的 Relay 和唯一的 Target。若必须升级 Relay，先准备好新进程再切换 Caddy 上游，以缩短中断时间。

## 回滚

如果升级后出现异常：

1. 停止当前 Target。
2. 恢复备份的二进制和配置；把 `pool` 改回 `1` 可回到单会话模式。
3. 保持 Relay token、端到端 secret、channel 和 WSS URL 不变。
4. 启动旧 Target，等待 Edge 自动重连，再检查 Relay WebUI 的配对状态。

配置文件格式没有破坏性变化；`pool: 0` 仅由支持按需池的 `v0.3.0` Target 解释。旧版 Target 不理解自动池时，请使用 `pool: 1`。

## 升级验收清单

- [ ] `molex version` 显示期望版本。
- [ ] Relay、Target、Edge 的 `remote`、`token`、`secret` 和 channel 配置一致。
- [ ] Target 使用 `pool: 0` 或明确的固定池大小。
- [ ] Relay WebUI 显示每条连接的来源 IP、端点、角色、状态和 peer ID。
- [ ] 多个 Edge 可以同时建立 TCP 连接，数据没有串流。
- [ ] 停止并重新启动 Target 后，Edge 能收到指导性重连提示并恢复。
- [ ] Edge 在路由未就绪时显示“未监听”，没有误报为正在监听。
- [ ] 检查 `/healthz`、服务日志和系统资源，确认没有异常的 Socket 或文件描述符增长。

详见[完整图文使用手册](user-guide.zh-CN.md)、[架构与协议](architecture.md)和[测试与发布检查](testing.md)。
