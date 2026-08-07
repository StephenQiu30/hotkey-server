import { describe, expect, it, vi } from "vitest";
import { createSSEParser } from "@/lib/notificationStream";

describe("createSSEParser", () => {
  it("parses split frames, comments, CRLF and multiline data", () => {
    const onFrame = vi.fn();
    const parser = createSSEParser(onFrame);

    parser.push(": heartbeat\r\nid: 12\r\nevent: event.up");
    parser.push("dated\r\ndata: {\"id\":12,\r\ndata: \"ok\":true}\r\n\r\n");
    parser.finish();

    expect(onFrame).toHaveBeenCalledWith({
      id: "12",
      event: "event.updated",
      data: '{"id":12,\n"ok":true}',
    });
  });

  it("ignores frames without business data", () => {
    const onFrame = vi.fn();
    const parser = createSSEParser(onFrame);
    parser.push(": heartbeat\n\nretry: 1000\n\n");
    parser.finish();
    expect(onFrame).not.toHaveBeenCalled();
  });

  it("does not create a phantom blank line when CRLF is split across chunks", () => {
    const onFrame = vi.fn();
    const parser = createSSEParser(onFrame);
    parser.push("id: 2\r");
    parser.push("\nevent: alert.triggered\r\ndata: {}\r\n\r\n");
    expect(onFrame).toHaveBeenCalledOnce();
    expect(onFrame).toHaveBeenCalledWith({ id: "2", event: "alert.triggered", data: "{}" });
  });
});
