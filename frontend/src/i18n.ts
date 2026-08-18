import type { MappingState, Mode, PeerStatus, Role, RuntimeState } from "./types";

export type Locale = "en" | "zh-CN";

export const LANGUAGE_STORAGE_KEY = "molex:language";

export const copy = {
  en: {
    languageShort: "EN",
    switchLanguage: "Switch to Chinese",
    loading: "Loading MoleX",
    brandSubtitle: "Secure transit console",
    previewData: "Preview data",
    webConsole: "Web console",
    consoleLabels: {
      relay: "Relay console",
      target: "Target console",
      edge: "Edge console",
    } satisfies Record<Mode, string>,
    signInTitle: "Sign in to MoleX",
    signInDescription: "Enter the management password for this relay.",
    password: "Management password",
    signIn: "Sign in",
    firstRunSetup: "First-run setup",
    createPassword: "Create your management password",
    createPasswordDescription: "This password protects the relay Web console on this device. It is stored locally.",
    newPassword: "New password",
    confirmPassword: "Confirm password",
    passwordRequirement: "Use at least 12 characters.",
    passwordMismatch: "The passwords do not match.",
    finishSetup: "Finish setup",
    logout: "Sign out",
    themePreferences: {
      system: "System theme",
      light: "Light theme",
      dark: "Dark theme",
    },
    useSystemTheme: "Follow system theme",
    useLightTheme: "Use light theme",
    useDarkTheme: "Use dark theme",
    chooseRoleTitle: "Choose this device's role",
    chooseRoleDescription: "This console manages one role per device. Pick how this MoleX node participates.",
    roleEdgeTitle: "Edge",
    roleEdgeDescription: "Access published services from this device through local port mappings.",
    roleTargetTitle: "Target",
    roleTargetDescription: "Publish reachable intranet services so edges can map them.",
    roleRelayTitle: "Relay",
    roleRelayDescription: "Relays are servers with a management password. Create the configuration from the command line, then restart the console:",
    copyCommand: "Copy command",
    currentRouteStatus: "Current route status",
    wssEntry: "WSS entry",
    relayHub: "Relay hub",
    pairedPeers: "Paired peers",
    encryptedHub: "Encrypted hub",
    localMappings: "Local mappings",
    publishedServicesNode: "Published services",
    secured: "Secured",
    pairing: "Pairing",
    standby: "Standby",
    connection: "Connection",
    connectionDescriptionTarget: "Point this target at the relay, then publish services below.",
    connectionDescriptionEdge: "Point this edge at the relay; the published catalog appears below.",
    relayConfiguration: "Relay configuration",
    relayConfigurationDescription: "Public rendezvous endpoint behind Caddy.",
    listenAddress: "Listen address",
    wssEndpoint: "Relay WSS address",
    accessToken: "Access token",
    nodeName: "Node name",
    accessTokens: "Access tokens",
    accessTokensDescription: "Each token admits exactly one target and any number of edges. Values stay visible to the relay operator.",
    createToken: "Create token",
    tokenLifetime: "Lifetime",
    tokenLifetimeNever: "Never expires",
    tokenLifetime1d: "1 day",
    tokenLifetime7d: "7 days",
    tokenLifetime30d: "30 days",
    tokenLifetime90d: "90 days",
    tokenLifetime365d: "1 year",
    tokenExpiredTag: "Expired",
    tokenExpiresAt: "Expires",
    tokenNeverExpires: "Never expires",
    changeTokenLifetime: "Change token lifetime",
    tokenNotePlaceholder: "Note, e.g. office-nas",
    noTokensYet: "No tokens yet. Create one and paste it into the target and edge consoles.",
    tokenTargetOnline: "Target online",
    tokenTargetOffline: "No target",
    tokenEdgeCount: "edges",
    tokenDisabledTag: "Disabled",
    disableToken: "Disable",
    enableToken: "Enable",
    deleteToken: "Delete token",
    revealToken: "Show token",
    hideToken: "Hide token",
    copyToken: "Copy token",
    copied: "Copied",
    createdAt: "Created",
    rotateToken: "Rotate",
    tokenGraceUntil: "Previous value valid until",
    tokenGroups: "Token groups",
    tokenGroupsDescription: "Join one or more Relay tokens in this process. A second group does not need another instance.",
    addGroup: "Add group",
    groupName: "Group name",
    groupNamePlaceholder: "Name, e.g. office",
    deleteGroup: "Remove group",
    visibleGroups: "Visible to",
    visibleToAllGroups: "All groups",
    catalogGroup: "Group",
    catalogGroupOffline: "This group is offline",
    graceDays: "Grace days",
    caddySetup: "Caddy setup",
    caddySetupDescription: "Generate a production Caddyfile for the Relay data path and Web console.",
    caddyOfficialGuide: "Official installation guide",
    relayDomain: "Relay domain",
    adminDomain: "Web console domain",
    downloadCaddyfile: "Download Caddyfile",
    publishedServices: "Published services",
    publishedServicesDescription: "Backends this target forwards to. Edges can only map what is listed here.",
    addService: "Add service",
    serviceName: "Service name",
    serviceAddress: "Address (host:port)",
    deleteService: "Delete service",
    saveServices: "Save services",
    noServicesYet: "No services published yet. Add the intranet addresses this target may reach.",
    serviceStreams: "streams",
    serviceLastError: "Last error",
    serviceCatalog: "Service catalog",
    serviceCatalogDescription: "Services published by the target. Check one to map it to a local port.",
    applyMappings: "Apply mappings",
    localPort: "Local port",
    lanVisible: "LAN visible",
    lanVisibleHint: "Bind 0.0.0.0 so other devices on your network can connect.",
    suggestPort: "Pick a free port",
    catalogStartHint: "Start the edge to load the catalog from the target.",
    catalogWaiting: "Waiting for the target service catalog…",
    catalogOffline: "The target is offline. Mappings resume when it reconnects.",
    noCatalogServices: "The target has not published any services yet.",
    mappingUnpublished: "Not published by the target",
    mappingStates: {
      listening: "Listening",
      waiting: "Waiting",
      error: "Error",
    } satisfies Record<MappingState, string>,
    connectionsCount: "conns",
    transferred: "transferred",
    liveUpdatesLost: "Live updates disconnected; reconnecting…",
    loadingData: "Loading data",
    configurationSaved: "Configuration saved",
    save: "Save",
    start: "Start",
    stop: "Stop",
    runtime: "Runtime",
    ready: "Ready",
    refreshRuntime: "Refresh runtime",
    state: "State",
    mode: "Mode",
    role: "Role",
    listen: "Listen",
    notListening: "Not listening",
    notExposed: "Not exposed",
    ciphertextRelay: "Ciphertext relay",
    aesGcmSession: "AES-GCM session",
    trustNote: "Token holders and the relay operator share this group's credentials.",
    connectedClients: "Connected clients",
    noConnectedClients: "No clients connected",
    disconnectPeer: "Disconnect",
    tokenColumn: "Token",
    unknownAddress: "Unknown address",
    connectedAt: "Connected",
    connectedFor: "Online for",
    forwardEndpoint: "Forward endpoint",
    routeId: "Route ID",
    pairedWith: "Paired with",
    waitingForPeer: "Waiting for peer",
    platform: "Platform",
    connectionSource: "Connection",
    trustedProxy: "Trusted proxy",
    directSocket: "Direct socket",
    lastActivity: "Last traffic",
    noTrafficYet: "No traffic yet",
    traffic: "Relay traffic",
    received: "RX",
    sent: "TX",
    frames: "frames",
    ago: "ago",
    unnamedClient: "Unnamed client",
    activity: "Activity",
    noActivity: "No activity yet",
    states: {
      idle: "Idle",
      starting: "Starting",
      connecting: "Connecting",
      running: "Running",
      stopping: "Stopping",
      error: "Error",
    } satisfies Record<RuntimeState, string>,
    modes: {
      relay: "Relay",
      target: "Target",
      edge: "Edge",
    } satisfies Record<Mode, string>,
    roles: {
      edge: "Edge",
      target: "Target",
      hub: "Hub",
    } satisfies Record<Role | "hub", string>,
    peerStates: {
      waiting: "Waiting",
      paired: "Paired",
    } satisfies Record<PeerStatus, string>,
  },
  "zh-CN": {
    languageShort: "中",
    switchLanguage: "切换到英文",
    loading: "正在载入 MoleX",
    brandSubtitle: "安全传输控制台",
    previewData: "预览数据",
    webConsole: "Web 控制台",
    consoleLabels: {
      relay: "Relay 控制台",
      target: "Target 控制台",
      edge: "Edge 控制台",
    } satisfies Record<Mode, string>,
    signInTitle: "登录 MoleX",
    signInDescription: "请输入此 Relay 的管理密码。",
    password: "管理密码",
    signIn: "登录",
    firstRunSetup: "首次运行设置",
    createPassword: "创建管理密码",
    createPasswordDescription: "此密码用于保护本机的 Relay Web 控制台，仅保存在本机。",
    newPassword: "新密码",
    confirmPassword: "确认密码",
    passwordRequirement: "请使用至少 12 个字符。",
    passwordMismatch: "两次输入的密码不一致。",
    finishSetup: "完成设置",
    logout: "退出登录",
    themePreferences: {
      system: "跟随系统",
      light: "浅色主题",
      dark: "深色主题",
    },
    useSystemTheme: "切换为跟随系统",
    useLightTheme: "切换到浅色主题",
    useDarkTheme: "切换到深色主题",
    chooseRoleTitle: "选择此设备的角色",
    chooseRoleDescription: "每台设备的控制台只管理一种角色。请选择这个 MoleX 节点的用途。",
    roleEdgeTitle: "Edge（访问端）",
    roleEdgeDescription: "把已发布的远端服务映射到本机端口来访问。",
    roleTargetTitle: "Target（服务端）",
    roleTargetDescription: "发布本机可达的内网服务，供各 Edge 勾选映射。",
    roleRelayTitle: "Relay（中继）",
    roleRelayDescription: "Relay 属于服务器部署并需要管理密码。请先在命令行创建配置，再重启控制台：",
    copyCommand: "复制命令",
    currentRouteStatus: "当前路由状态",
    wssEntry: "WSS 入口",
    relayHub: "中继枢纽",
    pairedPeers: "已配对节点",
    encryptedHub: "加密枢纽",
    localMappings: "本地映射",
    publishedServicesNode: "已发布服务",
    secured: "已加密",
    pairing: "配对中",
    standby: "待机",
    connection: "连接设置",
    connectionDescriptionTarget: "先连接 Relay，然后在下方发布服务。",
    connectionDescriptionEdge: "连接 Relay 后，下方会显示已发布的服务目录。",
    relayConfiguration: "Relay 配置",
    relayConfigurationDescription: "位于 Caddy 之后的公网会合端点。",
    listenAddress: "监听地址",
    wssEndpoint: "Relay WSS 地址",
    accessToken: "接入 Token",
    nodeName: "节点名称",
    accessTokens: "接入 Token",
    accessTokensDescription: "每个 Token 只允许一个 Target 接入，Edge 数量不限。Token 值对 Relay 管理员保持可见。",
    createToken: "创建 Token",
    tokenLifetime: "有效期",
    tokenLifetimeNever: "无限时长",
    tokenLifetime1d: "1 天",
    tokenLifetime7d: "7 天",
    tokenLifetime30d: "30 天",
    tokenLifetime90d: "90 天",
    tokenLifetime365d: "1 年",
    tokenExpiredTag: "已过期",
    tokenExpiresAt: "到期",
    tokenNeverExpires: "无限时长",
    changeTokenLifetime: "修改 Token 有效期",
    tokenNotePlaceholder: "备注，例如 office-nas",
    noTokensYet: "还没有 Token。创建一个并粘贴到 Target 和 Edge 控制台。",
    tokenTargetOnline: "Target 在线",
    tokenTargetOffline: "无 Target",
    tokenEdgeCount: "个 Edge",
    tokenDisabledTag: "已停用",
    disableToken: "停用",
    enableToken: "启用",
    deleteToken: "删除 Token",
    revealToken: "显示 Token",
    hideToken: "隐藏 Token",
    copyToken: "复制 Token",
    copied: "已复制",
    createdAt: "创建于",
    rotateToken: "轮换",
    tokenGraceUntil: "旧值有效至",
    tokenGroups: "Token 组",
    tokenGroupsDescription: "一个进程可同时加入多个 Relay Token。第二组不必再开实例。",
    addGroup: "添加组",
    groupName: "组名",
    groupNamePlaceholder: "名称，例如 office",
    deleteGroup: "移除组",
    visibleGroups: "可见组",
    visibleToAllGroups: "全部组",
    catalogGroup: "分组",
    catalogGroupOffline: "此组当前离线",
    graceDays: "宽限天数",
    caddySetup: "Caddy 配置",
    caddySetupDescription: "为 Relay 数据路径和 Web 控制台生成可直接使用的生产 Caddyfile。",
    caddyOfficialGuide: "官方安装指南",
    relayDomain: "中继域名",
    adminDomain: "Web 控制台域名",
    downloadCaddyfile: "下载 Caddyfile",
    publishedServices: "已发布服务",
    publishedServicesDescription: "此 Target 可转发的后端地址。Edge 只能映射这里列出的服务。",
    addService: "添加服务",
    serviceName: "服务名称",
    serviceAddress: "地址（host:port）",
    deleteService: "删除服务",
    saveServices: "保存服务",
    noServicesYet: "尚未发布任何服务。请添加此 Target 可以访问的内网地址。",
    serviceStreams: "条流",
    serviceLastError: "最近错误",
    serviceCatalog: "服务目录",
    serviceCatalogDescription: "Target 发布的服务。勾选后映射到本地端口。",
    applyMappings: "应用映射",
    localPort: "本地端口",
    lanVisible: "局域网可见",
    lanVisibleHint: "绑定 0.0.0.0，允许局域网内其他设备访问。",
    suggestPort: "分配空闲端口",
    catalogStartHint: "启动 Edge 后将从 Target 加载服务目录。",
    catalogWaiting: "正在等待 Target 服务目录…",
    catalogOffline: "Target 离线。它重新连接后映射会自动恢复。",
    noCatalogServices: "Target 尚未发布任何服务。",
    mappingUnpublished: "Target 未发布此服务",
    mappingStates: {
      listening: "监听中",
      waiting: "等待中",
      error: "错误",
    } satisfies Record<MappingState, string>,
    connectionsCount: "次连接",
    transferred: "已传输",
    liveUpdatesLost: "实时连接已断开，正在重连…",
    loadingData: "正在加载数据",
    configurationSaved: "配置已保存",
    save: "保存",
    start: "启动",
    stop: "停止",
    runtime: "运行状态",
    ready: "就绪",
    refreshRuntime: "刷新运行状态",
    state: "状态",
    mode: "模式",
    role: "角色",
    listen: "监听",
    notListening: "未监听",
    notExposed: "未暴露",
    ciphertextRelay: "密文中继",
    aesGcmSession: "AES-GCM 会话",
    trustNote: "持有 Token 的各端与 Relay 管理员共享本组凭据。",
    connectedClients: "已连接客户端",
    noConnectedClients: "暂无客户端连接",
    disconnectPeer: "断开",
    tokenColumn: "Token",
    unknownAddress: "未知地址",
    connectedAt: "接入于",
    connectedFor: "在线时长",
    forwardEndpoint: "转发端点",
    routeId: "路由标识",
    pairedWith: "配对节点",
    waitingForPeer: "等待对端接入",
    platform: "运行平台",
    connectionSource: "连接方式",
    trustedProxy: "可信代理",
    directSocket: "直接连接",
    lastActivity: "最近流量",
    noTrafficYet: "暂无流量",
    traffic: "中继流量",
    received: "接收",
    sent: "发送",
    frames: "帧",
    ago: "前",
    unnamedClient: "未命名客户端",
    activity: "活动记录",
    noActivity: "暂无活动记录",
    states: {
      idle: "空闲",
      starting: "启动中",
      connecting: "连接中",
      running: "运行中",
      stopping: "停止中",
      error: "错误",
    } satisfies Record<RuntimeState, string>,
    modes: {
      relay: "中继",
      target: "目标端",
      edge: "边缘端",
    } satisfies Record<Mode, string>,
    roles: {
      edge: "边缘端",
      target: "目标端",
      hub: "中继枢纽",
    } satisfies Record<Role | "hub", string>,
    peerStates: {
      waiting: "等待中",
      paired: "已配对",
    } satisfies Record<PeerStatus, string>,
  },
} as const;

