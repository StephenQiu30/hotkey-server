import path from "node:path";
import type { GenerateServiceProps } from "@umijs/openapi";

const repositoryRoot = process.cwd();
const publishedSchemaPath = path.resolve(
  repositoryRoot,
  "../docs/openapi/swagger.json",
);

const openapiConfig = {
  requestImportStatement:
    "import { request, type RequestOptions } from '@/lib/request';",
  requestOptionsType: "RequestOptions",
  schemaPath: process.env.HOTKEY_OPENAPI_SCHEMA ?? publishedSchemaPath,
  serversPath: path.resolve(repositoryRoot, "src/services/hotkey"),
  projectName: "hotkey-server",
  namespace: "HotKeyAPI",
  enumStyle: "string-literal",
  declareType: "type",
  nullable: false,
  isCamelCase: true,
} satisfies GenerateServiceProps;

export default openapiConfig;
