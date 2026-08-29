import * as React from "react";
import { Slot as SlotPrimitive } from "radix-ui";
import { cva, type VariantProps } from "class-variance-authority";

import { cn } from "@/lib/utils";

const pageShellVariants = cva("app-page", {
  variants: {
    align: {
      start: "",
      center: "flex min-h-full items-center justify-center",
    },
  },
  defaultVariants: {
    align: "start",
  },
});

interface PageShellProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof pageShellVariants> {
  asChild?: boolean;
}

const PageShell = React.forwardRef<HTMLDivElement, PageShellProps>(
  ({ align, asChild = false, className, ...props }, ref) => {
    const Comp = asChild ? SlotPrimitive.Slot : "div";

    return (
      <Comp
        ref={ref}
        data-slot="page-shell"
        className={cn(pageShellVariants({ align }), className)}
        {...props}
      />
    );
  },
);
PageShell.displayName = "PageShell";

export { PageShell, pageShellVariants };
