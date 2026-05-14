import * as React from "react";
import { cn } from "@/lib/utils";

export const Textarea = React.forwardRef<HTMLTextAreaElement, React.TextareaHTMLAttributes<HTMLTextAreaElement>>(({ className, ...props }, ref) => (
  <textarea
    className={cn(
      "ios-card-muted ios-focus-ring flex min-h-28 w-full border border-input bg-card px-3 py-2 text-sm shadow-sm transition-colors placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-55",
      className
    )}
    ref={ref}
    {...props}
  />
));
Textarea.displayName = "Textarea";