const zhValidationErrors: Record<string, string> = {
  "listen: address is required": "监听地址不能为空",
  "listen: must use host:port form": "监听地址必须使用 host:port 格式",
  "listen: port must be between 0 and 65535": "监听端口必须在 0 到 65535 之间",
  "remote: WebSocket endpoint is required": "Relay 地址不能为空",
  "remote: must be a valid ws:// or wss:// URL": "Relay 地址必须是有效的 ws:// 或 wss:// URL",
  "remote: scheme must be ws or wss": "Relay 地址协议必须是 ws 或 wss",
  "remote: must contain a host and no credentials or fragment": "Relay 地址必须包含主机，且不能包含凭据或片段",
  "remote: unencrypted ws is allowed only on loopback; use wss for remote relays": "未加密的 ws 仅允许用于本机回环地址；远程 Relay 请使用 wss",
  "token must contain at least 16 characters": "Token 至少需要 16 个字符",
  "use either the single token field or the tokens group list, not both": "请只使用单个 token 字段或多组 tokens 列表，不要同时填写",
  "graceDays must be between 1 and 30": "宽限天数必须在 1 到 30 之间",
  "lifetime must be one of: never, 1d, 7d, 30d, 90d, 365d": "有效期必须是：无限、1 天、7 天、30 天、90 天或 1 年",
  "name: must be at most 64 bytes": "节点名称最多允许 64 字节",
  "name: must not contain control characters": "节点名称不能包含控制字符",
  "name: must be valid UTF-8": "节点名称必须是有效文本",
  "mode must be relay, target, or edge": "模式必须是 relay、target 或 edge",
  "web password must contain at least 12 characters": "管理密码至少需要 12 个字符",
  "authentication required": "登录已过期，请重新登录",
  "invalid credentials": "管理密码错误",
  "too many login attempts; try again later": "登录尝试过多，请稍后重试",
  "cross-origin request rejected": "跨站请求已被拒绝",
  "invalid CSRF token": "安全令牌无效，请刷新页面重试",
  "MoleX is already running": "MoleX 已在运行",
  "stop MoleX before changing its configuration": "请先停止 MoleX，再修改配置",
  "token not found": "未找到该 Token",
  "peer not found or the relay is not running": "未找到该客户端，或 Relay 未在运行",
  "peer id is required": "缺少客户端标识",
  "the console role is already configured": "控制台角色已经确定",
  "the browser can only bootstrap target or edge roles; create relay configurations with `molex config init --mode relay`": "浏览器只能初始化 Target 或 Edge 角色；Relay 请使用命令 `molex config init --mode relay` 创建配置",
  "this console only accepts loopback connections; use SSH forwarding for remote access": "此控制台仅接受本机回环连接；远程访问请使用 SSH 转发",
  "this console only accepts local host names": "此控制台仅接受本机主机名",
};

