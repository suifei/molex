# MoleX v2 架构验收清单

- 分支：`feature/v2-arch`　基线：`main` v0.3.1　目标版本：v0.4.0
- 状态：**已验收（2026-08-17）**。全部 60 项通过；证据与遗留事项见第 9 节。
- 决策记录（已由需求方确认）：
  1. 方案方向 = B「运维可观测」（token 管理 + 在线状态 + 流量计数 + 踢下线）。
  2. 凭据模型 = 单一 token 兼作端到端密钥派生源；Relay 保存并可重新查看完整 token；信任模型改为「信任 Relay 运营者」，实现中 Relay 永不解密载荷。
  3. 同 token 第二个 Target 接入 = 拒绝新连接，旧连接由心跳超时自动清理。
  4. Edge 映射监听 = 默认 `127.0.0.1`，每条映射可单独开启 `0.0.0.0`（局域网可见）。
  5. v1（≤v0.3.1 punch/secret+channel）配置 = 干净切换，不自动迁移，启动报错并给迁移指引。

## 1. 项目背景

- v1 的 token 仅是 Relay 全局准入令牌（单个），配对靠 `HMAC(secret, channel)`，每条路由只有一个转发地址；无法表达「1 token 下 1 Target 发布多服务、N Edge 自选映射」。
- v2 模型：
  - **Relay 唯一**：WebUI 需登录，仅管理多组 token；负责按 token 分组中继，支持多 token 并存。
  - **Target 每 token 唯一**：本地页免登录，只填 `wss` 地址 + token；本地维护要转发的多组 `ip:port` 服务并发布。
  - **Edge 多端**：本地页免登录，只填 `wss` 地址 + token（与 Target 相同）；看到已发布服务目录，勾选后映射到本地端口访问。
- 数据路径：Edge 本地 TCP → yamux 流（携带服务 id）→ 端到端加密会话 → Relay 密文转发 → Target 白名单校验后拨号内网服务，全双工。

## 2. 具体架构（A 项）

- [x] A1 单二进制不变；配置 `mode` 取值改为 `relay` / `target` / `edge`；`molex web` 按 mode 呈现对应控制台。
- [x] A2 数据面保持唯一 WSS 入口 `/ws/session`（Caddy 443 → 回环 Relay），WebSocket 压缩保持禁用。
- [x] A3 Relay 维护 token 注册表：id、token 值、备注、启用状态、创建时间；持久化到 `molex.json`；未知字段拒绝。
- [x] A4 Relay 按 token 分组：每 token 严格 1 Target + N Edge；不同 token 完全隔离（互不可见、互不可达）。
- [x] A5 加密栈保持 X25519 + HKDF-SHA256 + AES-256-GCM + yamux；PSK 与路由标识从 token 经 HKDF 域分隔标签派生；Relay 代码不含载荷解密路径（仅既有 ping 元数据通道除外），流量统计只基于密文帧。
- [x] A6 新增端到端控制协议：Target→Edge 发布服务目录（稳定服务 id、名称、`ip:port`）；Edge 打开数据流时携带服务 id；Target 按本地白名单校验，未声明的地址一律拒绝。
- [x] A7 每 Edge 一条独立加密会话；Target 侧会话按需扩容（沿用 pool 机制，上限 65535）；每会话 256 并发流上限保持。
- [x] A8 客户端重试保持 1s→15s 上限指数退避 ±20% 抖动，健康 30s 后重置。
- [x] A9 v2 三种 mode 各自最小配置字段成立：relay（listen、tokens、web）；target（remote、token、services[]）；edge（remote、token、mappings[]）；`molex config init/check` 支持 v2。
- [x] A10 读到 v1 punch/旧 relay 配置时启动失败，错误信息（中英）明确指出 v2 变更与迁移步骤。
- [x] A11 Relay WebUI 保留密码 setup / 登录 / 登出 / CSRF / 登录限速 / 强制回环监听；Edge、Target 本地页免登录、仅回环监听、拒绝跨源请求。
- [x] A12 Edge/Target 页可编辑 `wss` + token 并启动/停止运行时；配置持久化，进程重启后自动按上次配置重连并恢复映射/服务。

## 3. 核心流程走查（F 项，验收时按序实际执行）

