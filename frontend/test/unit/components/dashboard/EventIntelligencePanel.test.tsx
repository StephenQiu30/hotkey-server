import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { EventIntelligencePanel } from "@/components/dashboard/EventIntelligencePanel";

const event: HotKeyAPI.RadarEventResponse = {
  event_id: 11,
  attention: 86,
  momentum: 91,
  breadth: 75,
  independent_source_count: 3,
};

const intelligence: HotKeyAPI.EventIntelligenceResponse = {
  event_id: 11,
  claims: [
    {
      id: 1,
      normalized_claim: "首条可核查声明",
      evidence: [
        {
          content_id: 101,
          stance: "supporting",
          excerpt: "首条声明的支持证据",
        },
      ],
    },
    {
      id: 2,
      normalized_claim: "第二条可核查声明",
      evidence: [
        {
          content_id: 102,
          stance: "contradicting",
          excerpt: "第二条声明的反向证据",
        },
      ],
    },
  ],
};

describe("EventIntelligencePanel", () => {
  it("separates source coverage, importance and monitor relevance", () => {
	render(<EventIntelligencePanel event={event} />);

	expect(screen.getByText("3 个独立来源")).toBeInTheDocument();
	expect(screen.getByText(/不据此判断事件真假或来源可信度/)).toBeInTheDocument();
	expect(screen.getByRole("progressbar", { name: "传播动量 91" })).toBeInTheDocument();
	expect(screen.getByText("选择监控后查看事件与监控规则的相关程度。")).toBeInTheDocument();
	expect(screen.queryByText(/置信度|已核实|多源印证/)).not.toBeInTheDocument();
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

  it("organizes claims as a keyboard-accessible Radix accordion", async () => {
    const user = userEvent.setup();
    render(
      <EventIntelligencePanel event={event} intelligence={intelligence} />
    );

    const firstClaim = screen.getByRole("button", {
      name: /首条可核查声明/,
    });
    const secondClaim = screen.getByRole("button", {
      name: /第二条可核查声明/,
    });

    expect(firstClaim).toHaveAttribute("data-state", "open");
    expect(secondClaim).toHaveAttribute("data-state", "closed");
    expect(screen.getByText("首条声明的支持证据")).toBeVisible();
    expect(screen.queryByText("第二条声明的反向证据")).not.toBeInTheDocument();

    secondClaim.focus();
    await user.keyboard("{Enter}");

    expect(secondClaim).toHaveAttribute("data-state", "open");
    expect(screen.getByText("第二条声明的反向证据")).toBeVisible();
  });
});
