import { getAccessToken } from "@/lib/authSession";

export interface SSEFrame {
  id: string;
  event: string;
  data: string;
}

export async function openNotificationStream(
  afterID: number,
  signal: AbortSignal,
): Promise<ReadableStream<Uint8Array>> {
  const token = getAccessToken();
  if (!token) throw new Error("notification stream access token is unavailable");

  const cursor = Math.max(0, Math.trunc(afterID));
  const headers = new Headers({
    Accept: "text/event-stream",
    Authorization: `Bearer ${token}`,
  });
  if (cursor > 0) headers.set("Last-Event-ID", String(cursor));

  const response = await fetch(`/api/v1/notifications/stream?after_id=${cursor}`, {
    cache: "no-store",
    credentials: "include",
    headers,
    method: "GET",
    signal,
  });
  if (!response.ok) throw new Error(`notification stream returned HTTP ${response.status}`);
  if (!response.body) throw new Error("notification stream response body is unavailable");
  return response.body;
}

export interface SSEParser {
  push(chunk: string): void;
  finish(): void;
}

export function createSSEParser(onFrame: (frame: SSEFrame) => void): SSEParser {
  let buffer = "";
  let id = "";
  let event = "message";
  let data: string[] = [];

  const dispatch = () => {
    if (data.length > 0) onFrame({ id, event, data: data.join("\n") });
    id = "";
    event = "message";
    data = [];
  };

  const consumeLine = (line: string) => {
    if (line === "") {
      dispatch();
      return;
    }
    if (line.startsWith(":")) return;
    const separator = line.indexOf(":");
    const field = separator < 0 ? line : line.slice(0, separator);
    let value = separator < 0 ? "" : line.slice(separator + 1);
    if (value.startsWith(" ")) value = value.slice(1);
    if (field === "id" && !value.includes("\0")) id = value;
    if (field === "event") event = value || "message";
    if (field === "data") data.push(value);
  };

  const drain = (flush: boolean) => {
    for (;;) {
      let boundary = -1;
      let width = 1;
      for (let index = 0; index < buffer.length; index += 1) {
        if (buffer[index] === "\n") {
          boundary = index;
          break;
        }
        if (buffer[index] === "\r") {
          if (index + 1 === buffer.length && !flush) return;
          boundary = index;
          width = buffer[index + 1] === "\n" ? 2 : 1;
          break;
        }
      }
      if (boundary < 0) break;
      consumeLine(buffer.slice(0, boundary));
      buffer = buffer.slice(boundary + width);
    }
    if (flush && buffer !== "") {
      consumeLine(buffer);
      buffer = "";
    }
  };

  return {
    push(chunk) {
      buffer += chunk;
      drain(false);
    },
    finish() {
      drain(true);
      dispatch();
    },
  };
}

export async function consumeNotificationStream(
  stream: ReadableStream<Uint8Array>,
  onFrame: (frame: SSEFrame) => void,
): Promise<void> {
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  const parser = createSSEParser(onFrame);
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      parser.push(decoder.decode(value, { stream: true }));
    }
    parser.push(decoder.decode());
    parser.finish();
  } finally {
    reader.releaseLock();
  }
}
