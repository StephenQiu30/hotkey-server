import process from "node:process";
import { execFileSync } from "node:child_process";
import {
  buildReleaseImageInventory,
  writeReleaseImageInventory,
} from "./release-image-inventory.mjs";

const inventory = buildReleaseImageInventory(required("HOTKEY_RELEASE_GIT_REVISION"), inspectImage);
writeReleaseImageInventory(required("HOTKEY_RELEASE_IMAGE_INVENTORY_OUTPUT"), inventory);
process.stdout.write("Release image inventory recorded: applications=3 upstream=4\n");

function inspectImage(reference) {
  const payload = execFileSync(
    "docker",
    ["image", "inspect", "--format", "{{json .}}", reference],
    { encoding: "utf8", maxBuffer: 1024 * 1024, stdio: ["ignore", "pipe", "pipe"] },
  );
  const image = JSON.parse(payload);
  return { id: image.Id, repoDigests: image.RepoDigests ?? [] };
}

function required(name) {
  const value = process.env[name]?.trim();
  if (!value) throw new Error(`${name} is required`);
  return value;
}
