import type { Metadata } from "next";
import { connection } from "next/server";

import { RegisterForm } from "@/app/register/components/register-form";

export const metadata: Metadata = {
  title: "初始化账户",
  robots: { index: false, follow: false },
};

export default async function RegisterPage() {
  await connection();

  return <RegisterForm />;
}
