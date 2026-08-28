import {
  closeSync,
  lstatSync,
  openSync,
  readFileSync,
  readdirSync,
  writeFileSync,
} from "node:fs";
import { basename, relative, resolve } from "node:path";

const identifierPattern = /^[a-z][a-z0-9_]{1,63}$/;
const maximumCanaries = 64;
const maximumSurfaces = 64;
const maximumFiles = 50_000;
const maximumBytes = 1024 * 1024 * 1024;

export function scanSecretSurfaces({ canaries, surfaces }) {
  const normalizedCanaries = normalizeCanaries(canaries);
  const normalizedSurfaces = normalizeSurfaces(surfaces);
  const leaks = [];
  let filesScanned = 0;
  let bytesScanned = 0;

  for (const surface of normalizedSurfaces) {
    const root = resolve(surface.path);
    const rootStat = safeLstat(root, `secret scan surface ${surface.id} is missing`);
    if (rootStat.isSymbolicLink()) {
      throw new Error(`secret scan surface ${surface.id} is a symbolic link`);
    }
    const files = rootStat.isDirectory() ? listFiles(root) : [{ absolute: root, relative: basename(root) }];
    for (const file of files) {
      const stat = safeLstat(file.absolute, `secret scan file disappeared from ${surface.id}`);
      if (!stat.isFile()) {
        throw new Error(`secret scan surface ${surface.id} contains a non-regular file`);
      }
      filesScanned += 1;
      bytesScanned += stat.size;
      if (filesScanned > maximumFiles || bytesScanned > maximumBytes) {
        throw new Error("secret scan input exceeds its bounded file or byte limit");
      }
      const contents = readFileSync(file.absolute);
      for (const canary of normalizedCanaries) {
        if (contents.includes(canary.bytes)) {
          leaks.push({
            canary_id: canary.id,
            surface_id: surface.id,
            relative_path: file.relative,
          });
        }
      }
    }
  }

  return {
    canary_ids: normalizedCanaries.map(({ id }) => id),
    surface_ids: normalizedSurfaces.map(({ id }) => id),
    files_scanned: filesScanned,
    bytes_scanned: bytesScanned,
    leaks,
  };
}

export function writeSecretScanReport(path, result, runID) {
  if (typeof runID !== "string" || runID.trim() === "" || runID.length > 128) {
    throw new Error("secret scan run ID is invalid");
  }
  const report = {
    version: "hotkey-secret-surface-scan-v2",
    run_id: runID,
    canary_ids: result.canary_ids,
    surface_ids: result.surface_ids,
    files_scanned: result.files_scanned,
    bytes_scanned: result.bytes_scanned,
    leaks: result.leaks,
  };
  const descriptor = openSync(path, "wx", 0o600);
  try {
    writeFileSync(descriptor, `${JSON.stringify(report, null, 2)}\n`);
  } finally {
    closeSync(descriptor);
  }
}

function normalizeCanaries(canaries) {
  if (!Array.isArray(canaries) || canaries.length === 0 || canaries.length > maximumCanaries) {
    throw new Error("secret scan canary list is invalid");
  }
  const identifiers = new Set();
  const values = new Set();
  return canaries.map((canary) => {
    if (!canary || !identifierPattern.test(canary.id) || typeof canary.value !== "string" || canary.value.trim() !== canary.value || canary.value.length < 24) {
      throw new Error("secret scan canary is invalid");
    }
    if (identifiers.has(canary.id) || values.has(canary.value)) {
      throw new Error("secret scan canary IDs and values must be unique");
    }
    identifiers.add(canary.id);
    values.add(canary.value);
    return { id: canary.id, bytes: Buffer.from(canary.value, "utf8") };
  });
}

function normalizeSurfaces(surfaces) {
  if (!Array.isArray(surfaces) || surfaces.length === 0 || surfaces.length > maximumSurfaces) {
    throw new Error("secret scan surface list is invalid");
  }
  const identifiers = new Set();
  return surfaces.map((surface) => {
    if (!surface || !identifierPattern.test(surface.id) || typeof surface.path !== "string" || surface.path.trim() === "") {
      throw new Error("secret scan surface is invalid");
    }
    if (identifiers.has(surface.id)) {
      throw new Error("secret scan surface IDs must be unique");
    }
    identifiers.add(surface.id);
    return { id: surface.id, path: surface.path };
  });
}

function listFiles(root) {
  const files = [];
  const visit = (directory) => {
    for (const entry of readdirSync(directory, { withFileTypes: true }).sort((left, right) => left.name.localeCompare(right.name))) {
      const absolute = resolve(directory, entry.name);
      if (entry.isSymbolicLink()) {
        throw new Error(`secret scan surface contains a symbolic link at ${relative(root, absolute)}`);
      }
      if (entry.isDirectory()) {
        visit(absolute);
      } else if (entry.isFile()) {
        files.push({ absolute, relative: relative(root, absolute) });
      } else {
        throw new Error(`secret scan surface contains a non-regular file at ${relative(root, absolute)}`);
      }
    }
  };
  visit(root);
  return files;
}

function safeLstat(path, message) {
  try {
    return lstatSync(path);
  } catch {
    throw new Error(message);
  }
}
