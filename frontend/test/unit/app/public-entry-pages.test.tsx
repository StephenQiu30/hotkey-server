import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import HomePage from "@/app/page";
import NotFound from "@/app/not-found";
import AuthShell from "@/components/auth/AuthShell";

describe("公开入口页面", () => {
  it("首页清晰表达 AI 热点监控定位与核心工作流", () => {
    render(<HomePage />);

    expect(
      screen.getByRole("heading", { name: "重要变化，先形成判断" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "今日情报简报" })).toBeInTheDocument();
    expect(screen.getByText("证据来源")).toBeInTheDocument();
    expect(screen.getByText("持续监测")).toBeInTheDocument();
    expect(screen.getByText("AI 识别与判断")).toBeInTheDocument();
    expect(screen.getByText("形成共识与行动")).toBeInTheDocument();
    expect(screen.getByText("财新")).toBeInTheDocument();
    expect(screen.getByText("36氪")).toBeInTheDocument();
    expect(screen.getByText("虎嗅")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "切换到暗色模式" })).toBeInTheDocument();
  });

  it("首页的完整情报入口指向受保护的报告页面", () => {
    render(<HomePage />);

    expect(screen.getByRole("link", { name: "查看完整情报" })).toHaveAttribute(
      "href",
      "/dashboard/reports",
    );
  });

  it("首页使用中性的高对比设计令牌", () => {
    render(<HomePage />);

    expect(
      screen.getByRole("heading", { name: "把公开信号，变成可以核验的团队判断" }),
    ).toBeInTheDocument();
    expect(screen.getByText("2026-08-06")).toHaveClass("text-muted-foreground");
    expect(screen.getByText("连续 4 天上升")).toHaveClass("text-success");
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
