import Image from "next/image";
import Link from "next/link";
import { ArrowRight, Check } from "lucide-react";
import { PublicHeader } from "@/components/PublicHeader";
import { BrandLogo } from "@/components/BrandLogo";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";

const supportedSources = [
  "X / Twitter",
  "Hacker News",
  "RSS / Atom",
  "Bilibili",
];

const evidence = [
  { source: "X：官方团队发布端侧模型能力", time: "2 小时前" },
  { source: "Hacker News：开发者集中讨论部署体验", time: "5 小时前" },
  { source: "Bilibili：相关演示开始获得持续传播", time: "8 小时前" },
];

const workflow = [
  {
    step: "01",
    title: "定义监控目标",
    body: "描述需要关注的主体、动作与排除条件，由 AI 生成可审核的查询扩展。",
  },
  {
    step: "02",
    title: "七源聚合与 AI 分析",
    body: "并行采集七类正式来源，完成相关性判断、去重、证据状态与热点聚合。",
  },
  {
    step: "03",
    title: "实时推送与邮件",
    body: "按热度、相关性和时间稳定排序，并用可恢复实时连接与邮件提示升温事件。",
  },
];

export default function HomePage() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <PublicHeader />

      <main id="main-content" tabIndex={-1}>
        <section className="mx-auto flex min-h-[calc(100svh-72px)] max-w-[1440px] items-center px-5 sm:px-8">
          <div className="grid w-full items-center gap-12 py-16 lg:grid-cols-[1.05fr_1.25fr_.7fr] lg:gap-8 lg:py-0">
            <div className="max-w-[520px] lg:pl-9">
              <h1
                aria-label="全网热点，先一步看见"
                className="text-[clamp(3rem,5.3vw,5.25rem)] font-normal leading-[0.98] tracking-[-0.065em]"
              >
                全网热点，
                <br />
                先一步看见
              </h1>
              <div className="mt-8 flex flex-col gap-3 sm:flex-row">
                <Button asChild size="lg" className="rounded-lg px-6">
                  <Link href="/register">开始使用</Link>
                </Button>
                <Button
                  asChild
                  size="lg"
                  variant="outline"
                  className="rounded-lg px-6"
                >
                  <a href="#briefing">查看热点示例</a>
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
              <p>七类正式来源持续聚合</p>
              <p>AI 查询扩展、相关性与可信度分析</p>
              <p>WebSocket 实时推送与邮件通知</p>
            </div>
          </div>
        </section>

        <section id="product" className="scroll-mt-24">
          <div className="mx-auto max-w-[1440px] px-5 pt-28 sm:px-8 lg:pt-36">
            <div className="grid gap-8 lg:grid-cols-[minmax(0,1fr)_400px] lg:items-end lg:gap-16">
              <h2 className="max-w-5xl text-[clamp(2.8rem,5.4vw,5.5rem)] font-normal leading-[0.98] tracking-[-0.065em]">
                把多源信号，变成可追溯的热点事件
              </h2>
              <p className="max-w-sm text-lg leading-7 text-muted-foreground lg:pb-2">
                从多源发现、相关性和证据状态到 Heat v2 排序，HotKey
                只维护一条可追溯的热点链路。
              </p>
            </div>

            <section
              id="briefing"
              aria-label="多源热点示例"
              className="mt-16 scroll-mt-24 border-t border-border lg:mt-20"
            >
              <header className="flex flex-col gap-3 py-6 sm:flex-row sm:items-center">
                <div className="flex items-baseline gap-4">
                  <p className="text-sm font-semibold">多源热点示例</p>
                  <time
                    dateTime="2026-08-06"
                    className="mono text-xs text-muted-foreground"
                  >
                    2026-08-06
                  </time>
                </div>
                <div className="flex flex-wrap items-center gap-x-5 gap-y-2 sm:ml-auto sm:justify-end">
                  <span className="inline-flex items-center gap-1.5 text-sm font-medium">
                    <Check
                      aria-hidden="true"
                      className="h-3.5 w-3.5 text-success"
                    />
                    值得关注
                  </span>
                  <span className="text-xs text-muted-foreground">
                    更新于 14:20
                  </span>
                </div>
              </header>

              <div className="grid gap-16 pb-8 pt-10 lg:grid-cols-12 lg:gap-x-20 lg:pb-12 lg:pt-14">
                <article className="min-w-0 lg:col-span-7">
                  <p className="text-sm font-medium text-muted-foreground">
                    微事件 · 端侧 AI
                  </p>
                  <h3 className="mt-4 max-w-3xl text-3xl font-semibold leading-[1.15] sm:text-4xl">
                    AI Agent 进入端侧部署阶段，生态合作加速落地
                  </h3>
                  <p className="mt-5 max-w-2xl text-base leading-8 text-muted-foreground">
                    X、Hacker News 与 Bilibili
                    的相关内容在同一观测窗口内出现，经目标匹配与内容家族去重后形成一个微事件。
                  </p>
                  <div className="mt-12 grid gap-5 sm:grid-cols-[1fr_auto] sm:items-end">
                    <div>
                      <p className="text-sm text-muted-foreground">
                        Heat v2 · 近 6 小时
                      </p>
                      <p className="mt-1 text-sm text-success">
                        传播速度正在上升
                      </p>
                    </div>
                    <p className="mono text-5xl font-medium leading-none">
                      92
                      <span className="ml-1 text-sm font-normal text-muted-foreground">
                        /100
                      </span>
                    </p>
                  </div>
                  <Progress
                    aria-label="事件热度 92 分"
                    className="mt-5 h-1.5"
                    value={92}
                  />
                </article>

                <div className="min-w-0 lg:col-span-5">
                  <aside>
                    <h3 className="text-base font-semibold">多源证据状态</h3>
                    <ul className="mt-7 space-y-6">
                      {evidence.map((item) => (
                        <li
                          key={item.source}
                          className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-baseline sm:gap-6"
                        >
                          <span className="text-sm leading-6">
                            {item.source}
                          </span>
                          <span className="text-xs text-muted-foreground sm:text-right">
                            {item.time}
                          </span>
                        </li>
                      ))}
                    </ul>
                    <Link
                      href="/dashboard/contents"
                      className="mt-8 inline-flex items-center gap-1.5 text-sm font-medium text-foreground no-underline hover:opacity-55"
                    >
                      查看热点雷达 <ArrowRight className="h-4 w-4" />
                    </Link>
                  </aside>

                  <aside className="mt-14 max-w-lg">
                    <h3 className="text-base font-semibold">相关性说明</h3>
                    <p className="mt-3 text-sm leading-7 text-muted-foreground">
                      内容命中已发布监控目标；当前为多来源覆盖。AI
                      辅助判断证据关系，不把模型分数显示为事实真伪概率。
                    </p>
                    <dl className="mt-6 flex items-baseline justify-between gap-6 text-xs">
                      <dt className="text-muted-foreground">用途</dt>
                      <dd className="mono font-semibold">人工复核</dd>
                    </dl>
                  </aside>
                </div>
              </div>
            </section>
          </div>
        </section>

        <section id="workflow" className="scroll-mt-24">
          <div className="mx-auto max-w-[1440px] px-5 pt-16 sm:px-8">
            <div className="grid gap-14 lg:grid-cols-[.8fr_1.2fr] lg:gap-20">
              <div>
                <h2 className="max-w-xl text-[clamp(2.4rem,4vw,4.5rem)] font-normal leading-[1.02] tracking-[-0.06em]">
                  一条链路，持续找到正在升温的多源事件
                </h2>
              </div>
              <div className="space-y-12 lg:pt-2">
                {workflow.map((item) => (
                  <article
                    key={item.step}
                    className="grid gap-5 sm:grid-cols-[64px_180px_1fr] sm:items-start"
                  >
                    <span className="mono text-xs text-muted-foreground">
                      {item.step}
                    </span>
                    <h3 className="text-lg font-semibold">{item.title}</h3>
                    <p className="max-w-xl text-sm leading-7 text-muted-foreground">
                      {item.body}
                    </p>
                  </article>
                ))}
              </div>
            </div>
          </div>
        </section>

        <section
          id="sources"
          aria-labelledby="sources-heading"
          className="scroll-mt-24"
        >
          <div className="mx-auto grid max-w-[1440px] gap-12 px-5 py-20 sm:px-8 lg:grid-cols-[.8fr_1.2fr] lg:items-start lg:gap-20 lg:py-24">
            <div>
              <h2
                id="sources-heading"
                className="max-w-lg text-3xl font-normal leading-tight sm:text-4xl"
              >
                正式来源，持续聚合
              </h2>
              <p className="mt-4 max-w-md text-base leading-7 text-muted-foreground">
                来源连接遵守官方或授权边界，热点事件始终保留可回溯证据。
              </p>
            </div>
            <ul
              aria-label="当前支持的来源"
              className="grid grid-cols-2 gap-x-8 gap-y-10 sm:grid-cols-4 lg:pt-2"
            >
              {supportedSources.map((source) => (
                <li
                  key={source}
                  className="text-base font-semibold tracking-[-0.03em]"
                >
                  {source}
                </li>
              ))}
            </ul>
          </div>
        </section>

        <section id="pricing" className="scroll-mt-24 bg-muted/35">
          <div className="mx-auto flex max-w-[1440px] flex-col gap-8 px-5 py-24 sm:px-8 lg:flex-row lg:items-end lg:justify-between lg:py-32">
            <div>
              <p className="text-sm text-muted-foreground">从第一条监控开始</p>
              <h2 className="mt-4 max-w-3xl text-[clamp(2.5rem,4.5vw,5rem)] font-normal leading-[1] tracking-[-0.06em]">
                让下一次判断更早，也更可回溯。
              </h2>
            </div>
            <Button
              asChild
              size="lg"
              className="self-start rounded-lg lg:self-auto"
            >
              <Link href="/register">
                创建账号 <ArrowRight />
              </Link>
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
