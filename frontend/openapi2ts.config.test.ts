import { describe, expect, it } from "vitest";

import { preserveAcceptedResponseTypes } from "./openapi2ts.config";

describe("preserveAcceptedResponseTypes", () => {
  it("exposes a 202 schema to the generator without replacing runtime semantics", () => {
    const accepted = { description: "accepted" };
    const document = {
      paths: {
        "/api/jobs": {
          post: { responses: { "202": accepted } },
        },
      },
    };

    expect(preserveAcceptedResponseTypes(document)).toBe(document);
    expect(document.paths["/api/jobs"].post.responses).toEqual({
      "200": accepted,
      "202": accepted,
    });
  });

  it("does not replace an explicitly documented 200 response", () => {
    const ok = { description: "ok" };
    const accepted = { description: "accepted" };
    const document = {
      paths: {
        "/api/jobs": {
          post: { responses: { "200": ok, "202": accepted } },
        },
      },
    };

    preserveAcceptedResponseTypes(document);

    expect(document.paths["/api/jobs"].post.responses["200"]).toBe(ok);
  });
});
