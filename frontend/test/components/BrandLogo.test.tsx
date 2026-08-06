import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { BrandLogo } from "@/components/BrandLogo";

describe("BrandLogo", () => {
  it("renders the approved radar mark with the product name", () => {
    render(<BrandLogo />);

    expect(screen.getByRole("img", { name: "HotKey" })).toBeInTheDocument();
    expect(screen.getByText("HotKey")).toBeInTheDocument();
  });

  it("supports a custom product title", () => {
    render(<BrandLogo title="HotKey Studio" />);

    expect(screen.getByText("HotKey Studio")).toBeInTheDocument();
  });
});
