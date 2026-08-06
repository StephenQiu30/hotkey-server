"use client";

import { useState, useEffect, useRef } from "react";
import { useGSAP } from "@gsap/react";
import gsap from "gsap";
import { Button } from "@/components/ui/button";
import { CheckCircle } from "lucide-react";
import AuthShell from "@/components/auth/AuthShell";
import PasswordFields from "@/components/auth/PasswordFields";
import { postAuthPasswordResetsConfirm } from "@/services/hotkey/hotkey-server/identity";
import { toast } from "sonner";

export default function ResetPasswordPage() {
  const [ticket, setTicket] = useState("");
  const [loading, setLoading] = useState(false);
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  useGSAP(() => { gsap.from(".rp-fade", { y: 10, opacity: 0, duration: 0.4, ease: "power3.out" }); }, { scope: containerRef });

  useEffect(() => {
    const stored = sessionStorage.getItem("verification_ticket");
    if (!stored) { window.location.href = "/forgot-password"; return; }
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

  if (!ticket) return null;

  if (success) {
    return (
      <AuthShell title="密码已重置" subtitle="请使用新密码登录">
        <div className="text-center">
          <CheckCircle className="mx-auto mb-3 h-8 w-8 text-primary" />
          <p className="mb-5 text-xs text-muted-foreground">密码重置成功</p>
          <a href="/login"><Button className="h-10 w-full rounded-md text-sm font-medium shadow-button">前往登录</Button></a>
        </div>
      </AuthShell>
    );
  }

  return (
    <div ref={containerRef}>
      <AuthShell title="设置新密码" subtitle="请输入新密码">
        <div className="rp-fade">
          <form onSubmit={handleReset} className="space-y-3">
            <PasswordFields prefix="reset" password={password} confirmPassword={confirmPassword}
              onPasswordChange={setPassword} onConfirmChange={setConfirmPassword} />
            {error && <p className="text-xs text-red-700">{error}</p>}
            <Button type="submit" disabled={loading || !password} className="h-10 w-full rounded-md text-sm font-medium shadow-button">
              {loading ? "重置中..." : "重置密码"}
            </Button>
          </form>
        </div>
      </AuthShell>
    </div>
  );
}
