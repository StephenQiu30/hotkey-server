import { ExternalLink, ShieldAlert } from "lucide-react";
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

const SOGOU_PLATFORM_HELP =
  "https://data.open.sogou.com/data-resource/help.html?type=1";

const authorizationRequirements = [
  "书面搜索结果读取授权与适用条款",
  "固定 HTTPS 端点、认证 scope 与 env 凭据引用",
  "配额、分页、归属标记和删除同步规则",
  "可控沙盒或 fixture，以及通过的 Connector 契约测试",
] as const;

export function SogouAuthorizationCard() {
  return (
    <section aria-labelledby="sogou-authorization-title" className="mt-6">
      <Card className="gap-0 py-0 shadow-none">
        <CardHeader className="gap-3 border-b border-border">
          <div className="flex flex-wrap items-center gap-3">
            <ShieldAlert className="size-5 text-muted-foreground" />
            <CardTitle id="sogou-authorization-title">
              搜狗授权搜索
            </CardTitle>
            <Badge variant="outline">需要授权</Badge>
          </div>
          <CardDescription className="max-w-3xl leading-6">
            搜狗公开开放平台用于站方向搜狗提交结构化资源，不是搜索结果读取
            API。HotKey 不抓取搜索结果页，也不会创建或调度搜狗来源连接。
          </CardDescription>
        </CardHeader>
        <CardContent className="pt-6">
          <p className="text-sm font-medium">解除阻塞前必须全部具备</p>
          <ul className="mt-3 grid gap-2 text-sm text-muted-foreground sm:grid-cols-2">
            {authorizationRequirements.map((requirement) => (
              <li key={requirement} className="flex gap-2 leading-6">
                <span aria-hidden="true">•</span>
                <span>{requirement}</span>
              </li>
            ))}
          </ul>
        </CardContent>
        <CardFooter className="flex-col items-stretch gap-3 sm:flex-row sm:items-center">
          <Button asChild variant="outline" className="w-full sm:w-auto">
            <a href={SOGOU_PLATFORM_HELP} target="_blank" rel="noreferrer">
              查看官方开放平台说明
              <ExternalLink />
            </a>
          </Button>
          <Button disabled className="w-full sm:w-auto">
            授权资料未齐备
          </Button>
        </CardFooter>
      </Card>
    </section>
  );
}
