import type { Metadata } from "next";
import { connection } from "next/server";

import { LoginForm } from "@/app/login/components/login-form";

export const metadata: Metadata = {
  title: "登录",
  robots: {
    index: false,
    follow: false,
  },
};

export default async function LoginPage() {
  await connection();

  return <LoginForm />;
}
