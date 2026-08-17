import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App, { applyRuntimeEvent, THEME_STORAGE_KEY } from "./App";
import { api } from "./api";
import type { StreamHandlers } from "./api";
import { LANGUAGE_STORAGE_KEY } from "./i18n";
import type { RuntimeStatus, SessionState } from "./types";

const relaySession: SessionState = {
  authenticated: true,
  csrfToken: "csrf",
  mode: "relay",
  modeLocked: true,
  authRequired: true,
};

const edgeSession: SessionState = {
  authenticated: true,
  csrfToken: "csrf",
  mode: "edge",
  modeLocked: true,
  authRequired: false,
};

const targetSession: SessionState = {
  authenticated: true,
  csrfToken: "csrf",
  mode: "target",
  modeLocked: true,
  authRequired: false,
};

beforeEach(() => {
  localStorage.clear();
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: vi.fn().mockImplementation(() => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("relay console", () => {
  it("completes first-run password setup before showing the console", async () => {
    vi.spyOn(api, "getSession")
      .mockResolvedValueOnce({ ...relaySession, authenticated: false, setupRequired: true })
      .mockResolvedValue(relaySession);
    const setup = vi.spyOn(api, "setup").mockResolvedValue();
    vi.spyOn(api, "getConfig").mockResolvedValue({ mode: "relay", listen: "127.0.0.1:8080", tokens: [] });
    vi.spyOn(api, "getStatus").mockResolvedValue({ state: "idle", message: "Ready" });
    vi.spyOn(api, "getEvents").mockResolvedValue([]);
    vi.spyOn(api, "getTokens").mockResolvedValue([]);

    render(<App />);
    await screen.findByRole("heading", { name: "Create your management password" });
    fireEvent.change(screen.getByLabelText("New password"), { target: { value: "correct-horse-battery-staple" } });
    fireEvent.change(screen.getByLabelText("Confirm password"), { target: { value: "correct-horse-battery-staple" } });
    fireEvent.click(screen.getByRole("button", { name: "Finish setup" }));

    await screen.findByRole("heading", { name: "Relay configuration" });
    expect(setup).toHaveBeenCalledWith("correct-horse-battery-staple");
  });

  it("requires the relay password and localizes login failures", async () => {
    vi.spyOn(api, "getSession").mockResolvedValue({ ...relaySession, authenticated: false });
    vi.spyOn(api, "login").mockRejectedValue(new Error("invalid credentials"));

    render(<App />);
    await screen.findByRole("heading", { name: "Sign in to MoleX" });
    fireEvent.change(screen.getByLabelText("Management password"), { target: { value: "wrong-password" } });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("invalid credentials"));

    fireEvent.click(screen.getByRole("button", { name: "Switch to Chinese" }));
    expect(screen.getByRole("heading", { name: "登录 MoleX" })).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("管理密码错误");
  });

  it("signs in, manages tokens, and signs out", async () => {
    vi.spyOn(api, "getSession")
      .mockResolvedValueOnce({ ...relaySession, authenticated: false })
      .mockResolvedValue(relaySession);
    const login = vi.spyOn(api, "login").mockResolvedValue();
    const logout = vi.spyOn(api, "logout").mockResolvedValue();
    vi.spyOn(api, "getConfig").mockResolvedValue({ mode: "relay", listen: "127.0.0.1:8080", tokens: [] });
    vi.spyOn(api, "getStatus").mockResolvedValue({ state: "idle", message: "Ready" });
    vi.spyOn(api, "getEvents").mockResolvedValue([]);
    vi.spyOn(api, "getTokens").mockResolvedValue([
      { id: "tok-1", token: "mx2_existing-token-value-0123456789", note: "office", createdAt: "2026-08-11T04:30:00Z" },
    ]);
    const createToken = vi.spyOn(api, "createToken").mockResolvedValue({
      id: "tok-2",
      token: "mx2_created-token-value-0123456789",
      note: "lab",
      createdAt: "2026-08-11T05:00:00Z",
    });
    const updateToken = vi.spyOn(api, "updateToken").mockResolvedValue({
      id: "tok-1",
      token: "mx2_existing-token-value-0123456789",
      note: "office",
      disabled: true,
    });

    render(<App />);
    await screen.findByRole("heading", { name: "Sign in to MoleX" });
    fireEvent.change(screen.getByLabelText("Management password"), { target: { value: "correct-horse-battery-staple" } });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));

    await screen.findByRole("heading", { name: "Relay configuration" });
    expect(login).toHaveBeenCalledWith("correct-horse-battery-staple");
    expect(await screen.findByText("office")).toBeInTheDocument();

    // The token value stays masked until revealed.
    expect(screen.queryByText("mx2_existing-token-value-0123456789")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show token" }));
    expect(screen.getByText("mx2_existing-token-value-0123456789")).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("Note, e.g. office-nas"), { target: { value: "lab" } });
    fireEvent.click(screen.getByRole("button", { name: "Create token" }));
    await waitFor(() => expect(createToken).toHaveBeenCalledWith("lab"));
    expect(await screen.findByText("mx2_created-token-value-0123456789")).toBeInTheDocument();

    fireEvent.click(screen.getAllByRole("button", { name: "Disable" })[0]);
    await waitFor(() => expect(updateToken).toHaveBeenCalledWith("tok-1", { disabled: true }));

    const rotateToken = vi.spyOn(api, "rotateToken").mockResolvedValue({
      id: "tok-2",
      token: "mx2_rotated-token-value-0123456789",
      note: "lab",
      previousToken: "mx2_created-token-value-0123456789",
      previousExpiresAt: "2026-08-20T05:00:00Z",
    });
    fireEvent.click(screen.getAllByRole("button", { name: "Rotate" })[1]);
    await waitFor(() => expect(rotateToken).toHaveBeenCalledWith("tok-2", 3));
    expect(await screen.findByText("mx2_rotated-token-value-0123456789")).toBeInTheDocument();
    expect(screen.getByText(/Previous value valid until/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));
    await screen.findByRole("heading", { name: "Sign in to MoleX" });
    expect(logout).toHaveBeenCalledOnce();
  });

  it("groups relay peers by token and offers disconnect", async () => {
    vi.spyOn(api, "getSession").mockResolvedValue(relaySession);
    vi.spyOn(api, "getConfig").mockResolvedValue({ mode: "relay", listen: "127.0.0.1:8080", tokens: [] });
    vi.spyOn(api, "getTokens").mockResolvedValue([
      { id: "tok-alpha", token: "mx2_alpha-token-value-0123456789", note: "alpha" },
    ]);
    vi.spyOn(api, "getStatus").mockResolvedValue({
      state: "running",
      mode: "relay",
      listen: "127.0.0.1:8080",
      message: "Relay is accepting WebSocket sessions",
      peers: [
        {
          id: "1",
          ip: "198.51.100.10",
          name: "office-edge",
          role: "edge",
          status: "paired",
          tokenId: "tok-alpha",
          platform: "windows/amd64",
          routeId: "a1b2c3d4e5f6",
          peerId: "2",
          peerName: "home-target",
          proxied: true,
          connectedAt: "2026-08-11T04:30:00Z",
          lastActivityAt: "2026-08-11T04:32:00Z",
          bytesReceived: 4096,
          bytesSent: 2048,
        },
        {
          id: "2",
          ip: "2001:db8::20",
          name: "home-target",
          role: "target",
          status: "paired",
          tokenId: "tok-alpha",
          platform: "linux/amd64",
          routeId: "a1b2c3d4e5f6",
          connectedAt: "2026-08-11T04:31:00Z",
        },
      ],
    });
    vi.spyOn(api, "getEvents").mockResolvedValue([
      { type: "relay_peer_connected", level: "info", message: "Edge connected from 198.51.100.10", time: "2026-08-11T04:30:00Z" },
    ]);
    const disconnect = vi.spyOn(api, "disconnectPeer").mockResolvedValue();

    render(<App />);
    await screen.findByRole("heading", { name: "Connected clients" });
    expect(screen.getByText("198.51.100.10")).toBeInTheDocument();
    expect(screen.getByText("2001:db8::20")).toBeInTheDocument();
    expect(await screen.findByText("Target online")).toBeInTheDocument();
    expect(screen.getByText(/1 edges/)).toBeInTheDocument();
    expect(screen.getAllByText("tok-alpha").length).toBeGreaterThan(0);

    fireEvent.click(screen.getAllByRole("button", { name: "Disconnect" })[0]);
    await waitFor(() => expect(disconnect).toHaveBeenCalledWith("1"));

    fireEvent.click(screen.getByRole("button", { name: "Switch to Chinese" }));
    expect(screen.getByRole("heading", { name: "已连接客户端" })).toBeInTheDocument();
    expect(screen.getByText("边缘端已从 198.51.100.10 接入")).toBeInTheDocument();
  });
});

