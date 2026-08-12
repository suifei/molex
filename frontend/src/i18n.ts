import type { Mode, PeerStatus, Role, RuntimeState } from "./types";

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
    signInTitle: "Sign in to MoleX",
    signInDescription: "Enter the management password for this node.",
    password: "Management password",
    signIn: "Sign in",
    firstRunSetup: "First-run setup",
    createPassword: "Create your management password",
    createPasswordDescription: "This password protects the Web console on this device. It is stored locally and is never sent to Relay.",
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
    currentRouteStatus: "Current route status",
    wssEntry: "WSS entry",
    relayHub: "Relay hub",
    pairedPeers: "Paired peers",
    localPort: "Local port",
    encryptedHub: "Encrypted hub",
    channel: "Channel",
    targetService: "Target service",
    targetPool: "Target session pool",
    forwardingRules: "Forwarding rules",
    forwardingRulesDescription: "Run several independent Edge or Target routes from this one Web console.",
    addRule: "Add rule",
    deleteRule: "Delete rule",
    singleRuleCompatibility: "The fields above are the active forwarding rule. Add rules to manage several routes together.",
    secured: "Secured",
    pairing: "Pairing",
    standby: "Standby",
    routeConfiguration: "Route configuration",
    publicRendezvousEndpoint: "Public rendezvous endpoint",
    localAccessEndpoint: "Local access endpoint",
    privateServiceEndpoint: "Private service endpoint",
    mode: "Mode",
    relay: "Relay",
    client: "Client",
    listenAddress: "Listen address",
    admissionToken: "Admission token",
    caddySetup: "Caddy setup",
    caddySetupDescription: "Generate a production Caddyfile for the Relay data path and Web console.",
    caddyOfficialGuide: "Official installation guide",
    relayDomain: "Relay domain",
    adminDomain: "Web console domain",
    downloadCaddyfile: "Download Caddyfile",
    clientRole: "Client role",
		nodeName: "Node name",
    edgeListener: "Edge listener",
    relayEndpoint: "Relay endpoint",
    localListen: "Local listen",
    endToEndSecret: "End-to-end secret",
    relayToken: "Relay token",
    configurationSaved: "Configuration saved",
    save: "Save",
    start: "Start",
    stop: "Stop",
    runtime: "Runtime",
    ready: "Ready",
    refreshRuntime: "Refresh runtime",
    state: "State",
    role: "Role",
    listen: "Listen",
    notListening: "Not listening",
    notExposed: "Not exposed",
    ciphertextRelay: "Ciphertext relay",
    aesGcmSession: "AES-GCM session",
    connectedClients: "Connected clients",
    noConnectedClients: "No clients connected",
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
      punch: "Punch",
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
    signInTitle: "登录 MoleX",
    signInDescription: "请输入此节点的管理密码。",
    password: "管理密码",
    signIn: "登录",
    firstRunSetup: "首次运行设置",
    createPassword: "创建管理密码",
    createPasswordDescription: "此密码用于保护本机 Web 控制台，仅保存在本机，不会发送给 Relay。",
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
    currentRouteStatus: "当前路由状态",
    wssEntry: "WSS 入口",
    relayHub: "中继枢纽",
    pairedPeers: "已配对节点",
    localPort: "本地端口",
    encryptedHub: "加密枢纽",
    channel: "通道",
    targetService: "目标服务",
    targetPool: "目标会话池",
    forwardingRules: "转发规则",
    forwardingRulesDescription: "在同一个 Web 控制台中运行多条相互独立的 Edge 或 Target 路由。",
    addRule: "新增规则",
    deleteRule: "删除规则",
    singleRuleCompatibility: "当前使用上方的单条转发配置；新增规则后可统一管理多条路由。",
    secured: "已加密",
    pairing: "配对中",
    standby: "待机",
    routeConfiguration: "路由配置",
    publicRendezvousEndpoint: "公网会合端点",
    localAccessEndpoint: "本地访问端点",
    privateServiceEndpoint: "内网服务端点",
    mode: "模式",
    relay: "中继",
    client: "穿透",
    listenAddress: "监听地址",
    admissionToken: "接入令牌",
    caddySetup: "Caddy 配置",
    caddySetupDescription: "为 Relay 数据路径和 Web 控制台生成可直接使用的生产 Caddyfile。",
    caddyOfficialGuide: "官方安装指南",
    relayDomain: "中继域名",
    adminDomain: "Web 控制台域名",
    downloadCaddyfile: "下载 Caddyfile",
    clientRole: "客户端角色",
		nodeName: "节点名称",
    edgeListener: "边缘监听端",
    relayEndpoint: "中继端点",
    localListen: "本地监听",
    endToEndSecret: "端到端密钥",
    relayToken: "中继令牌",
    configurationSaved: "配置已保存",
    save: "保存",
    start: "启动",
    stop: "停止",
    runtime: "运行状态",
    ready: "就绪",
    refreshRuntime: "刷新运行状态",
    state: "状态",
    role: "角色",
    listen: "监听",
    notListening: "未监听",
    notExposed: "未暴露",
    ciphertextRelay: "密文中继",
    aesGcmSession: "AES-GCM 会话",
    connectedClients: "已连接客户端",
    noConnectedClients: "暂无客户端连接",
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
      punch: "穿透",
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
  "role must be edge or target": "客户端角色必须是边缘端或目标端",
  "secret must contain at least 16 characters": "密钥至少需要 16 个字符",
  "remote: use a ws:// or wss:// URL": "中继端点必须使用 ws:// 或 wss:// URL",
  "remote: WebSocket endpoint is required": "中继端点不能为空",
  "remote: must be a valid ws:// or wss:// URL": "中继端点必须是有效的 ws:// 或 wss:// URL",
  "remote: scheme must be ws or wss": "中继端点协议必须是 ws 或 wss",
  "remote: must contain a host and no credentials or fragment": "中继端点必须包含主机，且不能包含凭据或片段",
  "remote: unencrypted ws is allowed only on loopback; use wss for remote relays": "未加密的 ws 仅允许用于本机回环地址；远程中继请使用 wss",
  "tunnel.remote channel is required": "通道不能为空",
  "tunnel.remote channel must be at most 128 characters": "通道最多允许 128 个字符",
	"tunnel.name: must be at most 64 bytes": "节点名称最多允许 64 字节",
  "tunnel.name: must not contain control characters": "节点名称不能包含控制字符",
  "tunnel.name: must be valid UTF-8": "节点名称必须是有效文本",
  "tunnel.pool must be between 1 and 64": "目标会话池必须在 1 到 64 之间",
  "web password must contain at least 12 characters": "管理密码至少需要 12 个字符",
  "tunnel.local: address is required": "目标服务地址不能为空",
  "tunnel.local: must use host:port form": "目标服务地址必须使用 host:port 格式",
  "tunnel.local: port must be between 0 and 65535": "目标服务端口必须在 0 到 65535 之间",
  "mode must be relay or punch": "模式必须是中继或穿透",
  "token must contain at least 16 characters when set": "令牌填写后至少需要 16 个字符",
  "authentication required": "登录已过期，请重新登录",
  "invalid credentials": "管理密码错误",
  "too many login attempts; try again later": "登录尝试过多，请稍后重试",
  "cross-origin request rejected": "跨站请求已被拒绝",
  "invalid CSRF token": "安全令牌无效，请重新登录",
  "MoleX is already running": "MoleX 已在运行",
  "stop MoleX before changing its configuration": "请先停止 MoleX，再修改配置",
};

