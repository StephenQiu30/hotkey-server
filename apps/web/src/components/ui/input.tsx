import * as React from "react";
import { cn } from "@/lib/utils";

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(({ className, ...props }, ref) => (
  <input
    className={cn(
      "ios-card-muted ios-focus-ring flex min-h-11 w-full border border-input bg-card px-3 py-2 text-sm shadow-sm transition-colors placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-55",
      className
    )}
    ref={ref}
    {...props}
  />
));
Input.displayName = "Input";
