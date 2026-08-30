import { Radar } from "lucide-react";
import { cn } from "@/lib/utils";

interface BrandLogoProps {
  title?: string;
  className?: string;
  markClassName?: string;
}

export function BrandLogo({
  title = "HotKey",
  className,
  markClassName,
}: BrandLogoProps) {
  return (
    <span className={cn("inline-flex items-center gap-2", className)}>
      <Radar
        role="img"
        aria-label="HotKey"
        strokeWidth={1.8}
        className={cn("h-4 w-4 shrink-0 text-foreground", markClassName)}
      />
      <span aria-hidden="true">{title}</span>
    </span>
  );
}