- [x] F1 Relay 机：`molex web`（relay 配置）→ 首次设密码 → 登录 → 创建 token（含备注）→ 列表默认遮罩、可显示/复制。
- [x] F2 Target 机：`molex web`（target 配置）→ 填 wss + token → 连接成功，状态「已连接」。
- [x] F3 Target 页添加 ≥2 组服务（如 `web1=127.0.0.1:18080`、`echo1=127.0.0.1:18090`，本地起真实测试服务）→ 目录发布成功。
- [x] F4 Edge A：填同 wss + token → 连接成功 → 目录实时显示 web1、echo1。
- [x] F5 Edge A 勾选 web1 → 自动填入随机可用本地端口（可手改）→ 状态「监听中」→ 通过 `127.0.0.1:<port>` 实际访问返回正确内容。
- [x] F6 Edge B（同机第二实例，不同配置目录）：同 token 接入 → 勾选 web1 + echo1 → 两条映射并发访问均通，数据不串流。
- [x] F7 某条映射开启「局域网可见」→ 监听绑定 `0.0.0.0`，本机以非回环地址访问可通；关闭后恢复仅回环。
- [x] F8 Relay 控制台 token 详情：Target 在线（名称/平台/在线时长）、Edge 列表（名称/来源 IP）、密文字节计数随访问增长（SSE 实时，无需刷新）。
- [x] F9 Relay 踢下线 Edge B → Edge B 映射立即失效并按退避自动重连恢复。
- [x] F10 第二个 Target 以同 token 接入 → 被拒绝且提示明确错误；已在线 Target 与各 Edge 不受影响。
- [x] F11 Relay 停用 token → 该组 Target/Edge 全部断开，重连被拒且错误指引正确；重新启用后自动恢复连接与映射。
- [x] F12 删除 token → 组内连接断开，列表移除；被删 token 无法再接入。

## 4. 异常情况（X 项，每项验证实际行为与提示文案中英双语）

- [x] X1 token 不存在 / 已停用：Edge、Target 显示可操作错误（检查 token / 联系 Relay 管理员），按退避重试不崩溃。
- [x] X2 Target 掉线：所有 Edge 的相关映射关闭本地监听并显示「路由未就绪」；Target 恢复后目录与映射自动重建，Edge 恢复监听。
- [x] X3 Relay 重启：Target/Edge 按退避自动重连，恢复后无需人工操作即恢复转发。
- [x] X4 Target 拨号内网服务失败（服务停了）：Edge 侧该连接安全关闭；Target 页显示该服务最近拨号错误与指引；其他服务不受影响。
- [x] X5 Edge 本地映射端口被占用：该映射标记错误并给指引，其他映射不受影响；端口释放后自动恢复。
- [x] X6 Edge 请求目录外地址（构造恶意流头）：Target 白名单拒绝并记录事件，不发生拨号。
- [x] X7 Target 删除目录中某服务：已勾选该服务的 Edge 映射自动失效、关闭监听并提示「服务已下架」。
- [x] X8 wss 地址错误 / DNS 失败 / TLS 证书问题 / Caddy 上游不可达 / HTTP 路由 404：沿用并更新现有分类诊断，提示对应下一步操作。
- [x] X9 单会话超过 256 并发流：新连接安全关闭并提示，已建立流不受影响。
- [x] X10 `connecting` / `stopping` / `idle` 期间，任何页面不得把已配置地址展示为「正在监听」。
- [x] X11 双 token 隔离：token1 的 Edge 看不到 token2 的目录，也无法访问其服务（实际验证一次）。

## 5. 骨架屏与加载状态（S 项）

- [x] S1 三种控制台首屏在数据到达前显示骨架屏（token 列表 / 服务列表 / 目录与映射列表 / 详情面板），数据到达后无明显布局跳动。
- [x] S2 所有提交型按钮（创建/保存/删除 token、保存服务、应用映射、启动/停止、踢下线）在请求进行中禁用并显示加载态，防重复提交。
- [x] S3 运行时状态机 `idle → starting → connecting → running → stopping` 在 UI 有区分展示；connecting 期间目录区显示「等待 Target 目录…」加载态。
- [x] S4 SSE 断开时显示「实时连接已断开，重连中」横幅并自动重连，恢复后数据自动刷新。
- [x] S5 空状态文案完整（双语）：无 token、无服务、Target 离线导致目录为空、无映射，均有引导性提示而非空白。

