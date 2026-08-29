import type { ReactNode } from "react";
import { Card, CardContent } from "@/components/ui/card";
import { cn } from "@/lib/utils";

interface OverviewMetricProps {
  label: string;
  value: ReactNode;
  className?: string;
}

export function OverviewMetric({
  label,
  value,
  className,
}: OverviewMetricProps) {
  return (
    <Card
      variant="ring"
      className={cn(
        "min-h-32 justify-between",
        className,
      )}
      data-slot="overview-metric"
    >
      <CardContent className="flex h-full flex-col justify-between p-5">
        <p className="text-xs font-medium text-muted-foreground">{label}</p>
        <p className="mono mt-8 text-3xl font-medium leading-none tracking-[-0.055em] sm:text-4xl">
          {value}
        </p>
      </CardContent>
    </Card>
  );
}