// Suffix translations for indexed entries such as tokens[0].token or
// services[2].address, keyed by the text after the closing bracket.
const zhIndexedSuffixes: Array<[string, string]> = [
  [".id is required", "：缺少 id"],
  [".id: duplicate token id", "：Token id 重复"],
  [".id: a group name is required when joining several groups", "：加入多组时必须填写组名"],
  [".id: duplicate group name", "：组名重复"],
  [".id: duplicate service id", "：服务 id 重复"],
  [".previousToken must contain at least 16 characters", "：旧 Token 至少需要 16 个字符"],
  [".previousToken must differ from the current token", "：旧 Token 必须与当前值不同"],
  [".previousExpiresAt is required while a previous token is kept", "：保留旧 Token 时必须填写到期时间"],
  [".group is required when the edge joined several groups", "：加入多组时必须指定映射所属组"],
  [".token must contain at least 16 characters", "：Token 至少需要 16 个字符"],
  [".token: duplicate token value", "：Token 值重复"],
  [".note: must be at most 64 bytes", "：备注最多允许 64 字节"],
  [".note: must not contain control characters", "：备注不能包含控制字符"],
  [".name is required", "：服务名称不能为空"],
  [".name: duplicate service name", "：服务名称重复"],
  [".name: must be at most 64 bytes", "：服务名称最多允许 64 字节"],
  [".name: must not contain control characters", "：服务名称不能包含控制字符"],
  [".address: address is required", "：地址不能为空"],
  [".address: must use host:port form", "：地址必须使用 host:port 格式"],
  [".address: port must be between 1 and 65535", "：端口必须在 1 到 65535 之间"],
  [".service is required", "：缺少服务 id"],
  [".service: duplicate mapping for one service", "：同一服务只能映射一次"],
  [".port must be between 1 and 65535", "：本地端口必须在 1 到 65535 之间"],
  [".port: duplicate local port", "：本地端口重复"],
];

