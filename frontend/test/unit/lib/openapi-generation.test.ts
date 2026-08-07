import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

import openapiConfig from "../../../openapi2ts.config";

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../..");

describe("Umi OpenAPI generation contract", () => {
  it("uses the published repository OpenAPI document and the standard output directory", () => {
    expect(openapiConfig.schemaPath).toBe(
      path.resolve(repositoryRoot, "../docs/openapi/swagger.json"),
    );
    expect(openapiConfig.serversPath).toBe(path.resolve(repositoryRoot, "src/services/hotkey"));
    expect(openapiConfig.projectName).toBe("hotkey-server");
    expect(openapiConfig.namespace).toBe("HotKeyAPI");
    expect(openapiConfig.enumStyle).toBe("string-literal");
    expect(openapiConfig.declareType).toBe("type");
    expect(openapiConfig.nullable).toBe(false);
    expect(openapiConfig.isCamelCase).toBe(true);
  });

  it("provides a deterministic contract check around the official openapi2ts CLI", () => {
    const packageJSON = JSON.parse(
      fs.readFileSync(path.resolve(repositoryRoot, "package.json"), "utf8"),
    );
    const checkScript = path.resolve(repositoryRoot, "scripts/check-openapi.mjs");

    expect(packageJSON.scripts["openapi:generate"]).toBe("openapi2ts");
    expect(packageJSON.scripts["openapi:check"]).toBe("node scripts/check-openapi.mjs");
    expect(fs.existsSync(checkScript)).toBe(true);

    const checkSource = fs.readFileSync(checkScript, "utf8");
    expect(checkSource).toContain("npm run openapi:generate");
    expect(checkSource).toContain("src/services/hotkey/hotkey-server");
    expect(checkSource).toContain("../docs/openapi/swagger.json");
  });

  it("routes generated requests through the shared Axios adapter", () => {
    expect(openapiConfig.requestImportStatement).toBe(
      "import { request, type RequestOptions } from '@/lib/request';",
    );
    expect(openapiConfig.requestOptionsType).toBe("RequestOptions");

    const generatedIdentityService = path.resolve(
      repositoryRoot,
      "src/services/hotkey/hotkey-server/identity.ts",
    );
    expect(fs.readFileSync(generatedIdentityService, "utf8").replaceAll('"', "'")).toContain(
      openapiConfig.requestImportStatement,
    );
  });

  it("generates the content document reader from the server contract", () => {
    const generatedContentsService = path.resolve(
      repositoryRoot,
      "src/services/hotkey/hotkey-server/contents.ts",
    );
    const generatedTypes = path.resolve(
      repositoryRoot,
      "src/services/hotkey/hotkey-server/typings.d.ts",
    );
    const serviceSource = fs.readFileSync(generatedContentsService, "utf8");
    const typeSource = fs.readFileSync(generatedTypes, "utf8");

    expect(serviceSource).toContain("export async function getContentsIdDocument");
    expect(serviceSource).toContain("export async function deleteContentsId");
    expect(serviceSource).toContain(
      "HotKeyAPI.ContentResultHttpContentDocumentResponse",
    );
    expect(typeSource).toContain("type ContentDocumentResponse");
    expect(typeSource).toContain("data?: ContentDocumentResponse");
    expect(typeSource).toMatch(
      /availability\?:\s*["']ready["']\s*\|\s*["']not_captured["']\s*\|\s*["']unavailable["']/,
    );
    expect(typeSource).toMatch(/unavailable_reason\?:/);
  });

  it("keeps application code on the generated server client only", () => {
    const legacyServices = [
      "auth.ts",
      "content.ts",
      "health.ts",
      "hotEvents.ts",
      "monitors.ts",
      "notifications.ts",
      "reports.ts",
      "topics.ts",
      "trending.ts",
      "trends.ts",
      "typings.d.ts",
    ];

    for (const file of legacyServices) {
      expect(fs.existsSync(path.resolve(repositoryRoot, "src/services", file))).toBe(false);
    }

    const sourceRoot = path.resolve(repositoryRoot, "src");
    const generatedClientRoot = path.resolve(
      sourceRoot,
      "services/hotkey/hotkey-server",
    );
    const queue = [sourceRoot];
    const sourceFiles: string[] = [];
    while (queue.length) {
      const current = queue.pop()!;
      for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
        const resolved = path.join(current, entry.name);
        if (entry.isDirectory()) queue.push(resolved);
        else if (/\.(ts|tsx)$/.test(entry.name)) sourceFiles.push(resolved);
      }
    }

    for (const file of sourceFiles) {
      const source = fs.readFileSync(file, "utf8");
      expect(source, file).not.toMatch(/@\/services\/(?!hotkey\/hotkey-server)/);
      if (!file.startsWith(`${generatedClientRoot}${path.sep}`)) {
        expect(source, file).not.toMatch(
          /\/api\/v1\/contents\/[^'"`]+\/document/,
        );
        expect(source, file).not.toMatch(/interface\s+ContentDocument/);
      }
    }
  });
});
