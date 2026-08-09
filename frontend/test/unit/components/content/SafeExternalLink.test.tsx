import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SafeExternalLink } from "@/components/content/SafeExternalLink";

describe("SafeExternalLink", () => {
  it.each([
    ["http://example.test/path", "http://example.test/path"],
    ["https://example.test/path?q=hotkey", "https://example.test/path?q=hotkey"],
  ])("allows an absolute HTTP(S) URL %s", (href, expected) => {
    render(<SafeExternalLink href={href}>打开来源</SafeExternalLink>);

    const link = screen.getByRole("link", { name: "打开来源" });
    expect(link).toHaveAttribute("href", expected);
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });

  it.each([
    "javascript:alert(1)",
    "data:text/html,unsafe",
    "file:///tmp/evidence.md",
    "blob:https://example.test/unsafe",
    "/relative/path",
    "//example.test/protocol-relative",
    "mailto:editor@example.test",
    "not a URL",
  ])("renders rejected URL %s as non-interactive text", (href) => {
    render(<SafeExternalLink href={href}>不可打开</SafeExternalLink>);

    expect(screen.queryByRole("link", { name: "不可打开" })).not.toBeInTheDocument();
    expect(screen.getByText("不可打开")).toBeInTheDocument();
  });
});