const zhRuntimeMessages: Record<string, string> = {
  Ready: "就绪",
  Starting: "正在启动",
  Stopping: "正在停止",
  "Connecting to relay": "正在连接中继",
  "Relay is accepting WebSocket sessions": "中继正在接受 WebSocket 会话",
  "Encrypted route is ready": "加密路由已就绪",
  Stopped: "已停止",
  "Could not open an encrypted stream": "无法打开加密流",
  "This local connection could not open an encrypted stream. Retry it; if this repeats, check peer health and reduce simultaneous connection attempts.": "此本地连接无法打开加密流。请重试；如果问题反复出现，请检查对端健康状态并减少同时发起的连接数。",
  "This local connection could not be forwarded because the encrypted route was interrupted. MoleX is reconnecting; retry after the route is ready.": "加密路由中断，因此此次本地连接未能转发。MoleX 正在重连；请在路由恢复就绪后重试。",
  "Local connection routed": "本地连接已转发",
  "Target is ready to receive streams": "目标端已准备接收数据流",
  "Encrypted stream reached target service": "加密流已到达目标服务",
  "Edge and target sessions paired": "边缘端与目标端会话已配对",
	"The route reached its 256 concurrent connection limit. Close idle connections or start another route, then retry.": "此路由已达到 256 条并发连接上限。请关闭空闲连接或启动另一条路由，然后重试。",
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
  ["generate secret: ", "生成密钥失败："],
];

