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
import { safeRedirect } from "@/lib/safeRedirect";
import { useAuthStore } from "@/stores/authStore";

export default function LoginPage() {
  const [email, setEmail] = useState(""); const [password, setPassword] = useState(""); const [loading, setLoading] = useState(false);
  const login = useAuthStore((state) => state.login); const router = useRouter();
  const submit = async (event: React.FormEvent) => { event.preventDefault(); if (!email || !password) return; setLoading(true); try { await login({ email, password }); router.push(safeRedirect(new URLSearchParams(window.location.search).get("redirect"))); } catch (reason) { toast.error(reason instanceof Error ? reason.message : "登录失败"); } finally { setLoading(false); } };
  return <AuthShell title="登录工作台" subtitle="继续你的热点情报工作"><form onSubmit={submit} className="space-y-5"><div><Label htmlFor="email">邮箱</Label><Input id="email" type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} className="mt-2 h-11" placeholder="name@example.com" /></div><div><div className="flex justify-between"><Label htmlFor="password">密码</Label><Link href="/forgot-password" className="text-xs text-blue-700">忘记密码？</Link></div><PasswordInput id="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} className="mt-2 h-11" /></div><Button type="submit" disabled={loading || !email || !password} className="h-11 w-full">{loading ? "登录中…" : "进入工作台"}</Button></form><p className="mt-6 text-center text-sm text-slate-600">还没有账号？ <Link href="/register" className="text-foreground underline underline-offset-4">创建账号</Link></p></AuthShell>;
}
