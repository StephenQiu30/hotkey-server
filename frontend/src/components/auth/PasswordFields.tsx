"use client";

import { useState } from "react";
import { Label } from "@/components/ui/label";
import PasswordInput from "@/components/auth/PasswordInput";

interface PasswordFieldsProps {
  prefix?: string;
  password?: string;
  confirmPassword?: string;
  onPasswordChange?: (value: string) => void;
  onConfirmChange?: (value: string) => void;
  error?: string;
}

export default function PasswordFields({
  prefix = "password", password = "", confirmPassword = "",
  onPasswordChange, onConfirmChange, error,
}: PasswordFieldsProps) {
  const [localPassword, setLocalPassword] = useState(password);
  const [localConfirm, setLocalConfirm] = useState(confirmPassword);
  const [localError, setLocalError] = useState("");

  const handlePasswordChange = (value: string) => {
    setLocalPassword(value); setLocalError(""); onPasswordChange?.(value);
  };
  const handleConfirmChange = (value: string) => {
    setLocalConfirm(value);
    if (value && value !== (onPasswordChange ? password : localPassword)) setLocalError("两次输入的密码不一致");
    else setLocalError("");
    onConfirmChange?.(value);
  };

  const displayError = error || localError;

  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label htmlFor={`${prefix}-password`} className="text-xs font-medium">密码</Label>
        <PasswordInput id={`${prefix}-password`} placeholder="至少 8 位，含大小写字母和数字"
          autoComplete="new-password"
          value={onPasswordChange ? password : localPassword}
          onChange={(e) => handlePasswordChange(e.target.value)}
          className="h-10 rounded-lg border-border bg-slate-50 text-sm" />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`${prefix}-confirm`} className="text-xs font-medium">确认密码</Label>
        <PasswordInput id={`${prefix}-confirm`} placeholder="再次输入密码"
          autoComplete="new-password"
          value={onConfirmChange ? confirmPassword : localConfirm}
          onChange={(e) => handleConfirmChange(e.target.value)}
          className={`h-9 rounded-lg border-border bg-slate-50 text-xs ${displayError ? "border-destructive" : ""}`} />
      </div>
      {displayError && <p className="text-xs text-red-700">{displayError}</p>}
      <p className="text-[11px] text-slate-600">密码要求：至少 8 位，含大小写字母和数字</p>
    </div>
  );
}
