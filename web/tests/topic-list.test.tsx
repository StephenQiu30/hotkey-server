import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { TopicList } from "@/components/topic-list";

describe("TopicList", () => {
  it("shows topic title and trend direction", () => {
    render(
      <TopicList
        topics={[
          {
            id: 1,
            title: "Agent launch",
            currentHeat: 123,
            trendDirection: "up",
          },
        ]}
      />
    );
    expect(screen.getByText("Agent launch")).toBeInTheDocument();
    expect(screen.getByText("up")).toBeInTheDocument();
  });

  it("renders multiple topics", () => {
    render(
      <TopicList
        topics={[
          {
            id: 1,
            title: "Agent launch",
            currentHeat: 123,
            trendDirection: "up",
          },
          {
            id: 2,
            title: "API pricing",
            currentHeat: 45,
            trendDirection: "down",
          },
        ]}
      />
    );
    expect(screen.getByText("Agent launch")).toBeInTheDocument();
    expect(screen.getByText("API pricing")).toBeInTheDocument();
  });
});
