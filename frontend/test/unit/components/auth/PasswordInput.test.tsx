import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import PasswordInput from "@/components/auth/PasswordInput";
import { Label } from "@/components/ui/label";

describe("PasswordInput", () => {
  it("允许用户显示并再次隐藏密码", async () => {
    const user = userEvent.setup();
    render(
      <div>
        <Label htmlFor="test-password">密码</Label>
        <PasswordInput id="test-password" defaultValue="Secret123" />
      </div>,
    );

    const input = screen.getByLabelText("密码");
    expect(input).toHaveAttribute("type", "password");

    await user.click(screen.getByRole("button", { name: "显示密码" }));
    expect(input).toHaveAttribute("type", "text");
    expect(screen.getByRole("button", { name: "隐藏密码" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );

    await user.click(screen.getByRole("button", { name: "隐藏密码" }));
    expect(input).toHaveAttribute("type", "password");
  });
});