describe("edge console", () => {
  it("works without a login flow and shows the catalog start hint while idle", async () => {
    vi.spyOn(api, "getSession").mockResolvedValue(edgeSession);
    vi.spyOn(api, "getConfig").mockResolvedValue({
      mode: "edge",
      remote: "wss://relay.example.com/ws/session",
      token: "mx2_edge-token-0123456789",
      mappings: [],
    });
    vi.spyOn(api, "getStatus").mockResolvedValue({ state: "idle", message: "Ready" });
    vi.spyOn(api, "getEvents").mockResolvedValue([]);

    render(<App />);
    await screen.findByRole("heading", { name: "Connection" });
    expect(screen.getByLabelText("Relay WSS address")).toHaveValue("wss://relay.example.com/ws/session");
    expect(screen.getByLabelText("Access token")).toBeInTheDocument();
    expect(screen.getByText("Start the edge to load the catalog from the target.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Sign out" })).not.toBeInTheDocument();
    expect(screen.queryByText("Listening")).not.toBeInTheDocument();
  });

  it("maps a catalog service to a suggested local port and applies it", async () => {
    vi.spyOn(api, "getSession").mockResolvedValue(edgeSession);
    vi.spyOn(api, "getConfig").mockResolvedValue({
      mode: "edge",
      remote: "wss://relay.example.com/ws/session",
      token: "mx2_edge-token-0123456789",
      mappings: [{ service: "svc-web", port: 28080 }],
    });
    vi.spyOn(api, "getStatus").mockResolvedValue({
      state: "running",
      mode: "edge",
      message: "Encrypted route is ready; waiting for the target service catalog",
      catalog: {
        online: true,
        services: [
          { id: "svc-web", name: "web", address: "10.188.200.16:30927" },
          { id: "svc-ssh", name: "ssh", address: "10.188.200.16:22" },
        ],
      },
      mappings: [
        {
          service: "svc-web",
          serviceName: "web",
          address: "10.188.200.16:30927",
          listen: "127.0.0.1:28080",
          state: "listening",
          connections: 3,
          bytes: 2048,
        },
      ],
    });
    vi.spyOn(api, "getEvents").mockResolvedValue([]);
    const suggest = vi.spyOn(api, "suggestPort").mockResolvedValue(31234);
    const saveMappings = vi.spyOn(api, "saveMappings").mockImplementation(async (mappings) => mappings);

    render(<App />);
    await screen.findByRole("heading", { name: "Connection" });
    expect(screen.getByText("web")).toBeInTheDocument();
    expect(screen.getByText("Listening")).toBeInTheDocument();
    expect(screen.getByText("127.0.0.1:28080")).toBeInTheDocument();
    expect(screen.getByText(/3 conns/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("checkbox", { name: "Service catalog: ssh" }));
    await waitFor(() => expect(suggest).toHaveBeenCalled());
    expect(screen.getByLabelText("Local port ssh")).toHaveValue(31234);

    fireEvent.click(screen.getByRole("button", { name: "Apply mappings" }));
    await waitFor(() => expect(saveMappings).toHaveBeenCalledWith([
      { service: "svc-ssh", group: undefined, port: 31234, lan: undefined },
      { service: "svc-web", group: undefined, port: 28080, lan: undefined },
    ]));
  });

  it("shows the reconnect banner when live updates drop", async () => {
    vi.spyOn(api, "getSession").mockResolvedValue(edgeSession);
    vi.spyOn(api, "getConfig").mockResolvedValue({
      mode: "edge",
      remote: "wss://relay.example.com/ws/session",
      token: "mx2_edge-token-0123456789",
    });
    vi.spyOn(api, "getStatus").mockResolvedValue({ state: "running", mode: "edge", message: "Encrypted route is ready; waiting for the target service catalog" });
    vi.spyOn(api, "getEvents").mockResolvedValue([]);
    let handlers: StreamHandlers | undefined;
    vi.spyOn(api, "subscribe").mockImplementation((incoming) => {
      handlers = incoming;
      return () => undefined;
    });

    render(<App />);
    await screen.findByRole("heading", { name: "Connection" });
    act(() => handlers?.onError?.());
    expect(await screen.findByText("Live updates disconnected; reconnecting…")).toBeInTheDocument();
  });

  it("shows a skeleton while the console data loads", async () => {
    vi.spyOn(api, "getSession").mockResolvedValue(edgeSession);
    let resolveConfig: ((config: { mode: "edge" }) => void) | undefined;
    vi.spyOn(api, "getConfig").mockReturnValue(new Promise((resolve) => { resolveConfig = resolve; }));
    vi.spyOn(api, "getStatus").mockResolvedValue({ state: "idle", message: "Ready" });
    vi.spyOn(api, "getEvents").mockResolvedValue([]);

    render(<App />);
    await screen.findByRole("status", { name: "Loading data" });
    act(() => resolveConfig?.({ mode: "edge" }));
    await screen.findByRole("heading", { name: "Connection" });
    expect(screen.queryByRole("status", { name: "Loading data" })).not.toBeInTheDocument();
  });
});

