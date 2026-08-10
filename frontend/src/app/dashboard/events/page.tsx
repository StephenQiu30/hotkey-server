import { Suspense } from "react";
import { Loader2 } from "lucide-react";
import { MicroEventWorkspace } from "@/components/dashboard/MicroEventWorkspace";

export default function EventsPage() {
  return (
    <Suspense
      fallback={
        <div className="flex min-h-72 items-center justify-center" aria-label="加载热点事件">
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
        </div>
      }
    >
      <MicroEventWorkspace />
    </Suspense>
  );
}
