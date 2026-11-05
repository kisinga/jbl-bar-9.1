/* Local credential derivation shared by portable setup and known-vector tests. */
"use strict";
(() => {
  const encoder = new TextEncoder();
  const bytes = value => typeof value === "string" ? encoder.encode(value) : value;
  const concat = (...parts) => {
    parts = parts.map(bytes);
    const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
    let offset = 0;
    for (const part of parts) { out.set(part, offset); offset += part.length; }
    return out;
  };
  const repeat = (block, length) => {
    const out = new Uint8Array(length);
    for (let i = 0; i < length; i++) out[i] = block[i % block.length];
    return out;
  };
  const digest = async (...parts) => new Uint8Array(await crypto.subtle.digest("SHA-512", concat(...parts)));
  const crypt64 = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz";
  async function passwordHash(password, salt) {
    if (password.length < 8 || password.length > 256 || password.includes("\0")) throw Error("Use an SSH password of 8–256 characters.");
    if (salt === undefined) salt = Array.from(crypto.getRandomValues(new Uint8Array(16)), n => crypt64[n & 63]).join("");
    if (!/^[./0-9A-Za-z]{1,16}$/.test(salt)) throw Error("Invalid password salt.");
    const key = bytes(password), saltBytes = bytes(salt);
    let alternate = await digest(key, saltBytes, key);
    const initial = [key, saltBytes, repeat(alternate, key.length)];
    for (let n = key.length; n; n >>= 1) initial.push(n & 1 ? alternate : key);
    alternate = await digest(...initial);
    const p = repeat(await digest(repeat(key, key.length * key.length)), key.length);
    const s = repeat(await digest(repeat(saltBytes, saltBytes.length * (16 + alternate[0]))), saltBytes.length);
    for (let n = 0; n < 5000; n++) {
      const parts = [n & 1 ? p : alternate];
      if (n % 3) parts.push(s);
      if (n % 7) parts.push(p);
      parts.push(n & 1 ? alternate : p);
      alternate = await digest(...parts);
    }
    const permutation = [[0,21,42],[22,43,1],[44,2,23],[3,24,45],[25,46,4],[47,5,26],[6,27,48],[28,49,7],[50,8,29],[9,30,51],[31,52,10],[53,11,32],[12,33,54],[34,55,13],[56,14,35],[15,36,57],[37,58,16],[59,17,38],[18,39,60],[40,61,19],[62,20,41]];
    let encoded = "";
    for (const [high, middle, low] of permutation) {
      let value = alternate[high] << 16 | alternate[middle] << 8 | alternate[low];
      for (let n = 0; n < 4; n++) { encoded += crypt64[value & 63]; value >>>= 6; }
    }
    let value = alternate[63];
    for (let n = 0; n < 2; n++) { encoded += crypt64[value & 63]; value >>>= 6; }
    return `$6$${salt}$${encoded}`;
  }
  const hex = data => Array.from(new Uint8Array(data), n => n.toString(16).padStart(2, "0")).join("");
  async function wifiCredentials(ssid, password) {
    const name = bytes(ssid);
    if (name.length < 1 || name.length > 32) throw Error("Wi-Fi name must be 1–32 UTF-8 bytes.");
    if (!/^[\x20-\x7e]{8,63}$/.test(password)) throw Error("Wi-Fi password must be 8–63 printable ASCII characters.");
    const key = await crypto.subtle.importKey("raw", bytes(password), "PBKDF2", false, ["deriveBits"]);
    const psk = await crypto.subtle.deriveBits({name:"PBKDF2", hash:"SHA-1", salt:name, iterations:4096}, key, 256);
    return {ssid:hex(name), psk:hex(psk)};
  }
  function subnet(address, prefix) {
    const parts = address.trim().split(".");
    if (parts.length !== 4 || parts.some(p => !/^(0|[1-9][0-9]{0,2})$/.test(p) || +p > 255)) throw Error("Enter your computer’s IPv4 address, for example 192.168.1.20.");
    const octets = parts.map(Number);
    if (!(octets[0] === 10 || octets[0] === 172 && octets[1] >= 16 && octets[1] <= 31 || octets[0] === 192 && octets[1] === 168)) throw Error("Use your private home network address.");
    prefix = Number(prefix);
    if (!Number.isInteger(prefix) || prefix < 16 || prefix > 30) throw Error("Unsupported network size.");
    const number = octets.reduce((n, p) => n * 256 + p, 0);
    const network = (number & (0xffffffff << (32 - prefix))) >>> 0;
    return [24,16,8,0].map(shift => network >>> shift & 255).join(".") + "/" + prefix;
  }
  async function configuration(settings) {
    const allowed = subnet(settings.address, settings.prefix);
    const port = Number(settings.port ?? 80);
    if (!Number.isInteger(port) || !(port === 80 || port >= 1024 && port <= 65535 && port !== 2222)) throw Error("Web port must be 80 or 1024–65535, excluding SSH port 2222.");
    if (!["check","install","verify","configure","uninstall","migrate","update"].includes(settings.action)) throw Error("Unknown action.");
    const lines = ["# Private local JBL Bar 9.1 configuration", "format=1", "action=" + settings.action,
      "ssh=" + !!settings.ssh, "web_remote=" + !!settings.web, "auto_standard=" + !!settings.standard,
      "smart_mode_memory=" + !!(settings.standard && settings.memory),
      "allowed_cidr=" + allowed, "web_port=" + port];
    if (settings.ssh) lines.push("ssh_password_hash=" + await passwordHash(settings.password));
    if (settings.ssid || settings.wifiPassword) {
      const wifi = await wifiCredentials(settings.ssid, settings.wifiPassword);
      lines.push("wifi_ssid_hex=" + wifi.ssid, "wifi_psk_hex=" + wifi.psk);
    }
    return lines.join("\n") + "\n";
  }
  // Pure entry points also support independent known-vector and browser tests.
  const api = {passwordHash, wifiCredentials, subnet, configuration};
  if (typeof module !== "undefined") module.exports = api;
  globalThis.JBLSetup = api;
})();
