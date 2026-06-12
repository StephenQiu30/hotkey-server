import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { AdminRunTable } from "@/components/admin-run-table";

describe("AdminRunTable", () => {
  it("renders run status and fetched count", () => {
    render(
      <AdminRunTable
        runs={[{ id: 1, status: "failed", fetchedCount: 0 }]}
      />
    );
    expect(screen.getByText("failed")).toBeInTheDocument();
    expect(screen.getByText("0")).toBeInTheDocument();
  });

  it("renders multiple runs", () => {
    render(
      <AdminRunTable
        runs={[
          { id: 1, status: "failed", fetchedCount: 0 },
          { id: 2, status: "success", fetchedCount: 42 },
        ]}
      />
    );
    expect(screen.getByText("failed")).toBeInTheDocument();
    expect(screen.getByText("success")).toBeInTheDocument();
    expect(screen.getByText("42")).toBeInTheDocument();
  });
});
