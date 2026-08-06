"use client";

import { useState, useEffect, useCallback } from "react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Mail } from "lucide-react";
import {
  postAuthEmailVerifications,
  postAuthEmailVerificationsConfirm,
} from "@/services/hotkey/hotkey-server/identity";
import {
  VerificationFlow,
  VerificationPurpose,
  VerificationStep,
} from "@/lib/domainEnums";

interface EmailVerificationStepProps {
  purpose: VerificationFlow;
  onConfirmed: (ticket: string, email: string) => void;
}

export default function EmailVerificationStep({ purpose, onConfirmed }: EmailVerificationStepProps) {
  const apiPurpose =
    purpose === VerificationFlow.Registration
      ? VerificationPurpose.Registration
      : VerificationPurpose.PasswordReset;
  const [step, setStep] = useState<VerificationStep>(VerificationStep.Send);
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [loading, setLoading] = useState(false);
  const [countdown, setCountdown] = useState(0);
  const [sendError, setSendError] = useState("");

  useEffect(() => {
    if (countdown <= 0) return;
    const id = setInterval(() => setCountdown((c) => c - 1), 1000);
    return () => clearInterval(id);
  }, [countdown]);

  const handleSend = useCallback(async () => {
    if (!email) return;
    setLoading(true); setSendError("");
    try {
      await postAuthEmailVerifications({ email, purpose: apiPurpose });
      setStep(VerificationStep.Confirm); setCountdown(60);
    } catch (err: any) { setSendError(err.message ?? "操作失败"); }
    finally { setLoading(false); }
  }, [apiPurpose, email]);

  const handleConfirm = useCallback(async () => {
    if (code.length !== 6) return;
    setLoading(true);
    try {
      const res = await postAuthEmailVerificationsConfirm({ email, purpose: apiPurpose, code });
      const ticket = res.data?.verification_ticket;
      if (ticket) onConfirmed(ticket, email);
    } catch (err: any) { setSendError(err.message ?? "验证失败"); }
    finally { setLoading(false); }
  }, [apiPurpose, email, code, onConfirmed]);

  if (step === VerificationStep.Send) {
    return (
      <form
        className="space-y-3"
        onSubmit={(event) => {
          event.preventDefault();
          void handleSend();
        }}
      >
        <div className="space-y-1.5">
          <Label htmlFor="verify-email" className="text-sm font-medium">邮箱</Label>
          <div className="relative">
            <Mail className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input id="verify-email" type="email" placeholder="name@example.com" autoComplete="email" required
              autoCapitalize="none" spellCheck={false}
              value={email} onChange={(e) => { setEmail(e.target.value); setSendError(""); }}
              className={`h-10 border-border bg-background pl-8 text-sm ${sendError ? "border-destructive" : ""}`} />
          </div>
          {sendError && <p role="alert" className="text-xs text-destructive">{sendError}</p>}
        </div>
        <Button type="submit" disabled={!email || loading} className="h-11 w-full">
          {loading ? "发送中..." : "发送验证码"}
        </Button>
      </form>
    );
  }

  return (
    <form
      className="space-y-3"
      onSubmit={(event) => {
        event.preventDefault();
        void handleConfirm();
      }}
    >
      <p className="text-center text-sm text-muted-foreground">
        验证码已发送至 <span className="font-medium text-foreground">{email}</span>
      </p>
      <div className="space-y-1.5">
        <Label htmlFor="verify-code" className="text-xs font-medium">验证码</Label>
        <Input id="verify-code" placeholder="输入 6 位验证码" maxLength={6} inputMode="numeric"
          autoComplete="one-time-code"
          value={code} onChange={(e) => { setCode(e.target.value.replace(/\D/g, "")); setSendError(""); }}
          className={`h-10 border-border bg-background text-center font-mono text-sm tracking-[0.3em] ${sendError ? "border-destructive" : ""}`} />
        {sendError && <p role="alert" className="text-xs text-destructive">{sendError}</p>}
      </div>
      <Button type="submit" disabled={code.length !== 6 || loading} className="h-11 w-full">
        {loading ? "验证中..." : "验证"}
      </Button>
      <Button
        type="button"
        variant="link"
        size="sm"
        onClick={handleSend}
        disabled={countdown > 0}
        className="w-full text-xs"
      >
        {countdown > 0 ? `${countdown}秒后可重新发送` : "重新发送验证码"}
      </Button>
    </form>
  );
}
