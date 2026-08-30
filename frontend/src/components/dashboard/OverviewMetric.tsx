import type { ReactNode } from "react";
import { Card, CardContent } from "@/components/ui/card";
import { cn } from "@/lib/utils";

interface OverviewMetricProps {
  label: string;
  value: ReactNode;
  description?: string;
  icon?: ReactNode;
  tone?: "signal" | "blue" | "heat" | "success";
  className?: string;
}

const toneClasses = {
  signal: "bg-muted text-muted-foreground",
  blue: "bg-muted text-muted-foreground",
  heat: "bg-muted text-muted-foreground",
  success: "bg-muted text-muted-foreground",
} as const;

export function OverviewMetric({
  label,
  value,
  description,
  icon,
  tone = "signal",
  className,
}: OverviewMetricProps) {
  return (
    <Card
      variant="subtle"
      className={cn(
        "overview-metric min-h-36 justify-between overflow-hidden bg-card/85 backdrop-blur-sm",
        className,
      )}
      data-slot="overview-metric"
      data-tone={tone}
    >
      <CardContent className="flex h-full flex-col justify-between p-5">
        <div className="flex items-start justify-between gap-3">
          <div>
            <p className="text-xs font-semibold text-foreground">{label}</p>
            {description ? (
              <p className="mt-1 text-[11px] leading-4 text-muted-foreground">{description}</p>
            ) : null}
          </div>
          {icon ? (
            <span className={cn("inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg [&_svg]:h-4 [&_svg]:w-4", toneClasses[tone])}>
              {icon}
            </span>
          ) : null}
        </div>
        <p className="mono mt-7 text-3xl font-semibold leading-none tracking-[-0.04em] sm:text-4xl">{value}</p>
      </CardContent>
    </Card>
  );
}
