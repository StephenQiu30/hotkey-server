import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { APIErrorCode } from "@/lib/domainEnums";
import { HotKeyAPIError } from "@/lib/request";

const mocks = vi.hoisted(() => ({
  postRegistrations: vi.fn(),
  push: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mocks.push }),
}));

vi.mock("@gsap/react", () => ({
  useGSAP: () => undefined,
}));

vi.mock("@/services/hotkey/hotkey-server/identity", () => ({
  postAuthRegistrations: mocks.postRegistrations,
}));

vi.mock("@/components/auth/EmailVerificationStep", () => ({
  default: ({ onConfirmed }: { onConfirmed: (ticket: string, email: string) => void }) => (
    <button type="button" onClick={() => onConfirmed("verified-ticket", "existing@example.test")}>
      模拟邮箱验证成功
    </button>
  ),
}));

import RegisterPage from "@/app/register/page";

describe("RegisterPage", () => {
  beforeEach(() => {
    mocks.postRegistrations.mockReset();
    mocks.push.mockReset();
  });

  async function reachProfileStep() {
    const user = userEvent.setup();
    render(<RegisterPage />);
    await user.click(screen.getByRole("button", { name: "模拟邮箱验证成功" }));
    await user.type(screen.getByLabelText("显示名称"), "Existing User");
    await user.type(screen.getByLabelText("密码", { selector: "input" }), "DemoPass123");
    await user.type(screen.getByLabelText("确认密码", { selector: "input" }), "DemoPass123");
    return user;
  }

  it("邮箱冲突时明确提示已有账号并阻止重复提交", async () => {
    mocks.postRegistrations.mockRejectedValue(
      new HotKeyAPIError(409, "conflict", null, APIErrorCode.VersionConflict),
    );
    const user = await reachProfileStep();

    await user.click(screen.getByRole("button", { name: "完成注册" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "该邮箱已注册，请直接登录或找回密码。",
    );
    expect(screen.getByRole("link", { name: "去登录" })).toHaveAttribute("href", "/login");
    expect(screen.getByRole("link", { name: "找回密码" })).toHaveAttribute(
      "href",
      "/forgot-password",
    );
    expect(screen.getByRole("button", { name: "完成注册" })).toBeDisabled();
  });

  it("验证凭证失效时允许返回邮箱验证步骤", async () => {
    mocks.postRegistrations.mockRejectedValue(
      new HotKeyAPIError(400, "verification invalid", null, APIErrorCode.InvalidVerification),
    );
    const user = await reachProfileStep();

    await user.click(screen.getByRole("button", { name: "完成注册" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "验证凭证已失效，请重新验证邮箱。",
    );

    await user.click(screen.getByRole("button", { name: "重新验证邮箱" }));
    expect(screen.getByRole("button", { name: "模拟邮箱验证成功" })).toBeInTheDocument();
  });
});
