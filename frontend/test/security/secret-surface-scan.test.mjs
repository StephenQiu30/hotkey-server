import { mkdtempSync, mkdirSync, readFileSync, symlinkSync, writeFileSync } from "node:fs";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";
import { execFile } from "node:child_process";
import { describe, expect, it } from "vitest";
import { generateSyntheticCanaries, persistSyntheticCanaries } from "./generate-secret-canaries.mjs";
import { scanSecretSurfaces, writeSecretScanReport } from "./secret-surface-scan.mjs";

const canaries = [
  { id: "jwt", value: "fixture-jwt-secret-0123456789abcdef" },
  { id: "cookie", value: "fixture-cookie-secret-0123456789" },
];

describe("secret surface scanner", () => {
  it("scans nested text and binary files without recording secret values", () => {
    const root = mkdtempSync(join(tmpdir(), "hotkey-secret-scan-"));
    mkdirSync(join(root, "nested"));
    writeFileSync(join(root, "nested", "bundle.bin"), Buffer.from([0, 1, 2, 3]));
    writeFileSync(join(root, "response.json"), '{"code":"INVALID_REQUEST"}\n');

    const result = scanSecretSurfaces({
      canaries,
      surfaces: [{ id: "runtime", path: root }],
    });

    expect(result).toMatchObject({ files_scanned: 2, leaks: [] });
    expect(JSON.stringify(result)).not.toContain(canaries[0].value);
    expect(JSON.stringify(result)).not.toContain(canaries[1].value);
  });

  it("reports only the canary ID and relative path when a leak is found", () => {
    const root = mkdtempSync(join(tmpdir(), "hotkey-secret-leak-"));
    writeFileSync(join(root, "container.log"), `prefix ${canaries[0].value} suffix`);

    const result = scanSecretSurfaces({
      canaries,
      surfaces: [{ id: "logs", path: root }],
    });

    expect(result.leaks).toEqual([
      { canary_id: "jwt", surface_id: "logs", relative_path: "container.log" },
    ]);
    expect(JSON.stringify(result)).not.toContain(canaries[0].value);
  });

  it("fails closed for missing surfaces, symlinks and invalid canaries", () => {
    const root = mkdtempSync(join(tmpdir(), "hotkey-secret-invalid-"));
    writeFileSync(join(root, "target.txt"), "safe");
    symlinkSync(join(root, "target.txt"), join(root, "link.txt"));

    expect(() => scanSecretSurfaces({ canaries, surfaces: [{ id: "missing", path: join(root, "missing") }] })).toThrow(/missing/);
    expect(() => scanSecretSurfaces({ canaries, surfaces: [{ id: "links", path: root }] })).toThrow(/symbolic link/);
    expect(() => scanSecretSurfaces({ canaries: [{ id: "blank", value: "  " }], surfaces: [{ id: "file", path: join(root, "target.txt") }] })).toThrow(/canary/);
  });

  it("writes an exclusive sanitized report and refuses to overwrite evidence", () => {
    const root = mkdtempSync(join(tmpdir(), "hotkey-secret-report-"));
    const surface = join(root, "surface.txt");
    const report = join(root, "report.json");
    writeFileSync(surface, "safe");
    const result = scanSecretSurfaces({ canaries, surfaces: [{ id: "response", path: surface }] });

    writeSecretScanReport(report, result, "fixture-run");

    const saved = JSON.parse(readFileSync(report, "utf8"));
    expect(saved).toMatchObject({ version: "hotkey-secret-surface-scan-v2", run_id: "fixture-run", leaks: [] });
    expect(readFileSync(report, "utf8")).not.toContain("fixture-jwt-secret");
    expect(() => writeSecretScanReport(report, result, "fixture-run")).toThrow();
  });
});

