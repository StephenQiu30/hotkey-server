import type { ReactNode } from "react";

interface PageHeaderProps {
  eyebrow: string;
  title: string;
  description: ReactNode;
  action?: ReactNode;
}

export function PageHeader({
  eyebrow,
  title,
  description,
  action,
}: PageHeaderProps) {
  return (
    <header
      aria-label={title}
      role="group"
      className="flex flex-col gap-8 pb-10 sm:flex-row sm:items-end sm:justify-between"
    >
      <div className="min-w-0">
        <p className="mono text-[11px] font-medium uppercase leading-4 text-muted-foreground">
          {eyebrow}
        </p>
        <h1 className="mt-3 text-3xl font-semibold leading-[1.1] tracking-[-0.055em] sm:text-[2.5rem]">
          {title}
        </h1>
        <div className="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground sm:text-base">
          {description}
        </div>
      </div>
      {action ? (
        <div
          data-testid="page-header-action"
          className="w-full shrink-0 [&>*]:w-full sm:w-auto sm:[&>*]:w-auto"
        >
          {action}
        </div>
      ) : null}
    </header>
  );
}
