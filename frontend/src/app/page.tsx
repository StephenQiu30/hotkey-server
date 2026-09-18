import { connection } from "next/server";

import { FoundationHome } from "@/app/components/foundation-home";

export default async function Home() {
  await connection();

  return <FoundationHome />;
}