const zhClientGuidance: Record<string, string> = {
  "The encrypted route ended unexpectedly. Check the relay and peer; MoleX will keep retrying.": "加密路由意外结束。请检查中继和对端；MoleX 会继续自动重试。",
  "The relay WebSocket route was not found (HTTP 404). Check that the URL ends with /ws/session and Caddy forwards that path.": "未找到中继 WebSocket 路由（HTTP 404）。请确认 URL 以 /ws/session 结尾，并确认 Caddy 已转发该路径。",
  "The relay is limiting connection attempts (HTTP 429). Wait before retrying and check the relay logs.": "中继正在限制连接尝试（HTTP 429）。请稍后重试并检查中继日志。",
  "No matching peer joined before the pairing timeout. Start the other client and verify that Edge and Target use the same channel, secret, token, and complementary roles.": "配对超时前没有匹配的对端加入。请启动另一端，并确认 Edge 与 Target 使用相同的通道、密钥和令牌，且角色互补。",
  "The encrypted handshake failed. Verify that Edge and Target use the same channel and secret with complementary roles.": "加密握手失败。请确认 Edge 与 Target 使用相同的通道和密钥，且角色互补。",
  "The relay hostname could not be resolved. Check the hostname and this machine's DNS settings.": "无法解析中继主机名。请检查主机名和本机 DNS 设置。",
  "The relay connection was refused. Start Caddy or MoleX Relay, verify the configured port, and check the firewall.": "中继拒绝连接。请启动 Caddy 或 MoleX Relay，核对配置端口并检查防火墙。",
  "TLS verification failed. Check the certificate hostname and chain, and verify this machine's system time.": "TLS 验证失败。请检查证书域名和证书链，并确认本机系统时间正确。",
  "The relay connection timed out. Check network reachability, Caddy, and firewall rules.": "连接中继超时。请检查网络连通性、Caddy 和防火墙规则。",
  "The relay or peer closed the encrypted route. Retry the local connection after the route is ready.": "中继或对端已关闭加密路由。请在路由恢复就绪后重试本地连接。",
};

export function initialLocale(): Locale {
  const saved = localStorage.getItem(LANGUAGE_STORAGE_KEY);
  if (saved === "en" || saved === "zh-CN") return saved;
  return navigator.language.toLowerCase().startsWith("zh") ? "zh-CN" : "en";
}

export function secretActionLabel(locale: Locale, action: "show" | "hide" | "generate", label: string): string {
  if (locale === "zh-CN") {
    const verbs = { show: "显示", hide: "隐藏", generate: "生成" } as const;
    return `${verbs[action]}${label}`;
  }
  const verbs = { show: "Show", hide: "Hide", generate: "Generate" } as const;
  return `${verbs[action]} ${label}`;
}

export function localizeValidationError(message: string, locale: Locale): string {
  if (locale === "en") return message;
  if (message.includes("; ")) {
    return message.split("; ").map((part) => localizeValidationError(part, locale)).join("；");
  }
  if (zhValidationErrors[message]) return zhValidationErrors[message];
  const prefix = zhErrorPrefixes.find(([source]) => message.startsWith(source));
  return prefix ? prefix[1] + message.slice(prefix[0].length) : message;
}