describe("multi-group consoles", () => {
  it("lets a target restrict a service to selected token groups", async () => {
    vi.spyOn(api, "getSession").mockResolvedValue(targetSession);
    vi.spyOn(api, "getConfig").mockResolvedValue({
      mode: "target",
      remote: "wss://relay.example.com/ws/session",
      tokens: [
        { id: "office", token: "mx2_office-token-0123456789" },
        { id: "lab", token: "mx2_lab-token-0123456789" },
      ],
      services: [{ id: "svc-web", name: "web", address: "10.188.200.16:30927" }],
    });
    vi.spyOn(api, "getStatus").mockResolvedValue({ state: "idle", message: "Ready" });
    vi.spyOn(api, "getEvents").mockResolvedValue([]);
    const saveServices = vi.spyOn(api, "saveServices").mockImplementation(async (services) => services);

    render(<App />);
    await screen.findByRole("heading", { name: "Connection" });
    expect(screen.getByText("Visible to")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("checkbox", { name: "lab" }));
    const save = screen.getByRole("button", { name: "Save services" });
    await waitFor(() => expect(save).toBeEnabled());
    fireEvent.click(save);
    await waitFor(() => expect(saveServices).toHaveBeenCalledWith([
      { id: "svc-web", name: "web", address: "10.188.200.16:30927", groups: ["office"] },
    ]));
  });

  it("maps a grouped catalog service with its group name", async () => {
    vi.spyOn(api, "getSession").mockResolvedValue(edgeSession);
    vi.spyOn(api, "getConfig").mockResolvedValue({
      mode: "edge",
      remote: "wss://relay.example.com/ws/session",
      tokens: [
        { id: "office", token: "mx2_office-token-0123456789" },
        { id: "lab", token: "mx2_lab-token-0123456789" },
      ],
      mappings: [],
    });
    vi.spyOn(api, "getStatus").mockResolvedValue({
      state: "running",
      mode: "edge",
      catalog: {
        online: true,
        services: [
          { id: "svc-web", name: "web", address: "10.0.0.5:80", group: "office" },
        ],
        groups: [
          { group: "office", online: true, services: [{ id: "svc-web", name: "web", address: "10.0.0.5:80", group: "office" }] },
          { group: "lab", online: true, services: [{ id: "svc-api", name: "api", address: "10.0.0.6:8080", group: "lab" }] },
        ],
      },
    });
    vi.spyOn(api, "getEvents").mockResolvedValue([]);
    vi.spyOn(api, "suggestPort").mockResolvedValue(31001);
    const saveMappings = vi.spyOn(api, "saveMappings").mockImplementation(async (mappings) => mappings);

    render(<App />);
    await screen.findByRole("heading", { name: "Connection" });
    fireEvent.click(screen.getByRole("checkbox", { name: "Service catalog: office / web" }));
    await waitFor(() => expect(screen.getByLabelText("Local port office / web")).toHaveValue(31001));
    fireEvent.click(screen.getByRole("button", { name: "Apply mappings" }));
    await waitFor(() => expect(saveMappings).toHaveBeenCalledWith([
      { service: "svc-web", group: "office", port: 31001, lan: undefined },
    ]));
  });
});

