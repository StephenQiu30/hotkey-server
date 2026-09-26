import { describe, expect, it } from "vitest";

import { ApiRequestError } from "@/request";
import {
  readTopicFieldErrors,
  topicErrorAction,
  tryBeginTopicSubmission,
} from "./topic-validation";

describe("topic form errors", () => {
  it("keeps 422 details at the matching editable field", () => {
    const error = new ApiRequestError({
      kind: "http",
      status: 422,
      code: "request_validation_failed",
      message: "输入不符合要求",
      details: [
        {
          location: ["body", "collection_interval_seconds"],
          message: "必须不少于 600",
          type: "greater_than_equal",
        },
        {
          location: ["body", "match_any", 0],
          message: "关键词过长",
          type: "string_too_long",
        },
      ],
    });
    expect(readTopicFieldErrors(error)).toEqual({
      collection_interval_seconds: "必须不少于 600",
      match_any: "关键词过长",
    });
  });

  it("routes 401 to login and 409 to a conflict that preserves the draft", () => {
    expect(
      topicErrorAction(
        new ApiRequestError({
          kind: "http",
          status: 401,
          code: "invalid_session",
          message: "会话失效",
        }),
      ),
    ).toBe("login");
    expect(
      topicErrorAction(
        new ApiRequestError({
          kind: "http",
          status: 409,
          code: "topic_version_conflict",
          message: "版本冲突",
        }),
      ),
    ).toBe("conflict");
  });

  it("allows one pending submission and releases the gate after completion", () => {
    const lock = { current: false };
    expect(tryBeginTopicSubmission(lock)).toBe(true);
    expect(tryBeginTopicSubmission(lock)).toBe(false);
    lock.current = false;
    expect(tryBeginTopicSubmission(lock)).toBe(true);
  });
});