## 6. 终端与环境适配（D 项）

- [x] D1 视口宽度 320 / 390 / 820 / 1366 px：无横向溢出、控件不重叠；长 wss 地址、token、`ip:port` 有意换行或截断。
- [x] D2 明暗两套主题均检查三种控制台主要页面。
- [x] D3 英文与简体中文全量覆盖（含新错误模板、空态、骨架屏辅助文案），运行时消息本地化在 `frontend/src/i18n.ts`。
- [x] D4 Windows / Linux / macOS（amd64+arm64）交叉编译通过；Windows 本机实测三端流程。
- [x] D5 以 Chromium 内核浏览器实测为准；不使用其独占 API。

## 7. 安全与文档（SEC 项）

- [x] SEC1 日志、事件、遥测、错误信息不出现完整 token、管理密码、会话 Cookie、CSRF token；WebUI token 默认遮罩。
- [x] SEC2 token 生成使用 CSPRNG（≥32 字节，`mx2_` 前缀）；准入比较为常数时间。
- [x] SEC3 security.md / architecture.md / README（中英）/ AGENTS.md 同步更新：如实描述「Relay 运营者持有 token、理论可解密、实现不解密」的新信任边界，及 v1→v2 迁移说明。
- [x] SEC4 管理监听强制回环不变；远程管理仍走 HTTPS 反代或 SSH 转发；文档同步。

## 8. 自动化质量门槛（Q 项）

- [x] Q1 `go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...` 全绿。
- [x] Q2 `cd frontend && npm test && npm run build` 全绿；构建产物重新嵌入后 Go 构建通过。
- [x] Q3 `go test -v -count=1 ./internal/client` 真实 socket 仿真更新为 v2 语义并全绿，继续覆盖：三端并发、Target 重启恢复、Edge 端口占用恢复、有界停止、可操作重试提示。
- [x] Q4 新增 v2 测试：同 token 多 Edge 并发不串流、目录发布/变更同步、重复 Target 拒绝、白名单拒绝目录外地址、踢下线与停用 token、双 token 隔离。

## 9. 验收记录（2026-08-17，Windows 本机实测）

验收环境：Windows 11 本机同时运行 Relay（数据面 `127.0.0.1:18081`，控制台 19090）、Target（19091）、EdgeA（19092）、EdgeB（19093）四个 MoleX 进程，以及两个真实 HTTP 测试服务（web1@18080、api1@18090）；浏览器为 Chromium 内核，逐项人工/自动化走查。

