import {
  ChartNoAxesCombinedIcon,
  MessageSquareTextIcon,
  RadarIcon,
} from "lucide-react";

import {
  Card,
  CardAction,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

const capabilities = [
  {
    title: "发现公开线索",
    description: "集中查看公开来源中的热点、作品与讨论线索。",
    icon: RadarIcon,
  },
  {
    title: "整理事件脉络",
    description: "按时间、来源与讨论关系组织事件信息。",
    icon: MessageSquareTextIcon,
  },
  {
    title: "形成可读结论",
    description: "汇总趋势、观点与证据，保持结论可追溯。",
    icon: ChartNoAxesCombinedIcon,
  },
];

export function CapabilityOverview() {
  return (
    <section
      id="capabilities"
      className="bg-muted px-5 py-20 sm:px-8 sm:py-28 xl:px-0 2xl:py-32"
    >
      <div className="mx-auto max-w-7xl">
        <div className="max-w-2xl">
          <h2 className="text-3xl font-semibold tracking-tight sm:text-4xl">
            从线索到结论，保持信息完整。
          </h2>
        </div>

        <div className="mt-12 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:gap-6">
          {capabilities.map(({ title, description, icon: Icon }) => (
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
            </Card>
          ))}
        </div>
      </div>
    </section>
  );
}
