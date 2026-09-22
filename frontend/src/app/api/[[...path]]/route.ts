const DEFAULT_API_ORIGIN = "http://127.0.0.1:8867";
const API_TIMEOUT_MS = 15_000;

const REQUEST_HEADER_BLOCKLIST = new Set([
  "connection",
  "content-length",
  "host",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
]);
const RESPONSE_HEADER_BLOCKLIST = new Set([
  "connection",
  "content-encoding",
  "content-length",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
]);

type ApiRouteContext = {
  params: Promise<{ path?: string[] }>;
};

type StreamingRequestInit = RequestInit & {
  duplex?: "half";
};

function getApiOrigin(): URL {
  const origin = new URL(process.env.HOTKEY_API_ORIGIN ?? DEFAULT_API_ORIGIN);
  if (origin.protocol !== "http:" && origin.protocol !== "https:") {
    throw new Error("HOTKEY_API_ORIGIN must use http or https");
  }
  return origin;
}

function createDestination(request: Request, path: string[] | undefined): URL {
  const destinationPath = ["api", ...(path ?? [])]
    .map(encodeURIComponent)
    .join("/");
  const destination = new URL(`/${destinationPath}`, getApiOrigin());
  destination.search = new URL(request.url).search;
  return destination;
}

function createUpstreamHeaders(request: Request): Headers {
  const headers = new Headers();
  request.headers.forEach((value, name) => {
    const normalized = name.toLowerCase();
    if (
      !REQUEST_HEADER_BLOCKLIST.has(normalized) &&
      !normalized.startsWith("x-forwarded-")
    ) {
      headers.append(name, value);
    }
  });
  return headers;
}

function createClientHeaders(upstream: Response): Headers {
  const headers = new Headers();
  upstream.headers.forEach((value, name) => {
    const normalized = name.toLowerCase();
    if (
      normalized !== "set-cookie" &&
      !RESPONSE_HEADER_BLOCKLIST.has(normalized)
    ) {
      headers.append(name, value);
    }
  });
  for (const cookie of upstream.headers.getSetCookie()) {
    headers.append("set-cookie", cookie);
  }
  return headers;
}

function createProxyError(status: 502 | 504): Response {
  const requestId = crypto.randomUUID();
  const body = {
    code: status === 504 ? "upstream_timeout" : "upstream_error",
    message: status === 504 ? "上游服务响应超时" : "无法连接后端服务",
    request_id: requestId,
  } satisfies HotKeyAPI.ErrorView;

  return Response.json(body, {
    status,
    headers: {
      "Cache-Control": "no-store",
      "X-Request-ID": requestId,
    },
  });
}

async function proxyApiRequest(
  request: Request,
  context: ApiRouteContext,
): Promise<Response> {
  const { path } = await context.params;
  const method = request.method.toUpperCase();
  const body = method === "GET" || method === "HEAD" ? undefined : request.body;
  const init: StreamingRequestInit = {
    body,
    cache: "no-store",
    headers: createUpstreamHeaders(request),
    method,
    redirect: "manual",
    signal: AbortSignal.timeout(API_TIMEOUT_MS),
  };
  if (body) {
    init.duplex = "half";
  }

  try {
    const upstream = await fetch(createDestination(request, path), init);
    const responseBody =
      method === "HEAD" || upstream.status === 204 || upstream.status === 304
        ? null
        : upstream.body;
    return new Response(responseBody, {
      headers: createClientHeaders(upstream),
      status: upstream.status,
      statusText: upstream.statusText,
    });
  } catch (error) {
    const timedOut =
      error instanceof DOMException && error.name === "TimeoutError";
    return createProxyError(timedOut ? 504 : 502);
  }
}

export const GET = proxyApiRequest;
export const HEAD = proxyApiRequest;
export const POST = proxyApiRequest;
export const PUT = proxyApiRequest;
export const PATCH = proxyApiRequest;
export const DELETE = proxyApiRequest;
export const OPTIONS = proxyApiRequest;
