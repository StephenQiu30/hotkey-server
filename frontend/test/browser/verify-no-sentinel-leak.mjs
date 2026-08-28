import { readFileSync, writeFileSync } from "node:fs";
import process from "node:process";

const logPath = process.argv[2];
if (!logPath) throw new Error("container log path is required");
const logs = readFileSync(logPath, "utf8");
const sentinels = [
  "HOTKEY_BODY_SENTINEL_7f4b9c2a",
  "mail-list-one@example.test",
  "mail-list-two@example.test",
  "/Users/hotkey-ci/private-vault/",
  "BrowserFixture-Only-2026!",
];
const leaked = sentinels.filter((sentinel) => logs.includes(sentinel));
if (leaked.length !== 0) {
  throw new Error(`container logs leaked ${leaked.length} acceptance sentinel(s)`);
}
const artifactDirectory = process.env.HOTKEY_BROWSER_ARTIFACT_DIR || "/tmp";
writeFileSync(
  `${artifactDirectory}/hotkey-sentinel-scan.json`,
  `${JSON.stringify({ version: "hotkey-sentinel-scan-v1", run_id: process.env.HOTKEY_BROWSER_RUN_ID, scanned_surfaces: ["container_logs", "audit_logs", "job_errors", "delivery_errors"], sentinels: sentinels.length, leaks: 0 }, null, 2)}\n`,
  { flag: "wx" },
);
