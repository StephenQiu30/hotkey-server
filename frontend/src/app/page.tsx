import Link from "next/link";
import {
  ArrowRight,
  Check,
  FileCheck2,
  Network,
  Radar,
  ScanSearch,
  ShieldCheck,
  Sparkles,
} from "lucide-react";
import { PublicHeader } from "@/components/PublicHeader";
import { BrandLogo } from "@/components/BrandLogo";
import { HomeHero } from "@/components/home/HomeHero";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";

const supportedSources = [
  "X / Twitter",
  "Hacker News",
  "RSS / Atom",
  "Bilibili",
  "微博",
  "搜狗授权搜索",
];

const evidence = [
  { source: "X：官方团队发布端侧模型能力", time: "2 小时前", tone: "bg-foreground/70" },
  { source: "Hacker News：开发者集中讨论部署体验", time: "5 小时前", tone: "bg-muted-foreground" },
  { source: "Bilibili：相关演示开始持续传播", time: "8 小时前", tone: "bg-foreground/35" },
];

const workflow = [
  {
    step: "01",
    icon: ScanSearch,
    title: "定义监控目标",
    body: "描述要关注的主体、动作与排除条件，让监控从一个关键词变成可审核的观察意图。",
    tone: "bg-muted text-foreground",
  },
  {
    step: "02",
    icon: Network,
    title: "多源聚合与 AI 分析",
    body: "持续收集授权信号，完成相关性判断、内容家族去重、证据覆盖与事件聚合。",
    tone: "bg-muted text-foreground",
  },
  {
    step: "03",
    icon: Radar,
    title: "实时推送与邮件",
    body: "当事件加速、证据变多或状态变化时及时提醒，同时保留人工复核的判断入口。",
    tone: "bg-muted text-foreground",
  },
];

