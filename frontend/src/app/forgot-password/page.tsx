"use client";

import { useRef } from "react";
import Link from "next/link";
import { useGSAP } from "@gsap/react";
import gsap from "gsap";
import { ArrowLeft } from "lucide-react";
import AuthShell from "@/components/auth/AuthShell";
import EmailVerificationStep from "@/components/auth/EmailVerificationStep";
import { VerificationFlow } from "@/lib/domainEnums";

export default function ForgotPasswordPage() {
  const containerRef = useRef<HTMLDivElement>(null);

  useGSAP(() => { gsap.from(".fp-fade", { y: 10, duration: 0.4, ease: "power3.out" }); }, { scope: containerRef });

  const handleConfirmed = (_ticket: string) => {
    sessionStorage.setItem("verification_ticket", _ticket);
    window.location.href = "/reset-password";
  };

  return (
    <div ref={containerRef}>
      <AuthShell title="找回密码" subtitle="验证邮箱后重置密码">
        <div className="fp-fade"><EmailVerificationStep purpose={VerificationFlow.PasswordReset} onConfirmed={handleConfirmed} /></div>
        <div className="mt-6 text-center">
          <Link href="/login" className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground">
            <ArrowLeft aria-hidden="true" className="h-3.5 w-3.5" /> 返回登录
          </Link>
        </div>
      </AuthShell>
    </div>
  );
}
