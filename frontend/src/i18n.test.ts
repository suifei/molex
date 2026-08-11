import { describe, expect, it } from "vitest";
import { localizeRuntimeMessage } from "./i18n";

describe("runtime message localization", () => {
  it("localizes retry timing and relay authentication guidance", () => {
    const message = "Route unavailable; retrying in 1.2s. Relay authentication was rejected (HTTP 401). Make the relay token identical on Relay, Edge, and Target.";

    expect(localizeRuntimeMessage(message, "zh-CN")).toBe(
      "路由不可用；将在 1.2 秒后重试。中继拒绝了身份验证（HTTP 401）。请确保 Relay、Edge 和 Target 使用完全相同的中继令牌。",
    );
    expect(localizeRuntimeMessage(message, "en")).toBe(message);
  });

  it("localizes listener recovery guidance", () => {
    const message = "Route unavailable; retrying in 900ms. The local listener could not start on 127.0.0.1:2222. Stop the process using that address or choose a free listen address; MoleX will keep retrying.";

    expect(localizeRuntimeMessage(message, "zh-CN")).toContain("将在 900 毫秒后重试");
    expect(localizeRuntimeMessage(message, "zh-CN")).toContain("停止占用该地址的进程");
  });

  it("localizes target service recovery guidance while retaining its address", () => {
    const message = "Target service at 127.0.0.1:22 is unavailable. Start the service or correct tunnel.local, then retry the Edge connection. Details: connect: connection refused";

    expect(localizeRuntimeMessage(message, "zh-CN")).toBe(
      "目标服务 127.0.0.1:22 不可用。请启动该服务或修正 tunnel.local，然后重新发起 Edge 连接。详情：connect: connection refused",
    );
  });

  it("localizes relay peer connection activity while retaining IPv4 and IPv6 addresses", () => {
    expect(localizeRuntimeMessage("Edge connected from 198.51.100.10", "zh-CN")).toBe(
      "边缘端已从 198.51.100.10 接入",
    );
    expect(localizeRuntimeMessage("Target disconnected from 2001:db8::20", "zh-CN")).toBe(
      "目标端 2001:db8::20 已断开连接",
    );
  });
});
