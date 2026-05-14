"use client";

import { ArrowLeft } from "lucide-react";
import Link from "next/link";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { clearAuthToken, getCurrentUser, getAuthLoginUrl } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export default function LoginPage() {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getCurrentUser()
      .then(() => {
        router.replace("/app");
      })
      .catch(() => {
        clearAuthToken();
      });
  }, [router]);

  const handleLogin = async () => {
    try {
      const response = await getAuthLoginUrl();
      window.location.href = response.authorization_url;
    } catch (err) {
      setError(err instanceof Error ? err.message : "启动 GitHub 登录失败。");
    }
  };

  return (
    <main className="min-h-screen bg-background">
      <section className="mx-auto flex min-h-screen w-full max-w-md items-center justify-center px-4">
        <Card className="ios-shell-card w-full">
          <CardHeader>
            <CardTitle>登录 AI Hotspot Radar</CardTitle>
            <CardDescription>请使用 GitHub 账号授权登录后访问工作台。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {error ? <p className="rounded-md bg-destructive/12 p-3 text-sm text-destructive">{error}</p> : null}
            <Button className="w-full" size="lg" onClick={handleLogin}>
              GitHub 登录
            </Button>
            <Button asChild variant="secondary" className="w-full">
              <Link href="/">
                <ArrowLeft className="mr-2 h-4 w-4" />
                回到首页
              </Link>
            </Button>
          </CardContent>
        </Card>
      </section>
    </main>
  );
}
