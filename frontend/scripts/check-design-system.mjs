import { readFile, readdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const SOURCE_EXTENSIONS = new Set([".ts", ".tsx", ".css", ".md"]);
const RAW_PIXEL_PATTERN = /\b\d+(?:\.\d+)?px\b/g;
const ARBITRARY_LAYOUT_PATTERN =
  /(?:^|[\s"'])(-?(?:h|w|min-h|min-w|max-h|max-w|p[trblxy]?|m[trblxy]?|gap|space-[xy]|text|leading|tracking|rounded|top|right|bottom|left|inset[xy]?|translate-[xy])-\[[^\]]+\])/g;

async function listFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const nested = await Promise.all(
    entries.map(async (entry) => {
      const entryPath = path.join(directory, entry.name);
      if (entry.isDirectory()) return listFiles(entryPath);
      return SOURCE_EXTENSIONS.has(path.extname(entry.name)) ? [entryPath] : [];
    }),
  );

  return nested.flat();
}

function lineNumberAt(source, index) {
  return source.slice(0, index).split("\n").length;
}

export async function findDesignViolations(projectRoot) {
  const absoluteRoot = path.resolve(projectRoot);
  const files = await listFiles(path.join(absoluteRoot, "src"));
  const designFiles = [
    path.join(absoluteRoot, "README.md"),
    path.join(absoluteRoot, "DESIGN.md"),
  ];
  const violations = [];

  for (const file of [...files, ...designFiles]) {
    const source = await readFile(file, "utf8");
    const relativePath = path.relative(absoluteRoot, file);

    for (const match of source.matchAll(RAW_PIXEL_PATTERN)) {
      violations.push(
        `${relativePath}:${lineNumberAt(source, match.index)} — raw pixel value ${match[0]}`,
      );
    }

    for (const match of source.matchAll(ARBITRARY_LAYOUT_PATTERN)) {
      violations.push(
        `${relativePath}:${lineNumberAt(source, match.index)} — arbitrary layout class ${match[1]}`,
      );
    }
  }

  return violations;
}

async function main() {
  const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
  const projectRoot = path.resolve(scriptDirectory, "..");
  const violations = await findDesignViolations(projectRoot);

  if (violations.length > 0) {
    console.error("Design-system violations:\n");
    for (const violation of violations) console.error(`- ${violation}`);
    process.exitCode = 1;
    return;
  }

  console.log("Design-system scales are valid.");
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
