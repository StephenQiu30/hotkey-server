import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MonitorList } from "@/components/monitor-list";

describe("MonitorList", () => {
  it("renders monitor cards", () => {
    render(
      <MonitorList
        monitors={[
          { id: 1, name: "OpenAI", queryText: "openai agent" },
        ]}
      />
    );
    expect(screen.getByText("OpenAI")).toBeInTheDocument();
    expect(screen.getByText("openai agent")).toBeInTheDocument();
  });

  it("renders multiple monitors", () => {
    render(
      <MonitorList
        monitors={[
          { id: 1, name: "OpenAI", queryText: "openai agent" },
          { id: 2, name: "Anthropic", queryText: "claude" },
        ]}
      />
    );
    expect(screen.getByText("OpenAI")).toBeInTheDocument();
    expect(screen.getByText("Anthropic")).toBeInTheDocument();
  });
});
