import { describe, expect, it } from "vitest";
import { localizeRuntimeMessage, localizeValidationError } from "./i18n";

describe("runtime message localization", () => {
  it("localizes retry timing and token rejection guidance", () => {
    const message = "Route unavailable; retrying in 1.2s. The relay rejected this token (HTTP 401). Copy a valid token from the relay console and paste the exact value here.";

    expect(localizeRuntimeMessage(message, "zh-CN")).toBe(
      "路由不可用；将在 1.2 秒后重试。Relay 拒绝了此 Token（HTTP 401）。请从 Relay 控制台复制有效 Token 并原样粘贴。",
    );
    expect(localizeRuntimeMessage(message, "en")).toBe(message);
  });

  it("localizes duplicate-target and revoked-token rejections", () => {
    expect(localizeRuntimeMessage(
      "Another Target is already connected with this token. Each token accepts exactly one Target; stop the other Target or use a different token.",
      "zh-CN",
    )).toContain("每个 Token 只允许一个 Target");
    expect(localizeRuntimeMessage(
      "A second Target from 203.0.113.9 was rejected for token tok-alpha; each token accepts exactly one Target",
      "zh-CN",
    )).toBe("来自 203.0.113.9 的第二个 Target 已被拒绝（Token tok-alpha）；每个 Token 只允许一个 Target");
    expect(localizeRuntimeMessage(
      "Token tok-alpha was disabled or removed; 3 connected client(s) were disconnected",
      "zh-CN",
    )).toBe("Token tok-alpha 已停用或删除；3 个已连接客户端被断开");
    expect(localizeRuntimeMessage(
      "Token tok-alpha was rotated; the previous value stays valid until 2026-08-20T00:00:00Z",
      "zh-CN",
    )).toBe("Token tok-alpha 已轮换；旧值有效至 2026-08-20T00:00:00Z");
  });

  it("localizes catalog and mapping lifecycle messages", () => {
    expect(localizeRuntimeMessage("Target published 2 service(s)", "zh-CN")).toBe("Target 发布了 2 个服务");
    expect(localizeRuntimeMessage("Applied 3 local mapping(s)", "zh-CN")).toBe("已应用 3 条本地映射");
    expect(localizeRuntimeMessage("Published 4 service(s) to connected edges", "zh-CN")).toBe("已向连接中的 Edge 发布 4 个服务");
    expect(localizeRuntimeMessage(
      "Encrypted route is down; local mapping listeners are closed until it recovers",
      "zh-CN",
    )).toContain("路由恢复后自动重开");
    expect(localizeRuntimeMessage(
      'Local mapping for service "web" recovered and is listening on 127.0.0.1:28080',
      "zh-CN",
    )).toBe("服务“web”的本地映射已恢复，正在 127.0.0.1:28080 监听");
  });

  it("localizes service dial failures while retaining name, address, and detail", () => {
    const message = 'Service "web" at 10.188.200.16:30927 is unavailable. Start the backend service or correct its address in the service list, then retry from the Edge. Details: connect: connection refused';

    expect(localizeRuntimeMessage(message, "zh-CN")).toBe(
      "服务“web”（10.188.200.16:30927）不可用。请启动该后端服务或在服务列表中修正地址，然后从 Edge 重试。详情：connect: connection refused",
    );
  });

  it("localizes listener conflicts with service context", () => {
    const message = 'The local listener for service "web" could not start on 127.0.0.1:28080. Stop the process using that address or pick another port; MoleX keeps retrying automatically. Details: bind: address already in use';

    const localized = localizeRuntimeMessage(message, "zh-CN");
    expect(localized).toContain("服务“web”");
    expect(localized).toContain("127.0.0.1:28080");
    expect(localized).toContain("停止占用该地址的进程");
  });

  it("localizes relay peer connection activity while retaining IPv4 and IPv6 addresses", () => {
    expect(localizeRuntimeMessage("Edge connected from 198.51.100.10", "zh-CN")).toBe(
      "边缘端已从 198.51.100.10 接入",
    );
    expect(localizeRuntimeMessage("Target disconnected from 2001:db8::20", "zh-CN")).toBe(
      "目标端 2001:db8::20 已断开连接",
    );
  });

  it("localizes target readiness and degradation with nested guidance", () => {
    expect(localizeRuntimeMessage(
      "Target is ready: 3 live session(s); one hot-standby session is kept for the next edge",
      "zh-CN",
    )).toBe("Target 已就绪：3 条活跃会话；始终保留一条热备会话等待下一个 Edge");
    expect(localizeRuntimeMessage(
      "Target session pool is degraded: 2 session(s) still connected. Route unavailable; retrying in 1s. The relay or peer closed the encrypted route. Retry the local connection after the route is ready.",
      "zh-CN",
    )).toContain("Target 会话池运行降级：仍有 2 条会话连接。路由不可用；将在 1 秒后重试");
  });
});

describe("validation error localization", () => {
  it("localizes indexed token, service, and mapping errors", () => {
    expect(localizeValidationError("tokens[0].token must contain at least 16 characters", "zh-CN")).toBe(
      "第 1 条Token：Token 至少需要 16 个字符",
    );
    expect(localizeValidationError("services[1].address: must use host:port form", "zh-CN")).toBe(
      "第 2 条服务：地址必须使用 host:port 格式",
    );
    expect(localizeValidationError("mappings[2].port must be between 1 and 65535", "zh-CN")).toBe(
      "第 3 条映射：本地端口必须在 1 到 65535 之间",
    );
  });

  it("localizes the legacy configuration migration error", () => {
    const message = "legacy v1 configuration: this file uses the MoleX v1 layout (mode \"punch\" with role, secret, and tunnel). MoleX v2 uses mode \"relay\", \"target\", or \"edge\" with tokens, services, and mappings. Recreate it with `molex config init --mode <relay|target|edge>` and see the v2 migration notes in the README";
    const localized = localizeValidationError(message, "zh-CN");
    expect(localized).toContain("v1 旧版配置");
    expect(localized).toContain("molex config init");
  });

  it("keeps combined messages split and localized", () => {
    const localized = localizeValidationError(
      "token must contain at least 16 characters; mappings[0].port must be between 1 and 65535",
      "zh-CN",
    );
    expect(localized).toBe("Token 至少需要 16 个字符；第 1 条映射：本地端口必须在 1 到 65535 之间");
  });
});
