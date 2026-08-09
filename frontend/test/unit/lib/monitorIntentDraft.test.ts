import { describe, expect, it } from "vitest";
import {
  emptyMonitorIntentDraft,
  monitorIntentDraftFromResponse,
  monitorIntentDraftRequest,
  validateMonitorIntentDraft,
} from "@/lib/monitorIntentDraft";

describe("monitorIntentDraft", () => {
  it("keeps clause operator and field as independent values", () => {
    const draft = emptyMonitorIntentDraft();
    draft.objective = "跟踪公开发布的 AI Agent 产品更新";
    draft.clauses = [
      { operator: "must", field: "action", value: "发布" },
      { operator: "must_not", field: "term", value: "招聘" },
    ];

    expect(monitorIntentDraftRequest(draft, 0)).toEqual({
      expected_resource_version: 0,
      objective: "跟踪公开发布的 AI Agent 产品更新",
      clauses: [
        { operator: "must", field: "action", value: "发布" },
        { operator: "must_not", field: "term", value: "招聘" },
      ],
      entities: [],
      examples: [],
    });
  });

  it("normalizes aliases and removes empty optional rows without changing meaning", () => {
    const request = monitorIntentDraftRequest(
      {
        objective: "  监控 OpenAI 的产品发布  ",
        clauses: [{ operator: "should", field: "phrase", value: "  new model  " }],
        entities: [
          {
            canonicalId: " openai ",
            displayName: " OpenAI ",
            aliasesText: " ChatGPT, open ai，ChatGPT ",
            ambiguityNote: "  仅指公司  ",
          },
        ],
        examples: [
          { label: "positive", text: "  OpenAI 发布新模型  " },
          { label: "negative", text: "   " },
        ],
      },
      4,
    );

    expect(request).toMatchObject({
      expected_resource_version: 4,
      objective: "监控 OpenAI 的产品发布",
      entities: [
        {
          canonical_id: "openai",
          display_name: "OpenAI",
          aliases: ["ChatGPT", "open ai"],
          ambiguity_note: "仅指公司",
        },
      ],
      examples: [{ label: "positive", text: "OpenAI 发布新模型" }],
    });
  });

  it("maps a safe API projection without inventing candidate provenance", () => {
    const draft = monitorIntentDraftFromResponse({
      monitor_id: 8,
      draft_id: 9,
      resource_version: 2,
      objective: "产品发布",
      clauses: [{ operator: "must", field: "action", value: "发布" }],
      entities: [
        {
          canonical_id: "openai",
          display_name: "OpenAI",
          aliases: ["ChatGPT"],
          ambiguity_note: "公司",
        },
      ],
      examples: [{ label: "positive", text: "产品发布公告" }],
    });

    expect(draft.entities[0]).toEqual({
      canonicalId: "openai",
      displayName: "OpenAI",
      aliasesText: "ChatGPT",
      ambiguityNote: "公司",
    });
    expect(draft.clauses[0]).toEqual({ operator: "must", field: "action", value: "发布" });
  });

  it("rejects ambiguous or unbounded drafts before an API call", () => {
    const draft = emptyMonitorIntentDraft();
    expect(validateMonitorIntentDraft(draft)).toBe("请填写监控目标");

    draft.objective = "产品发布";
    draft.clauses = [{ operator: "must", field: "term", value: "" }];
    expect(validateMonitorIntentDraft(draft)).toBe("请补全或删除空的意图条件");

    draft.clauses = [];
    draft.entities = [
      { canonicalId: "", displayName: "OpenAI", aliasesText: "", ambiguityNote: "" },
    ];
    expect(validateMonitorIntentDraft(draft)).toBe("请补全或删除缺少身份的实体");
  });
});