export default function HomePage() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <PublicHeader />

      <main id="main-content" tabIndex={-1}>
        <HomeHero />

        <section id="product" className="scroll-mt-24">
          <div className="mx-auto max-w-[1440px] px-5 pt-24 sm:px-8 lg:pt-32">
            <div className="grid gap-8 lg:grid-cols-[minmax(0,1fr)_420px] lg:items-end lg:gap-16">
              <div>
                <p className="mono text-xs font-semibold uppercase text-signal">From signal to event</p>
                <h2 className="mt-4 max-w-5xl text-balance text-[clamp(2.8rem,5.2vw,5.4rem)] font-semibold leading-[.98] tracking-[-0.045em]">
                  把多源信号，变成可追溯的热点事件
                </h2>
              </div>
              <p className="max-w-md text-base leading-8 text-muted-foreground lg:pb-2">
                不再把高互动内容直接等同于热点。HotKey 关注跨来源变化、传播速度与证据覆盖，并说明为什么此刻值得关注。
              </p>
            </div>

            <section
              id="briefing"
              aria-label="多源热点示例"
              className="relative mt-14 scroll-mt-24 overflow-hidden rounded-[1.75rem] bg-card [box-shadow:var(--shadow-card)] lg:mt-20"
            >
              <header className="flex flex-col gap-4 bg-secondary/25 px-6 py-6 sm:flex-row sm:items-center sm:px-8">
                <div className="flex flex-wrap items-baseline gap-3">
                  <p className="text-sm font-semibold">示例情报</p>
                  <span className="rounded-full bg-secondary px-2.5 py-1 text-[11px] font-medium text-muted-foreground">演示数据</span>
                  <span className="mono text-xs text-muted-foreground">最近 6 小时</span>
                </div>
                <div className="flex flex-wrap items-center gap-x-5 gap-y-2 sm:ml-auto sm:justify-end">
                  <span className="inline-flex items-center gap-1.5 text-sm font-medium text-foreground">
                    <Check aria-hidden="true" className="h-3.5 w-3.5" />
                    多源证据已覆盖
                  </span>
                </div>
              </header>

              <div className="grid lg:grid-cols-[minmax(0,1.25fr)_minmax(360px,.75fr)]">
                <article className="min-w-0 p-6 sm:p-8 lg:p-10">
                  <div className="flex flex-wrap items-center gap-2 text-xs font-medium">
                    <span className="rounded-full bg-muted px-3 py-1 text-foreground">聚合事件</span>
                    <span className="rounded-full bg-muted px-3 py-1 text-foreground">正在升温</span>
                  </div>
                  <h3 className="mt-6 max-w-3xl text-3xl font-semibold leading-[1.12] sm:text-4xl">
                    AI Agent 进入端侧部署阶段，生态合作加速落地
                  </h3>
                  <p className="mt-5 max-w-2xl text-base leading-8 text-muted-foreground">
                    X、Hacker News 与 Bilibili 的相关内容在同一观测窗口出现，经目标匹配和内容家族去重后聚合为一个可追溯事件。
                  </p>
                  <div className="mt-10 grid gap-6 sm:grid-cols-3">
                    <div>
                      <p className="text-xs text-muted-foreground">Heat · 当前</p>
                      <p className="mono mt-2 text-4xl font-semibold text-foreground">92<span className="ml-1 text-sm font-normal text-muted-foreground">/100</span></p>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">相关性</p>
                      <p className="mono mt-2 text-4xl font-semibold text-foreground">88<span className="ml-1 text-sm font-normal text-muted-foreground">%</span></p>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">独立出处</p>
                      <p className="mono mt-2 text-4xl font-semibold text-foreground">03</p>
                    </div>
                  </div>
                  <p className="mt-8 text-sm font-medium text-foreground">传播速度正在上升</p>
                  <Progress aria-label="事件热度 92 分" className="mt-3 h-2 bg-muted [&>div]:bg-foreground/65" value={92} />
                </article>

                <aside className="bg-secondary/35 p-6 sm:p-8 lg:p-10">
                  <div className="flex items-center gap-2">
                    <FileCheck2 aria-hidden="true" className="h-5 w-5 text-muted-foreground" />
                    <h3 className="text-base font-semibold">多源证据状态</h3>
                  </div>
                  <ul className="mt-7 space-y-5">
                    {evidence.map((item) => (
                      <li key={item.source} className="grid grid-cols-[8px_minmax(0,1fr)] gap-3">
                        <span aria-hidden="true" className={`mt-2 h-2 w-2 rounded-full ${item.tone}`} />
                        <div>
                          <p className="text-sm leading-6">{item.source}</p>
                          <p className="mt-1 text-xs text-muted-foreground">{item.time}</p>
                        </div>
                      </li>
                    ))}
                  </ul>
                  <div className="mt-8 rounded-xl bg-card/85 p-4 [box-shadow:var(--shadow-card)]">
                    <p className="text-xs font-semibold text-muted-foreground">为什么现在提醒</p>
                    <p className="mt-2 text-sm leading-6">多个独立来源在相近窗口出现，传播速度持续上升，且与已发布监控目标高度相关。</p>
                  </div>
                  <Link href="/dashboard/events" className="mt-7 inline-flex items-center gap-1.5 text-sm font-semibold text-primary no-underline hover:opacity-75">
                    查看事件雷达 <ArrowRight className="h-4 w-4" />
                  </Link>
                </aside>
              </div>
            </section>
          </div>
        </section>

        <section id="workflow" className="scroll-mt-24">
          <div className="mx-auto max-w-[1440px] px-5 py-24 sm:px-8 lg:py-32">
            <div className="max-w-3xl">
              <p className="mono text-xs font-semibold uppercase text-signal">A traceable workflow</p>
              <h2 className="mt-4 text-balance text-[clamp(2.5rem,4.3vw,4.6rem)] font-semibold leading-[1] tracking-[-0.04em]">
                从观察目标，到可行动的事件判断
              </h2>
            </div>
            <div className="mt-14 grid gap-4 lg:grid-cols-3">
              {workflow.map((item) => {
                const Icon = item.icon;
                return (
                  <article key={item.step} className="rounded-2xl bg-card p-6 [box-shadow:var(--shadow-card)] sm:p-8">
                    <div className="flex items-center justify-between">
                      <span className={`inline-flex h-11 w-11 items-center justify-center rounded-xl ${item.tone}`}>
                        <Icon aria-hidden="true" className="h-5 w-5" />
                      </span>
                      <span className="mono text-xs text-muted-foreground">{item.step}</span>
                    </div>
                    <h3 className="mt-9 text-xl font-semibold">{item.title}</h3>
                    <p className="mt-4 text-sm leading-7 text-muted-foreground">{item.body}</p>
                  </article>
                );
              })}
            </div>
          </div>
        </section>

        <section id="sources" aria-labelledby="sources-heading" className="scroll-mt-24 bg-secondary/45">
          <div className="mx-auto grid max-w-[1440px] gap-14 px-5 py-24 sm:px-8 lg:grid-cols-[.78fr_1.22fr] lg:gap-20 lg:py-32">
            <div>
              <div className="inline-flex h-12 w-12 items-center justify-center rounded-xl bg-muted text-foreground">
                <ShieldCheck aria-hidden="true" className="h-6 w-6" />
              </div>
              <h2 id="sources-heading" className="mt-7 max-w-xl text-4xl font-semibold leading-tight sm:text-5xl">
                来源有边界，判断有出处
              </h2>
              <p className="mt-5 max-w-lg text-base leading-8 text-muted-foreground">
                只连接官方 API、RSS、Atom 或授权 Feed。AI 辅助整理相关性和证据关系，不把模型分数包装成事实真伪概率。
              </p>
            </div>
            <div>
              <p className="text-sm font-semibold">代表来源</p>
              <ul aria-label="当前支持的来源" className="mt-6 grid grid-cols-2 gap-3 sm:grid-cols-3">
                {supportedSources.map((source, index) => (
                  <li key={source} className="rounded-xl bg-card px-4 py-5 text-sm font-semibold [box-shadow:var(--shadow-card)]">
                    <span className="mono mr-2 text-[10px] text-signal">{String(index + 1).padStart(2, "0")}</span>
                    {source}
                  </li>
                ))}
              </ul>
              <div className="mt-5 flex items-start gap-3 rounded-xl bg-muted p-4 text-sm leading-6 text-muted-foreground">
                <Sparkles aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
                来源能力会随授权与运行状态变化，工作台始终显示当前覆盖和降级情况。
              </div>
            </div>
          </div>
        </section>

        <section id="start" className="scroll-mt-24 px-5 py-20 sm:px-8 lg:py-28">
          <div className="relative mx-auto max-w-[1376px] overflow-hidden rounded-2xl bg-secondary px-6 py-16 text-foreground sm:px-10 lg:flex lg:items-end lg:justify-between lg:gap-12 lg:px-16 lg:py-20">
            <div className="relative">
              <p className="text-sm text-muted-foreground">从第一条监控开始</p>
              <h2 className="mt-4 max-w-4xl text-balance text-[clamp(2.6rem,4.7vw,5rem)] font-semibold leading-[.98] tracking-[-0.04em]">
                让下一次判断更早，也更可回溯
              </h2>
              <p className="mt-5 max-w-xl text-base leading-7 text-muted-foreground">建立观察目标，等待信号汇聚，再从证据出发形成判断。</p>
            </div>
            <Button asChild size="lg" className="relative mt-9 h-12 self-start rounded-lg px-6 lg:mt-0 lg:self-auto">
              <Link href="/register">创建监控 <ArrowRight /></Link>
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
