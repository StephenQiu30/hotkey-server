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
        className="space-y-5"
        onSubmit={(event) => {
          event.preventDefault();
          void handleSend();
        }}
      >
        <div className="space-y-2">
          <Label htmlFor="verify-email">邮箱</Label>
          <div className="relative">
            <Mail className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input id="verify-email" type="email" placeholder="name@example.com" autoComplete="email" required
              autoCapitalize="none" spellCheck={false}
              value={email} onChange={(e) => { setEmail(e.target.value); setSendError(""); }}
              aria-invalid={Boolean(sendError)}
              aria-describedby={sendError ? "verify-email-error" : undefined}
              className="h-11 pl-9" />
          </div>
          {sendError && <p id="verify-email-error" role="alert" className="text-sm leading-5 text-destructive">{sendError}</p>}
        </div>
        <Button type="submit" disabled={!email || loading} className="h-11 w-full disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100">
          {loading ? "发送中…" : "发送验证码"}
        </Button>
      </form>
    );
  }

  return (
    <form
      className="space-y-5"
      onSubmit={(event) => {
        event.preventDefault();
        void handleConfirm();
      }}
    >
      <p role="status" aria-live="polite" className="text-sm leading-6 text-muted-foreground">
        验证码已发送至
        <span className="block break-all font-medium text-foreground">{email}</span>
      </p>
      <div className="space-y-2">
        <Label htmlFor="verify-code">验证码</Label>
        <Input id="verify-code" placeholder="输入 6 位验证码" maxLength={6} inputMode="numeric"
          autoComplete="one-time-code"
          value={code} onChange={(e) => { setCode(e.target.value.replace(/\D/g, "")); setSendError(""); }}
          aria-invalid={Boolean(sendError)}
          aria-describedby={sendError ? "verify-code-error" : undefined}
          className="h-11 text-center font-mono text-base tracking-[0.24em] placeholder:font-sans placeholder:text-sm placeholder:tracking-normal" />
        {sendError && <p id="verify-code-error" role="alert" className="text-sm leading-5 text-destructive">{sendError}</p>}
      </div>
      <Button type="submit" disabled={code.length !== 6 || loading} className="h-11 w-full disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100">
        {loading ? "验证中…" : "验证"}
      </Button>
      <Button
        type="button"
        variant="link"
        size="sm"
        onClick={() => void handleSend()}
        disabled={countdown > 0}
        className="h-auto min-h-8 w-full p-0 text-sm text-muted-foreground hover:text-foreground disabled:text-muted-foreground disabled:opacity-100"
      >
        {countdown > 0 ? `${countdown}秒后可重新发送` : "重新发送验证码"}
      </Button>
    </form>
  );
}
