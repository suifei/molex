import type {
  Config,
  MappingEntry,
  Mode,
  RuntimeEvent,
  RuntimeStatus,
  ServiceEntry,
  SessionState,
  TokenEntry,
  ValidationResult,
} from "./types";

const STORAGE_KEY = "molex:preview-config";
const UNAUTHORIZED_EVENT = "molex:unauthorized";
const previewMode = import.meta.env.DEV;

const defaultConfig: Config = {
  mode: "edge",
  remote: "wss://molex.example.com/ws/session",
  token: "",
  name: "",
  mappings: [],
};

export interface StreamHandlers {
  onEvent: (event: RuntimeEvent) => void;
  onOpen?: () => void;
  onError?: () => void;
}

let csrfToken = "";
let mockStatus: RuntimeStatus = { state: "idle", message: "Ready" };
let mockEvents: RuntimeEvent[] = [];
let mockTokens: TokenEntry[] = [];
const mockListeners = new Set<(event: RuntimeEvent) => void>();
let startTimer: number | undefined;

function emitMock(event: Omit<RuntimeEvent, "time">) {
  const complete = { ...event, time: new Date().toISOString() } as RuntimeEvent;
  if (!complete.transient) mockEvents = [...mockEvents.slice(-99), complete];
  mockListeners.forEach((listener) => listener(complete));
}

function loadMockConfig(): Config {
  const stored = localStorage.getItem(STORAGE_KEY);
  if (!stored) return structuredClone(defaultConfig);
  try {
    const loaded = JSON.parse(stored) as Partial<Config>;
    return { ...structuredClone(defaultConfig), ...loaded };
  } catch {
    return structuredClone(defaultConfig);
  }
}

function saveMockConfig(config: Config) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(config));
}

