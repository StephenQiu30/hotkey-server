import Image from "next/image";
import Link from "next/link";
import { ArrowRight, Check, RadioTower, Sparkles } from "lucide-react";
import { PublicHeader } from "@/components/PublicHeader";
import { BrandLogo } from "@/components/BrandLogo";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";

const supportedSources = [
  "财新",
  "36氪",
  "澎湃",
  "人民网",
  "第一财经",
  "界面新闻",
  "虎嗅",
];

const evidence = [
  { source: "36氪：多家厂商发布端侧 Agent 计划", time: "2 小时前" },
  { source: "极客公园：端侧 AI 成为下一阶段关键战场", time: "5 小时前" },
  { source: "官方发布：新一代端侧模型更新", time: "8 小时前" },
];

const workflow = [
  {
    step: "01",
    title: "持续监测",
    body: "连接可信公开来源，让重要信号在产生时就进入同一条情报链路。",
  },
  {
    step: "02",
    title: "AI 识别与判断",
    body: "合并重复信息，解释变化原因，并保留每一个判断对应的来源证据。",
  },
  {
    step: "03",
    title: "形成共识与行动",
    body: "把事件、证据和建议整理成可阅读、可订阅、可回溯的团队简报。",
  },
];

export default function HomePage() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <PublicHeader />

      <main>
        <section className="mx-auto grid min-h-[calc(100svh-72px)] max-w-[1440px] grid-rows-[1fr_auto] px-5 sm:px-8">
          <div className="grid items-center gap-12 py-16 lg:translate-y-7 lg:grid-cols-[1.05fr_1.25fr_.7fr] lg:gap-8 lg:py-0">
            <div className="max-w-[520px] lg:pl-9">
              <h1
                aria-label="重要变化，先形成判断"
                className="text-[clamp(3rem,5.3vw,5.25rem)] font-normal leading-[0.98] tracking-[-0.065em]"
              >
                重要变化，
                <br />
                先形成判断
              </h1>
              <div className="mt-8 flex flex-col gap-3 sm:flex-row">
                <Button asChild size="lg" className="rounded-lg px-6">
                  <Link href="/register">开始使用</Link>
                </Button>
                <Button asChild size="lg" variant="outline" className="rounded-lg px-6">
                  <a href="#briefing">查看今日简报</a>
                </Button>
              </div>
            </div>

            <div className="relative min-h-[340px] lg:min-h-[520px] lg:-translate-x-4">
              <Image
                src="/images/hotkey-signal-radar.png"
                alt="由多层信号轨道组成的 HotKey 雷达"
                fill
                priority
                sizes="(max-width: 1024px) 100vw, 42vw"
                className="object-contain dark:invert lg:scale-[1.2]"
              />
            </div>

            <div className="space-y-6 text-lg leading-tight lg:pl-8">
              <p>持续监测公开信号</p>
              <p>识别真正重要的变化</p>
              <p>让判断建立在证据之上</p>
            </div>
          </div>

          <div id="sources" className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-7">
            {supportedSources.map((source) => (
              <div
                key={source}
                className="flex min-h-20 items-center justify-center text-base font-semibold tracking-[-0.03em] sm:min-h-24 lg:min-h-[140px] lg:items-start lg:pt-5"
              >
                {source}
              </div>
            ))}
          </div>
        </section>

        <section id="product">
          <div className="mx-auto max-w-[1440px] px-5 py-28 sm:px-8 lg:py-44">
            <div className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_340px] lg:items-end">
              <h2 className="max-w-5xl text-[clamp(2.8rem,5.4vw,5.5rem)] font-normal leading-[0.98] tracking-[-0.065em]">
                把公开信号，变成可以核验的团队判断
              </h2>
              <p className="max-w-sm text-lg leading-7 text-muted-foreground lg:pb-2">
                从发现变化到阅读证据，HotKey 把情报判断放在同一个可追溯工作流里。
              </p>
            </div>

            <Card
              id="briefing"
              role="region"
              aria-label="今日情报简报"
              className="mt-16 scroll-mt-24 lg:mt-24"
            >
              <CardHeader className="flex flex-col gap-3 space-y-0 px-0 py-5 sm:flex-row sm:items-center">
                <div className="flex items-baseline gap-4">
                  <h3 className="text-base font-semibold">今日情报简报</h3>
                  <span className="mono text-xs text-muted-foreground">2026-08-06</span>
                </div>
                <div className="flex items-center gap-2 sm:ml-auto">
                  <Badge variant="outline" className="gap-1.5">
                    <Check className="h-3 w-3 text-success" />
                    值得关注
                  </Badge>
                  <span className="text-xs text-muted-foreground">更新于 14:20</span>
                </div>
              </CardHeader>

              <CardContent className="grid gap-12 p-0 lg:grid-cols-[1.2fr_.8fr_.7fr] lg:gap-10">
                <article className="py-6">
                  <span className="mono text-xs text-muted-foreground">01 / SIGNAL</span>
                  <h3 className="mt-5 max-w-2xl text-2xl font-semibold leading-tight sm:text-3xl">
                    AI Agent 进入端侧部署阶段，生态合作加速落地
                  </h3>
                  <p className="mt-4 max-w-2xl text-sm leading-7 text-muted-foreground">
                    多家头部厂商宣布端侧 AI Agent 产品计划，能力正从云端下沉到终端设备，带来算力、隐私与交互体验的系统性变化。
                  </p>
                  <div className="mt-12 flex items-end justify-between">
                    <div>
                      <p className="text-xs text-muted-foreground">热度趋势 · 近 7 天</p>
                      <p className="mt-1 text-xs text-success">连续 4 天上升</p>
                    </div>
                    <p className="mono text-4xl font-medium">
                      92<span className="ml-1 text-xs font-normal text-muted-foreground">/100</span>
                    </p>
                  </div>
                  <Progress
                    aria-label="事件热度 92 分"
                    className="mt-4"
                    value={92}
                  />
                </article>

                <aside className="py-6 lg:px-4">
                  <p className="text-sm font-semibold">证据来源</p>
                  <div className="mt-6 space-y-6">
                    {evidence.map((item) => (
                      <div key={item.source} className="grid gap-2">
                        <p className="text-sm leading-6">{item.source}</p>
                        <span className="text-xs text-muted-foreground">{item.time}</span>
                      </div>
                    ))}
                  </div>
                  <Link
                    href="/dashboard/reports"
                    className="mt-5 inline-flex items-center gap-1.5 text-sm font-medium text-foreground no-underline hover:opacity-55"
                  >
                    查看完整情报 <ArrowRight className="h-4 w-4" />
                  </Link>
                </aside>

                <aside className="self-start rounded-2xl bg-muted/55 p-6 sm:p-8">
                  <Sparkles className="h-5 w-5" />
                  <p className="mt-5 text-sm font-semibold">AI 判断</p>
                  <p className="mt-3 text-sm leading-7 text-muted-foreground">
                    这是从“云上能力”到“终端体验”的关键拐点，建议持续跟踪端侧模型压缩、隐私计算与生态伙伴落地节奏。
                  </p>
                  <div className="mt-8 flex items-center justify-between text-xs">
                    <span className="text-muted-foreground">置信度</span>
                    <span className="mono font-semibold">85%</span>
                  </div>
                </aside>
              </CardContent>
            </Card>
          </div>
        </section>

        <section id="workflow">
          <div className="mx-auto max-w-[1440px] px-5 py-28 sm:px-8 lg:py-40">
            <div className="grid gap-12 lg:grid-cols-[.8fr_1.2fr]">
              <div>
                <RadioTower className="h-6 w-6" />
                <h2 className="mt-6 max-w-xl text-[clamp(2.4rem,4vw,4.5rem)] font-normal leading-[1.02] tracking-[-0.06em]">
                  让情报系统像团队一样持续工作
                </h2>
              </div>
              <div className="space-y-10 lg:pt-2">
                {workflow.map((item) => (
                  <article key={item.step} className="grid gap-5 sm:grid-cols-[64px_180px_1fr] sm:items-start">
                    <span className="mono text-xs text-muted-foreground">{item.step}</span>
                    <h3 className="text-lg font-semibold">{item.title}</h3>
                    <p className="max-w-xl text-sm leading-7 text-muted-foreground">{item.body}</p>
                  </article>
                ))}
              </div>
            </div>
          </div>
        </section>

        <section id="pricing" className="bg-muted/35">
          <div className="mx-auto flex max-w-[1440px] flex-col gap-8 px-5 py-24 sm:px-8 lg:flex-row lg:items-end lg:justify-between lg:py-32">
            <div>
              <p className="text-sm text-muted-foreground">从第一条监控开始</p>
              <h2 className="mt-4 max-w-3xl text-[clamp(2.5rem,4.5vw,5rem)] font-normal leading-[1] tracking-[-0.06em]">
                让下一次判断更早，也更可靠。
              </h2>
            </div>
            <Button asChild size="lg" className="self-start rounded-lg lg:self-auto">
              <Link href="/register">创建账号 <ArrowRight /></Link>
            </Button>
          </div>
        </section>
      </main>

      <footer>
        <div className="mx-auto flex max-w-[1440px] flex-col gap-3 px-5 py-8 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between sm:px-8">
          <BrandLogo className="font-semibold text-foreground" />
          <span>© 2026 HotKey · 让热点判断建立在证据之上</span>
        </div>
      </footer>
    </div>
  );
}