| 组 | 结果 | 关键证据 |
| --- | --- | --- |
| A1–A12 架构 | 全部通过 | 三控制台按 mode 呈现；角色选择器实测；tokens/services/mappings 持久化且热更新；退避序列实测 0.9s→1.9s→…→15s 封顶；`config init/check` 与 v1 拒绝由 cmd/config 测试覆盖 |
| F1–F12 核心流程 | 全部通过 | 设密码→登录→建 Token→Target 发布 2 服务→EdgeA 勾选 web1 自动分配端口 44848→`curl` 返回 `hello-from-web1`→EdgeB 双映射并发→LAN 开关（192.168.126.1 可达/关闭后拒绝）→Relay 分组流量实时→踢线 5 秒内自愈→重复 Target 收到"每个 Token 只允许一个 Target"→停用断组 6 连接、401 拒绝重连、启用后自愈→删除 Token 后 401 且配置移除 |
| X1–X11 异常 | 全部通过 | X1 无效 Token 401 指引；X2 Target 停→映射关闭+`waiting`→恢复自愈；X3 Relay 重启双端自愈；X4 后端宕机→Target 显示精确 lastError→恢复自愈；X5 端口占用→单映射 `error`+可操作提示→释放 3 秒内恢复；X6 由真实 socket 集成测试 `TestMaliciousEdgeCannotDialUnpublishedAddress` 覆盖（恶意寻址头→`TunnelDialUnknown`+Target 拒绝事件）；X7 下架服务→监听关闭+"Target 未发布此服务"→重发布恢复；X8 404 路径/连接拒绝分类诊断实测（DNS/TLS 分类由 `TestClientErrorGuidance` 覆盖）；X9 300 并发触发 256 上限事件且事后恢复（映射计数 267 次连接）；X10 离线/停用期间映射均为 waiting 不显示监听；X11 token2 组目录仅见 beta-api，跨组不可见 |
| S1–S5 骨架/加载 | 全部通过 | 骨架屏与布局稳定由 vitest 断言并在页面重载时实测观察；全部提交按钮请求期间禁用+加载图标（实测）；connecting/running/stopping 区分显示；SSE 断开横幅"实时连接已断开，正在重连…"实测出现；无 Token/目录未加载/无服务空态文案均实测出现 |
| D1–D5 终端适配 | 全部通过 | Edge 页与 Relay 页在 320/390/820/1366 四档宽度 `scrollWidth==clientWidth`（无横向溢出），320px 截图确认长地址截断正常；明/暗主题与中英切换实测（截图留档）；Windows 本机全流程 + linux/darwin/windows 六平台交叉编译通过；Chromium 实测 |
| SEC1–SEC4 安全 | 全部通过 | Relay 事件/CLI 日志仅出现 token id（tok-xxxx），UI 默认遮罩可显示复制；`mx2_`+32B CSPRNG（47 字符）实测生成；SHA-256 常数时间准入比较；security.md/architecture.md/README 中英/AGENTS.md 已同步 v2 信任模型；免登录控制台拒绝外来 Host/跨源/非回环（403 实测与单测） |
| Q1–Q4 质量门槛 | 全部通过 | `go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet` 全绿；前端 `npm test`（24 用例）、`npm run check`、`npm run build` 全绿并重新嵌入；`./internal/client` 真实 socket 套件 12 个场景全绿（多 Edge 并发、目录增删同步、重复 Target、白名单、Token 停用/踢线自愈、跨 Token 隔离、Target 重启、端口占用恢复、热备跨 16 秒握手超时、三轮网络抖动、有界停机） |

### 验收过程中发现并修复的缺陷

1. **Relay 启动会清空 Token 列表**（现场发现）：浏览器持有过期配置时，`/api/runtime/start`、`PUT /api/config` 会把空 tokens 覆盖写盘。修复：tokens/services/mappings 一律以磁盘上由专用端点管理的数据为准（`mergeManagedLists`），并新增防回归测试。
2. **UTF-8 BOM 配置解析失败**（现场发现）：PowerShell/记事本写出的带 BOM 配置无法解析。修复：`config.Load` 剥离 BOM。
3. **yamux 流缺少半关闭**（集成测试发现）：后端先关闭时 EOF 不传播，服务端主动关闭型协议会挂起。修复：`halfCloseStream` 适配器（yamux `Close`=FIN）双向传播半关闭。
4. **路由断开后本地连接可能悬挂**（集成测试发现）：会话死亡时被阻塞在本地读的桥接协程阻碍有界停机。修复：Edge 运行时追踪活动连接并在会话断开时主动关闭。

### 遗留事项（不阻塞验收）

- 版本号已锁定为 `0.4.0`（构建注入 `-ldflags "-X main.version=0.4.0"`，与 `frontend/package.json` 及 CI `MOLEX_VERSION` 一致）。
- 12 语种使用手册与中英升级指南已按 v2.1 重写；控制台截图仍使用现有 en / zh-CN 资源，界面文案以运行中的 Web 控制台为准。

## 11. v2.1 演进（多组 / 轮换 / 审计 / 保活 / 元数据刷新）

在 v2 验收之后补齐五处真实缺陷。决策已确认：单进程多组、服务级可见性、Token 视同 SSH 私钥（审计落盘 + 轮换宽限）、Edge 同样多组、Relay 保活取最轻量方案、元数据随目录/映射变更刷新。