const zhIndexedPrefixes: Record<string, string> = {
  tokens: "Token",
  services: "服务",
  mappings: "映射",
};

const zhRuntimeMessages: Record<string, string> = {
  Ready: "就绪",
  Starting: "正在启动",
  Stopping: "正在停止",
  "Connecting to relay": "正在连接 Relay",
  "Relay is accepting WebSocket sessions": "Relay 正在接受 WebSocket 会话",
  Stopped: "已停止",
  "Encrypted route is ready; waiting for the target service catalog": "加密路由已就绪，正在等待 Target 服务目录",
  "Encrypted route is down; local mapping listeners are closed until it recovers": "加密路由已断开；本地映射监听已关闭，路由恢复后自动重开",
  "Target is ready to receive streams": "Target 已准备接收数据流",
  "Edge and target sessions paired": "Edge 与 Target 会话已配对",
  "Local connection routed": "本地连接已转发",
  "An edge requested an address that is not in the published service list; the request was refused": "某个 Edge 请求了未发布的地址，已按白名单拒绝",
  "The route reached its 256 concurrent connection limit. Close idle connections or start another route, then retry.": "此路由已达到 256 条并发连接上限。请关闭空闲连接后重试。",
  "This local connection could not open an encrypted stream. Retry it; if this repeats, check peer health and reduce simultaneous connection attempts.": "此本地连接无法打开加密流。请重试；如果问题反复出现，请检查对端健康状态并减少同时发起的连接数。",
  "This local connection could not be forwarded because the encrypted route was interrupted. MoleX is reconnecting; retry after the route is ready.": "加密路由中断，因此此次本地连接未能转发。MoleX 正在重连；请在路由恢复就绪后重试。",
  "The target did not answer the forwarding request in time. Check the target's health and retry.": "Target 未及时响应转发请求。请检查 Target 健康状态后重试。",
  "Waiting for the encrypted route": "等待加密路由",
  "Waiting for the target service catalog": "等待 Target 服务目录",
  "The target does not publish this service; it stays inactive until published again": "Target 未发布此服务；重新发布后映射自动恢复",
  "The relay rejected this token (HTTP 401). Copy a valid token from the relay console and paste the exact value here.": "Relay 拒绝了此 Token（HTTP 401）。请从 Relay 控制台复制有效 Token 并原样粘贴。",
  "The relay reports this token is disabled or expired (HTTP 403). Ask the relay administrator to enable it, extend its lifetime, or issue a new one.": "Relay 提示此 Token 已停用或过期（HTTP 403）。请联系 Relay 管理员启用、延长有效期或签发新 Token。",
  "Another Target is already connected with this token. Each token accepts exactly one Target; stop the other Target or use a different token.": "此 Token 已有另一个 Target 在线。每个 Token 只允许一个 Target；请停止另一个 Target 或改用其他 Token。",
  "The relay administrator disabled this token. Ask for a new or re-enabled token before reconnecting.": "Relay 管理员停用了此 Token。请获取新 Token 或等待重新启用后再连接。",
  "This token has expired. Ask the relay administrator to extend its lifetime or issue a new token before reconnecting.": "此 Token 已过期。请联系 Relay 管理员延长有效期或签发新 Token 后再连接。",
  "The relay administrator disconnected this client. It reconnects automatically; contact the administrator if access stays blocked.": "Relay 管理员断开了此客户端。它会自动重连；若持续无法接入请联系管理员。",
};

