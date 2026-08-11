import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App, { applyRuntimeEvent, THEME_STORAGE_KEY } from "./App";
import { api } from "./api";
import { LANGUAGE_STORAGE_KEY } from "./i18n";

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

describe("MoleX console", () => {
  it("requires the node password and localizes login failures", async () => {
    vi.spyOn(api, "getSession").mockResolvedValue({ authenticated: false });
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

  it("loads the console after login and returns to login after logout", async () => {
    vi.spyOn(api, "getSession").mockResolvedValue({ authenticated: false });
    const login = vi.spyOn(api, "login").mockResolvedValue();
    const logout = vi.spyOn(api, "logout").mockResolvedValue();

    render(<App />);
    await screen.findByRole("heading", { name: "Sign in to MoleX" });
    fireEvent.change(screen.getByLabelText("Management password"), { target: { value: "correct-horse-battery-staple" } });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));

    await screen.findByRole("heading", { name: "Route configuration" });
    expect(login).toHaveBeenCalledWith("correct-horse-battery-staple");
    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));
    await screen.findByRole("heading", { name: "Sign in to MoleX" });
    expect(logout).toHaveBeenCalledOnce();
  });

  it("switches between client roles without losing shared fields", async () => {
    render(<App />);
    await screen.findByRole("heading", { name: "Route configuration" });
    const remote = screen.getByLabelText("Relay endpoint") as HTMLInputElement;
    fireEvent.change(remote, { target: { value: "wss://relay.example.net/ws/session" } });
    fireEvent.click(screen.getByRole("button", { name: "Target service", pressed: false }));
    expect(screen.getByLabelText("Target service")).toBeInTheDocument();
    expect((screen.getByLabelText("Relay endpoint") as HTMLInputElement).value).toBe("wss://relay.example.net/ws/session");
  });

  it("shows actionable validation before starting", async () => {
    render(<App />);
    await screen.findByRole("heading", { name: "Route configuration" });
    fireEvent.click(screen.getByRole("button", { name: "Start" }));
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("secret must contain at least 16 characters"));
  });

  it("does not advertise the configured Edge port while it is idle", async () => {
    render(<App />);
    await screen.findByRole("heading", { name: "Route configuration" });

    expect(screen.getByText("Not listening")).not.toHaveClass("mono");
  });

  it("renders the relay-specific controls", async () => {
    render(<App />);
    await screen.findByRole("heading", { name: "Route configuration" });
    fireEvent.click(screen.getByRole("button", { name: "Relay", pressed: false }));
    expect(screen.getByLabelText("Listen address")).toBeInTheDocument();
    expect(screen.getByLabelText("Admission token")).toBeInTheDocument();
    expect(screen.queryByLabelText("Relay endpoint")).not.toBeInTheDocument();
  });

  it("shows an empty connected-client list for a relay", async () => {
    vi.spyOn(api, "getConfig").mockResolvedValue({
      mode: "relay",
      role: "edge",
      secret: "",
      token: "relay-token-0123456789",
      listen: "127.0.0.1:8080",
      remote: "",
      tunnel: { local: "", remote: "", name: "" },
    });
    vi.spyOn(api, "getStatus").mockResolvedValue({
      state: "running",
      mode: "relay",
      listen: "127.0.0.1:8080",
      message: "Relay is accepting WebSocket sessions",
      peers: [],
    });
    vi.spyOn(api, "getEvents").mockResolvedValue([]);

    render(<App />);
    await screen.findByRole("heading", { name: "Connected clients" });
    expect(screen.getByText("127.0.0.1:8080")).toHaveClass("mono");
    expect(screen.getByText("No clients connected")).toBeInTheDocument();
  });

  it("renders relay client IPs, roles, states, times, and Chinese activity", async () => {
    vi.spyOn(api, "getConfig").mockResolvedValue({
      mode: "relay",
      role: "edge",
      secret: "",
      token: "relay-token-0123456789",
      listen: "127.0.0.1:8080",
      remote: "",
      tunnel: { local: "", remote: "", name: "" },
    });
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
					endpoint: "127.0.0.1:2222",
					relayEndpoint: "wss://relay.example.com/ws/session",
					platform: "windows/amd64",
					routeId: "a1b2c3d4e5f6",
					peerId: "2",
					peerName: "home-target",
					proxied: true,
					connectedAt: "2026-08-11T04:30:00Z",
					lastActivityAt: "2026-08-11T04:32:00Z",
					bytesReceived: 4096,
					bytesSent: 2048,
					framesReceived: 8,
					framesSent: 6,
				},
				{
					id: "2",
					ip: "2001:db8::20",
					name: "home-target",
					role: "target",
					status: "waiting",
					endpoint: "192.168.1.20:22",
					relayEndpoint: "wss://relay.example.com/ws/session",
					platform: "linux/amd64",
					routeId: "a1b2c3d4e5f6",
					connectedAt: "2026-08-11T04:31:00Z",
					lastActivityAt: "0001-01-01T00:00:00Z",
				},
      ],
    });
    vi.spyOn(api, "getEvents").mockResolvedValue([
      {
        type: "relay_peer_connected",
        level: "info",
        message: "Edge connected from 198.51.100.10",
        time: "2026-08-11T04:30:00Z",
      },
    ]);

    render(<App />);
    await screen.findByRole("heading", { name: "Connected clients" });
    expect(screen.getByText("198.51.100.10")).toBeInTheDocument();
    expect(screen.getByText("2001:db8::20")).toBeInTheDocument();
    expect(screen.getByText("Paired")).toBeInTheDocument();
    expect(screen.getByText("Waiting")).toBeInTheDocument();
		expect(screen.getAllByText("office-edge").length).toBeGreaterThan(0);
		expect(screen.getAllByText("home-target").length).toBeGreaterThan(0);
		expect(screen.getByText("127.0.0.1:2222")).toBeInTheDocument();
		expect(screen.getByText("192.168.1.20:22")).toBeInTheDocument();
		expect(screen.getAllByText("a1b2c3d4e5f6")).toHaveLength(2);
		expect(screen.getByText("windows/amd64")).toBeInTheDocument();
		expect(screen.getByText("Trusted proxy")).toBeInTheDocument();
		expect(screen.getAllByText("wss://relay.example.com/ws/session")).toHaveLength(2);
		expect(screen.getByText("No traffic yet")).toBeInTheDocument();
		expect(document.querySelectorAll(".peer-detail-time")).toHaveLength(2);

    fireEvent.click(screen.getByRole("button", { name: "Switch to Chinese" }));
    expect(screen.getByRole("heading", { name: "已连接客户端" })).toBeInTheDocument();
    expect(screen.getByText("已配对")).toBeInTheDocument();
    expect(screen.getByText("等待中")).toBeInTheDocument();
		expect(screen.getAllByText("转发端点")).toHaveLength(2);
		expect(screen.getByText("可信代理")).toBeInTheDocument();
		expect(screen.getByText("暂无流量")).toBeInTheDocument();
		expect(screen.getByText("边缘端已从 198.51.100.10 接入")).toBeInTheDocument();
  });

	it("applies peer events in order and ignores late traffic after disconnect", () => {
		const connectedAt = "2026-08-11T04:30:00Z";
		const initial = {
			state: "running" as const,
			peers: [{ id: "1", ip: "198.51.100.10", role: "edge" as const, status: "paired" as const, connectedAt }],
		};
		const removed = applyRuntimeEvent(initial, {
			type: "relay_peer_disconnected",
			level: "info",
			message: "Edge disconnected from 198.51.100.10",
			time: connectedAt,
			peerChange: { action: "remove", peers: initial.peers },
		});
		expect(removed.peers).toEqual([]);

		const lateStats = applyRuntimeEvent(removed, {
			type: "relay_peer_stats",
			level: "info",
			message: "",
			transient: true,
			time: connectedAt,
			peerChange: { action: "update", peers: [{ ...initial.peers[0], bytesReceived: 4096 }] },
		});
		expect(lateStats.peers).toEqual([]);
	});

  it("switches the complete interface to Chinese and persists the preference", async () => {
    render(<App />);
    await screen.findByRole("heading", { name: "Route configuration" });

    fireEvent.click(screen.getByRole("button", { name: "Switch to Chinese" }));

    expect(screen.getByRole("heading", { name: "路由配置" })).toBeInTheDocument();
    expect(screen.getByLabelText("中继端点")).toBeInTheDocument();
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

    render(<App />);
    await screen.findByRole("heading", { name: "Route configuration" });

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
    render(<App />);
    await screen.findByRole("heading", { name: "路由配置" });

    fireEvent.click(screen.getByRole("button", { name: "启动" }));

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("密钥至少需要 16 个字符"));
  });
});