describe("target console", () => {
  it("edits and saves the published service list", async () => {
    vi.spyOn(api, "getSession").mockResolvedValue(targetSession);
    vi.spyOn(api, "getConfig").mockResolvedValue({
      mode: "target",
      remote: "wss://relay.example.com/ws/session",
      token: "mx2_target-token-0123456789",
      services: [{ id: "svc-web", name: "web", address: "10.188.200.16:30927" }],
    });
    vi.spyOn(api, "getStatus").mockResolvedValue({
      state: "running",
      mode: "target",
      message: "Target is ready to receive streams",
      services: [{ id: "svc-web", name: "web", address: "10.188.200.16:30927", streams: 7, lastError: "connect: connection refused" }],
    });
    vi.spyOn(api, "getEvents").mockResolvedValue([]);
    const saveServices = vi.spyOn(api, "saveServices").mockImplementation(async (services) => services.map((service, index) => ({
      ...service,
      id: service.id || `svc-generated-${index}`,
    })));

    render(<App />);
    await screen.findByRole("heading", { name: "Connection" });
    expect(screen.getByDisplayValue("web")).toBeInTheDocument();
    expect(screen.getByText(/7 streams/)).toBeInTheDocument();
    expect(screen.getByText(/connection refused/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Add service" }));
    const nameInputs = screen.getAllByLabelText("Service name");
    const addressInputs = screen.getAllByLabelText("Address (host:port)");
    fireEvent.change(nameInputs[1], { target: { value: "ssh" } });
    fireEvent.change(addressInputs[1], { target: { value: "10.188.200.16:22" } });
    fireEvent.click(screen.getByRole("button", { name: "Save services" }));
    await waitFor(() => expect(saveServices).toHaveBeenCalledWith([
      { id: "svc-web", name: "web", address: "10.188.200.16:30927" },
      { id: "", name: "ssh", address: "10.188.200.16:22" },
    ]));
  });
});

describe("role bootstrap", () => {
  it("offers edge and target roles and locks the chosen one", async () => {
    vi.spyOn(api, "getSession").mockResolvedValue({ ...edgeSession, modeLocked: false });
    const bootstrap = vi.spyOn(api, "bootstrap").mockResolvedValue(targetSession);
    vi.spyOn(api, "getConfig").mockResolvedValue({ mode: "target" });
    vi.spyOn(api, "getStatus").mockResolvedValue({ state: "idle", message: "Ready" });
    vi.spyOn(api, "getEvents").mockResolvedValue([]);

    render(<App />);
    await screen.findByRole("heading", { name: "Choose this device's role" });
    expect(screen.getByText("molex config init --mode relay")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Target/ }));
    await waitFor(() => expect(bootstrap).toHaveBeenCalledWith("target"));
    await screen.findByRole("heading", { name: "Connection" });
  });
});

