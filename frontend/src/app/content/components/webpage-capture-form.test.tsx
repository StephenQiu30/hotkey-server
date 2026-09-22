import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));

import {
  resolvePendingOperation,
  validateWebPageTarget,
  WebPageCaptureForm,
} from "./webpage-capture-form";

describe("webpage capture form", () => {
  it("renders one focused URL action without exposing collector controls", () => {
    const html = renderToStaticMarkup(createElement(WebPageCaptureForm));

    expect(html).toContain("添加网页");
    expect(html).toContain('type="url"');
    expect(html).toContain("创建任务");
    expect(html).not.toContain("Firecrawl");
    expect(html).not.toContain("脚本");
    expect(html).not.toContain("引擎");
  });

  it.each([
    ["", "请输入要采集的网页地址。"],
    ["example.com", "请输入完整的 http:// 或 https:// 地址。"],
    ["file:///tmp/example", "请输入完整的 http:// 或 https:// 地址。"],
    ["https://name:value@example.com", "网页地址不能包含用户名或密码。"],
    ["https://example.com/article", null],
    ["http://example.com", null],
  ])("validates the public URL shape for %s", (target, expected) => {
    expect(validateWebPageTarget(target)).toBe(expected);
  });

  it("reuses the operation ID only for a retry of the same target", () => {
    const createId = vi
      .fn<() => string>()
      .mockReturnValueOnce("operation-1")
      .mockReturnValueOnce("operation-2");

    const first = resolvePendingOperation(
      null,
      "https://example.com/a",
      createId,
    );
    const retry = resolvePendingOperation(
      first,
      "https://example.com/a",
      createId,
    );
    const changed = resolvePendingOperation(
      retry,
      "https://example.com/b",
      createId,
    );

    expect(retry).toBe(first);
    expect(changed.operationId).toBe("operation-2");
    expect(createId).toHaveBeenCalledTimes(2);
  });
});
