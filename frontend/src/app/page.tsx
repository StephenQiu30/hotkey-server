import { connection } from "next/server";

import { HomeContent } from "@/app/components/home-content";

export default async function Home() {
  await connection();

  return <HomeContent />;
}
