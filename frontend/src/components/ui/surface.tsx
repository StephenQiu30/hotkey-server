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
        ring: "[box-shadow:var(--shadow-card)]",
        subtle: "bg-muted/40 shadow-none",
        interactive:
          "[box-shadow:var(--shadow-card)] transition-[background-color,box-shadow,transform] hover:bg-muted/40 hover:[box-shadow:var(--shadow-card-hover)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring active:translate-y-px",
        danger:
          "bg-destructive/10 text-foreground shadow-none",
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
