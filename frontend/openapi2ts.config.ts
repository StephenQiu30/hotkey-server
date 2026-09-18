export default {
  schemaPath:
    process.env.HOTKEY_OPENAPI_URL ?? "http://127.0.0.1:8867/openapi.json",
  serversPath: "./src",
  projectName: "api",
  requestLibPath: "@/request",
  requestOptionsType: "import('@/request').RequestOptions",
  namespace: "HotKeyAPI",
  nullable: true,
  enumStyle: "string-literal",
  isCamelCase: true,
  declareType: "type",
} as const;
