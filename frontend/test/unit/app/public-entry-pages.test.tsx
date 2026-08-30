import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import HomePage from "@/app/page";
import NotFound from "@/app/not-found";
import AuthShell from "@/components/auth/AuthShell";

describe("公开入口页面", () => {
  it("首页清晰表达多源 AI 热点监控定位与核心工作流", () => {
    render(<HomePage />);

    expect(
      screen.getByRole("heading", { name: "在热点成为热点前，看见它的升温轨迹" })
    ).toBeInTheDocument();
    expect(
      screen.getByText(/把零散内容聚合为可解释的事件、证据链与提醒/)
    ).toBeInTheDocument();
    expect(
      screen.getByRole("img", { name: "多来源信号汇聚成热点事件的动态轨迹" })
    ).toHaveAttribute("data-animation", "gsap-three");
    expect(
      screen.getByRole("region", { name: "多源热点示例" })
    ).toBeInTheDocument();
    expect(screen.getByText("多源证据状态")).toBeInTheDocument();
    expect(screen.getByText("定义监控目标")).toBeInTheDocument();
    expect(screen.getByText("多源聚合与 AI 分析")).toBeInTheDocument();
    expect(screen.getByText("实时推送与邮件")).toBeInTheDocument();
    const sourceList = screen.getByRole("list", { name: "当前支持的来源" });
    expect(within(sourceList).getByText("X / Twitter")).toBeInTheDocument();
    expect(within(sourceList).getByText("RSS / Atom")).toBeInTheDocument();
    expect(within(sourceList).getByText("Hacker News")).toBeInTheDocument();
    expect(within(sourceList).getByText("Bilibili")).toBeInTheDocument();
    expect(within(sourceList).getByText("微博")).toBeInTheDocument();
    expect(within(sourceList).getByText("搜狗授权搜索")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /切换到.*模式/ })
    ).not.toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/可信来源|置信度|更可靠/);
  });

  it("首页的事件入口指向受保护的事件雷达", () => {
    render(<HomePage />);

    expect(
      screen.getByRole("region", { name: "多源热点示例" })
    ).not.toHaveAttribute("data-slot", "card");
    expect(screen.getByRole("link", { name: "跳到主要内容" })).toHaveAttribute(
      "href",
      "#main-content"
    );
    expect(screen.getByRole("main")).toHaveAttribute("id", "main-content");
    expect(screen.getByRole("link", { name: "查看事件雷达" })).toHaveAttribute(
      "href",
      "/dashboard/events"
    );
  });

  it("首页目录顺序与顶层滚动章节保持一致", () => {
    render(<HomePage />);

    const navigation = screen.getByRole("navigation", { name: "首页导航" });
    expect(
      within(navigation)
        .getAllByRole("link")
        .slice(0, 4)
        .map((link) => link.getAttribute("href"))
    ).toEqual(["#product", "#workflow", "#sources", "#start"]);
    expect(
      within(navigation).getByRole("link", { name: "来源与边界" })
    ).toHaveAttribute("href", "#sources");
    expect(
      within(navigation).getByRole("link", { name: "登录" })
    ).toHaveAttribute("href", "/login");
    expect(
      within(navigation).getByRole("link", { name: "创建监控" })
    ).toHaveAttribute("href", "/register");

    const main = screen.getByRole("main");
    expect(
      Array.from(main.children)
        .filter((element) => element.id)
        .map((element) => element.id)
    ).toEqual(["product", "workflow", "sources", "start"]);

    fireEvent.click(screen.getByRole("button", { name: "打开首页导航" }));
    expect(
      screen.getByLabelText("关闭首页导航")
    ).toHaveAttribute("aria-expanded", "true");
    const mobileNavigation = screen.getByRole("navigation", {
      name: "移动端首页导航",
    });
    expect(
      within(mobileNavigation).getByRole("link", { name: "登录" })
    ).toHaveAttribute("href", "/login");
    expect(
      within(mobileNavigation).getByRole("link", { name: "创建监控" })
    ).toHaveAttribute("href", "/register");
  });

  it("首页使用克制的中性情报设计令牌", () => {
    render(<HomePage />);

    expect(
      screen.getByRole("heading", { name: "把多源信号，变成可追溯的热点事件" })
    ).toBeInTheDocument();
    expect(screen.getByText("最近 6 小时")).toHaveClass("text-muted-foreground");
    expect(screen.getByText("传播速度正在上升")).toHaveClass("text-foreground");
    expect(screen.getByText("传播速度正在上升")).not.toHaveClass(
      "text-success",
      "text-warning",
      "text-signal-blue"
    );
    expect(
      within(screen.getByRole("region", { name: "多源热点示例" })).getByText("/100")
    ).toHaveClass("text-muted-foreground");
    expect(screen.getAllByText(/小时前$/)).toHaveLength(3);
    for (const timestamp of screen.getAllByText(/小时前$/)) {
      expect(timestamp).toHaveClass("text-muted-foreground");
    }
  });

  it("认证页沿用首页的中性信号场与无边框表面", () => {
    render(
      <AuthShell title="登录" subtitle="继续使用">
        <span>表单内容</span>
      </AuthShell>
    );

    const brandRegion = screen.getByRole("complementary", {
      name: "HotKey 品牌介绍",
    });
    const authCanvas = brandRegion.parentElement?.parentElement;
    const formCard = screen
      .getByText("表单内容")
      .closest('[data-slot="card"]');
    const formSurface = formCard?.parentElement;

    expect(brandRegion).toBeInTheDocument();
    expect(
      screen.getByRole("img", {
        name: "多来源信号汇聚成热点事件的动态轨迹",
      })
    ).toHaveAttribute("data-animation", "gsap-three");
    expect(authCanvas).toHaveClass("bg-background");
    expect(brandRegion.querySelector(".intelligence-grid")).toBeInTheDocument();
    expect(formSurface).toHaveClass(
      "auth-form-surface",
      "rounded-2xl",
      "bg-card"
    );
    expect(formCard).toHaveClass("rounded-none", "border-0", "bg-transparent");
    expect(screen.getByRole("link", { name: "HotKey 首页" })).toHaveAttribute(
      "href",
      "/"
    );
    expect(screen.getByRole("link", { name: "返回产品首页" })).toHaveAttribute(
      "href",
      "/"
    );
  });

  it("未找到页面提供中文说明和可恢复的导航入口", () => {
    render(<NotFound />);

    expect(
      screen.getByRole("heading", { name: "页面不存在" })
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "返回首页" })).toHaveAttribute(
      "href",
      "/"
    );
    expect(screen.getByRole("link", { name: "登录工作台" })).toHaveAttribute(
      "href",
      "/login"
    );
  });

  it("未找到页面的编号与说明使用可读的前景色", () => {
    render(<NotFound />);

    expect(screen.getByText("404")).toHaveClass("text-foreground");
    expect(
      screen.getByText(
        "你访问的地址可能已被移动或删除。可以返回首页，或登录后继续使用工作台。"
      )
    ).toHaveClass("text-muted-foreground");
  });
});
