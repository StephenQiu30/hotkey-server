import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import HomePage from "@/app/page";
import NotFound from "@/app/not-found";
import AuthShell from "@/components/auth/AuthShell";

describe("公开入口页面", () => {
  it("首页清晰表达多源 AI 热点监控定位与核心工作流", () => {
    render(<HomePage />);

    expect(
      screen.getByRole("heading", { name: "全网热点，先一步看见" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "多源热点示例" })).toBeInTheDocument();
    expect(screen.getByText("多源证据状态")).toBeInTheDocument();
    expect(screen.getByText("定义监控目标")).toBeInTheDocument();
    expect(screen.getByText("七源聚合与 AI 分析")).toBeInTheDocument();
    expect(screen.getByText("实时推送与邮件")).toBeInTheDocument();
    expect(screen.getByText("X / Twitter")).toBeInTheDocument();
    expect(screen.getByText("RSS / Atom")).toBeInTheDocument();
    expect(screen.getByText("Hacker News")).toBeInTheDocument();
    expect(screen.getByText("Bilibili")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "切换到暗色模式" })).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/可信来源|置信度|更可靠/);
  });

  it("首页的热点入口指向受保护的微事件页面", () => {
    render(<HomePage />);

    expect(screen.getByRole("link", { name: "查看热点事件" })).toHaveAttribute(
      "href",
      "/dashboard/events",
    );
  });

  it("首页使用中性的高对比设计令牌", () => {
    render(<HomePage />);

    expect(
      screen.getByRole("heading", { name: "把多源信号，变成可追溯的热点事件" }),
    ).toBeInTheDocument();
    expect(screen.getByText("2026-08-06")).toHaveClass("text-muted-foreground");
    expect(screen.getByText("传播速度正在上升")).toHaveClass("text-success");
    expect(screen.getByText("/100")).toHaveClass("text-muted-foreground");
    expect(screen.getAllByText(/小时前$/)).toHaveLength(3);
    expect(screen.getByAltText("由多层信号轨道组成的 HotKey 雷达")).toHaveClass(
      "dark:invert",
    );
    for (const timestamp of screen.getAllByText(/小时前$/)) {
      expect(timestamp).toHaveClass("text-muted-foreground");
    }
  });

  it("认证页的品牌介绍区域使用可识别的辅助地标", () => {
    render(
      <AuthShell title="登录" subtitle="继续使用">
        <span>表单内容</span>
      </AuthShell>,
    );

    expect(
      screen.getByRole("complementary", { name: "HotKey 品牌介绍" }),
    ).toBeInTheDocument();
  });

  it("未找到页面提供中文说明和可恢复的导航入口", () => {
    render(<NotFound />);

    expect(screen.getByRole("heading", { name: "页面不存在" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "返回首页" })).toHaveAttribute("href", "/");
    expect(screen.getByRole("link", { name: "登录工作台" })).toHaveAttribute(
      "href",
      "/login",
    );
  });

  it("未找到页面的编号与说明使用可读的前景色", () => {
    render(<NotFound />);

    expect(screen.getByText("404")).toHaveClass("text-foreground");
    expect(
      screen.getByText("你访问的地址可能已被移动或删除。可以返回首页，或登录后继续使用工作台。"),
    ).toHaveClass("text-muted-foreground");
  });
});
