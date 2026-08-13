# MoleX v0.3.1 发行说明

[English](release-v0.3.1.md) | **简体中文**

`v0.3.1` 是面向长期运行稳定性和开箱即用体验的补丁版本，重点修复 `v0.3.0` 自适应 Target 池的备用连接循环和潜在 Socket 增殖。

## 必须升级哪些端

- **Relay：必须升级。** 新版 Relay 会长期保留未配对 Target 热备，不再按普通 Edge 配对超时关闭它。
- **Target：必须升级。** 新版 Target 的热备握手可长期等待，停止时仍能立即取消；每个池槽只扩容一次，反复重连不会累积多余 Socket。
- **Edge：协议兼容，可暂留 `v0.3.0`。** 建议随后滚动统一到 `v0.3.1`，便于版本管理。

只升级 Relay 或只升级 Target 都不能完整解决 `v0.3.0` 的周期性接入/断开现象。生产环境请先升级 Relay，再升级 Target，最后按需滚动升级 Edge。

## 修复与改进

- Relay 将等待中的 Target 视为长期热备，Edge 的过期等待仍保持有界。
- Target 等待对端 hello 时不再受 15 秒短握手期限影响；收到 hello 后，密钥确认仍使用新的 15 秒安全期限。
- 上下文取消会主动关闭握手 WebSocket，避免停止过程卡在读取。
- 自适应池槽首次配对后只触发一次扩容，消除长期网络抖动下的备用 Socket 增殖。
- WebUI 默认优先监听 `127.0.0.1:9090`，冲突时自动向后选择可用回环端口，成功后打印 URL 并打开默认浏览器。
- 显式 `--listen` 严格固定端口；服务器、SSH 转发和反向代理建议同时使用 `--open-browser=false`。
- CLI 的 Target 池帮助文本修正为 `0`（自适应）或 `1-65535`。

## 验证范围

- Target 热备等待超过旧的 15 秒握手期限后，仍使用原连接完成配对和真实 TCP Echo。
- 4 个 Edge 与 4 个 Target 会话并发转发，无串流。
- 3 个 Edge 经故障代理经历延迟、连接拒绝、突然断联和三轮网络抖动后全部恢复。
- Target 重启、Edge 监听占用恢复、FIFO 排队、有界停止、错误指导和握手取消。
- 全量 Go 测试、竞态检测、`go vet`、前端测试和构建。
- Windows、Linux、macOS 的 amd64/arm64 六个发布目标交叉编译。

## WebUI 运维

交互使用可以直接运行：

```bash
molex web --config molex.json --autostart
```

服务器和固定反向代理使用：

```bash
molex web --config molex.json --autostart \
  --listen 127.0.0.1:9090 \
  --open-browser=false
```

完整升级与回滚步骤见[版本升级指南](upgrade-guide.zh-CN.md)。
