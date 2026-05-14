import Link from "next/link";
import { Check, Sparkles } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

const plans = [
  {
    name: "Private",
    price: "自部署",
    description: "适合个人或小团队在本机/私有环境直接使用。",
    items: ["单用户工作台", "关键词与多源管理", "日报/周报生成", "SMTP 通知降级"],
  },
  {
    name: "Team",
    price: "后续规划",
    description: "保留给团队协作、成员和权限管理。",
    items: ["组织与成员", "角色权限", "团队报告", "审计记录"],
  },
  {
    name: "Business",
    price: "后续规划",
    description: "保留给公网 SaaS、订阅和高级集成。",
    items: ["订阅计费", "额度控制", "Webhook", "企业级部署"],
  },
];

export default function PricingPage() {
  return (
    <main className="min-h-screen bg-background">
      <header className="border-b border-border bg-white">
        <div className="mx-auto flex max-w-7xl items-center justify-between px-4 py-4 md:px-6">
          <Link className="ios-shell-card flex min-h-11 items-center gap-3 rounded-full border border-border/80 bg-white/70 px-3 font-extrabold" href="/">
            <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary text-primary-foreground">
              <Sparkles className="h-5 w-5" />
            </span>
            AI Hotspot Radar
          </Link>
          <Button asChild>
            <Link href="/app">进入工作台</Link>
          </Button>
        </div>
      </header>
      <section className="mx-auto max-w-7xl px-4 py-14 md:px-6">
        <Badge className="mb-5">Delivery Boundary</Badge>
        <p className="text-xs font-semibold uppercase tracking-[0.24em] text-primary">delivery first design</p>
        <h1 className="max-w-3xl text-4xl font-extrabold leading-tight md:text-5xl">先把私有部署体验做好，再扩展商业 SaaS。</h1>
        <p className="mt-5 max-w-2xl text-lg leading-8 text-muted-foreground">
          首版不接真实支付，也不启动 Stripe 流程。界面下面仅展示当前交付边界与后续规划。
        </p>
        <div className="mt-10 grid gap-4 lg:grid-cols-3">
          {plans.map((plan, index) => (
            <Card className={`ios-shell-card ${index === 0 ? "border-primary/70 shadow-md" : "border-border/80"}`} key={plan.name}>
              <CardHeader className="gap-4">
                <div className="flex items-center justify-between gap-3">
                  <CardTitle>{plan.name}</CardTitle>
                  {index === 0 ? <Badge variant="success">当前首版</Badge> : <Badge variant="muted">占位</Badge>}
                </div>
                <p className="text-3xl font-extrabold">{plan.price}</p>
                <CardDescription>{plan.description}</CardDescription>
              </CardHeader>
              <CardContent className="grid gap-3">
                <ul className="grid gap-3">
                  {plan.items.map((item) => (
                    <li className="flex gap-2 text-sm text-muted-foreground" key={item}>
                      <Check className="mt-0.5 h-4 w-4 text-emerald-600" />
                      <span>{item}</span>
                    </li>
                  ))}
                </ul>
              </CardContent>
            </Card>
          ))}
        </div>
      </section>
    </main>
  );
}
