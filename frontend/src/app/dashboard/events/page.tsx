import { redirect } from "next/navigation";

// Preserve historical notification links while the product surface uses the
// single persisted hotspot radar instead of the legacy MicroEvent workspace.
export default function LegacyEventsRedirect() {
  redirect("/dashboard/contents");
}
