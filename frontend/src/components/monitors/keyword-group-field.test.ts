import { describe, expect, it } from "vitest";

import { parseKeywordLines } from "./keyword-group-field";

describe("parseKeywordLines", () => {
  it("keeps user order while removing blank lines", () => {
    expect(
      parseKeywordLines(" Brand \n\n\u54c1\u724c\t\u53ec\u56de \n"),
    ).toEqual(["Brand", "\u54c1\u724c\t\u53ec\u56de"]);
  });
});
