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
      <img
        src="/icon.svg"
        alt="HotKey"
        width={20}
        height={20}
        className={cn("h-4 w-4 shrink-0", markClassName)}
      />
      <span aria-hidden="true">{title}</span>
    </span>
  );
}
