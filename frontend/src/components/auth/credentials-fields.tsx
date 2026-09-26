"use client";

import { type ReactNode, useState } from "react";
import { EyeIcon, EyeOffIcon } from "lucide-react";

import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";

type PasswordFieldProps = {
  id: string;
  name: string;
  label: string;
  autoComplete: "current-password" | "new-password";
  disabled: boolean;
  description?: string;
  invalid?: boolean;
  error?: string;
  onChange?: () => void;
};

export function PasswordField({
  id,
  name,
  label,
  autoComplete,
  disabled,
  description,
  invalid = false,
  error,
  onChange,
}: PasswordFieldProps) {
  const [isVisible, setIsVisible] = useState(false);
  const descriptionId = description ? `${id}-description` : undefined;
  const errorId = error ? `${id}-error` : undefined;

  return (
    <Field data-disabled={disabled} data-invalid={invalid}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <InputGroup className="h-11">
        <InputGroupInput
          id={id}
          name={name}
          type={isVisible ? "text" : "password"}
          autoComplete={autoComplete}
          minLength={12}
          maxLength={128}
          required
          disabled={disabled}
          aria-invalid={invalid}
          aria-describedby={errorId ?? descriptionId}
          onChange={onChange}
          className="h-full"
        />
        <InputGroupAddon align="inline-end">
          <InputGroupButton
            aria-label={isVisible ? `隐藏${label}` : `显示${label}`}
            aria-pressed={isVisible}
            disabled={disabled}
            onClick={() => setIsVisible((visible) => !visible)}
          >
            {isVisible ? (
              <EyeOffIcon aria-hidden="true" />
            ) : (
              <EyeIcon aria-hidden="true" />
            )}
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
      {description ? (
        <FieldDescription id={descriptionId}>{description}</FieldDescription>
      ) : null}
      {error ? <FieldError id={errorId}>{error}</FieldError> : null}
    </Field>
  );
}

type CredentialsFieldsProps = {
  disabled: boolean;
  passwordAutoComplete: "current-password" | "new-password";
  children?: ReactNode;
};

export function CredentialsFields({
  disabled,
  passwordAutoComplete,
  children,
}: CredentialsFieldsProps) {
  return (
    <FieldGroup>
      <Field data-disabled={disabled}>
        <FieldLabel htmlFor="username">用户名</FieldLabel>
        <Input
          id="username"
          name="username"
          autoComplete="username"
          minLength={3}
          maxLength={64}
          required
          disabled={disabled}
          placeholder="输入用户名"
          className="h-11"
        />
      </Field>
      <PasswordField
        id="password"
        name="password"
        label="密码"
        autoComplete={passwordAutoComplete}
        disabled={disabled}
        description={
          passwordAutoComplete === "new-password"
            ? "至少 12 个字符。"
            : undefined
        }
      />
      {children}
    </FieldGroup>
  );
}
