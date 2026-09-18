import {
  ChartNoAxesCombinedIcon,
  MessageSquareTextIcon,
  RadarIcon,
} from "lucide-react";

import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

const foundations = [
  {
    title: "发现公开线索",
    description: "为后续接入的公开来源保留统一入口、状态与证据边界。",
    note: "来源能力将在业务切片中逐项验收",
    icon: RadarIcon,
  },
  {
    title: "整理事件脉络",
    description: "以事件、讨论和证据为中心组织信息，减少界面层的重复判断。",
    note: "页面只消费生成的 OpenAPI 客户端",
    icon: MessageSquareTextIcon,
  },
  {
    title: "形成可读结论",
    description: "把趋势、观点和来源状态放进同一套清晰、可追溯的工作流。",
    note: "分析能力以真实数据验收为准",
    icon: ChartNoAxesCombinedIcon,
  },
];

export function FoundationOverview() {
  return (
    <section
      id="foundation"
      className="bg-muted px-5 py-20 sm:px-8 sm:py-28 xl:px-0 2xl:py-32"
    >
      <div className="mx-auto max-w-7xl">
        <div className="max-w-2xl">
          <p className="text-muted-foreground font-mono text-xs tracking-widest uppercase">
            Product foundation
          </p>
          <h2 className="mt-4 text-3xl font-semibold tracking-tight sm:text-4xl">
            围绕一个可信的信息闭环。
          </h2>
          <p className="text-muted-foreground mt-4 text-base leading-7">
            当前页面只证明前端工程与视觉基础，不把未来的采集、分析或提醒能力描述为已经上线。
          </p>
        </div>

        <div className="mt-12 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:gap-6">
          {foundations.map(({ title, description, note, icon: Icon }) => (
            <Card key={title}>
              <CardHeader>
                <CardTitle>{title}</CardTitle>
                <CardDescription>{description}</CardDescription>
                <CardAction>
                  <span className="bg-secondary text-secondary-foreground flex size-9 items-center justify-center rounded-lg">
                    <Icon aria-hidden="true" className="size-4" />
                  </span>
                </CardAction>
              </CardHeader>
              <CardContent>
                <div className="bg-muted h-20 rounded-lg" aria-hidden="true" />
              </CardContent>
              <CardFooter>
                <p className="text-muted-foreground text-xs leading-5">
                  {note}
                </p>
              </CardFooter>
            </Card>
          ))}
        </div>
      </div>
    </section>
  );
}