describe("synthetic secret generator", () => {
  it("creates independent masked values and injects only the required backend settings", () => {
    let fill = 1;
    const generated = generateSyntheticCanaries((size) => Buffer.alloc(size, fill++));
    const root = mkdtempSync(join(tmpdir(), "hotkey-secret-generator-"));
    const githubEnvironment = join(root, "github.env");
    const backendEnvironment = join(root, "backend.env");
    writeFileSync(githubEnvironment, "");
    writeFileSync(backendEnvironment, "HOTKEY_JWT_SECRET=\nHOTKEY_VERIFICATION_HMAC_SECRET=\nHOTKEY_SOURCE_CREDENTIAL_MASTER_KEY=\n");
    const masked = [];

    persistSyntheticCanaries(generated, githubEnvironment, backendEnvironment, (value) => masked.push(value));

    const values = Object.values(generated);
    expect(masked).toEqual(values);
    expect(new Set(values).size).toBe(values.length);
    expect(Buffer.from(generated.HOTKEY_SOURCE_CREDENTIAL_MASTER_KEY, "base64")).toHaveLength(32);
    const exported = readFileSync(githubEnvironment, "utf8");
    const backend = readFileSync(backendEnvironment, "utf8");
    for (const [name, value] of Object.entries(generated)) {
      expect(exported).toContain(`${name}=${value}\n`);
    }
    for (const name of ["HOTKEY_JWT_SECRET", "HOTKEY_VERIFICATION_HMAC_SECRET", "HOTKEY_SOURCE_CREDENTIAL_MASTER_KEY", "HOTKEY_SMTP_PASSWORD"]) {
      expect(backend).toContain(`${name}=${generated[name]}\n`);
    }
    expect(backend).not.toContain(generated.POSTGRES_PASSWORD);
    expect(backend).not.toContain(generated.HOTKEY_TEST_COOKIE_SECRET);
  });
});

describe("secret surface exercise", () => {
  it("uses a write-only source credential and saves only sanitized responses", async () => {
    const root = mkdtempSync(join(tmpdir(), "hotkey-secret-exercise-"));
    const sourceToken = "fixture-source-token-0123456789abcdef";
    const cookieSecret = "fixture-cookie-token-0123456789abcdef";
    const requests = [];
    const server = createServer(async (request, response) => {
      const chunks = [];
      for await (const chunk of request) chunks.push(chunk);
      const body = Buffer.concat(chunks).toString("utf8");
      requests.push({ method: request.method, url: request.url, headers: request.headers, body });
      response.setHeader("content-type", request.url === "/metrics" ? "text/plain" : "application/json");
      if (request.url === "/api/v1/auth/login") {
        response.end(JSON.stringify({ data: { access_token: "fixture-access-token-0123456789abcdef" } }));
      } else if (request.url === "/api/v1/source-connections" && request.method === "POST") {
        response.statusCode = 201;
        response.end(JSON.stringify({ data: { id: 17, credential_configured: true } }));
      } else if (request.url === "/api/v1/source-connections/not-a-number") {
        response.statusCode = 401;
        response.setHeader("x-request-id", "fixture-request");
        response.end(JSON.stringify({ code: 10001, message: "unauthorized", data: null }));
      } else if (request.url === "/metrics") {
        response.end("hotkey_http_requests_total 1\n");
      } else {
        response.statusCode = 404;
        response.end(JSON.stringify({ error: { code: "NOT_FOUND" } }));
      }
    });
    await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address();
    try {
      await promisify(execFile)(process.execPath, [join(process.cwd(), "test/security/exercise-secret-surfaces.mjs")], {
        env: {
          ...process.env,
          HOTKEY_BROWSER_API_ORIGIN: `http://127.0.0.1:${address.port}`,
          HOTKEY_SECRET_ADMIN_EMAIL: "admin@example.test",
          HOTKEY_BROWSER_PASSWORD: "fixture-browser-password",
          HOTKEY_TEST_SOURCE_TOKEN: sourceToken,
          HOTKEY_TEST_COOKIE_SECRET: cookieSecret,
          HOTKEY_BROWSER_ARTIFACT_DIR: root,
        },
      });
    } finally {
      await new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
    }

    const sourceRequest = requests.find((request) => request.url === "/api/v1/source-connections");
    expect(JSON.parse(sourceRequest.body)).toMatchObject({ preset_id: "x", credential: sourceToken });
    const errorRequest = requests.find((request) => request.url === "/api/v1/source-connections/not-a-number");
    expect(errorRequest.headers.cookie).toContain(cookieSecret);
    for (const name of ["hotkey-secret-source-response.json", "hotkey-secret-http-error.json", "hotkey-secret-metrics.txt"]) {
      const saved = readFileSync(join(root, name), "utf8");
      expect(saved).not.toContain(sourceToken);
      expect(saved).not.toContain(cookieSecret);
    }
  });
});