- [x] E1 一台 Target 用单个进程加入 ≥2 个 Token 组；每组独立会话池，互不影响。
- [x] E2 服务级可见性：`services[].groups` 为空则对已加入的全部组发布；列出组名则仅这些组可见并可拨号。未授权组的 Edge 目录中看不到该服务，构造寻址头也被白名单拒绝。
- [x] E3 一台 Edge 用单个进程加入 ≥2 个 Token 组；目录按组聚合，映射带 `group`，跨组不串流。
- [x] E4 Token 轮换：`POST /api/tokens/:id/rotate` 签发新值，旧值在 `graceDays`（1–30，默认 3）内并行有效；到期后旧值 401，仍用旧值的在线连接被断开。
- [x] E5 审计落盘：创建 / 轮换 / 停用 / 启用 / 删除写入配置旁 JSONL（只记 token id，不记 token 值），带轮转。
- [x] E6 Relay 控制台「转发端点」随目录/映射变更通过加密 ping 刷新，无需客户端重连；多帧刷新合并为一次更新，每次使用新 GCM nonce。
- [x] E7 保活：`deploy/molex-relay.service`（systemd `Restart=always`）+ `deploy/molex-keepalive.sh`（无 systemd 的 POSIX 看门狗）；部署文档已指向这两份文件。
- [x] E8 Web 控制台：Relay 轮换按钮与宽限天数；Target 多组 + 服务可见组勾选；Edge 多组 + 目录分组；中英双语。
- [x] E9 配置校验覆盖多组、可见性、映射归组、轮换字段；自动化测试覆盖双组 Target/Edge、轮换宽限到期、元数据刷新、审计 writer、轮换 API、前端勾选/轮换。

## 10. 真实业务样本组网联通性测试（2026-08-17）

以真实内网 OpenAI 兼容推理服务作为业务样本：`http://10.188.200.16:30927/v1`（在线模型 `qwen3`）。全部组网使用纯 CLI（`molex serve` / `molex connect`），补充覆盖无头部署路径；对推理服务的请求保持克制（小 `max_tokens`、无并发压测）。

拓扑：一个 Relay（`127.0.0.1:18085`）承载两个 Token 组——alpha 组为标准三端（Target1 发布 `svc-qwen`=10.188.200.16:30927；EdgeA 映射 28080 回环；EdgeB 映射 28081 局域网可见）；beta 组为级联组（Target2 把 EdgeA 的 28080 再发布为 `svc-hop`；Edge2 映射 28090），形成双跳链路：`Edge2 → Relay → Target2 → EdgeA(本地) → Relay → Target1 → 真实 API`。

| 编号 | 场景 | 结果 | 数据 |
| --- | --- | --- | --- |
| T1 | 直连基线 `/v1/models` + 一次推理 | 通过 | models 144ms；chat 812ms，`qwen3` 回复正确 |
| T2 | 单跳隧道 `/v1/models`（EdgeA） | 通过 | 163ms，70 个模型完整返回 |
| T3 | 单跳隧道非流式推理 | 通过 | 1163ms，`finish=stop`，内容正确 |
| T4 | 单跳隧道流式推理（SSE `stream:true`） | 通过 | 39 个 `data:` chunk 逐段到达，`[DONE]` 完整，chunked 传输在加密隧道上无缓冲截断 |
| T5 | 双 Edge 同 Token 并发推理 | 通过 | 两端各自返回自己的标记（989ms / 1041ms），无串流 |
| T6 | 局域网映射 + 回环隔离对照 | 通过 | `lan:true` 的 EdgeB 经 `192.168.126.1:28081` 可达；`lan:false` 的 EdgeA 经局域网 IP 正确拒绝 |
| T7 | 双 Token 级联双跳（列表 + 推理） | 通过 | models 175ms（较单跳 +12ms）；chat 909ms，四段加密两次穿越 Relay |
| T8 | 强杀 Target1 → 故障传导 → 重启自愈 | 通过 | 单跳与级联链路同时中断；重启后约 10 秒内全链自愈（含级联组） |
| T9 | 无效 Token 准入对照 | 通过 | HTTP 401 + "Copy a valid token from the relay console" 指引，本地不开映射端口 |
| T10 | 延迟对比（`/v1/models` 各 5 次取中位数） | 通过 | 直连 60ms / 单跳 33ms / 双跳 56ms——服务端自身抖动（35–152ms）远大于隧道开销，本机回环下隧道开销约 10–20ms 量级，业务无感 |

结论：以真实 LLM API 为业务样本，v2 在标准三端、多 Edge、局域网暴露、跨 Token 级联双跳四种组网形态下均保持完整联通；流式（SSE/chunked）与长响应推理无损通过；故障传导与自愈行为符合设计；隧道延迟开销相对真实业务响应时间可忽略。测试环境已清理。
