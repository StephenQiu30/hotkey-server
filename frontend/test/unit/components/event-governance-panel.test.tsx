import { cleanup, render, screen } from "@testing-library/react";
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

    await user.click(screen.getByRole("combobox", { name: "合并目标" }));
    expect(
      screen.queryByRole("option", { name: "已归档目标" })
    ).not.toBeInTheDocument();
    await user.click(screen.getByRole("option", { name: "目标事件" }));
    await user.click(screen.getByRole("button", { name: "合并" }));
    await user.click(screen.getByRole("button", { name: "确认合并" }));
    expect(actions.onMerge).toHaveBeenCalledWith(target);

    await user.click(screen.getAllByRole("checkbox")[0]);
    await user.click(screen.getByRole("button", { name: "拆分" }));
    await user.click(screen.getByRole("button", { name: "确认拆分" }));
    expect(actions.onSplit).toHaveBeenCalledWith([members[0]]);
  });

  it("keeps terminal events read-only even for an admin", () => {
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
    expect(screen.getByRole("combobox", { name: "合并目标" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "锁定内容 31" })).toBeDisabled();
  });
});
