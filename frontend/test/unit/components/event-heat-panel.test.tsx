import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { EventHeatPanel } from "@/components/dashboard/EventHeatPanel";

describe("EventHeatPanel", () => {
  afterEach(cleanup);

  it("distinguishes unavailable engagement from a zero score", () => {
    render(
      <EventHeatPanel
        loading={false}
        error={false}
        heat={{
          heat_score: 62,
          components: {
            independence: 25,
            content_velocity: 20,
            source_breadth: 50,
            recency: 97,
            credibility: 80,
          },
        }}
      />
    );

    expect(screen.getByText("互动表现不可用")).toBeInTheDocument();
    expect(screen.getByText(/并未按 0 分处理/)).toBeInTheDocument();
  });

  it("labels a legacy aggregate instead of inventing components", () => {
    render(
      <EventHeatPanel loading={false} error={false} heat={{ heat_score: 42 }} />
    );

    expect(screen.getByText("组成尚未记录")).toBeInTheDocument();
    expect(screen.getByText(/旧版热度快照/)).toBeInTheDocument();
  });
});
