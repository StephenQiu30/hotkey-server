import { ExternalLink, ShieldOff } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

const officialLinks = [
  {
    href: "https://duckduckgo.com/duckduckgo-help-pages/features/instant-answers-and-other-features",
    label: "即时答案说明",
  },
  {
    href: "https://duckduckgo.com/duckduckgo-help-pages/results/sources",
    label: "结果来源",
  },
  { href: "https://duckduckgo.com/terms", label: "服务条款" },
] as const;

const unlockRequirements = [
  "正式服务端读取权与适用条款",
  "固定端点、认证、版本、配额和 SLA",
  "存储、来源归属和删除同步规则",
  "官方沙盒或 fixture 与可验证失败契约",
] as const;

export function DuckDuckGoBoundaryCard() {
  return (
    <section aria-labelledby="duckduckgo-boundary-title" className="mt-6">
      <Card className="gap-0 py-0 shadow-none">
        <CardHeader className="gap-3 border-b border-border">
          <div className="flex flex-wrap items-center gap-3">
            <ShieldOff className="size-5 text-muted-foreground" />
            <CardTitle id="duckduckgo-boundary-title">
              DuckDuckGo Instant Answer
            </CardTitle>
            <Badge variant="outline">未开放</Badge>
          </div>
          <CardDescription className="max-w-3xl leading-6">
            Instant Answer 是搜索产品中的知识答案，不是通用网页搜索结果
            API。HotKey 不抓取 DuckDuckGo 页面，不创建或调度该来源，也不把它计入热度。
          </CardDescription>
        </CardHeader>
        <CardContent className="pt-6">
          <p className="text-sm font-medium">未来接入前必须全部具备</p>
          <ul className="mt-3 grid gap-2 text-sm text-muted-foreground sm:grid-cols-2">
            {unlockRequirements.map((requirement) => (
              <li key={requirement} className="flex gap-2 leading-6">
                <span aria-hidden="true">•</span>
                <span>{requirement}</span>
              </li>
            ))}
          </ul>
        </CardContent>
        <CardFooter className="flex-col items-stretch gap-3 sm:flex-row sm:flex-wrap sm:items-center">
          {officialLinks.map((link) => (
            <Button
              key={link.href}
              asChild
              variant="outline"
              className="w-full sm:w-auto"
            >
              <a href={link.href} target="_blank" rel="noreferrer">
                {link.label}
                <ExternalLink />
              </a>
            </Button>
          ))}
          <Button disabled className="w-full sm:w-auto">
            尚无正式 API 契约
          </Button>
        </CardFooter>
      </Card>
    </section>
  );
}
