const { chromium } = require("/usr/lib/node_modules/playwright");

chromium
  .launchServer({
    chromiumSandbox: true,
    host: "0.0.0.0",
    port: 3000,
    wsPath: "/",
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
