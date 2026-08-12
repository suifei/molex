import type { Config, RuntimeEvent, RuntimeStatus, ValidationResult } from "./types";

const STORAGE_KEY = "molex:preview-config";
const UNAUTHORIZED_EVENT = "molex:unauthorized";
const previewMode = import.meta.env.DEV;

const defaultConfig: Config = {
  mode: "punch",
  role: "edge",
  secret: "",
  token: "",
  listen: "127.0.0.1:2222",
  remote: "wss://molex.example.com/ws/session",
  tunnel: {
    local: "127.0.0.1:22",
    remote: "home-ssh",
    name: "",
    pool: 0,
    rules: [],
  },
};

export interface SessionState {
  authenticated: boolean;
  setupRequired?: boolean;
  csrfToken?: string;
}

let csrfToken = "";
let mockStatus: RuntimeStatus = { state: "idle", message: "Ready" };
let mockEvents: RuntimeEvent[] = [];
const mockListeners = new Set<(event: RuntimeEvent) => void>();
let startTimer: number | undefined;

function emitMock(event: Omit<RuntimeEvent, "time">) {
  const complete = { ...event, time: new Date().toISOString() } as RuntimeEvent;
  mockEvents = [...mockEvents.slice(-99), complete];
  mockListeners.forEach((listener) => listener(complete));
}

function loadMockConfig(): Config {
  const stored = localStorage.getItem(STORAGE_KEY);
  if (!stored) return structuredClone(defaultConfig);
  try {
    const loaded = JSON.parse(stored) as Partial<Config>;
    return { ...structuredClone(defaultConfig), ...loaded, tunnel: { ...defaultConfig.tunnel, ...(loaded.tunnel ?? {}), rules: loaded.tunnel?.rules ?? [] } };
  } catch {
    return structuredClone(defaultConfig);
  }
}

