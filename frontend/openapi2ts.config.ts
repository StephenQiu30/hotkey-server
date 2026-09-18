export default {
  schemaPath: "../docs/openapi/openapi.json",
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
