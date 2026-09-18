import { BellRingIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";

export function DesignPrinciples() {
  return (
    <section
      id="principles"
      className="px-5 py-20 sm:px-8 sm:py-28 xl:px-0 2xl:py-32"
    >
      <div className="mx-auto grid max-w-7xl grid-cols-1 gap-12 lg:grid-cols-2 lg:items-center 2xl:gap-16">
        <div>
          <Badge variant="secondary">组件优先 · 无边框设计</Badge>
          <h2 className="mt-5 text-3xl font-semibold tracking-tight sm:text-4xl">
            用层级表达结构，用组件保证一致。
          </h2>
          <p className="text-muted-foreground mt-5 max-w-xl text-base leading-7">
            导航、信息卡和内容分区依靠留白与浅色表面建立层级。输入框、键盘焦点、错误状态和浮层保留必要轮廓；页面只组合可复用组件，不复制视觉规则。
          </p>
        </div>

        <div className="bg-primary text-primary-foreground rounded-2xl p-6 sm:p-8 xl:p-10">
          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-primary-foreground/60 font-mono text-xs">
                SIGNAL / FOUNDATION
              </p>
              <p className="mt-2 text-lg font-medium">
                设计令牌已经映射到语义组件
              </p>
            </div>
            <BellRingIcon
              aria-hidden="true"
              className="text-primary-foreground/70 size-5"
            />
          </div>
          <div
            className="mt-10 grid grid-cols-3 gap-3 sm:gap-4"
            aria-label="设计颜色示例"
          >
            <div className="from-chart-1 to-chart-2 h-20 rounded-lg bg-linear-to-br" />
            <div className="from-chart-3 to-chart-4 h-20 rounded-lg bg-linear-to-br" />
            <div className="from-destructive to-chart-5 h-20 rounded-lg bg-linear-to-br" />
          </div>
        </div>
      </div>
    </section>
  );
}
