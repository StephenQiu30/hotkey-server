import { DesignPrinciples } from "./design-principles";
import { FoundationOverview } from "./foundation-overview";
import { FoundationStatus } from "./foundation-status";
import { HeroSection } from "./hero-section";
import { SiteHeader } from "./site-header";

export function FoundationHome() {
  return (
    <main className="bg-background min-h-screen overflow-hidden">
      <SiteHeader />
      <HeroSection />
      <FoundationOverview />
      <DesignPrinciples />
      <FoundationStatus />
    </main>
  );
}
