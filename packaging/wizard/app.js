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
  let method = "usb", sshTicket = "", sshFingerprint = "";
  async function api(path, body, onProgress) {
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
    if (type.includes("x-ndjson") && onProgress) {
      const reader = response.body.getReader(), decoder = new TextDecoder();
      let buffer = "", result;
      for (;;) {
        const {value, done} = await reader.read();
        buffer += decoder.decode(value, {stream: !done});
        if (buffer.length > 2 * 1024 * 1024) throw Error("Update response exceeded its limit. Check the bar’s connection before retrying.");
        let end;
        while ((end = buffer.indexOf("\n")) >= 0) {
          const line = buffer.slice(0,end); buffer = buffer.slice(end+1);
          if (!line.trim()) continue;
          const event = JSON.parse(line);
          if (event.progress) onProgress(event.progress);
          if (event.done) result = event;
        }
        if (done) break;
      }
      if (!result) throw Error("The connection ended before the update result arrived. The bar may still be updating; check its connection before retrying.");
      return result;
    }
    return type.includes("json") ? response.json() : response;
  }
  function message(text, error = false) {
    const out = $(method === "network" ? "ssh-result" : "result");
    out.textContent = text;
    out.dataset.error = String(error);
  }
  function state() {
    $("ssh-check").disabled = busy || restartNeeded || !hasPackage;
    $("ssh-trust-button").disabled = busy || restartNeeded || !hasPackage;
    $("ssh-update").disabled = busy || restartNeeded || !hasPackage || !sshTicket;
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
    $("ssh-result").dataset.busy = String(on && method === "network");
    $("ssh-update").setAttribute("aria-busy", String(on));
    for (const el of document.querySelectorAll(
      "#wizard input,#wizard select,#network-form input,#network-form button,#usb-method,#network-method,#update,#refresh,#self-update,#recovery",
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
    resetSSH();
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
      message("Toolkit ready. Choose USB setup or Network update to continue.");
    } catch (e) {
      $("package-status").textContent = e.message;
      message(e.message, true);
    } finally {
      working(false);
    }
  }
  $("drive").onchange = select;
  function resetSSH() {
    sshTicket = ""; sshFingerprint = "";
    $("ssh-trust").hidden = true;
    $("ssh-check").hidden = false;
    state();
  }
  function setMethod(value) {
    if (busy) return;
    method = value;
    $("wizard").hidden = value !== "usb";
    $("network-form").hidden = value !== "network";
    $("usb-method").setAttribute("aria-pressed", String(value === "usb"));
    $("network-method").setAttribute("aria-pressed", String(value === "network"));
    if (value !== "network") { $("ssh-login-password").value = ""; resetSSH(); }
  }
  $("usb-method").onclick = () => setMethod("usb");
  $("network-method").onclick = () => setMethod("network");
  for (const id of ["ssh-host", "ssh-login-password"]) $(id).oninput = resetSSH;
  const sshRequest = () => ({Host: $("ssh-host").value, Password: $("ssh-login-password").value});
  async function checkSSH(trust = false) {
    if (busy || !$("network-form").reportValidity()) return;
    working(true);
    message("Finding your bar and checking the SSH connection…");
    sshTicket = "";
    try {
      const data = await api("ssh/check", {...sshRequest(), Fingerprint: trust ? sshFingerprint : ""});
      sshFingerprint = data.fingerprint || "";
      $("ssh-fingerprint").textContent = sshFingerprint;
      $("ssh-trust").hidden = !data.needsTrust;
      $("ssh-check").hidden = !!data.needsTrust;
      sshTicket = data.ticket || "";
      if (data.current) $("ssh-login-password").value = "";
      message(data.message);
    } catch (e) { message(e.message, true); }
    finally { working(false); }
  }
  $("network-form").onsubmit = event => { event.preventDefault(); checkSSH(); };
  $("ssh-trust-button").onclick = () => checkSSH(true);
  $("ssh-update").onclick = async () => {
    if (busy || !sshTicket || !$("network-form").reportValidity()) return;
    const request = {...sshRequest(), Ticket: sshTicket};
    working(true);
    message("Transferring verified files and updating your bar. Keep the bar fully on…");
    sshTicket = "";
    $("ssh-reports").hidden = true;
    try {
      const data = await api("ssh/update", request, text => message(text));
      message(data.message, !data.success);
      $("ssh-reports-text").textContent = Object.entries(data.reports || {}).map(([name,text]) => name+"\n"+text).join("\n\n");
      $("ssh-reports").hidden = !Object.keys(data.reports || {}).length;
      $("ssh-reports").open = !data.success;
    } catch (e) { message(e.message, true); }
    finally { $("ssh-login-password").value = ""; working(false); }
  };
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
