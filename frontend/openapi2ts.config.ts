import { resolve } from "node:path";

export default {
  schemaPath: resolve(process.cwd(), "../docs/openapi/openapi.json"),
  serversPath: process.env.HOTKEY_OPENAPI_OUTPUT ?? "./src",
  projectName: "api",
  namespace: "API",
  isCamelCase: true,
  enumStyle: "string-literal",
  declareType: "type",
  requestOptionsType: "RequestOptions",
  requestImportStatement:
    "import { request, type RequestOptions } from '../request';",
};
