"use client";

import { useState, useRef } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useGSAP } from "@gsap/react";
import gsap from "gsap";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { User, ArrowLeft } from "lucide-react";
import { toast } from "sonner";
import AuthShell from "@/components/auth/AuthShell";
import EmailVerificationStep from "@/components/auth/EmailVerificationStep";
import PasswordFields from "@/components/auth/PasswordFields";
import { postAuthRegistrations } from "@/services/hotkey/hotkey-server/identity";
import { createRegisterRequest } from "@/lib/registerRequest";
import { APIErrorCode, RegistrationStep, VerificationFlow } from "@/lib/domainEnums";
import { HotKeyAPIError } from "@/lib/request";

type RegistrationRecovery = "conflict" | "verification" | null;

export default function RegisterPage() {
  const [step, setStep] = useState<RegistrationStep>(RegistrationStep.Email);
  const [ticket, setTicket] = useState("");
  const [loading, setLoading] = useState(false);
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState("");
  const [recovery, setRecovery] = useState<RegistrationRecovery>(null);
  const router = useRouter();
  const containerRef = useRef<HTMLDivElement>(null);

  useGSAP(() => {
    gsap.from(".rg-fade", { y: 10, duration: 0.4, ease: "power3.out" });
  }, { scope: containerRef, dependencies: [step] });

  const handleConfirmed = (tkt: string) => {
    setTicket(tkt); setError(""); setRecovery(null); setStep(RegistrationStep.Profile);
  };

  const restartVerification = () => {
    setTicket(""); setPassword(""); setConfirmPassword("");
    setError(""); setRecovery(null); setStep(RegistrationStep.Email);
  };

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!displayName) { setError("请输入显示名称"); return; }
    if (password.length < 8) { setError("密码至少 8 位"); return; }
    if (password !== confirmPassword) { setError("两次输入的密码不一致"); return; }
    setLoading(true); setError("");
    try {
      const response = await postAuthRegistrations(createRegisterRequest(ticket, password, displayName));
      if (!response.data) throw new Error("注册响应无效");
      toast.success("注册成功，请登录");
      router.push("/login");
    } catch (err: unknown) {
      if (err instanceof HotKeyAPIError && err.code === APIErrorCode.VersionConflict) {
        setPassword(""); setConfirmPassword("");
        setRecovery("conflict");
        setError("该邮箱已注册，请直接登录或找回密码。");
      } else if (err instanceof HotKeyAPIError && err.code === APIErrorCode.InvalidVerification) {
        setPassword(""); setConfirmPassword("");
        setRecovery("verification");
        setError("验证凭证已失效，请重新验证邮箱。");
      } else {
        setError(err instanceof Error ? err.message : "注册失败，请稍后重试");
      }
    }
    finally { setLoading(false); }
  };

  return (
    <div ref={containerRef}>
      <AuthShell title="创建账号" subtitle="建立你的第一条可追溯监控">
        <div className="rg-fade">
          {step === RegistrationStep.Email && <EmailVerificationStep purpose={VerificationFlow.Registration} onConfirmed={handleConfirmed} />}

          {step === RegistrationStep.Profile && (
            <form onSubmit={handleRegister} className="space-y-5">
              <div className="space-y-2">
                <Label htmlFor="display-name">显示名称</Label>
                <div className="relative">
                  <User aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <Input id="display-name" placeholder="您的昵称" value={displayName}
                    onChange={(e) => setDisplayName(e.target.value)}
                    className="h-11 pl-9" />
                </div>
              </div>
              <PasswordFields prefix="register" password={password} confirmPassword={confirmPassword}
                onPasswordChange={setPassword} onConfirmChange={setConfirmPassword} />
              {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
              {recovery === "conflict" && (
                <div className="flex items-center gap-4 text-sm">
                  <Link href="/login" className="font-medium text-primary hover:text-primary/80">去登录</Link>
                  <Link href="/forgot-password" className="text-muted-foreground hover:text-foreground">找回密码</Link>
                </div>
              )}
              {recovery === "verification" && (
                <Button type="button" variant="outline" onClick={restartVerification} className="h-11 w-full">
                  重新验证邮箱
                </Button>
              )}
              <Button type="submit" disabled={loading || recovery !== null} className="h-11 w-full disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100">
                {loading ? "注册中…" : "完成注册"}
              </Button>
            </form>
          )}
        </div>
        <div className="mt-6 text-center">
          <Link href="/login" className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground">
            <ArrowLeft aria-hidden="true" className="h-3.5 w-3.5" /> 已有账号？去登录
          </Link>
        </div>
      </AuthShell>
    </div>
  );
}
