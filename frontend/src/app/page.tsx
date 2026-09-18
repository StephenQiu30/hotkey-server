import { connection } from "next/server";

import { FoundationHome } from "@/features/foundation/components/foundation-home";

export default async function Home() {
  await connection();

  return <FoundationHome />;
}