const zhErrorPrefixes: Array<[string, string]> = [
  ["open config: ", "打开配置失败："],
  ["decode config: ", "解析配置失败："],
  ["create config directory: ", "创建配置目录失败："],
  ["create temporary config: ", "创建临时配置失败："],
  ["secure temporary config: ", "保护临时配置失败："],
  ["encode config: ", "编码配置失败："],
  ["flush config: ", "写入配置失败："],
  ["close config: ", "关闭配置失败："],
  ["replace config: ", "替换配置失败："],
  ["generate token: ", "生成 Token 失败："],
  ["generate id: ", "生成标识失败："],
];

const zhClientGuidance: Record<string, string> = {
  "The encrypted route ended unexpectedly. Check the relay and peer; MoleX will keep retrying.": "加密路由意外结束。请检查 Relay 和对端；MoleX 会继续自动重试。",
  "The relay WebSocket route was not found (HTTP 404). Check that the URL ends with /ws/session and Caddy forwards that path.": "未找到 Relay WebSocket 路由（HTTP 404）。请确认 URL 以 /ws/session 结尾，并确认 Caddy 已转发该路径。",
  "The relay is limiting connection attempts (HTTP 429). Wait before retrying and check the relay logs.": "Relay 正在限制连接尝试（HTTP 429）。请稍后重试并检查 Relay 日志。",
  "No Target answered before the pairing timeout. Start the Target for this token and check that both sides run MoleX v2 with the same token.": "配对超时前没有 Target 响应。请启动此 Token 对应的 Target，并确认两端都运行 MoleX v2 且使用同一 Token。",
  "The relay could not accept this session. Verify the relay route and token, then retry.": "Relay 无法接受此会话。请核对 Relay 路由与 Token 后重试。",
  "The relay rejected the session route for this token. Make sure Edge, Target, and Relay all run MoleX v2 and use the exact same token.": "Relay 拒绝了此 Token 的会话路由。请确认 Edge、Target 和 Relay 都运行 MoleX v2，并使用完全相同的 Token。",
  "The encrypted handshake failed. Verify that Edge and Target use the exact same token and run compatible MoleX v2 versions.": "加密握手失败。请确认 Edge 与 Target 使用完全相同的 Token，且版本兼容。",
  "The relay hostname could not be resolved. Check the hostname and this machine's DNS settings.": "无法解析 Relay 主机名。请检查主机名和本机 DNS 设置。",
  "The relay connection was refused. Start Caddy or MoleX Relay, verify the configured port, and check the firewall.": "Relay 拒绝连接。请启动 Caddy 或 MoleX Relay，核对配置端口并检查防火墙。",
  "TLS verification failed. Check the certificate hostname and chain, and verify this machine's system time.": "TLS 验证失败。请检查证书域名和证书链，并确认本机系统时间正确。",
  "The relay connection timed out. Check network reachability, Caddy, and firewall rules.": "连接 Relay 超时。请检查网络连通性、Caddy 和防火墙规则。",
  "The relay or peer closed the encrypted route. Retry the local connection after the route is ready.": "Relay 或对端已关闭加密路由。请在路由恢复就绪后重试本地连接。",
};

