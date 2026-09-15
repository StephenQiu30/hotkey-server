import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, normalize, relative, resolve, sep } from "node:path";

const source = resolve(process.env.HOTKEY_FRONTEND_SOURCE ?? "src");
const allowedTopLevel = new Set(["app", "features", "generated", "shared"]);
const legacy = [
  "App.tsx",
  "Login.tsx",
  "MonitorEditor.tsx",
  "client.ts",
  "api.generated.ts",
  "style.css",
];

function walk(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return walk(path);
    return /\.(ts|tsx)$/.test(entry.name) ? [path] : [];
  });
}

function layer(path) {
  return relative(source, path).split(sep)[0];
}

for (const name of legacy) {
  try {
    statSync(join(source, name));
    throw new Error(`Legacy frontend file remains: src/${name}`);
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
}

for (const path of walk(source)) {
  const current = layer(path);
  if (current.endsWith(".ts") || current.endsWith(".tsx")) {
    if (relative(source, path) !== "main.tsx") {
      throw new Error(`Unplanned root file: ${relative(source, path)}`);
    }
    continue;
  }
  if (!allowedTopLevel.has(current)) {
    throw new Error(`Unknown frontend layer: ${current}`);
  }
  const text = readFileSync(path, "utf8");
  const imports = [...text.matchAll(/from\s+["'](\.[^"']+)["']/g)].map(
    (match) => match[1],
  );
  for (const specifier of imports) {
    const target = normalize(resolve(dirname(path), specifier));
    if (!target.startsWith(source + sep)) {
      throw new Error(`Import escapes src: ${relative(source, path)}`);
    }
    const dependency = layer(target);
    const invalid =
      (current === "shared" && dependency !== "shared") ||
      (current === "features" &&
        !new Set(["features", "shared", "generated"]).has(dependency)) ||
      (current === "generated" &&
        !new Set(["generated", "shared"]).has(dependency));
    if (invalid) {
      throw new Error(
        `${current} cannot import ${dependency}: ${relative(source, path)}`,
      );
    }
  }
}
