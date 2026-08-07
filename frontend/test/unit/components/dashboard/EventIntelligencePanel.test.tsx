import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { EventIntelligencePanel } from "@/components/dashboard/EventIntelligencePanel";

const event: HotKeyAPI.RadarEventResponse = {
  event_id: 11,
  attention: 86,
  momentum: 91,
  breadth: 75,
  data_confidence: 82,
  confirmation: "disputed",
  confirmation_score: 48,
};

describe("EventIntelligencePanel", () => {
  it("separates evidence confirmation, importance and monitor relevance", () => {
    render(<EventIntelligencePanel event={event} />);

    expect(screen.getByText("存在争议")).toBeInTheDocument();
    expect(screen.getByText(/不是事实为真的概率/)).toBeInTheDocument();
    expect(screen.getByRole("progressbar", { name: "传播动量 91" })).toBeInTheDocument();
    expect(screen.getByText("选择监控后查看事件与监控规则的相关程度。")).toBeInTheDocument();
  });

  it("contains intelligence failures without hiding the event metrics", () => {
    render(
      <EventIntelligencePanel
        event={event}
        intelligenceError
        monitorSelected
      />,
    );

    expect(screen.getByText("事件研判暂时不可用")).toBeInTheDocument();
    expect(screen.getByText("重要性")).toBeInTheDocument();
    expect(screen.getByText("相关性分数等待事件命中该监控后生成。")).toBeInTheDocument();
  });
});