function validateMock(config: Config): ValidationResult {
  const errors: string[] = [];
  if (!config.listen && (config.mode === "relay" || config.role === "edge")) errors.push("listen: address is required");
  if (config.mode === "punch") {
    if (!/^wss?:\/\//.test(config.remote)) errors.push("remote: use a ws:// or wss:// URL");
    if (config.secret.trim().length < 16) errors.push("secret must contain at least 16 characters");
    if (!config.tunnel.remote.trim()) errors.push("tunnel.remote channel is required");
    if (config.role === "target" && !config.tunnel.local.trim()) errors.push("tunnel.local: address is required");
		const routes = config.tunnel.rules?.length ? config.tunnel.rules : [{ ...config.tunnel, listen: config.listen }];
		for (const [index, route] of routes.entries()) {
			const prefix = config.tunnel.rules?.length ? `tunnel.rules[${index}]` : "tunnel";
			if (!route.remote.trim()) errors.push(`${prefix}.remote channel is required`);
			if (config.role === "edge" && !route.listen.trim()) errors.push(`${prefix}.listen: address is required`);
			if (config.role === "target" && !route.local.trim()) errors.push(`${prefix}.local: address is required`);
			if (new TextEncoder().encode(route.name.trim()).length > 64) errors.push(`${prefix}.name: must be at most 64 bytes`);
			if (/\p{Cc}/u.test(route.name)) errors.push(`${prefix}.name: must not contain control characters`);
			if (config.role === "target" && (!Number.isInteger(route.pool ?? 0) || (route.pool ?? 0) < 0 || (route.pool ?? 0) > 65535)) errors.push(`${prefix}.pool must be 0 (auto) or between 1 and 65535`);
		}
  }
  if (config.token && config.token.trim().length < 16) errors.push("token must contain at least 16 characters when set");
  return { valid: errors.length === 0, errors };
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const method = options.method?.toUpperCase() ?? "GET";
  const headers = new Headers(options.headers);
  headers.set("Accept", "application/json");
  if (options.body !== undefined) headers.set("Content-Type", "application/json");
  if (method !== "GET" && method !== "HEAD" && csrfToken) headers.set("X-MoleX-CSRF", csrfToken);

  const response = await fetch(path, { ...options, method, headers, credentials: "same-origin" });
  if (response.status === 401) {
    csrfToken = "";
    window.dispatchEvent(new Event(UNAUTHORIZED_EVENT));
  }
  if (!response.ok) {
    const payload = await response.json().catch(() => ({})) as { error?: string };
    throw new Error(payload.error || `Request failed with HTTP ${response.status}`);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export const api = {
  isPreview: () => previewMode,
  unauthorizedEvent: UNAUTHORIZED_EVENT,

  async getSession(): Promise<SessionState> {
    if (previewMode) return { authenticated: true, csrfToken: "preview" };
    const session = await request<SessionState>("/api/session");
    csrfToken = session.csrfToken ?? "";
    return session;
  },

  async login(password: string): Promise<void> {
    if (previewMode) return;
    const session = await request<SessionState>("/api/login", {
      method: "POST",
      body: JSON.stringify({ password }),
    });
    csrfToken = session.csrfToken ?? "";
  },

  async setup(password: string): Promise<void> {
    if (previewMode) return;
    const session = await request<SessionState>("/api/setup", {
      method: "POST",
      body: JSON.stringify({ password }),
    });
    csrfToken = session.csrfToken ?? "";
  },

  async logout(): Promise<void> {
    if (previewMode) return;
    await request<void>("/api/logout", { method: "POST", body: "{}" });
    csrfToken = "";
  },

  async getConfig(): Promise<Config> {
    return previewMode ? loadMockConfig() : request<Config>("/api/config");
  },

  async saveConfig(config: Config): Promise<void> {
    if (previewMode) {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(config));
      return;
    }
    await request<void>("/api/config", { method: "PUT", body: JSON.stringify(config) });
  },

  async validateConfig(config: Config): Promise<ValidationResult> {
    return previewMode
      ? validateMock(config)
      : request<ValidationResult>("/api/config/validate", { method: "POST", body: JSON.stringify(config) });
  },

  async start(config: Config): Promise<void> {
    if (!previewMode) {
      await request<RuntimeStatus>("/api/runtime/start", { method: "POST", body: JSON.stringify(config) });
      return;
    }
    const validation = validateMock(config);
    if (!validation.valid) throw new Error(validation.errors.join("; "));
    await api.saveConfig(config);
    mockStatus = { state: "connecting", mode: config.mode, role: config.role, message: "Connecting to relay" };
    emitMock({ type: "client_connecting", level: "info", state: "connecting", message: "Connecting to relay" });
    window.clearTimeout(startTimer);
    startTimer = window.setTimeout(() => {
      const listen = config.mode === "relay" ? config.listen : config.role === "edge" ? config.listen : undefined;
      mockStatus = {
        state: "running",
        mode: config.mode,
        role: config.role,
        listen,
        message: config.mode === "relay" ? "Relay is accepting WebSocket sessions" : "Encrypted route is ready",
        startedAt: new Date().toISOString(),
      };
      emitMock({
        type: config.mode === "relay" ? "relay_listening" : "edge_listening",
        level: "info",
        state: "running",
        message: mockStatus.message!,
        listen,
      });
    }, 650);
  },

  async stop(): Promise<void> {
    if (!previewMode) {
      await request<RuntimeStatus>("/api/runtime/stop", { method: "POST", body: "{}" });
      return;
    }
    window.clearTimeout(startTimer);
    mockStatus = { state: "idle", message: "Stopped" };
    emitMock({ type: "runtime_stopped", level: "info", state: "idle", message: "Stopped" });
  },

  async getStatus(): Promise<RuntimeStatus> {
    return previewMode ? mockStatus : request<RuntimeStatus>("/api/runtime/status");
  },

  async getEvents(): Promise<RuntimeEvent[]> {
    return previewMode ? mockEvents : request<RuntimeEvent[]>("/api/events");
  },

  async generateSecret(): Promise<string> {
    if (previewMode) {
      const bytes = crypto.getRandomValues(new Uint8Array(32));
      const encoded = btoa(String.fromCharCode(...bytes)).replaceAll("+", "-").replaceAll("/", "_").replaceAll("=", "");
      return `mx1_${encoded}`;
    }
    const result = await request<{ secret: string }>("/api/secret", { method: "POST", body: "{}" });
    return result.secret;
  },

  subscribe(callback: (event: RuntimeEvent) => void): () => void {
    if (previewMode) {
      mockListeners.add(callback);
      return () => mockListeners.delete(callback);
    }
    const source = new EventSource("/api/events/stream", { withCredentials: true });
    source.onmessage = (message) => {
      try {
        callback(JSON.parse(message.data) as RuntimeEvent);
      } catch {
        // Ignore malformed event frames and keep the live stream available.
      }
    };
    return () => source.close();
  },
};