function validateMock(config: Config): ValidationResult {
  const errors: string[] = [];
  if (config.mode === "relay") {
    if (!config.listen) errors.push("listen: address is required");
  } else {
    if (!/^wss?:\/\//.test(config.remote ?? "")) errors.push("remote: must be a valid ws:// or wss:// URL");
    const groups = (config.tokens ?? []).filter((entry) => entry.token.trim());
    if (groups.length > 0) {
      if (config.token?.trim()) errors.push("use either the single token field or the tokens group list, not both");
      if (groups.length > 1 && groups.some((entry) => !entry.id.trim())) {
        errors.push("tokens[0].id: a group name is required when joining several groups");
      }
    } else if ((config.token ?? "").trim().length < 16) {
      errors.push("token must contain at least 16 characters");
    }
  }
  return { valid: errors.length === 0, errors };
}

function mockCatalog(): { id: string; name: string; address: string }[] {
  return [
    { id: "svc-demo-web", name: "web", address: "10.188.200.16:30927" },
    { id: "svc-demo-ssh", name: "ssh", address: "10.188.200.16:22" },
  ];
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
    if (previewMode) {
      const mode = loadMockConfig().mode;
      return { authenticated: true, csrfToken: "preview", mode, modeLocked: true, authRequired: mode === "relay" };
    }
    const session = await request<SessionState>("/api/session");
    csrfToken = session.csrfToken ?? "";
    return session;
  },

  async bootstrap(mode: Mode): Promise<SessionState> {
    if (previewMode) {
      const config = loadMockConfig();
      config.mode = mode;
      saveMockConfig(config);
      return { authenticated: true, csrfToken: "preview", mode, modeLocked: true, authRequired: false };
    }
    const session = await request<SessionState>("/api/bootstrap", {
      method: "POST",
      body: JSON.stringify({ mode }),
    });
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
      saveMockConfig(config);
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
    mockStatus = { state: "connecting", mode: config.mode, message: "Connecting to relay" };
    emitMock({ type: "client_connecting", level: "info", state: "connecting", message: "Connecting to relay" });
    window.clearTimeout(startTimer);
    startTimer = window.setTimeout(() => {
      if (config.mode === "relay") {
        mockStatus = {
          state: "running",
          mode: "relay",
          listen: config.listen,
          message: "Relay is accepting WebSocket sessions",
          startedAt: new Date().toISOString(),
          peers: [],
        };
        emitMock({ type: "relay_listening", level: "info", state: "running", message: mockStatus.message!, listen: config.listen });
        return;
      }
      if (config.mode === "target") {
        mockStatus = {
          state: "running",
          mode: "target",
          message: "Target is ready to receive streams",
          startedAt: new Date().toISOString(),
          services: (config.services ?? []).map((service) => ({ ...service, streams: 0 })),
        };
        emitMock({ type: "target_ready", level: "info", state: "running", message: mockStatus.message!, services: mockStatus.services });
        return;
      }
      const catalog = { online: true, services: mockCatalog() };
      const mappings = (config.mappings ?? []).map((mapping) => {
        const service = catalog.services.find((entry) => entry.id === mapping.service);
        return {
          service: mapping.service,
          serviceName: service?.name,
          address: service?.address,
          listen: `${mapping.lan ? "0.0.0.0" : "127.0.0.1"}:${mapping.port}`,
          lan: mapping.lan,
          state: (service ? "listening" : "waiting") as "listening" | "waiting",
          message: service ? undefined : "The target does not publish this service; it stays inactive until published again",
        };
      });
      mockStatus = {
        state: "running",
        mode: "edge",
        message: "Encrypted route is ready; waiting for the target service catalog",
        startedAt: new Date().toISOString(),
        catalog,
        mappings,
      };
      emitMock({
        type: "edge_catalog",
        level: "info",
        state: "running",
        message: `Target published ${catalog.services.length} service(s)`,
        catalog,
        mappings,
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

  async getTokens(): Promise<TokenEntry[]> {
    if (previewMode) return mockTokens;
    return request<TokenEntry[]>("/api/tokens");
  },

  async createToken(note: string): Promise<TokenEntry> {
    if (previewMode) {
      const bytes = crypto.getRandomValues(new Uint8Array(32));
      const encoded = btoa(String.fromCharCode(...bytes)).replaceAll("+", "-").replaceAll("/", "_").replaceAll("=", "");
      const entry: TokenEntry = {
        id: `tok-${Math.random().toString(16).slice(2, 12)}`,
        token: `mx2_${encoded}`,
        note,
        createdAt: new Date().toISOString(),
      };
      mockTokens = [...mockTokens, entry];
      return entry;
    }
    return request<TokenEntry>("/api/tokens", { method: "POST", body: JSON.stringify({ note }) });
  },

  async updateToken(id: string, changes: { note?: string; disabled?: boolean }): Promise<TokenEntry> {
    if (previewMode) {
      mockTokens = mockTokens.map((token) => (token.id === id ? { ...token, ...changes } : token));
      const updated = mockTokens.find((token) => token.id === id);
      if (!updated) throw new Error("token not found");
      return updated;
    }
    return request<TokenEntry>(`/api/tokens/${id}`, { method: "PUT", body: JSON.stringify(changes) });
  },

  async deleteToken(id: string): Promise<void> {
    if (previewMode) {
      mockTokens = mockTokens.filter((token) => token.id !== id);
      return;
    }
    await request<void>(`/api/tokens/${id}`, { method: "DELETE" });
  },

  async rotateToken(id: string, graceDays = 3): Promise<TokenEntry> {
    if (previewMode) {
      const current = mockTokens.find((token) => token.id === id);
      if (!current) throw new Error("token not found");
      const bytes = crypto.getRandomValues(new Uint8Array(32));
      const encoded = btoa(String.fromCharCode(...bytes)).replaceAll("+", "-").replaceAll("/", "_").replaceAll("=", "");
      const expires = new Date(Date.now() + graceDays * 24 * 60 * 60 * 1000).toISOString();
      const updated: TokenEntry = {
        ...current,
        previousToken: current.token,
        previousExpiresAt: expires,
        token: `mx2_${encoded}`,
      };
      mockTokens = mockTokens.map((token) => (token.id === id ? updated : token));
      return updated;
    }
    return request<TokenEntry>(`/api/tokens/${id}/rotate`, { method: "POST", body: JSON.stringify({ graceDays }) });
  },

  async disconnectPeer(id: string): Promise<void> {
    if (previewMode) return;
    await request<void>("/api/peers/disconnect", { method: "POST", body: JSON.stringify({ id }) });
  },

  async saveServices(services: ServiceEntry[]): Promise<ServiceEntry[]> {
    if (previewMode) {
      const config = loadMockConfig();
      config.services = services.map((service, index) => ({
        ...service,
        id: service.id || `svc-${Math.random().toString(16).slice(2, 10)}-${index}`,
      }));
      saveMockConfig(config);
      return config.services;
    }
    return request<ServiceEntry[]>("/api/services", { method: "PUT", body: JSON.stringify(services) });
  },

  async saveMappings(mappings: MappingEntry[]): Promise<MappingEntry[]> {
    if (previewMode) {
      const config = loadMockConfig();
      config.mappings = mappings;
      saveMockConfig(config);
      return mappings;
    }
    return request<MappingEntry[]>("/api/mappings", { method: "PUT", body: JSON.stringify(mappings) });
  },

  async suggestPort(lan: boolean): Promise<number> {
    if (previewMode) return 20000 + Math.floor(Math.random() * 20000);
    const result = await request<{ port: number }>("/api/ports/free", { method: "POST", body: JSON.stringify({ lan }) });
    return result.port;
  },

  subscribe(handlers: StreamHandlers): () => void {
    if (previewMode) {
      mockListeners.add(handlers.onEvent);
      handlers.onOpen?.();
      return () => mockListeners.delete(handlers.onEvent);
    }
    const source = new EventSource("/api/events/stream", { withCredentials: true });
    source.onopen = () => handlers.onOpen?.();
    source.onerror = () => handlers.onError?.();
    source.onmessage = (message) => {
      try {
        handlers.onEvent(JSON.parse(message.data) as RuntimeEvent);
      } catch {
        // Ignore malformed event frames and keep the live stream available.
      }
    };
    return () => source.close();
  },
};
