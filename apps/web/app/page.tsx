import Link from "next/link";
import { ArrowRight, BarChart3, BellRing, FileText, LockKeyhole, Search, ServerCog, Sparkles } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

const capabilities = [
  { title: "AI 热点检测", description: "围绕关键词自动抓取多源内容，分析真实性、相关性和重要性。", icon: Sparkles },
  { title: "即时全网搜索", description: "通过统一搜索页快速验证主题热度，结果不入库、不触发通知。", icon: Search },
  { title: "日报与周报", description: "基于 active 热点生成 Markdown 报告，可预览、发送和追踪状态。", icon: FileText },
  { title: "私有部署", description: "面向单用户自部署，不需要复杂租户、权限或付费平台。", icon: LockKeyhole },
];

export default function MarketingHomePage() {
  return (
    <main className="min-h-screen bg-background">
      <header className="border-b border-border bg-white/90 backdrop-blur">
        <div className="mx-auto flex max-w-7xl items-center justify-between px-4 py-4 md:px-6">
          <Link
            className="ios-shell-card flex min-h-11 items-center gap-3 rounded-full border border-border/80 px-3 transition hover:bg-muted/60 focus-visible:outline-none"
            href="/"
          >
            <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary text-primary-foreground">
              <Sparkles className="h-5 w-5" />
            </span>
            <span className="font-extrabold">AI Hotspot Radar</span>
          </Link>
          <nav className="hidden items-center gap-2 md:flex">
            <Button asChild variant="ghost">
              <Link href="/pricing">定价</Link>
            </Button>
            <Button asChild>
              <Link href="/app">进入工作台</Link>
            </Button>
          </nav>
        </div>
      </header>

      <section className="mx-auto grid max-w-7xl gap-10 px-4 py-14 md:px-6 lg:grid-cols-[1.05fr_.95fr] lg:py-20">
        <div className="flex flex-col justify-center">
          <Badge className="mb-5" variant="default">单用户私有部署 SaaS</Badge>
          <p className="text-sm font-semibold uppercase tracking-[0.24em] text-primary">AI 热点监控工作流</p>
          <h1 className="ios-reveal mt-4 max-w-4xl text-4xl font-extrabold leading-tight tracking-tight text-slate-950 md:text-6xl">
            AI 热点检测平台，从信息噪声里筛出真正值得看的信号。
          </h1>
          <p className="mt-6 max-w-2xl text-lg leading-8 text-muted-foreground">
            配置关键词，连接多源采集，使用 AI 判断相关性和真实性，再把 active 热点沉淀为通知、搜索结果、日报与周报。
          </p>
          <div className="mt-8 flex flex-col gap-3 sm:flex-row">
            <Button asChild size="lg">
              <Link href="/app">
                打开工作台
                <ArrowRight className="h-4 w-4" />
              </Link>
            </Button>
            <Button asChild size="lg" variant="secondary">
              <Link href="/login">GitHub 登录</Link>
            </Button>
            <Button asChild size="lg" variant="secondary">
              <Link href="/pricing">查看部署方案</Link>
            </Button>
          </div>
        </div>

        <div className="ios-shell-card p-4">
          <p className="mb-4 text-sm font-semibold text-muted-foreground">今日监控概览</p>
          <p className="text-2xl font-extrabold">24 活动 / 7 已过滤</p>
          <div className="mt-4 grid gap-3">
            {["OpenAI agent workflow", "AI video generation benchmark", "Enterprise search copilots"].map((title, index) => (
              <div className="ios-card-muted p-3" key={title}>
                <div className="flex items-center justify-between gap-3">
                  <span className="font-semibold">{title}</span>
                  <span className="rounded-full bg-sky-100 px-2 py-1 text-xs text-sky-700">活动</span>
                </div>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">相关性 {92 - index * 6}，已进入日报候选。</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="mx-auto grid max-w-7xl gap-4 px-4 pb-16 md:grid-cols-2 md:px-6 lg:grid-cols-4">
        {capabilities.map((item) => {
          const Icon = item.icon;
          return (
            <Card className="ios-reveal" key={item.title}>
              <CardHeader>
                <div className="mb-3 flex h-11 w-11 items-center justify-center rounded-lg bg-blue-50 text-primary">
                  <Icon className="h-5 w-5" />
                </div>
                <CardTitle>{item.title}</CardTitle>
                <CardDescription>{item.description}</CardDescription>
              </CardHeader>
            </Card>
          );
        })}
      </section>

      <section className="border-t border-border bg-white/80">
        <div className="mx-auto grid max-w-7xl gap-6 px-4 py-12 md:grid-cols-3 md:px-6">
          <Card className="ios-shell-card">
            <CardHeader>
              <ServerCog className="h-5 w-5 text-primary" />
              <CardTitle>后端闭环已就绪</CardTitle>
              <CardDescription>检测、搜索、通知、报告统一基于现有 FastAPI API。</CardDescription>
            </CardHeader>
          </Card>
          <Card className="ios-shell-card">
            <CardHeader>
              <BellRing className="h-5 w-5 text-primary" />
              <CardTitle>失败可追踪</CardTitle>
              <CardDescription>SMTP 未配置时记录 skipped，来源失败不会中断整体任务。</CardDescription>
            </CardHeader>
          </Card>
          <Card className="ios-shell-card">
            <CardHeader>
              <FileText className="h-5 w-5 text-primary" />
              <CardTitle>报告唯一入口</CardTitle>
              <CardDescription>日报与周报统一收敛到 reports API，前端不使用旧日报接口。</CardDescription>
            </CardHeader>
          </Card>
        </div>
      </section>
    </main>
  );
}
