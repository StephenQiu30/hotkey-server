import { describe, expect, it } from "vitest";
import {
  evidenceStanceLabel,
  getRadarEventTitle,
  reasonLabel,
  trendLabel,
} from "@/lib/radarPresentation";

describe("radarPresentation", () => {
  it("prefers Chinese titles and falls back without exposing an empty label", () => {
    expect(getRadarEventTitle({ title_zh: "中文标题", title_en: "English" })).toBe(
      "中文标题",
    );
    expect(getRadarEventTitle({ title_en: "English" })).toBe("English");
    expect(getRadarEventTitle({ event_id: 12 })).toBe("事件 #12");
  });

	it("turns backend enums into concise Chinese product language", () => {
	  expect(trendLabel("rising")).toBe("升温中");
	  expect(reasonLabel("source_breadth_growing")).toBe("来源覆盖正在扩大");
	  expect(reasonLabel("confirmation_changed")).toBe("证据集合发生变化");
	  expect(reasonLabel("unknown_reason")).toBe("出现新的重要变化");
	});

	it("keeps evidence relations descriptive rather than probabilistic", () => {
	  expect(evidenceStanceLabel("contradicts")).toBe("反驳");
	});
});
