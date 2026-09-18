import Link from "next/link";
import { ArrowLeftIcon } from "lucide-react";

import { PageState } from "@/components/patterns/page-state";
import { Button } from "@/components/ui/button";

export default function NotFound() {
  return (
    <PageState
      eyebrow="页面不存在"
      title="没有找到这个页面。"
      description="链接可能已经失效，或对应功能还没有进入当前实现范围。"
      action={
        <Button asChild size="lg">
          <Link href="/">
            <ArrowLeftIcon data-icon="inline-start" />
            返回首页
          </Link>
        </Button>
      }
    />
  );
}
