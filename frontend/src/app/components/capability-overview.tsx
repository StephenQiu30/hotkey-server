const steps = [
  {
    number: "01",
    title: "定义关注范围",
    description: "用关键词设定品牌、产品或话题。",
  },
  {
    number: "02",
    title: "汇集相关讨论",
    description: "按可用来源汇集相关讨论与线索。",
  },
  {
    number: "03",
    title: "沿来源回看",
    description: "顺着时间与原始内容理解变化。",
  },
];

export function CapabilityOverview() {
  return (
    <section
      id="how-it-works"
      className="px-5 pb-24 sm:px-8 sm:pb-32 xl:px-16 2xl:px-0"
    >
      <div className="border-border mx-auto max-w-7xl border-t pt-9">
        <h2 className="sr-only">如何追踪一个主题</h2>
        <p className="text-muted-foreground bg-secondary inline-flex rounded-full px-4 py-2 text-sm">
          品牌动态 · 产品反馈 · 行业议题
        </p>
        <div className="mt-24 grid gap-12 md:grid-cols-2 md:gap-16 lg:grid-cols-3">
          {steps.map(({ number, title, description }) => (
            <div key={number} className="flex flex-col gap-4">
              <span className="text-muted-foreground font-mono text-xs">
                {number}
              </span>
              <h3 className="text-xl font-semibold tracking-tight">{title}</h3>
              <p className="text-muted-foreground text-sm leading-7">
                {description}
              </p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
