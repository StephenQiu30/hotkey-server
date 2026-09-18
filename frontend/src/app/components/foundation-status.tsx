import { Badge } from "@/components/ui/badge";

export function FoundationStatus() {
  return (
    <section
      id="status"
      className="px-5 pb-20 sm:px-8 sm:pb-28 xl:px-0 2xl:pb-32"
    >
      <div className="bg-muted mx-auto flex max-w-7xl flex-col gap-6 rounded-2xl p-6 sm:flex-row sm:items-center sm:justify-between sm:p-8 xl:p-10">
        <div>
          <p className="font-medium">工程基础已就绪</p>
          <p className="text-muted-foreground mt-1 text-sm leading-6">
            后续业务页面按 app 路由、features 功能与生成 API
            客户端的固定边界继续实现。
          </p>
        </div>
        <Badge variant="secondary">Ready for feature slices</Badge>
      </div>
    </section>
  );
}
