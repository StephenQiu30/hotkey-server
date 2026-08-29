import * as React from "react";
import { Slot as SlotPrimitive } from "radix-ui";
import { cva, type VariantProps } from "class-variance-authority";

import { cn } from "@/lib/utils";

const surfaceVariants = cva(
  "rounded-lg bg-card text-card-foreground",
  {
    variants: {
      variant: {
        elevated: "[box-shadow:var(--shadow-card)]",
        ring: "[box-shadow:var(--shadow-border)]",
        subtle: "bg-muted/30 [box-shadow:var(--shadow-border)]",
        interactive:
          "[box-shadow:var(--shadow-border)] transition-[background-color,box-shadow,transform] hover:bg-muted/30 hover:[box-shadow:var(--shadow-card-hover)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring active:translate-y-px",
        danger:
          "bg-destructive/5 text-foreground ring-1 ring-inset ring-destructive/30",
        flat: "shadow-none",
      },
    },
    defaultVariants: {
      variant: "elevated",
    },
  },
);

interface SurfaceProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof surfaceVariants> {
  asChild?: boolean;
}

const Surface = React.forwardRef<HTMLDivElement, SurfaceProps>(
  ({ asChild = false, className, variant, ...props }, ref) => {
    const Comp = asChild ? SlotPrimitive.Slot : "div";

    return (
      <Comp
        ref={ref}
        data-slot="surface"
        data-variant={variant ?? "elevated"}
        className={cn(surfaceVariants({ variant }), className)}
        {...props}
      />
    );
  },
);
Surface.displayName = "Surface";

export { Surface, surfaceVariants };
export type { SurfaceProps };