export function localizeRuntimeMessage(message: string, locale: Locale): string {
  if (locale === "en") return message;
  if (zhRuntimeMessages[message]) return zhRuntimeMessages[message];

  const poolConnecting = message.match(/^Connecting Target session pool \((\d+) sessions\)$/);
  if (poolConnecting) return `正在连接目标会话池（${poolConnecting[1]} 条会话）`;

  const poolReady = message.match(/^Target session pool is ready: (\d+) of (\d+) sessions connected$/);
  if (poolReady) return `目标会话池已就绪：${poolReady[1]}/${poolReady[2]} 条会话已连接`;

  const poolDegraded = message.match(/^Target session pool is degraded: (\d+) of (\d+) sessions connected\. (.+)$/);
  if (poolDegraded) return `目标会话池运行降级：${poolDegraded[1]}/${poolDegraded[2]} 条会话已连接。${localizeRuntimeMessage(poolDegraded[3], locale)}`;

  const guidedRetry = message.match(/^Route unavailable; retrying in (.+?)\. (.+)$/);
  if (guidedRetry) return `路由不可用；将在 ${localizeDuration(guidedRetry[1])}后重试。${localizeClientGuidance(guidedRetry[2])}`;

  const retry = message.match(/^Session ended; retrying in ([^:]+): (.+)$/);
  if (retry) return `会话已结束；将在 ${retry[1]} 后重试：${retry[2]}`;

  const target = message.match(/^Target service at (.+) is unavailable\. Start the service or correct tunnel\.local, then retry the Edge connection\. Details: (.+)$/);
  if (target) return `目标服务 ${target[1]} 不可用。请启动该服务或修正 tunnel.local，然后重新发起 Edge 连接。详情：${target[2]}`;

  const unavailable = "Target service is unavailable: ";
  if (message.startsWith(unavailable)) return `目标服务不可用：${message.slice(unavailable.length)}`;

  const peerConnection = message.match(/^(Edge|Target) (connected from|disconnected from) (.+)$/);
  if (peerConnection) {
    const role = peerConnection[1] === "Edge" ? "边缘端" : "目标端";
    const address = peerConnection[3] === "unknown" ? "未知地址" : peerConnection[3];
    return peerConnection[2] === "connected from"
      ? `${role}已从 ${address} 接入`
      : `${role} ${address} 已断开连接`;
  }

  return localizeValidationError(message, locale);
}

function localizeClientGuidance(message: string): string {
  if (zhClientGuidance[message]) return zhClientGuidance[message];

  const listener = message.match(/^The local listener could not start on (.+)\. Stop the process using that address or choose a free listen address; MoleX will keep retrying\.$/);
  if (listener) return `无法在 ${listener[1]} 启动本地监听。请停止占用该地址的进程，或选择空闲的监听地址；MoleX 会继续自动重试。`;

  const authentication = message.match(/^Relay authentication was rejected \(HTTP (401|403)\)\. Make the relay token identical on Relay, Edge, and Target\.$/);
  if (authentication) return `中继拒绝了身份验证（HTTP ${authentication[1]}）。请确保 Relay、Edge 和 Target 使用完全相同的中继令牌。`;

  const gateway = message.match(/^The relay gateway is unavailable \(HTTP (502|503|504)\)\. Start MoleX Relay and verify Caddy's upstream address\.$/);
  if (gateway) return `中继网关不可用（HTTP ${gateway[1]}）。请启动 MoleX Relay，并核对 Caddy 的上游地址。`;

  const unexpectedHTTP = message.match(/^The relay returned HTTP (\d+) instead of opening a WebSocket\. Check the relay URL and Caddy routing\.$/);
  if (unexpectedHTTP) return `中继返回了 HTTP ${unexpectedHTTP[1]}，未能建立 WebSocket。请检查中继 URL 和 Caddy 路由。`;

  const generic = "Check relay reachability and verify that Edge and Target use the same channel, secret, token, and complementary roles. Details: ";
  if (message.startsWith(generic)) return `请检查中继连通性，并确认 Edge 与 Target 使用相同的通道、密钥和令牌，且角色互补。详情：${message.slice(generic.length)}`;

  return message;
}

function localizeDuration(duration: string): string {
  if (duration.endsWith("ms")) return `${duration.slice(0, -2)} 毫秒`;
  if (duration.endsWith("s")) return `${duration.slice(0, -1)} 秒`;
  return duration;
}
