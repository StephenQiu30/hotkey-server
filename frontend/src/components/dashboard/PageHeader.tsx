import type { ReactNode } from "react";

interface PageHeaderProps {
  eyebrow: string;
  title: string;
  description: string;
  action?: ReactNode;
}

export function PageHeader({
  title,
  description,
  action,
}: PageHeaderProps) {
  return (
    <header
      aria-label={title}
      role="group"
      className="flex flex-col gap-6 pb-8 sm:flex-row sm:items-end sm:justify-between"
    >
      <div className="min-w-0">
        <h1 className="text-3xl font-semibold tracking-[-0.045em]">{title}</h1>
        <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">
          {description}
        </p>
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
