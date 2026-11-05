"use strict";
(() => {
  const $ = (id) => document.getElementById(id);
  let token = location.hash.slice(1);
  // Keep authentication across reloads in this tab, never credentials/settings.
  try {
    token ||= sessionStorage.getItem("jbl-setup-token") || "";
    if (token) sessionStorage.setItem("jbl-setup-token", token);
  } catch {} // The launch link still works when session storage is unavailable.
  history.replaceState(null, "", location.pathname);
  let busy = false,
    hasPackage = false,
    restartNeeded = false,
    rows = [],
    selected = null;
  async function api(path, body) {
    const response = await fetch("/api/" + path, {
      method: body === undefined ? "GET" : "POST",
      headers: {
        "X-Session-Token": token,
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    });
    const type = response.headers.get("content-type") || "";
    if (!response.ok) {
      let message = type.includes("json")
        ? (await response.json()).error
        : await response.text();
      throw Error(message || "Setup request failed.");
    }
    return type.includes("json") ? response.json() : response;
  }
  function message(text, error = false) {
    $("result").textContent = text;
    $("result").dataset.error = String(error);
  }
  function state() {
    const d = selected?.drive,
      format = $("format").checked;
    $("format").disabled =
      busy || !d || d.readOnly || d.size < 64 * 2 ** 20 || d.size > 2 * 2 ** 40;
    $("erase-warning").hidden = !format;
    $("confirm").required = format;
    $("erase-device").textContent = d?.device || "Select a drive";
    $("password").disabled = busy || !$("ssh").checked;
    $("password").required = $("ssh").checked && !selected?.hasSSH;
    $("saved-password").hidden = !selected?.hasSSH;
    $("memory").disabled = busy || !$("standard").checked;
    if (!$("standard").checked) $("memory").checked = false;
    $("prepare").disabled =
      busy ||
      restartNeeded ||
      !hasPackage ||
      !d ||
      d.readOnly ||
      (!d.eligible && !format) ||
      (format && $("confirm").value !== d.device);
    $("review").textContent = !d
      ? "Select a drive to review where files will be written."
      : (format ? "Erase and prepare " : "Write toolkit/configuration to ") +
        d.name +
        " · " +
        d.device +
        " · " +
        (d.size / 2 ** 30).toFixed(1) +
        " GiB.";
  }
  function working(on) {
    busy = on;
    $("prepare").setAttribute("aria-busy", String(on));
    $("result").dataset.busy = String(on);
    for (const el of document.querySelectorAll(
      "#wizard input,#wizard select,#update,#refresh,#self-update,#recovery",
    ))
      el.disabled = on;
    state();
  }
  function network() {
    try {
      $("network").textContent =
        "Allowed home network: " +
        JBLSetup.subnet($("address").value, $("prefix").value);
    } catch {
      $("network").textContent =
        "Use your computer’s private IPv4 address on the soundbar’s home network.";
    }
  }
  function select() {
    selected = rows.find((row) => row.drive.id === $("drive").value) || null;
    const d = selected?.drive;
    $("drive-status").textContent = d
      ? (d.eligible ? "Ready · first FAT32 partition on MBR" : d.reason) +
        (selected.config ? " · Existing toolkit configuration found" : "")
      : "Insert a USB and select it above.";
    $("reports").hidden =
      !selected || !Object.keys(selected.reports || {}).length;
    $("reports-text").textContent = Object.entries(selected?.reports || {})
      .map(([name, text]) => name + "\n" + text)
      .join("\n\n");
    const status = (selected?.reports?.["STATUS.txt"] || "").split("\n")[0].trim();
    if (status) $("drive-status").textContent += " · Last soundbar result: " + status;
    if (/FAILED|BLOCKED/.test(status)) {
      try {
        const native = JSON.parse(selected.reports["native-check.json"] || "{}");
        if (typeof native.error === "string" && native.error) {
          $("drive-status").textContent += " · " + native.error;
          $("reports").open = true;
        }
      } catch { /* Keep raw reports available when a device report is incomplete. */ }
    }
    $("confirm").value = "";
    $("format").checked = false;
    const cfg = selected?.config;
    if (cfg) {
      for (const [field, key] of [
        ["web", "web_remote"],
        ["ssh", "ssh"],
        ["standard", "auto_standard"],
        ["memory", "smart_mode_memory"],
      ])
        $(field).checked = cfg[key] === "true";
      $("action").value = cfg.action || "check";
      if (cfg.allowed_cidr) {
        const [address, prefix] = cfg.allowed_cidr.split("/");
        $("address").value = address;
        $("prefix").value = prefix;
        network();
      }
    }
    state();
  }
  async function refresh() {
    const id = $("drive").value;
    rows = await api("drives");
    $("drive").replaceChildren(new Option("Select a USB drive…", ""));
    for (const row of rows) {
      const d = row.drive;
      $("drive").add(
        new Option(
          d.name +
            " · " +
            d.device +
            " · " +
            (d.size / 2 ** 30).toFixed(1) +
            " GiB" +
            (d.eligible ? "" : " · " + d.reason),
          d.id,
        ),
      );
    }
    if (rows.some((row) => row.drive.id === id)) $("drive").value = id;
    select();
    if (!rows.length)
      $("drive-status").textContent =
        "No removable USB detected. Insert one and refresh.";
  }
  async function download() {
    working(true);
    $("package-status").textContent =
      "Finding and verifying the latest public toolkit and recovery files…";
    try {
      const data = await api("download", {});
      hasPackage = true;
      $("package-status").textContent =
        "Toolkit " + data.version + " · downloaded and checksum-verified";
      $("recovery").hidden = !data.recoverySaved;
      $("self-update").hidden = !data.selfUpdateAvailable;
      message("Toolkit ready. Choose a USB to continue.");
    } catch (e) {
      $("package-status").textContent = e.message;
      message(e.message, true);
    } finally {
      working(false);
    }
  }
  $("drive").onchange = select;
  $("format").onchange = state;
  $("confirm").oninput = state;
  $("ssh").onchange = state;
  $("standard").onchange = state;
  $("address").oninput = network;
  $("prefix").onchange = network;
  $("network-select").onchange = () => {
    const option = $("network-select").selectedOptions[0];
    if (option.value) {
      $("address").value = option.value;
      $("prefix").value = option.dataset.prefix;
      network();
    }
  };
  $("refresh").onclick = async () => {
    working(true);
    try {
      await refresh();
    } catch (e) {
      message(e.message, true);
    } finally {
      working(false);
    }
  };
  $("update").onclick = download;
  $("recovery").onclick = async () => {
    try {
      const response = await api("recovery");
      const blob = await response.blob();
      const link = document.createElement("a");
      const object = URL.createObjectURL(blob);
      link.href = object;
      link.download = "jbl-toolkit-recovery.zip";
      link.click();
      setTimeout(() => URL.revokeObjectURL(object), 1000);
    } catch (e) {
      message(e.message, true);
    }
  };
  $("self-update").onclick = async () => {
    working(true);
    message(
      "Downloading and verifying the setup executable for this platform…",
    );
    try {
      const data = await api("self-update", {});
      restartNeeded = true;
      message(data.message);
      $("self-update").hidden = true;
    } catch (e) {
      message(e.message, true);
    } finally {
      working(false);
    }
  };
  $("wizard").onsubmit = async (event) => {
    event.preventDefault();
    if (!selected || busy) return;
    const d = selected.drive,
      format = $("format").checked;
    if (format && $("confirm").value !== d.device) {
      message(
        "Enter the selected drive identifier to confirm erasing it.",
        true,
      );
      return;
    }
    working(true);
    message(
      format
        ? "Preparing USB. Approve the system administrator prompt to format the selected drive. Do not disconnect it."
        : "Writing and reading back the selected USB. Do not disconnect it.",
    );
    try {
      const keepSSH =
        $("ssh").checked && !$("password").value && !!selected.hasSSH;
      let config = await JBLSetup.configuration({
        address: $("address").value,
        prefix: $("prefix").value,
        port: selected.config?.web_port ?? 80,
        action: $("action").value,
        web: $("web").checked,
        ssh: $("ssh").checked && !keepSSH,
        standard: $("standard").checked,
        memory: $("memory").checked,
        password: $("password").value,
        ssid: $("ssid").value,
        wifiPassword: $("wifiPassword").value,
      });
      if (keepSSH) config = config.replace(/^ssh=false$/m, "ssh=true");
      const data = await api("prepare", {
        ID: d.id,
        Config: config,
        Format: format,
        Confirm: format ? "ERASE " + $("confirm").value : d.device,
        KeepSSH: keepSSH,
      });
      message(data.message);
      $("password").value = "";
      $("wifiPassword").value = "";
      await refresh();
    } catch (e) {
      message(e.message, true);
    } finally {
      working(false);
    }
  };
  (async () => {
    working(true);
    try {
      const data = await api("info");
      $("version").textContent = "Setup " + data.version;
      hasPackage = !!data.packageVersion;
      if (data.localSource) {
        $("update").hidden = true;
        $("files-origin").textContent =
          "Built from this checkout. Restart setup to rebuild after source changes.";
      }
      $("package-status").textContent = hasPackage
        ? "Local toolkit " + data.packageVersion + " · package verified" + (data.features ? " · Front EQ " + (data.features.frontEQ ? "included" : "not included") + " · Bluetooth playback " + (data.features.bluetooth ? "included" : "not included") : "")
        : "No local toolkit package selected.";
      $("self-update").hidden = !data.canUpdateSelf;
      $("recovery").hidden = !data.hasRecovery;
      for (const n of data.networks) {
        const option = new Option(n.address + "/" + n.prefix, n.address);
        option.dataset.prefix = n.prefix;
        $("network-select").add(option);
      }
      if (data.networks.length) {
        $("network-select").value = data.networks[0].address;
        $("address").value = data.networks[0].address;
        $("prefix").value = String(data.networks[0].prefix);
        network();
      }
      await refresh();
    } catch (e) {
      message(e.message, true);
    } finally {
      working(false);
    }
    if (!hasPackage) await download();
  })();
})();
