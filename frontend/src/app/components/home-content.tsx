import { CapabilityOverview } from "./capability-overview";
import { HeroSection } from "./hero-section";
import { SiteHeader } from "./site-header";

export function HomeContent() {
  return (
    <main className="bg-background min-h-screen overflow-hidden">
      <SiteHeader />
      <HeroSection />
      <CapabilityOverview />
    </main>
  );
}
