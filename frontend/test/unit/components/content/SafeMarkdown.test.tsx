import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SafeMarkdown } from "@/components/content/SafeMarkdown";

describe("SafeMarkdown", () => {
  it("supports GFM while ignoring raw HTML", () => {
    const { container } = render(
      <SafeMarkdown
        markdown={[
          "# 事件正文",
          "",
          "| 出处 | 状态 |",
          "| --- | --- |",
          "| 官方 Feed | 完整正文 |",
          "",
          "- [x] 已归档",
          "",
          "<script>window.__hotkeyUnsafe = true</script>",
          "<iframe src=\"https://tracker.example.test\"></iframe>",
        ].join("\n")}
      />,
    );

    expect(screen.getByRole("heading", { name: "事件正文" })).toBeInTheDocument();
    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.getByRole("checkbox")).toBeChecked();
    expect(container.querySelector("script")).toBeNull();
    expect(container.querySelector("iframe")).toBeNull();
  });

  it("allows only absolute HTTP(S) Markdown links", () => {
    render(
      <SafeMarkdown
        markdown={[
          "[HTTP](http://example.test/source)",
          "[HTTPS](https://example.test/source)",
          "[JavaScript](javascript:alert(1))",
          "[Data](data:text/html,unsafe)",
          "[File](file:///tmp/evidence.md)",
          "[Blob](blob:https://example.test/id)",
          "[Relative](/dashboard/events)",
        ].join("\n\n")}
      />,
    );

    expect(screen.getByRole("link", { name: "HTTP" })).toHaveAttribute(
      "rel",
      "noopener noreferrer",
    );
    expect(screen.getByRole("link", { name: "HTTPS" })).toHaveAttribute(
      "href",
      "https://example.test/source",
    );
    for (const name of ["JavaScript", "Data", "File", "Blob", "Relative"]) {
      expect(screen.queryByRole("link", { name })).not.toBeInTheDocument();
      expect(screen.getByText(name)).toBeInTheDocument();
    }
  });

  it("blocks remote image loading and exposes only an explicit safe link", () => {
    const { container } = render(
      <SafeMarkdown
        markdown={[
          "![现场图片](https://images.example.test/evidence.png)",
          "",
          "![无效图片](data:image/png;base64,unsafe)",
        ].join("\n")}
      />,
    );

    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("[src]")).toBeNull();
    expect(screen.getAllByText("远程图片已阻止")).toHaveLength(2);
    expect(
      screen.getByRole("link", { name: "在新标签页打开图片：现场图片" }),
    ).toHaveAttribute("href", "https://images.example.test/evidence.png");
    expect(
      screen.queryByRole("link", { name: "在新标签页打开图片：无效图片" }),
    ).not.toBeInTheDocument();
  });

  it("caps unusually long input and clearly reports truncation", () => {
    render(
      <SafeMarkdown
        markdown={`可见开头\n\n${"x".repeat(100)}\n\n不可见结尾`}
        maxLength={32}
      />,
    );

    expect(screen.getByText(/可见开头/)).toBeInTheDocument();
    expect(screen.queryByText("不可见结尾")).not.toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent(
      "正文过长，仅展示前 32 个字符",
    );
  });
});
