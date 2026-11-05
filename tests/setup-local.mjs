// Linux source-launch integration using installed Go and local source.
// Python and BusyBox are deliberately absent from PATH.
// This never prepares or formats a drive.
import { spawn, execFileSync } from "node:child_process";
import { mkdtemp, mkdir, symlink, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import assert from "node:assert/strict";
if (process.platform !== "linux") {
  console.log(
    "Source launcher integration: Linux fixture only; native packaging tests cover other platforms.",
  );
  process.exit(0);
}
const root = fileURLToPath(new URL("../", import.meta.url));
const folder = await mkdtemp(join(tmpdir(), "jbl-local-source-"));
const cache = execFileSync("go", ["env", "GOCACHE"], {
  cwd: root,
  encoding: "utf8",
}).trim();
const bindir = join(folder, "path");
await mkdir(bindir);
for (const name of ["go", "lsblk"]) {
  const executable = execFileSync("/bin/sh", ["-c", "command -v " + name], {
    encoding: "utf8",
  }).trim();
  await symlink(executable, join(bindir, name));
}
const child = spawn(join(bindir, "go"), ["run", "./apps/usbsetup", "--repo", ".", "--no-open"], {
  cwd: root,
  env: {
    ...process.env,
    PATH: bindir,
    GOCACHE: cache,
  },
  detached: true,
});
let output = "",
  error = "";
child.stderr.on("data", (b) => (error += b));
try {
  const address = await new Promise((resolve, reject) => {
    const timer = setTimeout(
      () => reject(Error("Source startup timeout: " + error)),
      60000,
    );
    child.stdout.on("data", (b) => {
      output += b;
      const m = output.match(/http:\/\/127\.0\.0\.1:\d+\/#\w+/);
      if (m) {
        clearTimeout(timer);
        resolve(m[0]);
      }
    });
    child.on("error", (e) => {
      clearTimeout(timer);
      reject(e);
    });
    child.on("exit", (code) => {
      clearTimeout(timer);
      reject(Error("Source setup exited " + code + ": " + error));
    });
  });
  const url = new URL(address);
  const response = await fetch(url.origin + "/api/info", {
    headers: { "X-Session-Token": url.hash.slice(1) },
  });
  assert.equal(response.status, 200);
  const info = await response.json();
  assert.equal(info.version, "dev");
  assert.equal(
    info.packageVersion,
    (await readFile(join(root, "VERSION"), "utf8")).trim(),
  );
  assert.equal(info.hasRecovery, true);
  assert.equal(info.localSource, true);
  assert.deepEqual(info.features, {bluetooth:true, frontEQ:true});
  assert.match(error, /Building USB and recovery files from current source/);
  console.log(
    "Local script: PASS (current source rebuilt, matching recovery, installed Go, no Python/BusyBox on PATH, no USB writes)",
  );
} finally {
  try {
    process.kill(-child.pid, "SIGINT");
  } catch {}
  await new Promise((resolve) =>
    child.exitCode !== null || child.signalCode !== null
      ? resolve()
      : child.once("exit", resolve),
  );
  const generated = error.match(/Built files: (.+)\r?\n/);
  if (
    generated &&
    generated[1].startsWith(join(root, "build", "setup-payload-"))
  )
    await rm(generated[1].trim(), { recursive: true, force: true });
  await rm(folder, { recursive: true, force: true });
}
