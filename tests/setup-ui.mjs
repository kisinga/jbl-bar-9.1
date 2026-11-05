// Production embedded assets against a deterministic local API; never writes a drive.
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
const { chromium } = await import(
  process.env.PLAYWRIGHT_MODULE || "playwright"
);
const token = "browser-fixture-session",
  requests = [],
  errors = [],
  external = [];
let existing = false, nativeFailure = false;
const drive = {
  id: "fixture",
  device: "/dev/fixture",
  name: "Sample USB",
  size: 8 * 2 ** 30,
  eligible: true,
  readOnly: false,
};
const server = createServer(async (req, res) => {
  try {
    const pathname = new URL(req.url, "http://localhost").pathname;
    if (pathname.startsWith("/api/")) {
      assert.equal(req.headers["x-session-token"], token);
      let raw = "";
      for await (const chunk of req) raw += chunk;
      const body = raw ? JSON.parse(raw) : undefined;
      requests.push({ path: pathname, body });
      let result;
      switch (pathname) {
        case "/api/info":
          result = {
            version: "0.3.0-dev.9",
            packageVersion: "",
            networks: [{ address: "192.168.9.138", prefix: 27 }],
            canUpdateSelf: true,
            hasRecovery: false,
          };
          break;
        case "/api/drives":
          result = [
            {
              drive,
              hasSSH: existing,
              ...(existing
                ? {
                    config: {
                      action: "check",
                      ssh: "true",
                      web_remote: "true",
                      auto_standard: "true",
                      smart_mode_memory: "true",
                      allowed_cidr: "192.168.9.128/27",
                    },
                  }
                : {}),
              reports: {
                "STATUS.txt":
                  nativeFailure ? "PREFLIGHT_FAILED_CHECK_FIRMWARE_STORAGE_AND_NATIVE_SSH" : "CHECK_PASSED_READY_FOR_INSTALL_OR_CONFIGURE\n<script>window.injected=true</script>",
 "native-check.json": JSON.stringify(nativeFailure ? {power:"Standby",error:"turn the soundbar fully on before installation"} : {}),
              },
            },
          ];
          break;
        case "/api/download":
          result = {
            version: "0.3.0-dev.10",
            recoverySaved: true,
            selfUpdateAvailable: true,
          };
          break;
        case "/api/prepare":
          existing = body.Config.includes("ssh=true");
          result = { message: "USB written and read back successfully." };
          break;
        case "/api/self-update":
          result = {
            message:
              "Setup updated. Close this window and launch the same executable again.",
          };
          break;
        default:
          throw Error("Unexpected API " + pathname);
      }
      res.setHeader("Content-Type", "application/json");
      res.end(JSON.stringify(result));
      return;
    }
    const files = {
      "/": "wizard/index.html",
      "/app.js": "wizard/app.js",
      "/style.css": "wizard/style.css",
      "/favicon.svg": "wizard/favicon.svg",
      "/setup.js": "setup.js",
    };
    if (!files[pathname]) {
      res.writeHead(404).end();
      return;
    }
    res.setHeader(
      "Content-Type",
      pathname.endsWith(".js")
        ? "text/javascript"
        : pathname.endsWith(".css")
          ? "text/css"
          : pathname.endsWith(".svg") ? "image/svg+xml" : "text/html",
    );
    res.end(
      await readFile(
        new URL("../packaging/" + files[pathname], import.meta.url),
      ),
    );
  } catch (e) {
    errors.push(e.message);
    res.writeHead(500).end("Fixture failure");
  }
});
await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const origin = "http://127.0.0.1:" + server.address().port;
const browser = await chromium.launch({ headless: true });
try {
  const page = await browser.newPage();
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("request", (req) => {
    if (!req.url().startsWith(origin + "/")) external.push(req.url());
  });
  await page.goto(origin + "/#" + token);
  await page.waitForFunction(() =>
    document
      .querySelector("#package-status")
      .textContent.includes("checksum-verified"),
  );
  assert.equal(await page.evaluate(() => location.hash), "");
  await page.reload();
  await page.waitForFunction(() =>
    document
      .querySelector("#package-status")
      .textContent.includes("checksum-verified"),
  );
  assert.equal(await page.locator("#drive").inputValue(), "");
  assert.equal(await page.locator("#prepare").isDisabled(), true);
  assert.equal(await page.locator("#action").inputValue(), "check");
  assert.equal(await page.locator("#prefix").inputValue(), "27");
  assert.match(
    await page.locator("#network").textContent(),
    /192\.168\.9\.128\/27/,
  );
  await page.locator("#drive").selectOption("fixture");
  assert.equal(await page.locator("#prepare").isEnabled(), true);
  await page.locator("#reports").evaluate((el) => (el.open = true));
  assert.match(await page.locator("#reports-text").textContent(), /<script>/);
  assert.equal(await page.evaluate(() => window.injected), undefined);
  nativeFailure = true;
  await page.locator("#refresh").click();
  await page.waitForFunction(() => document.getElementById("drive-status").textContent.includes("turn the soundbar fully on before installation"));
  assert.equal(await page.locator("#reports").evaluate(el => el.open), true);
  nativeFailure = false;
  await page.locator("#refresh").click();
  await page.waitForFunction(() => !document.getElementById("drive-status").textContent.includes("turn the soundbar fully on before installation"));

  await page
    .locator("details")
    .last()
    .evaluate((el) => (el.open = true));
  await page.locator("#ssh").check();
  await page.locator("#password").fill("fixture-browser-password");
  assert.equal(await page.locator("#memory").isDisabled(), true);
  await page.locator("#standard").check();
  await page.locator("#memory").check();
  await page.locator("#ssid").fill("Fixture Wi-Fi");
  await page.locator("#wifiPassword").fill("fixture-wifi-secret");
  await page.locator("#prepare").click();
  await page.waitForFunction(
    () =>
      document.querySelector("#result").textContent ===
      "USB written and read back successfully.",
  );
  const first = requests.find((r) => r.path === "/api/prepare").body;
  assert.equal(first.ID, "fixture");
  assert.equal(first.Format, false);
  assert.match(first.Config, /^action=check$/m);
  assert.match(first.Config, /^ssh_password_hash=\$6\$/m);
  assert.match(first.Config, /^smart_mode_memory=true$/m);
  assert.match(first.Config, /^wifi_psk_hex=[a-f0-9]{64}$/m);
  assert(!JSON.stringify(first).includes("fixture-browser-password"));
  assert(!JSON.stringify(first).includes("fixture-wifi-secret"));
  assert.equal(await page.locator("#password").inputValue(), "");
  assert.equal(await page.locator("#wifiPassword").inputValue(), "");
  // The next operation retains existing SSH credentials without returning their hash.
  await page.waitForFunction(
    () => !document.querySelector("#saved-password").hidden,
  );
  await page.locator("#ssid").fill("");
  await page.locator("#action").selectOption("configure");
  await page.locator("#prepare").click();
  await page.waitForFunction(
    () => document.querySelector("#prepare").disabled === false,
  );
  const second = requests.filter((r) => r.path === "/api/prepare")[1].body;
  assert.equal(second.KeepSSH, true);
  assert(!second.Config.includes("ssh_password_hash="));
  // Erasure is never implicit, and the selected identifier must match exactly.
  await page.locator("#format").check();
  assert.equal(await page.locator("#prepare").isDisabled(), true);
  await page.locator("#confirm").fill("/dev/wrong");
  assert.equal(await page.locator("#prepare").isDisabled(), true);
  await page.locator("#confirm").fill("/dev/fixture");
  assert.equal(await page.locator("#prepare").isEnabled(), true);
  await page.locator("#prepare").click();
  await page.waitForFunction(
    () => document.querySelector("#prepare").disabled === false,
  );
  const third = requests.filter((r) => r.path === "/api/prepare")[2].body;
  assert.equal(third.Format, true);
  assert.equal(third.Confirm, "ERASE /dev/fixture");
  for (const width of [320, 390, 650, 900, 1440]) {
    await page.setViewportSize({ width, height: 1000 });
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
      `overflow at ${width}`,
    );
    for (const box of await page
      .locator("button:visible,select:visible,summary:visible,.check:visible")
      .evaluateAll((els) => els.map((el) => el.getBoundingClientRect().height)))
      assert(box >= 44, "small touch target");
  }
  await page.locator("#self-update").click();
  await page.waitForFunction(() =>
    document
      .querySelector("#result")
      .textContent.includes("launch the same executable again"),
  );
  assert.equal(await page.locator("#prepare").isDisabled(), true);
  assert.equal(await page.locator("#self-update").isHidden(), true);
  assert.deepEqual(errors, []);
  assert.deepEqual(external, []);
  console.log(
    "Portable setup browser: PASS (downloads, selection, credentials, reports, erase confirmation, self-update, responsive layouts)",
  );
} finally {
  await browser.close();
  await new Promise((resolve) => server.close(resolve));
}
