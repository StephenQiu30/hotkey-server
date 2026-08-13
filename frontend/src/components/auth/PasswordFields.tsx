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
  const errorId = `${prefix}-confirm-error`;
  const requirementsId = `${prefix}-requirements`;

  return (
    <div className="space-y-5">
      <div className="space-y-2">
        <Label htmlFor={`${prefix}-password`}>密码</Label>
        <PasswordInput id={`${prefix}-password`} placeholder="至少 8 位，含大小写字母和数字"
          autoComplete="new-password"
          value={onPasswordChange ? password : localPassword}
          onChange={(e) => handlePasswordChange(e.target.value)}
          aria-describedby={requirementsId}
          className="h-11" />
        <p id={requirementsId} className="text-xs leading-5 text-muted-foreground">至少 8 位，含大小写字母和数字</p>
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${prefix}-confirm`}>确认密码</Label>
        <PasswordInput id={`${prefix}-confirm`} placeholder="再次输入密码"
          autoComplete="new-password"
          value={onConfirmChange ? confirmPassword : localConfirm}
          onChange={(e) => handleConfirmChange(e.target.value)}
          aria-invalid={Boolean(displayError)}
          aria-describedby={displayError ? errorId : undefined}
          className="h-11" />
        {displayError && <p id={errorId} role="alert" className="text-sm leading-5 text-destructive">{displayError}</p>}
      </div>
    </div>
  );
}