export function initialLocale(): Locale {
  const saved = localStorage.getItem(LANGUAGE_STORAGE_KEY);
  if (saved === "en" || saved === "zh-CN") return saved;
  return navigator.language.toLowerCase().startsWith("zh") ? "zh-CN" : "en";
}

export function secretActionLabel(locale: Locale, action: "show" | "hide" | "generate" | "copy", label: string): string {
  if (locale === "zh-CN") {
    const verbs = { show: "显示", hide: "隐藏", generate: "生成", copy: "复制" } as const;
    return `${verbs[action]}${label}`;
  }
  const verbs = { show: "Show", hide: "Hide", generate: "Generate", copy: "Copy" } as const;
  return `${verbs[action]} ${label}`;
}

export function localizeValidationError(message: string, locale: Locale): string {
  if (locale === "en") return message;
  if (message.includes("; ")) {
    return message.split("; ").map((part) => localizeValidationError(part, locale)).join("；");
  }
  if (zhValidationErrors[message]) return zhValidationErrors[message];

  const indexed = message.match(/^(tokens|services|mappings)\[(\d+)\](.+)$/);
  if (indexed) {
    const prefix = zhIndexedPrefixes[indexed[1]];
    const suffix = zhIndexedSuffixes.find(([source]) => indexed[3] === source);
    if (prefix && suffix) {
      return `第 ${Number(indexed[2]) + 1} 条${prefix}${suffix[1]}`;
    }
  }
  const unknownGroup = message.match(/^(services|mappings)\[(\d+)\]\.(groups|group): unknown group name "(.+)"$/);
  if (unknownGroup) {
    const kind = unknownGroup[1] === "services" ? "服务" : "映射";
    return `第 ${Number(unknownGroup[2]) + 1} 条${kind}：未知组名“${unknownGroup[4]}”`;
  }

  const capped = message.match(/^(tokens|services|mappings): at most (\d+) entries are supported$/);
  if (capped) return `${zhIndexedPrefixes[capped[1]]}最多支持 ${capped[2]} 条`;

  const consoleMode = message.match(/^this console manages a "(relay|target|edge)" configuration; recreate the file with `molex config init` to change roles$/);
  if (consoleMode) return `此控制台只管理 ${consoleMode[1]} 配置；如需更换角色，请用 \`molex config init\` 重新创建配置文件`;

  const legacy = message.match(/legacy v1 configuration: (.+)$/);
  if (legacy) return "检测到 MoleX v1 旧版配置（punch/role/secret/tunnel）。MoleX v2 使用 relay、target、edge 三种模式与 Token、服务、映射。请运行 `molex config init --mode <relay|target|edge>` 重新创建配置，并参考 README 的 v2 迁移说明";

  const prefix = zhErrorPrefixes.find(([source]) => message.startsWith(source));
  return prefix ? prefix[1] + message.slice(prefix[0].length) : message;
}

