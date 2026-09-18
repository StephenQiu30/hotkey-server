import { readFile, readdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const SOURCE_EXTENSIONS = new Set([".ts", ".tsx", ".js", ".jsx", ".mjs"]);
const IMPORT_PATTERN =
  /(?:import|export)\s+(?:[\s\S]*?\s+from\s+)?["']([^"']+)["']|import\(\s*["']([^"']+)["']\s*\)/g;

async function listSourceFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const nested = await Promise.all(
    entries.map(async (entry) => {
      const entryPath = path.join(directory, entry.name);
      if (entry.isDirectory()) {
        return listSourceFiles(entryPath);
      }
      return SOURCE_EXTENSIONS.has(path.extname(entry.name)) ? [entryPath] : [];
    }),
  );

  return nested.flat();
}

function layerFor(relativePath) {
  const [first, second] = relativePath.split(path.sep);
  if (first === "app") return { name: "app" };
  if (first === "features") return { name: "features", feature: second };
  if (first === "components") return { name: "components" };
  if (first === "api") return { name: "api" };
  if (first === "lib") return { name: "lib" };
  if (relativePath === "request.ts") return { name: "request" };
  if (first === "proxy.ts") return { name: "proxy" };
  return { name: "other" };
}

function resolveInternalImport(sourceRoot, importer, specifier) {
  if (specifier.startsWith("@/")) {
    return path.join(sourceRoot, specifier.slice(2));
  }
  if (specifier.startsWith(".")) {
    return path.resolve(path.dirname(importer), specifier);
  }
  return null;
}

function explainViolation(importerLayer, importedLayer) {
  if (importerLayer.name === "features") {
    if (importedLayer.name === "app")
      return "features cannot import app routes";
    if (
      importedLayer.name === "features" &&
      importerLayer.feature !== importedLayer.feature
    ) {
      return "features cannot import another feature directly";
    }
  }

  if (
    importerLayer.name === "components" &&
    !["components", "lib"].includes(importedLayer.name)
  ) {
    return "base components may only import components or lib";
  }

  if (
    importerLayer.name === "api" &&
    !["api", "request"].includes(importedLayer.name)
  ) {
    return "generated API code may only import api siblings or request";
  }

  if (importerLayer.name === "lib" && importedLayer.name !== "lib") {
    return "lib cannot depend on application layers";
  }

  if (
    importerLayer.name === "request" &&
    !["lib", "request"].includes(importedLayer.name)
  ) {
    return "request transport cannot depend on UI or business layers";
  }

  return null;
}

export async function validateBoundaries(sourceRoot) {
  const absoluteRoot = path.resolve(sourceRoot);
  const files = await listSourceFiles(absoluteRoot);
  const violations = [];

  for (const file of files) {
    const source = await readFile(file, "utf8");
    const importerPath = path.relative(absoluteRoot, file);
    const importerLayer = layerFor(importerPath);

    for (const match of source.matchAll(IMPORT_PATTERN)) {
      const specifier = match[1] ?? match[2];
      const importedPath = resolveInternalImport(absoluteRoot, file, specifier);
      if (
        !importedPath ||
        !importedPath.startsWith(`${absoluteRoot}${path.sep}`)
      ) {
        continue;
      }

      const importedLayer = layerFor(path.relative(absoluteRoot, importedPath));
      const reason = explainViolation(importerLayer, importedLayer);
      if (reason) {
        violations.push(`${importerPath}: ${specifier} — ${reason}`);
      }
    }
  }

  return violations;
}

async function main() {
  const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
  const sourceRoot = path.resolve(scriptDirectory, "../src");
  const violations = await validateBoundaries(sourceRoot);

  if (violations.length > 0) {
    console.error("Frontend boundary violations:\n");
    for (const violation of violations) console.error(`- ${violation}`);
    process.exitCode = 1;
    return;
  }

  console.log("Frontend boundaries are valid.");
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