describe("shared shell", () => {
  it("applies peer and catalog events and ignores late traffic after disconnect", () => {
    const connectedAt = "2026-08-11T04:30:00Z";
    const initial: RuntimeStatus = {
      state: "running",
      peers: [{ id: "1", ip: "198.51.100.10", role: "edge", status: "paired", connectedAt }],
    };
    const removed = applyRuntimeEvent(initial, {
      type: "relay_peer_disconnected",
      level: "info",
      message: "Edge disconnected from 198.51.100.10",
      time: connectedAt,
      peerChange: { action: "remove", peers: initial.peers! },
    });
    expect(removed.peers).toEqual([]);

    const lateStats = applyRuntimeEvent(removed, {
      type: "relay_peer_stats",
      level: "info",
      message: "",
      transient: true,
      time: connectedAt,
      peerChange: { action: "update", peers: [{ ...initial.peers![0], bytesReceived: 4096 }] },
    });
    expect(lateStats.peers).toEqual([]);

    const catalogUpdate = applyRuntimeEvent(lateStats, {
      type: "edge_catalog",
      level: "info",
      message: "Target published 1 service(s)",
      time: connectedAt,
      catalog: { online: true, services: [{ id: "svc-1", name: "web", address: "10.0.0.5:80" }] },
      mappings: [{ service: "svc-1", state: "listening", listen: "127.0.0.1:28080" }],
    });
    expect(catalogUpdate.catalog?.services).toHaveLength(1);
    expect(catalogUpdate.mappings?.[0].state).toBe("listening");
  });

  it("switches the complete interface to Chinese and persists the preference", async () => {
    vi.spyOn(api, "getSession").mockResolvedValue(edgeSession);
    vi.spyOn(api, "getConfig").mockResolvedValue({ mode: "edge", remote: "wss://relay.example.com/ws/session", token: "mx2_edge-token-0123456789" });
    vi.spyOn(api, "getStatus").mockResolvedValue({ state: "idle", message: "Ready" });
    vi.spyOn(api, "getEvents").mockResolvedValue([]);

    render(<App />);
    await screen.findByRole("heading", { name: "Connection" });

    fireEvent.click(screen.getByRole("button", { name: "Switch to Chinese" }));

    expect(screen.getByRole("heading", { name: "连接设置" })).toBeInTheDocument();
    expect(screen.getByLabelText("Relay WSS 地址")).toBeInTheDocument();
    expect(screen.getByLabelText("节点名称")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "切换到英文" })).toBeInTheDocument();
    expect(document.documentElement.lang).toBe("zh-CN");
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBe("zh-CN");
  });

  it("follows system appearance and persists the three theme preferences", async () => {
    let systemIsDark = false;
    const listeners = new Set<() => void>();
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: vi.fn().mockImplementation(() => ({
        get matches() {
          return systemIsDark;
        },
        addEventListener: (_event: string, listener: () => void) => listeners.add(listener),
        removeEventListener: (_event: string, listener: () => void) => listeners.delete(listener),
      })),
    });
    vi.spyOn(api, "getSession").mockResolvedValue(edgeSession);
    vi.spyOn(api, "getConfig").mockResolvedValue({ mode: "edge", remote: "wss://relay.example.com/ws/session", token: "mx2_edge-token-0123456789" });
    vi.spyOn(api, "getStatus").mockResolvedValue({ state: "idle", message: "Ready" });
    vi.spyOn(api, "getEvents").mockResolvedValue([]);

    render(<App />);
    await screen.findByRole("heading", { name: "Connection" });

    expect(document.documentElement.dataset.theme).toBe("light");
    expect(document.documentElement.dataset.themePreference).toBe("system");
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe("system");

    act(() => {
      systemIsDark = true;
      listeners.forEach((listener) => listener());
    });
    expect(document.documentElement.dataset.theme).toBe("dark");

    fireEvent.click(screen.getByRole("button", { name: "System theme. Use light theme" }));
    expect(document.documentElement.dataset.theme).toBe("light");
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe("light");

    fireEvent.click(screen.getByRole("button", { name: "Light theme. Use dark theme" }));
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe("dark");

    fireEvent.click(screen.getByRole("button", { name: "Dark theme. Follow system theme" }));
    expect(document.documentElement.dataset.themePreference).toBe("system");
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe("system");
  });

  it("localizes validation messages in Chinese", async () => {
    localStorage.setItem(LANGUAGE_STORAGE_KEY, "zh-CN");
    vi.spyOn(api, "getSession").mockResolvedValue(edgeSession);
    vi.spyOn(api, "getConfig").mockResolvedValue({ mode: "edge", remote: "wss://relay.example.com/ws/session", token: "" });
    vi.spyOn(api, "getStatus").mockResolvedValue({ state: "idle", message: "Ready" });
    vi.spyOn(api, "getEvents").mockResolvedValue([]);
    vi.spyOn(api, "validateConfig").mockResolvedValue({ valid: false, errors: ["token must contain at least 16 characters"] });

    render(<App />);
    await screen.findByRole("heading", { name: "连接设置" });

    fireEvent.click(screen.getByRole("button", { name: "启动" }));

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("Token 至少需要 16 个字符"));
  });
});
