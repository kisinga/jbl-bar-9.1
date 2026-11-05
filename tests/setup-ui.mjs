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
let sshTrusted = false, sshCurrent = false, sshFailure = false, sshInterrupted = false;
const fingerprint = "SHA256:fixture-public-speaker-host-key-0123456789abc";
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
                      web_port: "8080",
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
        case "/api/ssh/check":
          assert.equal(body.Password, "fixture-ssh-login");
          if (!sshTrusted && body.Fingerprint !== fingerprint) {
            result = {needsTrust:true, fingerprint, message:"First connection: confirm this speaker’s identity to continue."};
          } else {
            sshTrusted = true;
            result = {success:true, fingerprint, current:sshCurrent, ...(sshCurrent ? {} : {ticket:"fixture-checked-package"}), message:sshCurrent ? "Your bar already has this toolkit build." : "Connected. Ready to update."};
          }
          break;
        case "/api/ssh/update":
          assert.equal(body.Password, "fixture-ssh-login");
          assert.equal(body.Ticket, "fixture-checked-package");
          res.setHeader("Content-Type", "application/x-ndjson");
          res.write(JSON.stringify({progress:"Fixture transfer verified; waiting for the bar…"})+"\n");
          await new Promise(resolve => setTimeout(resolve, 250));
          if (sshInterrupted) {res.end();return;}
          result = {done:true,success:!sshFailure, message:sshFailure ? "The bar stopped the update: PREFLIGHT_FAILED." : "Bar updated and installed files verified.", reports:{"STATUS.txt":sshFailure ? "PREFLIGHT_FAILED" : "INSTALLED_PENDING_REBOOT_TEST", "setup.log":"<script>window.networkInjected=true</script>"}};
          const record=JSON.stringify(result)+"\n";
          res.write(record.slice(0,50));
          res.end(record.slice(50));
          return;
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
    .locator("#usb-access")
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
  assert.match(first.Config, /^web_port=80$/m);
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
  assert.match(second.Config, /^web_port=8080$/m);
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
  const usbWrites = requests.filter(r => r.path === "/api/prepare").length;
  await page.locator("#network-method").click();
  assert.equal(await page.locator("#wizard").isHidden(), true);
  assert.equal(await page.locator("#ssh-host").inputValue(), "jbl.local");
  assert.equal(await page.locator("#ssh-update").isDisabled(), true);
  await page.locator("#ssh-check").click();
  assert.equal(requests.filter(r => r.path === "/api/ssh/check").length, 0);
  await page.locator("#ssh-login-password").fill("fixture-ssh-login");
  await page.locator("#ssh-check").click();
  await page.locator("#ssh-trust").waitFor({state:"visible"});
  assert.equal(await page.locator("#ssh-fingerprint").textContent(), fingerprint);
  assert.equal(await page.locator("#ssh-update").isDisabled(), true);
  await page.locator("#ssh-trust-button").click();
  await page.waitForFunction(() => !document.querySelector("#ssh-update").disabled);
  assert.equal(await page.locator("#ssh-trust").isHidden(), true);
  // Changing the target invalidates the prior connection check.
  await page.locator("#ssh-host").fill("192.168.1.50");
  assert.equal(await page.locator("#ssh-update").isDisabled(), true);
  await page.locator("#ssh-check").click();
  await page.waitForFunction(() => !document.querySelector("#ssh-update").disabled);
  assert.equal(requests.filter(r => r.path === "/api/ssh/check").at(-1).body.Host, "192.168.1.50");
  assert.equal(await page.evaluate(() => JSON.stringify({...sessionStorage,...localStorage}).includes("fixture-ssh-login")), false);
  await page.locator("#ssh-update").click();
  await page.waitForFunction(() => document.querySelector("#ssh-result").textContent.includes("Fixture transfer verified"));
  assert.equal(await page.locator("#usb-method").isDisabled(), true);
  assert.equal(await page.locator("#ssh-result").getAttribute("data-busy"), "true");
  await page.waitForFunction(() => document.querySelector("#ssh-result").textContent.includes("installed files verified"));
  assert.equal(await page.locator("#ssh-login-password").inputValue(), "");
  assert.equal(await page.locator("#ssh-update").isDisabled(), true);
  assert.equal(await page.evaluate(() => window.networkInjected), undefined);
  assert.equal(requests.filter(r => r.path === "/api/prepare").length, usbWrites);
  sshFailure = true;
  await page.locator("#ssh-login-password").fill("fixture-ssh-login");
  await page.locator("#ssh-check").click();
  await page.waitForFunction(() => !document.querySelector("#ssh-update").disabled);
  await page.locator("#ssh-update").click();
  await page.waitForFunction(() => document.querySelector("#ssh-result").dataset.error === "true");
  assert.equal(await page.locator("#ssh-login-password").inputValue(), "");
  assert.equal(await page.locator("#ssh-reports").evaluate(el => el.open), true);
  assert.match(await page.locator("#ssh-reports-text").textContent(), /<script>/);
  for (const width of [320, 390, 650, 900, 1440]) {
    await page.setViewportSize({width,height:1000});
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), "network form overflow at " + width);
  }
  sshInterrupted = true;
  await page.locator("#ssh-login-password").fill("fixture-ssh-login");
  await page.locator("#ssh-check").click();
  await page.waitForFunction(() => !document.querySelector("#ssh-update").disabled);
  await page.locator("#ssh-update").click();
  await page.waitForFunction(() => document.querySelector("#ssh-result").textContent.includes("before the update result arrived"));
  assert.equal(await page.locator("#ssh-result").getAttribute("data-error"), "true");
  assert.equal(await page.locator("#ssh-login-password").inputValue(), "");
  sshCurrent = true;
  await page.locator("#ssh-login-password").fill("fixture-ssh-login");
  await page.locator("#ssh-check").click();
  await page.waitForFunction(() => document.querySelector("#ssh-result").textContent.includes("already has"));
  assert.equal(await page.locator("#ssh-login-password").inputValue(), "");
  assert.equal(await page.locator("#ssh-update").isDisabled(), true);
  await page.locator("#usb-method").click();
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
    "Portable setup browser: PASS (USB, SSH discovery/credentials/trust/update/errors, downloads, self-update, responsive layouts)",
  );
} finally {
  await browser.close();
  await new Promise((resolve) => server.close(resolve));
}
