import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { EventGovernancePanel } from "@/components/dashboard/EventGovernancePanel";

const event: HotKeyAPI.RadarEventResponse = {
  event_id: 11,
  version: 3,
  title_zh: "当前事件",
  lifecycle_status: "closed",
};
const target: HotKeyAPI.RadarEventResponse = {
  event_id: 12,
  version: 5,
  title_zh: "目标事件",
  lifecycle_status: "active",
};
const archivedTarget: HotKeyAPI.RadarEventResponse = {
  event_id: 13,
  version: 2,
  title_zh: "已归档目标",
  lifecycle_status: "archived",
};
const members: HotKeyAPI.EventMemberResponse[] = [
  { id: 1, version: 2, event_id: 11, content_id: 31, membership_score: 90 },
  { id: 2, version: 4, event_id: 11, content_id: 32, membership_score: 80 },
];
const manyMembers: HotKeyAPI.EventMemberResponse[] = Array.from(
  { length: 12 },
  (_, index) => ({
    id: index + 1,
    version: 1,
    event_id: 11,
    content_id: index + 100,
    membership_score: 90 - index,
  })
);

function callbacks() {
  return {
    onToggleLock: vi.fn().mockResolvedValue(undefined),
    onLifecycle: vi.fn().mockResolvedValue(undefined),
    onMerge: vi.fn().mockResolvedValue(undefined),
    onSplit: vi.fn().mockResolvedValue(undefined),
  };
}

describe("EventGovernancePanel", () => {
  afterEach(cleanup);

  it("allows an admin to archive a closed event", async () => {
    const actions = callbacks();
    const user = userEvent.setup();
    render(
      <EventGovernancePanel
        event={event}
        events={[event, target, archivedTarget]}
        members={members}
        role="admin"
        loading={false}
        error={false}
        busy={false}
        {...actions}
      />
    );

    await user.click(screen.getByRole("combobox", { name: "目标生命周期" }));
    await user.click(screen.getByRole("option", { name: "已归档" }));
    await user.click(screen.getByRole("button", { name: "应用" }));
    expect(actions.onLifecycle).toHaveBeenCalledWith("archived");
  });

  it("confirms merge and split commands", async () => {
    const actions = callbacks();
    const user = userEvent.setup();
    render(
      <EventGovernancePanel
        event={event}
        events={[event, target, archivedTarget]}
        members={members}
        role="admin"
        loading={false}
        error={false}
        busy={false}
        {...actions}
      />
    );

    expect(
      screen.getByRole("tablist", { name: "治理操作" })
    ).toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "合并事件" }));
    await user.click(screen.getByRole("combobox", { name: "合并目标" }));
    expect(
      screen.queryByRole("option", { name: "已归档目标" })
    ).not.toBeInTheDocument();
    await user.click(screen.getByRole("option", { name: "目标事件" }));
    await user.click(screen.getByRole("button", { name: "合并" }));
    await user.click(screen.getByRole("button", { name: "确认合并" }));
    expect(actions.onMerge).toHaveBeenCalledWith(target);

    await user.click(screen.getAllByRole("checkbox")[0]);
    await user.click(screen.getByRole("tab", { name: "拆分事件" }));
    await user.click(screen.getByRole("button", { name: "拆分" }));
    await user.click(screen.getByRole("button", { name: "确认拆分" }));
    expect(actions.onSplit).toHaveBeenCalledWith([members[0]]);
  });

  it("keeps terminal events read-only even for an admin", async () => {
    const user = userEvent.setup();
    render(
      <EventGovernancePanel
        event={{ ...event, lifecycle_status: "archived" }}
        events={[event, target]}
        members={members}
        role="admin"
        loading={false}
        error={false}
        busy={false}
        {...callbacks()}
      />
    );

    expect(screen.getByRole("combobox", { name: "目标生命周期" })).toBeDisabled();
    await user.click(screen.getByRole("tab", { name: "合并事件" }));
    expect(screen.getByRole("combobox", { name: "合并目标" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "锁定内容 31" })).toBeDisabled();
  });

  it("keeps long member collections inside a labelled Radix scroll area", () => {
    render(
      <EventGovernancePanel
        event={event}
        events={[event, target]}
        members={manyMembers}
        role="admin"
        loading={false}
        error={false}
        busy={false}
        {...callbacks()}
      />
    );

    const memberList = screen.getByRole("region", {
      name: "聚类成员列表",
    });
    expect(memberList).toHaveAttribute("data-slot", "scroll-area");
    expect(
      within(memberList).getAllByRole("link", { name: /内容 #/ })
    ).toHaveLength(12);
    expect(screen.getByRole("tab", { name: "生命周期" })).toHaveAttribute(
      "data-state",
      "active"
    );
  });
});
