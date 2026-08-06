import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { EventHeatComparison } from "@/components/dashboard/EventHeatComparison";

describe("EventHeatComparison", () => {
  it("uses an explicit empty state instead of rendering a zero-only chart", () => {
    render(
      <EventHeatComparison
        events={[
          { id: 1, title_en: "First event", heat_score: 0 },
          { id: 2, title_en: "Second event" },
        ]}
      />,
    );

    expect(screen.getByText("热度尚未计算")).toBeInTheDocument();
    expect(screen.queryByTestId("event-heat-chart")).not.toBeInTheDocument();
  });

  it("mounts the comparison only when at least one score is positive", () => {
    render(
      <EventHeatComparison
        events={[
          { id: 1, title_en: "First event", heat_score: 42 },
          { id: 2, title_en: "Second event", heat_score: 0 },
        ]}
      />,
    );

    expect(screen.getByRole("heading", { name: "事件热度对比" })).toBeInTheDocument();
    expect(screen.getByTestId("event-heat-chart")).toBeInTheDocument();
  });
});
