import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, normalize, relative, resolve, sep } from "node:path";

const source = resolve(process.env.HOTKEY_FRONTEND_SOURCE ?? "src");
const allowedTopLevel = new Set(["api", "app", "features"]);
const allowedRootFiles = new Set(["main.tsx", "request.ts"]);
const legacy = [
  "App.tsx",
  "Login.tsx",
  "MonitorEditor.tsx",
  "client.ts",
  "api.generated.ts",
  "generated",
  "shared",
];

function walk(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return walk(path);
    return /\.(ts|tsx)$/.test(entry.name) ? [path] : [];
  });
}

function layer(path) {
  return relative(source, path)
    .split(sep)[0]
    .replace(/\.(ts|tsx)$/, "");
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
  const relativePath = relative(source, path);
  const current = layer(path);
  if (!relativePath.includes(sep)) {
    if (!allowedRootFiles.has(relativePath)) {
      throw new Error(`Unplanned root file: ${relativePath}`);
    }
  } else if (!allowedTopLevel.has(current)) {
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
      (current === "request" && dependency !== "request") ||
      (current === "features" &&
        !new Set(["features", "api", "request"]).has(dependency)) ||
      (current === "api" && !new Set(["api", "request"]).has(dependency));
    if (invalid) {
      throw new Error(
        `${current} cannot import ${dependency}: ${relative(source, path)}`,
      );
    }
  }
}
