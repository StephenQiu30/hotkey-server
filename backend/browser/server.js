const { chromium } = require("/usr/lib/node_modules/playwright");
const wsPath = /^ws:\/\/browser:3000(\/ws\/[0-9a-f]{48})$/.exec(
  process.env.HOTKEY_BROWSER_WS_URL ?? "",
)?.[1];

if (!wsPath) {
  process.stderr.write("Browser WS configuration invalid\n");
  process.exit(1);
}

chromium
  .launchServer({
    chromiumSandbox: true,
    host: "0.0.0.0",
    port: 3000,
    wsPath,
    proxy: { server: "http://browser-egress:3128", bypass: "<-loopback>" },
  })
  .then((server) => {
    process.stdout.write("Browser server ready\n");
    process.on("SIGTERM", () => {
      void server.close();
    });
  })
  .catch(() => {
    process.stderr.write("Browser server failed\n");
    process.exitCode = 1;
  });
