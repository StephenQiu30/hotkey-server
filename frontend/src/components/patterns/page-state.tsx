import type { ReactNode } from "react";

import { Badge } from "@/components/ui/badge";

type PageStateProps = {
  eyebrow: string;
  title: string;
  description: string;
  action?: ReactNode;
};

export function PageState({
  eyebrow,
  title,
  description,
  action,
}: PageStateProps) {
  return (
    <main className="bg-background flex min-h-screen items-center px-5 py-20 sm:px-8 xl:px-0">
      <section className="bg-muted mx-auto flex w-full max-w-xl flex-col items-start rounded-2xl p-6 sm:p-8 xl:p-10">
        <Badge variant="secondary">{eyebrow}</Badge>
        <h1 className="mt-5 text-3xl font-semibold tracking-tight sm:text-4xl">
          {title}
        </h1>
        <p className="text-muted-foreground mt-4 text-base leading-7">
          {description}
        </p>
        {action ? <div className="mt-8">{action}</div> : null}
      </section>
    </main>
  );
}
