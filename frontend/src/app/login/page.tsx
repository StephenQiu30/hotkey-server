"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import AuthShell from "@/components/auth/AuthShell";
import PasswordInput from "@/components/auth/PasswordInput";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Mail } from "lucide-react";
import { safeRedirect } from "@/lib/safeRedirect";
import { useAuthStore } from "@/stores/authStore";

export default function LoginPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const login = useAuthStore((state) => state.login);
  const router = useRouter();

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!email || !password) return;
    setLoading(true);
    try {
      await login({ email, password });
      router.push(
        safeRedirect(new URLSearchParams(window.location.search).get("redirect")),
      );
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "登录失败");
    } finally {
      setLoading(false);
    }
  };

  return (
    <AuthShell title="登录工作台" subtitle="继续你的热点情报工作">
      <form onSubmit={submit} className="space-y-6">
        <div className="space-y-2.5">
          <Label htmlFor="email">邮箱</Label>
          <div className="relative">
            <Mail aria-hidden="true" className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              id="email"
              type="email"
              autoComplete="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              className="h-12 rounded-lg bg-card pl-10"
              placeholder="name@example.com"
            />
          </div>
        </div>
        <div className="space-y-2.5">
          <div className="flex items-center justify-between">
            <Label htmlFor="password">密码</Label>
            <Link
              href="/forgot-password"
              className="rounded-sm text-xs text-muted-foreground no-underline transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
            >
              忘记密码？
            </Link>
          </div>
          <PasswordInput
            id="password"
            autoComplete="current-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            className="h-12 rounded-lg bg-card"
          />
        </div>
        <Button
          type="submit"
          disabled={loading || !email || !password}
          className="h-12 w-full rounded-lg disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100"
        >
          {loading ? "登录中…" : "进入工作台"}
        </Button>
      </form>
      <p className="mt-8 text-center text-sm text-muted-foreground">
        还没有账号？{" "}
        <Link
          href="/register"
          className="rounded-sm font-medium text-foreground no-underline hover:underline hover:underline-offset-4 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        >
          创建账号
        </Link>
      </p>
    </AuthShell>
  );
}
