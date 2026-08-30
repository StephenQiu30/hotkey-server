import { closeSync, openSync, unlinkSync, writeFileSync } from "node:fs";

const REVISION = /^[0-9a-f]{40}$/;
const IMAGE_ID = /^sha256:[0-9a-f]{64}$/;
const REPO_DIGEST = /^[^\s@]+@sha256:[0-9a-f]{64}$/;

const APPLICATION_IMAGES = [
  ["backend", "hotkey-server:env"],
  ["frontend", "hotkey-web:env"],
  ["python_agent", "hotkey-agent:env"],
];
const UPSTREAM_IMAGES = [
  ["postgres", "pgvector/pgvector:pg16"],
  ["redis", "redis:latest"],
  ["minio", "minio/minio:latest"],
  ["minio_client", "minio/mc:latest"],
];

export function buildReleaseImageInventory(gitRevision, inspectImage) {
  if (!REVISION.test(gitRevision)) throw new Error("complete git revision is required");
  if (typeof inspectImage !== "function") throw new Error("image inspector is required");
  return {
    version: "hotkey-release-image-inventory-v1",
    git_revision: gitRevision,
    applications: APPLICATION_IMAGES.map(([name, reference]) => inspect(name, reference, inspectImage, false)),
    upstream: UPSTREAM_IMAGES.map(([name, reference]) => inspect(name, reference, inspectImage, true)),
    differences: [],
  };
}

function inspect(name, reference, inspectImage, requireRepoDigest) {
  const observed = inspectImage(reference);
  if (observed == null || !IMAGE_ID.test(observed.id ?? "")) {
    throw new Error(`${name} image is not content-addressed`);
  }
  const repoDigests = [...new Set(observed.repoDigests ?? [])].sort();
  if (repoDigests.some((value) => !REPO_DIGEST.test(value))) {
    throw new Error(`${name} image has a malformed repository digest`);
  }
  const repository = imageRepository(reference);
  if (repoDigests.some((value) => !value.startsWith(`${repository}@sha256:`))) {
    throw new Error(`${name} repository digest does not match its image reference`);
  }
  if (requireRepoDigest && repoDigests.length === 0) {
    throw new Error(`${name} upstream image has no resolved repository digest`);
  }
  return {
    name,
    reference,
    image_id: observed.id,
    repo_digests: repoDigests,
  };
}

function imageRepository(reference) {
  const lastSlash = reference.lastIndexOf("/");
  const lastColon = reference.lastIndexOf(":");
  return lastColon > lastSlash ? reference.slice(0, lastColon) : reference;
}

export function writeReleaseImageInventory(output, inventory) {
  if (typeof output !== "string" || !output.startsWith("/")) {
    throw new Error("absolute image inventory output is required");
  }
  const payload = `${JSON.stringify(inventory, null, 2)}\n`;
  let descriptor;
  let complete = false;
  try {
    descriptor = openSync(output, "wx", 0o600);
    writeFileSync(descriptor, payload, { encoding: "utf8" });
    closeSync(descriptor);
    descriptor = undefined;
    complete = true;
  } finally {
    if (descriptor != null) closeSync(descriptor);
    if (!complete) {
      try { unlinkSync(output); } catch (error) {
        if (error?.code !== "ENOENT") throw error;
      }
    }
  }
}
