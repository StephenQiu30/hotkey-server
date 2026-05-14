"use client";

import { ShieldCheck } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { clearAuthToken, setAuthToken } from "@/lib/api";

export default function AuthCallbackPage() {
  const router = useRouter();
  const [message, setMessage] = useState("正在完成登录 ...");

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const token = params.get("token");
    const error = params.get("error");
    if (error) {
      setMessage(`登录失败：${error}`);
      return;
    }
    if (!token) {
      setMessage("未检测到登录凭证，请重新发起登录。");
      return;
    }
    clearAuthToken();
    setAuthToken(token);
    setMessage("登录成功，即将进入工作台...");
    router.replace("/app");
  }, [router]);

  return (
    <main className="min-h-screen bg-background">
      <section className="mx-auto flex min-h-screen w-full max-w-md items-center justify-center px-4">
        <Card className="ios-shell-card w-full">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <ShieldCheck className="h-5 w-5 text-primary" />
              GitHub 登录回调
            </CardTitle>
          </CardHeader>
            <CardContent className="space-y-4">
              <p className="text-sm text-muted-foreground">{message}</p>
              <Button asChild variant="secondary" className="w-full">
                <Link href="/login">返回登录页</Link>
              </Button>
            </CardContent>
        </Card>
      </section>
    </main>
  );
}
