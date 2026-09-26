import Link from "next/link";
import type { ReactNode } from "react";
import { ArrowLeftIcon } from "lucide-react";

import { BrandLockup } from "@/components/brand/brand-lockup";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

type AuthShellProps = {
  eyebrow: string;
  title: string;
  description: string;
  asideTitle: string;
  asideDescription: string;
  footer: ReactNode;
  children: ReactNode;
};

export function AuthShell({
  eyebrow,
  title,
  description,
  asideTitle,
  asideDescription,
  footer,
  children,
}: AuthShellProps) {
  return (
    <main className="grid min-h-screen lg:grid-cols-2">
      <section className="bg-muted relative isolate hidden min-h-screen flex-col justify-between overflow-hidden px-10 py-10 lg:flex xl:px-16 xl:py-14">
        <div aria-hidden="true" className="hero-wash absolute inset-0 -z-20" />
        <div
          aria-hidden="true"
          className="absolute top-1/2 -right-24 -z-10 aspect-square w-full max-w-2xl -translate-y-1/2 opacity-70"
        >
          <div className="hero-ripple absolute inset-0 rounded-full" />
          <div className="hero-ripple absolute inset-12 rounded-full" />
          <div className="hero-ripple absolute inset-24 rounded-full" />
          <div className="hero-ripple absolute inset-40 rounded-full" />
        </div>

        <BrandLockup href="/" showEnglish />

        <div className="max-w-lg pb-16">
          <Badge variant="secondary">知微见澜 · Ripplesight</Badge>
          <h2 className="mt-8 text-4xl leading-tight font-semibold tracking-tighter text-balance xl:text-5xl">
            {asideTitle}
          </h2>
          <p className="text-muted-foreground mt-6 max-w-md text-base leading-8 text-pretty">
            {asideDescription}
          </p>
        </div>

        <p className="text-muted-foreground text-sm">
          从微小线索，看见更大的变化。
        </p>
      </section>

      <section className="flex min-h-screen flex-col px-5 py-6 sm:px-8 lg:min-h-0 lg:px-12 lg:py-10 xl:px-20">
        <header className="flex items-center justify-between gap-4 lg:justify-end">
          <div className="lg:hidden">
            <BrandLockup href="/" />
          </div>
          <Button asChild variant="ghost" size="navigation">
            <Link href="/">
              <ArrowLeftIcon data-icon="inline-start" />
              返回首页
            </Link>
          </Button>
        </header>

        <Card className="auth-card mx-auto my-auto w-full max-w-lg gap-0 rounded-xl py-0">
          <CardHeader className="gap-0 px-6 pt-8 sm:px-10 sm:pt-10">
            <Badge variant="secondary" className="mb-5">
              {eyebrow}
            </Badge>
            <CardTitle
              asChild
              className="text-3xl leading-tight font-semibold tracking-tight sm:text-4xl"
            >
              <h1>{title}</h1>
            </CardTitle>
            <CardDescription className="mt-3 text-sm leading-6">
              {description}
            </CardDescription>
          </CardHeader>
          <CardContent className="px-6 pt-8 pb-7 sm:px-10">
            {children}
          </CardContent>
          <CardFooter className="bg-transparent px-6 pb-8 sm:px-10 sm:pb-10">
            {footer}
          </CardFooter>
        </Card>
      </section>
    </main>
  );
}
