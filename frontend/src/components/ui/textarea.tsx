import * as React from "react"

import { cn } from "@/lib/utils"

const Textarea = React.forwardRef<
  HTMLTextAreaElement,
  React.ComponentProps<"textarea">
>(({ className, ...props }, ref) => (
  <textarea
    data-slot="textarea"
    className={cn(
      "flex min-h-20 w-full rounded-md bg-background px-3 py-2 text-base [box-shadow:var(--shadow-control)] transition-[background-color,box-shadow] placeholder:text-muted-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring aria-invalid:outline-2 aria-invalid:outline-destructive disabled:cursor-not-allowed disabled:bg-muted disabled:opacity-60 md:text-sm",
      className,
    )}
    ref={ref}
    {...props}
  />
))
Textarea.displayName = "Textarea"

export { Textarea }
