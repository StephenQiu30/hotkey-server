import { cva, type VariantProps } from "class-variance-authority";
import * as React from "react";
import { cn } from "@/lib/utils";

const badgeVariants = cva("inline-flex w-fit items-center rounded-full border px-2.5 py-1 text-xs font-semibold", {
  variants: {
    variant: {
      default: "border-blue-200 bg-blue-50 text-blue-700",
      success: "border-emerald-200 bg-emerald-50 text-emerald-700",
      warning: "border-sky-200 bg-sky-50 text-sky-700",
      muted: "border-border bg-muted/80 text-muted-foreground",
      destructive: "border-red-200 bg-red-50 text-red-700",
    },
  },
  defaultVariants: {
    variant: "default",
  },
});

export interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement>, VariantProps<typeof badgeVariants> {}

export function Badge({ className, variant, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ variant, className }))} {...props} />;
}
