import Link from "next/link";
import {
  ArrowRight,
  CheckCircle2,
  Clock3,
  Sparkles,
} from "lucide-react";
import { BrandLogo } from "@/components/BrandLogo";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
} from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";

const evidence = [
  { source: "36氪：多家厂商发布端侧 Agent 计划", time: "2 小时前" },
  { source: "极客公园：端侧 AI 成为下一阶段关键战场", time: "5 小时前" },
  { source: "官方发布：新一代端侧模型更新", time: "8 小时前" },
];

const workflow = [
  {
    step: "01",
    title: "持续监测",
    body: "全网信号实时采集与去噪，覆盖新闻、社媒、论坛、研报与官方渠道。",
  },
  {
    step: "02",
    title: "AI 识别与判断",
    body: "多维模型评估事件影响力与可信度，提炼关键变化与背后逻辑。",
  },
  {
    step: "03",
    title: "形成共识与行动",
    body: "输出可验证的证据与建议，帮助团队快速对齐，推动决策落地。",
  },
];

export default function HomePage() {
  return (
    <div className="min-h-screen bg-white text-foreground">
      <header className="relative z-40 bg-white">
        <div className="mx-auto flex h-[72px] max-w-[1340px] items-center px-5 lg:px-8">
          <Link href="/" className="text-lg font-bold text-slate-950 no-underline">
            <BrandLogo markClassName="h-5 w-5" />
          </Link>
          <span className="ml-3 hidden rounded-full bg-blue-50 px-2.5 py-1 text-[10px] font-bold uppercase tracking-[.13em] text-blue-700 sm:inline-flex">
            Intelligence
          </span>

          <nav aria-label="首页导航" className="ml-auto flex items-center gap-1 sm:gap-2">
            <a href="#briefing" className="hidden px-3 py-2 text-sm text-muted-foreground no-underline hover:text-blue-700 md:block">产品</a>
            <a href="#workflow" className="hidden px-3 py-2 text-sm text-muted-foreground no-underline hover:text-blue-700 md:block">解决方案</a>
            <a href="#evidence" className="hidden px-3 py-2 text-sm text-muted-foreground no-underline hover:text-blue-700 lg:block">资源</a>
            <Button asChild variant="ghost" size="sm"><Link href="/login">登录</Link></Button>
            <Button asChild size="sm"><Link href="/register">开始使用</Link></Button>
          </nav>
        </div>
      </header>

      <main>
        <section className="bg-[#f8fbff] px-5 pb-9 pt-10 lg:px-8">
          <div className="mx-auto max-w-[1160px] text-center">
            <Badge variant="secondary" className="gap-2 border-0 bg-white px-3 py-2 font-medium text-blue-700 hover:bg-white">
              <Clock3 className="h-3.5 w-3.5" />
              <span>每天早上 07:30</span>
              <span className="text-slate-300">·</span>
              <span className="text-muted-foreground">由 AI 为您生成</span>
            </Badge>
            <h1
              className="mx-auto mt-6 max-w-4xl text-4xl font-semibold leading-[1.15] tracking-[-.035em] text-[#10213f] sm:text-5xl lg:text-[58px]"
              style={{ fontFamily: '"Songti SC", STSong, "Noto Serif CJK SC", serif' }}
            >
              重要变化，第一时间形成共识。
            </h1>
            <p className="mx-auto mt-4 max-w-2xl text-sm leading-7 text-slate-600 sm:text-base">
              从海量噪声中，锁定真正重要的变化，给团队清晰可行动的判断。
            </p>
          </div>

          <Card
            id="briefing"
            role="region"
            aria-label="今日情报简报"
            className="mx-auto mt-10 max-w-[1200px] overflow-hidden rounded-xl border-blue-100/80 shadow-sm"
          >
            <CardHeader className="flex flex-col gap-3 space-y-0 px-5 py-6 sm:flex-row sm:items-center sm:px-8">
              <div className="flex items-baseline gap-4">
                <h2 className="text-xl font-semibold tracking-[-.02em] text-[#10213f]">今日情报简报</h2>
                <span className="mono text-xs text-slate-600">2026-08-04</span>
              </div>
              <div className="sm:ml-auto flex items-center gap-4 text-xs text-slate-600">
                <span>基于全网信号实时分析</span>
                <Badge variant="secondary" className="gap-1.5 border-0 bg-emerald-50 text-emerald-700 hover:bg-emerald-50">
                  <CheckCircle2 className="h-3 w-3" />值得关注
                </Badge>
              </div>
            </CardHeader>

            <Separator className="bg-blue-50" />

            <CardContent className="grid p-0 lg:grid-cols-[minmax(0,1.6fr)_minmax(320px,.9fr)]">
              <article className="px-5 py-9 sm:px-8">
                <span className="mono text-xs font-semibold text-blue-600">01</span>
                <h3 className="mt-3 text-xl font-semibold leading-snug tracking-[-.02em] text-[#10213f] sm:text-2xl">
                  AI Agent 进入端侧部署阶段，生态合作加速落地
                </h3>
                <p className="mt-3 max-w-3xl text-sm leading-6 text-slate-600">
                  多家头部厂商在 8 月宣布端侧 AI Agent 产品计划与合作，意味着 Agent 能力正从云端下沉到终端设备，将带来算力、模型压缩、隐私安全与交互体验的系统性升级。
                </p>

                <div className="mt-7">
                  <div className="flex items-end justify-between gap-4">
                    <div>
                      <p className="text-xs text-slate-600">热度趋势（近 7 天）</p>
                      <p className="mt-1 text-[11px] text-emerald-700">连续 4 天上升</p>
                    </div>
                    <p className="mono text-2xl font-semibold text-blue-700">92<span className="ml-1 text-xs font-normal text-slate-600">/100</span></p>
                  </div>
                  <progress aria-label="事件热度 92 分" className="briefing-progress mt-3 w-full" value={92} max={100}>92%</progress>
                </div>
              </article>

              <aside id="evidence" className="bg-[#fbfdff] px-5 py-9 sm:px-8 lg:border-l lg:border-blue-50">
                <div>
                  <p className="text-sm font-semibold text-blue-700">为什么重要</p>
                  <p className="mt-2 text-sm leading-6 text-slate-600">
                    端侧部署将显著降低延迟与成本，扩大 AI 在消费电子、汽车、工业等场景的渗透，并重塑应用生态与商业模式。
                  </p>
                </div>
                <div className="mt-6">
                  <p className="text-sm font-semibold text-blue-700">证据来源</p>
                  <div className="mt-3 space-y-3">
                    {evidence.map((item) => (
                      <div key={item.source} className="flex items-start gap-2.5 text-xs leading-5 text-[#536782]">
                        <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-teal-600" />
                        <span className="min-w-0 flex-1">{item.source}</span>
                        <span className="shrink-0 text-[10px] text-slate-600">{item.time}</span>
                      </div>
                    ))}
                  </div>
                  <Link href="/dashboard/reports" className="mt-5 inline-flex items-center gap-1 text-xs font-semibold text-blue-700 no-underline hover:text-blue-800">
                    查看完整情报 <ArrowRight className="h-3.5 w-3.5" />
                  </Link>
                </div>
              </aside>
            </CardContent>

            <CardFooter className="flex flex-col items-start gap-3 bg-blue-50/70 px-5 py-5 text-sm sm:flex-row sm:items-center sm:px-8">
              <span className="inline-flex shrink-0 items-center gap-2 font-semibold text-blue-700"><Sparkles className="h-4 w-4" />AI 判断</span>
              <p className="text-xs leading-5 text-[#5d7290] sm:ml-3">
                这是一次从“云上能力”到“终端体验”的拐点事件，建议重点关注端侧模型压缩技术、隐私计算方案，以及生态伙伴的落地节奏。
              </p>
              <span className="mono shrink-0 text-xs text-slate-600 sm:ml-auto">置信度 <strong className="text-blue-700">85%</strong></span>
            </CardFooter>
          </Card>
        </section>

        <section id="workflow" className="bg-white px-5 pb-16 pt-9 lg:px-8">
          <div className="mx-auto max-w-[1340px]">
            <div className="text-center">
              <p className="text-[11px] font-bold uppercase tracking-[.22em] text-blue-600">How HotKey works</p>
              <h2
                className="mt-3 text-2xl font-semibold tracking-[-.025em] text-[#10213f] sm:text-3xl"
                style={{ fontFamily: '"Songti SC", STSong, "Noto Serif CJK SC", serif' }}
              >
                让情报转化为团队的行动力
              </h2>
            </div>

            <div className="mt-6 grid gap-10 md:grid-cols-3 md:gap-12">
              {workflow.map((item) => (
                <article key={item.step} className="flex items-start gap-4">
                  <span className="mono pt-1 text-sm font-semibold text-blue-600">{item.step}</span>
                  <div>
                    <h3 className="text-base font-semibold text-[#10213f]">{item.title}</h3>
                    <p className="mt-2 text-xs leading-5 text-slate-600">{item.body}</p>
                  </div>
                </article>
              ))}
            </div>

            <div className="mt-14 text-center">
              <Button asChild size="lg"><Link href="/register">开始使用 <ArrowRight /></Link></Button>
            </div>
          </div>
        </section>
      </main>

      <footer className="bg-[#f8fbff]">
        <div className="mx-auto flex max-w-[1240px] flex-col gap-3 px-5 py-8 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between lg:px-8">
          <BrandLogo className="font-semibold text-foreground" />
          <span>© 2026 HotKey · 让热点判断建立在证据之上</span>
        </div>
      </footer>
    </div>
  );
}
