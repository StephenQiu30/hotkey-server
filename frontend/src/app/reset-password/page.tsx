"use client";

import { useState, useEffect, useRef } from "react";
import Link from "next/link";
import { useGSAP } from "@gsap/react";
import gsap from "gsap";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { CheckCircle } from "lucide-react";
import AuthShell from "@/components/auth/AuthShell";
import PasswordFields from "@/components/auth/PasswordFields";
import { postAuthPasswordResetsConfirm } from "@/services/hotkey/hotkey-server/identity";
import { toast } from "sonner";

export default function ResetPasswordPage() {
  const [ticket, setTicket] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  useGSAP(() => { gsap.from(".rp-fade", { y: 10, opacity: 0, duration: 0.4, ease: "power3.out" }); }, { scope: containerRef });

  useEffect(() => {
    const stored = sessionStorage.getItem("verification_ticket");
    if (!stored) { window.location.replace("/forgot-password"); return; }
    setTicket(stored);
    sessionStorage.removeItem("verification_ticket");
  }, []);

  const handleReset = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!ticket) return;
    if (password.length < 8) { setError("密码至少 8 位"); return; }
    if (password !== confirmPassword) { setError("两次输入的密码不一致"); return; }
    setLoading(true); setError("");
    try {
      await postAuthPasswordResetsConfirm({ verification_ticket: ticket, password });
      setSuccess(true);
      toast.success("密码已重置");
    } catch (err: any) { setError(err.message ?? "密码重置失败"); }
    finally { setLoading(false); }
  };

  if (!ticket) {
    return (
      <AuthShell title="设置新密码" subtitle="正在验证重置凭证">
        <div aria-busy="true" aria-label="正在加载" className="space-y-5">
          <div className="space-y-2">
            <Skeleton className="h-4 w-16" />
            <Skeleton className="h-11 w-full" />
          </div>
          <div className="space-y-2">
            <Skeleton className="h-4 w-20" />
            <Skeleton className="h-11 w-full" />
          </div>
          <Skeleton className="h-11 w-full" />
        </div>
      </AuthShell>
    );
  }

  if (success) {
    return (
      <AuthShell title="密码已重置" subtitle="请使用新密码登录">
        <div role="status" className="text-center">
          <CheckCircle aria-hidden="true" className="mx-auto mb-3 h-8 w-8 text-primary" />
          <p className="mb-5 text-sm text-muted-foreground">密码重置成功</p>
          <Button asChild className="h-11 w-full">
            <Link href="/login">前往登录</Link>
          </Button>
        </div>
      </AuthShell>
    );
  }

  return (
    <div ref={containerRef}>
      <AuthShell title="设置新密码" subtitle="请输入新密码">
        <div className="rp-fade">
          <form onSubmit={handleReset} className="space-y-5">
            <PasswordFields prefix="reset" password={password} confirmPassword={confirmPassword}
              onPasswordChange={setPassword} onConfirmChange={setConfirmPassword} />
            {error && <p role="alert" className="text-sm leading-5 text-destructive">{error}</p>}
            <Button type="submit" disabled={loading || !password} className="h-11 w-full disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100">
              {loading ? "重置中…" : "重置密码"}
            </Button>
          </form>
        </div>
      </AuthShell>
    </div>
  );
}
