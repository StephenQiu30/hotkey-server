import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import EmailVerificationStep from "@/components/auth/EmailVerificationStep";
import { VerificationFlow } from "@/lib/domainEnums";

const mocks = vi.hoisted(() => ({
  sendVerification: vi.fn(),
  confirmVerification: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/identity", () => ({
  postAuthEmailVerifications: mocks.sendVerification,
  postAuthEmailVerificationsConfirm: mocks.confirmVerification,
}));

describe("EmailVerificationStep", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.sendVerification.mockResolvedValue({ data: {} });
  });

  it("does not send a verification request for an invalid email", async () => {
    const user = userEvent.setup();
    render(
      <EmailVerificationStep
        purpose={VerificationFlow.Registration}
        onConfirmed={vi.fn()}
      />,
    );

    await user.type(screen.getByRole("textbox", { name: "邮箱" }), "invalid-email");
    await user.click(screen.getByRole("button", { name: "发送验证码" }));

    expect(mocks.sendVerification).not.toHaveBeenCalled();
  });

  it("submits a valid email when the user presses Enter", async () => {
    const user = userEvent.setup();
    render(
      <EmailVerificationStep
        purpose={VerificationFlow.PasswordReset}
        onConfirmed={vi.fn()}
      />,
    );

    await user.type(
      screen.getByRole("textbox", { name: "邮箱" }),
      "reader@example.com{enter}",
    );

    await waitFor(() => {
      expect(mocks.sendVerification).toHaveBeenCalledWith({
        email: "reader@example.com",
        purpose: "password_reset",
      });
    });
  });
});
