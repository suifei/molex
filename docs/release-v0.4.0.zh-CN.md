# MoleX v0.4.0 发行说明

[English](release-v0.4.0.md) | **简体中文**

`v0.4.0` 是 v2 架构：Token 分组、已发布服务目录，以及相对 v1 punch 模型的干净切换。Relay、Target、Edge 必须一起升级。旧的 `punch` / `secret` / `channel` 文件会在启动时失败并给出迁移指引。

## 破坏性变更

v1（`≤v0.3.1`）配置不会被读取。请用 `molex config init --mode relay|target|edge` 重建每份 `molex.json`。v1 与 v2 不能混跑。按 Token 组切换：先 Relay，再该组的 Target 与 Edge。

详见[升级指南](upgrade-guide.zh-CN.md)。

## 必须升级哪些端

- **Relay：必须升级。** Token 注册表、每 Token 一个 Target + N 个 Edge、只转发密文、轮换/停用/删除、JSONL 审计。
- **Target：必须升级。** 实时发布服务目录，只拨号已发布地址；单进程可加入多组 Token。
- **Edge：必须升级。** 把已发布服务映射到本地端口；监听只在路由就绪且服务仍发布时存在。

## 主要变化

- 角色改为 `relay` / `target` / `edge`。一个接入 Token 就是一组信任关系：严格一个 Target，Edge 数量不限。同 Token 第二个 Target 会被拒绝。
- Target 发布 `ip:port` 服务，Edge 勾选需要的项。映射默认 `127.0.0.1`，每条可单独绑定局域网。
- 单个 Target 或 Edge 进程可通过 `tokens[]` 加入多组。用 `services[].groups` 限制可见性。多组时 Edge 映射必须带 `group`。
- 载荷保护仍为 TLS 1.3 内的 X25519 + HKDF-SHA256 + AES-256-GCM。PSK 由 Token 派生。发行版 Relay 永不解密载荷。
- Token 轮换让旧值在 1–30 天内继续有效（默认 3 天）。管理操作只把 token id 写入 JSONL 审计。
- Relay 控制台：密码登录、Token 管理、在线客户端、密文计数。Target / Edge 控制台：免登录、仅回环、同源与 CSRF。
- Linux 保活：`deploy/molex-relay.service` 或 `deploy/molex-keepalive.sh`。
- 目录与映射计数通过加密元数据通道刷新，无需重连。

## 验证范围

- 全量 Go 测试、竞态检测、`go vet`、前端测试与生产构建。
- 真实 Socket 客户端套件：多 Edge 并发、目录发布/下架、重复 Target 拒绝、白名单、Token 停用/踢线自愈、跨 Token 隔离、Target 重启、映射端口占用恢复、有界停机。
- Token 轮换宽限到期、元数据刷新合并、审计 writer、双组 Target/Edge。
- Windows、Linux、macOS 的 amd64/arm64 发布包。

## 安装之后

```bash
molex config init --mode relay --force
molex web --config molex.json --autostart
```

在 Relay 控制台创建 Token，再用同一 `wss://…/ws/session` 与 Token 初始化 Target 和 Edge。不要沿用 v1 的 `secret` 或 `channel`。

交互使用：

```bash
molex web --config molex.json --autostart
```

服务器和反向代理：

```bash
molex web --config molex.json --autostart \
  --listen 127.0.0.1:9090 \
  --open-browser=false
```

详见[使用手册](user-guide.zh-CN.md)、[架构与协议](architecture.md)和[安全模型](security.md)。
