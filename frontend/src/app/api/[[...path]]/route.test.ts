import { createServer, type Server } from "node:http";

import { afterEach, describe, expect, it, vi } from "vitest";

import * as route from "./route";

const ORIGINAL_API_ORIGIN = process.env.HOTKEY_API_ORIGIN;

afterEach(() => {
  vi.unstubAllGlobals();
  if (ORIGINAL_API_ORIGIN === undefined) {
    delete process.env.HOTKEY_API_ORIGIN;
  } else {
    process.env.HOTKEY_API_ORIGIN = ORIGINAL_API_ORIGIN;
  }
});

async function listen(server: Server): Promise<number> {
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (!address || typeof address === "string") {
    throw new Error("test server did not expose a TCP port");
  }
  return address.port;
}

async function close(server: Server): Promise<void> {
  await new Promise<void>((resolve, reject) => {
    server.close((error) => (error ? reject(error) : resolve()));
  });
}

describe("API route proxy", () => {
  it("exports only supported HTTP handlers", () => {
    expect(Object.keys(route).sort()).toEqual([
      "DELETE",
      "GET",
      "HEAD",
      "OPTIONS",
      "PATCH",
      "POST",
      "PUT",
    ]);
  });

  it("streams payloads and preserves credentials and response headers", async () => {
    let receivedBody = "";
    let receivedAuthorization = "";
    let receivedCookie = "";
    let receivedForwarded = "";
    const server = createServer((request, response) => {
      receivedAuthorization = request.headers.authorization ?? "";
      receivedCookie = request.headers.cookie ?? "";
      receivedForwarded = request.headers["x-forwarded-host"]?.toString() ?? "";
      request.setEncoding("utf8");
      request.on("data", (chunk: string) => {
        receivedBody += chunk;
      });
      request.on("end", () => {
        response.writeHead(206, {
          "Content-Disposition": 'attachment; filename="hotkey.bin"',
          "Content-Type": "application/octet-stream",
          "Set-Cookie": "session=renewed; HttpOnly",
          "X-Request-ID": "b64c7bc5-cf50-47ad-aa7d-82e5951d537a",
        });
        response.end(Buffer.from([0, 1, 2, 3]));
      });
    });
    const port = await listen(server);
    process.env.HOTKEY_API_ORIGIN = `http://127.0.0.1:${port}`;

    try {
      const response = await route.POST(
        new Request("http://web.test/api/files/example?download=1", {
          body: "payload",
          headers: {
            Authorization: "Bearer test-token",
            Cookie: "session=test-session",
            "Content-Type": "text/plain",
            "X-Forwarded-Host": "attacker.invalid",
          },
          method: "POST",
        }),
        { params: Promise.resolve({ path: ["files", "example"] }) },
      );

      expect(response.status).toBe(206);
      expect(new Uint8Array(await response.arrayBuffer())).toEqual(
        new Uint8Array([0, 1, 2, 3]),
      );
      expect(response.headers.get("content-type")).toBe(
        "application/octet-stream",
      );
      expect(response.headers.get("content-disposition")).toContain(
        "hotkey.bin",
      );
      expect(response.headers.get("set-cookie")).toContain("session=renewed");
      expect(response.headers.get("x-request-id")).toBe(
        "b64c7bc5-cf50-47ad-aa7d-82e5951d537a",
      );
      expect(receivedBody).toBe("payload");
      expect(receivedAuthorization).toBe("Bearer test-token");
      expect(receivedCookie).toBe("session=test-session");
      expect(receivedForwarded).toBe("");
    } finally {
      await close(server);
    }
  });

  it("returns a safe 502 with a proxy-owned request id when upstream is unavailable", async () => {
    const server = createServer();
    const port = await listen(server);
    await close(server);
    process.env.HOTKEY_API_ORIGIN = `http://127.0.0.1:${port}`;

    const response = await route.GET(
      new Request("http://web.test/api/health", {
        headers: {
          "X-Request-ID": "0125d835-4eab-4e6b-86fe-f5aa94fd9e50",
        },
      }),
      { params: Promise.resolve({ path: ["health"] }) },
    );
    const body = (await response.json()) as HotKeyAPI.ErrorView;

    expect(response.status).toBe(502);
    expect(body.code).toBe("upstream_error");
    expect(body.request_id).toBe(response.headers.get("x-request-id"));
    expect(body.request_id).not.toBe("0125d835-4eab-4e6b-86fe-f5aa94fd9e50");
  });

  it("distinguishes an upstream timeout", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockRejectedValue(new DOMException("secret", "TimeoutError")),
    );

    const response = await route.GET(
      new Request("http://web.test/api/health"),
      { params: Promise.resolve({ path: ["health"] }) },
    );
    const body = (await response.json()) as HotKeyAPI.ErrorView;

    expect(response.status).toBe(504);
    expect(body.code).toBe("upstream_timeout");
    expect(response.headers.get("cache-control")).toBe("no-store");
    expect(JSON.stringify(body)).not.toContain("secret");
  });
});
