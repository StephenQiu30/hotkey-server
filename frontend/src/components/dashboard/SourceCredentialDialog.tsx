"use client";

import { type FormEvent, useState } from "react";
import { KeyRound, Loader2 } from "lucide-react";
import PasswordInput from "@/components/auth/PasswordInput";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";

type Props = {
  busy: boolean;
  sourceName: string;
  onSubmit: (credential: string) => Promise<boolean>;
};

export function SourceCredentialDialog({ busy, sourceName, onSubmit }: Props) {
  const [open, setOpen] = useState(false);
  const [credential, setCredential] = useState("");
  const byteLength = new TextEncoder().encode(credential).length;
  const valid = Boolean(credential.trim()) && byteLength <= 16 * 1024;

  const changeOpen = (next: boolean) => {
    setOpen(next);
    if (!next) setCredential("");
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!valid || busy) return;
    if (await onSubmit(credential)) changeOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" className="gap-1.5">
          <KeyRound />
          替换凭据
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>替换来源凭据</DialogTitle>
            <DialogDescription>
              为“{sourceName}”写入新凭据。旧值不会显示，新值保存后会立即使旧凭据失效。
            </DialogDescription>
          </DialogHeader>
          <div className="py-5">
            <Label htmlFor="replacement-source-credential">新访问凭据</Label>
            <PasswordInput
              id="replacement-source-credential"
              className="mono mt-2"
              value={credential}
              onChange={(event) => setCredential(event.target.value)}
              autoComplete="new-password"
              maxLength={16 * 1024}
              placeholder="粘贴新的 API Key、Token 或 OAuth 凭据包"
              aria-invalid={credential.length > 0 && !valid}
            />
            <Alert className="mt-4">
              <KeyRound />
              <AlertDescription>
                新值只会提交一次并由服务端认证加密；页面、日志和 API 响应均不会回显。保存后请重新执行健康探测。
              </AlertDescription>
            </Alert>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={() => changeOpen(false)}
            >
              取消
            </Button>
            <Button type="submit" disabled={!valid || busy}>
              {busy && <Loader2 className="animate-spin" />}
              保存并替换
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
