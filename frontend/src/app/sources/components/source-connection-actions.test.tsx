import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({ useRouter: () => ({ replace: vi.fn() }) }));

import { SourceConnectionActions } from "./source-connection-actions";

const platform: HotKeyAPI.SourcePlatformView = {
  source_key: "x",
  display_name: "X",
  rollout_role: "required",
  status: "restricted",
  connection_version: null,
  connection_id: null,
  connection_status: null,
  has_credentials: false,
  credential_configured: false,
  credential_update_available: false,
  capabilities: [],
};

function render(value = platform) {
  return renderToStaticMarkup(
    createElement(SourceConnectionActions, {
      platform: value,
      onChanged: vi.fn(),
    }),
  );
}

describe("source connection controls", () => {
  it("does not offer a secret input or enable absent server credentials", () => {
    const html = render();
    expect(html).toContain("请维护者先配置服务端凭据");
    expect(html).toContain("disabled");
    expect(html).not.toContain("<input");
  });
  it("keeps disabling available even when credentials have been removed", () => {
    const html = render({
      ...platform,
      connection_version: 1,
      connection_status: "active",
    });
    expect(html).toContain("停用连接");
  });
  it("offers replacement only when server credentials have changed", () => {
    const html = render({
      ...platform,
      connection_version: 1,
      connection_status: "active",
      credential_configured: true,
      credential_update_available: true,
    });
    expect(html).toContain("替换连接");
  });
});