export function localizeRuntimeMessage(message: string, locale: Locale): string {
  if (locale === "en") return message;
  if (zhRuntimeMessages[message]) return zhRuntimeMessages[message];

  const published = message.match(/^Target published (\d+) service\(s\)$/);
  if (published) return `Target 发布了 ${published[1]} 个服务`;

  const applied = message.match(/^Applied (\d+) local mapping\(s\)$/);
  if (applied) return `已应用 ${applied[1]} 条本地映射`;

  const publishedToEdges = message.match(/^Published (\d+) service\(s\) to connected edges$/);
  if (publishedToEdges) return `已向连接中的 Edge 发布 ${publishedToEdges[1]} 个服务`;

  const adaptivePool = message.match(/^Connecting adaptive Target session pool \(up to (\d+) sessions\)$/);
  if (adaptivePool) return `正在连接按需 Target 会话池（最多 ${adaptivePool[1]} 条会话）`;

  const targetReady = message.match(/^Target is ready: (\d+) live session\(s\); one hot-standby session is kept for the next edge$/);
  if (targetReady) return `Target 已就绪：${targetReady[1]} 条活跃会话；始终保留一条热备会话等待下一个 Edge`;

  const poolDegraded = message.match(/^Target session pool is degraded: (\d+) session\(s\) still connected\. (.+)$/);
  if (poolDegraded) return `Target 会话池运行降级：仍有 ${poolDegraded[1]} 条会话连接。${localizeRuntimeMessage(poolDegraded[2], locale)}`;

  const streamReached = message.match(/^Encrypted stream reached service "(.+)"$/);
  if (streamReached) return `加密流已到达服务“${streamReached[1]}”`;

  const serviceDown = message.match(/^Service "(.+)" at (.+) is unavailable\. Start the backend service or correct its address in the service list, then retry from the Edge\. Details: (.+)$/);
  if (serviceDown) return `服务“${serviceDown[1]}”（${serviceDown[2]}）不可用。请启动该后端服务或在服务列表中修正地址，然后从 Edge 重试。详情：${serviceDown[3]}`;

  const mappingRecovered = message.match(/^Local mapping for (.+) recovered and is listening on (.+)$/);
  if (mappingRecovered) return `${localizeServiceRef(mappingRecovered[1])}的本地映射已恢复，正在 ${mappingRecovered[2]} 监听`;

  const listenerBlocked = message.match(/^The local listener for (.+) could not start on (.+)\. Stop the process using that address or pick another port; MoleX keeps retrying automatically\. Details: (.+)$/);
  if (listenerBlocked) return `${localizeServiceRef(listenerBlocked[1])}的本地监听无法在 ${listenerBlocked[2]} 启动。请停止占用该地址的进程或改用其他端口；MoleX 会自动重试。详情：${listenerBlocked[3]}`;

  const listenerBlockedShort = message.match(/^The local listener could not start on (.+)\. Stop the process using that address or pick another port; MoleX keeps retrying automatically\.$/);
  if (listenerBlockedShort) return `本地监听无法在 ${listenerBlockedShort[1]} 启动。请停止占用该地址的进程或改用其他端口；MoleX 会自动重试。`;

  const unpublishedStream = message.match(/^The target no longer publishes (.+)\. The catalog refreshes automatically; re-check the mapping afterwards\.$/);
  if (unpublishedStream) return `Target 已不再发布${localizeServiceRef(unpublishedStream[1])}。目录会自动刷新；刷新后请重新检查映射。`;

  const unreachableStream = message.match(/^The target could not reach (.+)\. Start the backend service or fix its address on the target console\.$/);
  if (unreachableStream) return `Target 无法连接${localizeServiceRef(unreachableStream[1])}。请启动该后端服务，或在 Target 控制台修正其地址。`;

  const duplicateTarget = message.match(/^A second Target from (.+) was rejected for token (.+); each token accepts exactly one Target$/);
  if (duplicateTarget) return `来自 ${duplicateTarget[1]} 的第二个 Target 已被拒绝（Token ${duplicateTarget[2]}）；每个 Token 只允许一个 Target`;

  const tokenRevoked = message.match(/^Token (.+) was disabled or removed; (\d+) connected client\(s\) were disconnected$/);
  if (tokenRevoked) return `Token ${tokenRevoked[1]} 已停用或删除；${tokenRevoked[2]} 个已连接客户端被断开`;

  const tokenExpiredSweep = message.match(/^Token (.+) expired; (\d+) connected client\(s\) were disconnected$/);
  if (tokenExpiredSweep) return `Token ${tokenExpiredSweep[1]} 已过期；${tokenExpiredSweep[2]} 个已连接客户端被断开`;

  const tokenCreatedExpiry = message.match(/^Token (.+) was created; it expires at (.+)$/);
  if (tokenCreatedExpiry) return `已创建 Token ${tokenCreatedExpiry[1]}；有效期至 ${tokenCreatedExpiry[2]}`;

  const tokenLifetimeCleared = message.match(/^Token (.+) lifetime is now unlimited$/);
  if (tokenLifetimeCleared) return `Token ${tokenLifetimeCleared[1]} 有效期已改为无限时长`;

  const tokenLifetimeSet = message.match(/^Token (.+) lifetime now expires at (.+)$/);
  if (tokenLifetimeSet) return `Token ${tokenLifetimeSet[1]} 有效期现为 ${tokenLifetimeSet[2]}`;

  const tokenCreated = message.match(/^Token (.+) was created$/);
  if (tokenCreated) return `已创建 Token ${tokenCreated[1]}`;

  const tokenRotated = message.match(/^Token (.+) was rotated; the previous value stays valid until (.+)$/);
  if (tokenRotated) return `Token ${tokenRotated[1]} 已轮换；旧值有效至 ${tokenRotated[2]}`;

  const tokenToggled = message.match(/^Token (.+) was (created|enabled|disabled|deleted)$/);
  if (tokenToggled) {
    const actions: Record<string, string> = { created: "已创建", enabled: "已启用", disabled: "已停用", deleted: "已删除" };
    return `Token ${tokenToggled[1]} ${actions[tokenToggled[2]]}`;
  }

  const kicked = message.match(/^(Edge|Target) (.+) was disconnected by the relay administrator$/);
  if (kicked) return `${kicked[1] === "Edge" ? "边缘端" : "目标端"} ${kicked[2]} 已被 Relay 管理员断开`;

  const guidedRetry = message.match(/^Route unavailable; retrying in (.+?)\. (.+)$/);
  if (guidedRetry) return `路由不可用；将在 ${localizeDuration(guidedRetry[1])}后重试。${localizeClientGuidance(guidedRetry[2])}`;

  const peerConnection = message.match(/^(Edge|Target) (connected from|disconnected from) (.+)$/);
  if (peerConnection) {
    const role = peerConnection[1] === "Edge" ? "边缘端" : "目标端";
    const address = peerConnection[3] === "unknown" ? "未知地址" : peerConnection[3];
    return peerConnection[2] === "connected from"
      ? `${role}已从 ${address} 接入`
      : `${role} ${address} 已断开连接`;
  }

  if (zhClientGuidance[message]) return zhClientGuidance[message];
  return localizeValidationError(message, locale);
}

