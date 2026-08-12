import { Suspense } from "react";
import { Loader2 } from "lucide-react";
import { MicroEventWorkspace } from "@/components/dashboard/MicroEventWorkspace";

export default function EventsPage() {
  return (
    <Suspense
      fallback={
        <div className="app-page" aria-label="加载热点事件">
          <div className="flex min-h-72 items-center justify-center">
            <Loader2 className="size-5 animate-spin text-muted-foreground" />
          </div>
        </div>
      }
    >
      <MicroEventWorkspace />
    </Suspense>
  );
}
