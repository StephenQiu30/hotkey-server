type OpenApiOperation = {
  responses?: Record<string, unknown>;
};

type OpenApiDocument = {
  paths?: Record<string, Record<string, unknown>>;
};

const HTTP_METHODS = ["get", "post", "put", "patch", "delete"] as const;

export function preserveAcceptedResponseTypes<Document extends OpenApiDocument>(
  document: Document,
): Document {
  for (const pathItem of Object.values(document.paths ?? {})) {
    for (const method of HTTP_METHODS) {
      const operation = pathItem[method] as OpenApiOperation | undefined;
      const responses = operation?.responses;
      if (
        responses?.["202"] !== undefined &&
        responses["200"] === undefined &&
        responses["201"] === undefined &&
        responses.default === undefined
      ) {
        responses["200"] = responses["202"];
      }
    }
  }
  return document;
}

export default {
  schemaPath:
    process.env.HOTKEY_OPENAPI_URL ?? "http://127.0.0.1:8867/openapi.json",
  serversPath: "./src",
  projectName: "api",
  requestLibPath: "@/request",
  requestOptionsType: "import('@/request').RequestOptions",
  namespace: "HotKeyAPI",
  nullable: false,
  enumStyle: "string-literal",
  isCamelCase: true,
  declareType: "type",
  hook: {
    afterOpenApiDataInited: preserveAcceptedResponseTypes,
  },
} as const;
