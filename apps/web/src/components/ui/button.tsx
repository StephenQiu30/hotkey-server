import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import * as React from "react";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex min-h-11 items-center justify-center gap-2 rounded-xl text-sm font-semibold transition-colors duration-200 focus-visible:outline-none disabled:pointer-events-none disabled:opacity-55 ios-focus-ring",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground shadow-sm shadow-blue-200/40 hover:bg-primary/90",
        secondary: "border border-border bg-white text-foreground shadow-[0_2px_10px_-6px_rgba(15,78,151,0.2)] hover:bg-muted",
        ghost: "text-muted-foreground hover:bg-muted hover:text-foreground",
        accent: "bg-accent text-accent-foreground shadow-sm hover:bg-amber-500/90",
        destructive: "bg-destructive text-destructive-foreground shadow-sm shadow-red-200/50 hover:bg-red-600",
      },
      size: {
        default: "px-4 py-2",
        sm: "min-h-9 px-3 py-1.5",
        lg: "min-h-12 px-5 py-3",
        icon: "h-11 w-11 p-0",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
);

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement>, VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(({ className, variant, size, asChild = false, ...props }, ref) => {
  const Comp = asChild ? Slot : "button";
  return <Comp className={cn(buttonVariants({ variant, size, className }))} ref={ref} {...props} />;
});
Button.displayName = "Button";

export { buttonVariants };