function localizeServiceRef(reference: string): string {
  const named = reference.match(/^service "(.+)"$/);
  if (named) return `服务“${named[1]}”`;
  const plain = reference.match(/^service (.+)$/);
  if (plain) return `服务 ${plain[1]}`;
  return reference;
}

function localizeClientGuidance(message: string): string {
  if (zhClientGuidance[message]) return zhClientGuidance[message];
  if (zhRuntimeMessages[message]) return zhRuntimeMessages[message];

  const gateway = message.match(/^The relay gateway is unavailable \(HTTP (502|503|504)\)\. Start MoleX Relay and verify Caddy's upstream address\.$/);
  if (gateway) return `Relay 网关不可用（HTTP ${gateway[1]}）。请启动 MoleX Relay，并核对 Caddy 的上游地址。`;

  const unexpectedHTTP = message.match(/^The relay returned HTTP (\d+) instead of opening a WebSocket\. Check the relay URL and Caddy routing\.$/);
  if (unexpectedHTTP) return `Relay 返回了 HTTP ${unexpectedHTTP[1]}，未能建立 WebSocket。请检查 Relay URL 和 Caddy 路由。`;

  const listener = message.match(/^The local listener could not start on (.+)\. Stop the process using that address or choose a free listen address; MoleX will keep retrying\.$/);
  if (listener) return `无法在 ${listener[1]} 启动本地监听。请停止占用该地址的进程，或选择空闲的监听地址；MoleX 会继续自动重试。`;

  const generic = "Check relay reachability and verify that Edge and Target use the same token. Details: ";
  if (message.startsWith(generic)) return `请检查 Relay 连通性，并确认 Edge 与 Target 使用相同的 Token。详情：${message.slice(generic.length)}`;

  return message;
}

function localizeDuration(duration: string): string {
  if (duration.endsWith("ms")) return `${duration.slice(0, -2)} 毫秒`;
  if (duration.endsWith("s")) return `${duration.slice(0, -1)} 秒`;
  return duration;
}
