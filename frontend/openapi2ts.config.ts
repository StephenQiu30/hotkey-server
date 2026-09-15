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
  hook: {
    afterOpenApiDataInited(document: {
      paths?: Record<
        string,
        Record<string, { responses?: Record<string, unknown> }>
      >;
    }) {
      for (const operations of Object.values(document.paths ?? {})) {
        for (const operation of Object.values(operations)) {
          if (operation.responses?.["202"] && !operation.responses["200"]) {
            operation.responses["200"] = operation.responses["202"];
          }
        }
      }
      return document;
    },
  },
};
