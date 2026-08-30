import { mkdtempSync, readFileSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import {
  buildReleaseImageInventory,
  writeReleaseImageInventory,
} from "./release-image-inventory.mjs";

const revision = "0123456789abcdef0123456789abcdef01234567";

describe("release image inventory", () => {
  it("records three application images and four resolved upstream images", () => {
    let ordinal = 0;
    const inventory = buildReleaseImageInventory(revision, (reference) => {
      ordinal += 1;
      return {
        id: `sha256:${ordinal.toString(16).padStart(64, "0")}`,
        repoDigests: reference.startsWith("hotkey-")
          ? []
          : [`${reference.split(":")[0]}@sha256:${(ordinal + 20).toString(16).padStart(64, "0")}`],
      };
    });

    expect(inventory).toMatchObject({
      version: "hotkey-release-image-inventory-v1",
      git_revision: revision,
      differences: [],
    });
    expect(inventory.applications.map((image) => image.name)).toEqual([
      "backend", "frontend", "python_agent",
    ]);
    expect(inventory.upstream.map((image) => image.name)).toEqual([
      "postgres", "redis", "minio", "minio_client",
    ]);
    expect(inventory.upstream.every((image) => image.repo_digests.length > 0)).toBe(true);
    expect(JSON.stringify(inventory)).not.toContain("Config");
    expect(JSON.stringify(inventory)).not.toContain("Env");
  });

  it("rejects an unresolved or malformed image identity", () => {
    expect(() => buildReleaseImageInventory(revision, () => ({ id: "latest", repoDigests: [] })))
      .toThrow("content-addressed");
    expect(() => buildReleaseImageInventory("short", () => ({
      id: `sha256:${"a".repeat(64)}`,
      repoDigests: [],
    }))).toThrow("revision");
    expect(() => buildReleaseImageInventory(revision, (reference) => ({
      id: `sha256:${"a".repeat(64)}`,
      repoDigests: reference.startsWith("hotkey-")
        ? []
        : [`unrelated/image@sha256:${"b".repeat(64)}`],
    }))).toThrow("repository digest does not match");
  });

  it("writes an exclusive private sanitized inventory", () => {
    const output = join(mkdtempSync(join(tmpdir(), "hotkey-release-images-")), "images.json");
    const inventory = {
      version: "hotkey-release-image-inventory-v1",
      git_revision: revision,
      applications: [],
      upstream: [],
      differences: [],
    };
    writeReleaseImageInventory(output, inventory);
    expect(JSON.parse(readFileSync(output, "utf8"))).toEqual(inventory);
    expect(statSync(output).mode & 0o777).toBe(0o600);
    expect(() => writeReleaseImageInventory(output, inventory)).toThrow();
  });
});
