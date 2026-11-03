# Strict data parser. Only whitelisted tokens reach the generated shell plan.
# Never source config.txt itself. Keep validation shared by builder and device.
function fail(message) { print "Configuration: " message > "/dev/stderr"; bad=1 }
function cidr(value, parts, octets, count, i, ip, unit, private) {
    if (split(value, parts, "/") != 2 || parts[2] !~ /^[0-9]+$/ || parts[2] < 16 || parts[2] > 30) return 0
    if (split(parts[1], octets, ".") != 4) return 0
    ip=0
    for (i=1; i<=4; i++) {
        if (octets[i] !~ /^[0-9]+$/ || octets[i] > 255 || sprintf("%d", octets[i]) != octets[i]) return 0
        ip=ip*256+octets[i]
    }
    private=(octets[1]==10 || (octets[1]==172 && octets[2]>=16 && octets[2]<=31) || (octets[1]==192 && octets[2]==168))
    unit=1
    for (i=0; i<32-parts[2]; i++) unit*=2
    return private && ip % unit == 0 && sprintf("%d", parts[2]) == parts[2]
}
{
    bytes+=length($0)+1
    if (bytes>8192) { fail("file too large"); exit 1 }
    sub(/\r$/, "")
    if (length($0)>256) { fail("line too long"); next }
    sub(/^[ \t]+/, ""); sub(/[ \t]+$/, "")
    if ($0=="" || $0 ~ /^#/) next
    if (split($0, fields, "=") != 2) { fail("expected key=value at line " NR); next }
    key=fields[1]; value=fields[2]
    sub(/[ \t]+$/, "", key); sub(/^[ \t]+/, "", value)
    if (key !~ /^(format|action|ssh|web_remote|auto_standard|allowed_cidr|web_port|ssh_password_hash|wifi_ssid_hex|wifi_psk_hex)$/) { fail("unknown key at line " NR); next }
    if (key in values) { fail("duplicate " key); next }
    values[key]=value
}
END {
    count=split("format action ssh web_remote auto_standard allowed_cidr web_port", keys, " ")
    for (i=1; i<=count; i++) if (!(keys[i] in values)) fail("missing " keys[i])
    if (values["format"]!="1") fail("unsupported format")
    if (values["action"] !~ /^(install|configure|uninstall|check|verify|migrate)$/) fail("invalid action")
    for (i=3; i<=5; i++) if (values[keys[i]] !~ /^(true|false)$/) fail("invalid boolean " keys[i])
    if (!cidr(values["allowed_cidr"])) fail("expected canonical private IPv4 subnet, /16 through /30")
    port=values["web_port"]
    if (port !~ /^[0-9]+$/ || sprintf("%d", port)!=port || (port!=80 && (port<1024 || port>65535)) || port==2222) fail("invalid web_port")
    if ("ssh_password_hash" in values) {
        hash=values["ssh_password_hash"]
        if (split(hash, crypt, "\\$")!=4 || crypt[1]!="" || crypt[2]!="6" || length(crypt[3])<1 || length(crypt[3])>16 || crypt[3] ~ /[^.\/0-9A-Za-z]/ || length(crypt[4])!=86 || crypt[4] ~ /[^.\/0-9A-Za-z]/) fail("invalid SSH password hash")
    }
    if (("wifi_ssid_hex" in values) != ("wifi_psk_hex" in values)) fail("Wi-Fi needs both SSID and key")
    if ("wifi_ssid_hex" in values) {
        ssid=values["wifi_ssid_hex"]; psk=values["wifi_psk_hex"]
        if (length(ssid)<2 || length(ssid)>64 || length(ssid)%2 || ssid ~ /[^0-9a-f]/ || length(psk)!=64 || psk ~ /[^0-9a-f]/) fail("invalid Wi-Fi credentials")
    }
    if (bad) exit 1
    for (i=1; i<=count; i++) print toupper(keys[i]) "=" values[keys[i]]
    # Only bounded crypt/hex alphabets reach shell assignments. Never execute
    # configuration text, and never emit a plaintext password into a plan.
    if ("ssh_password_hash" in values) print "SSH_PASSWORD_HASH='" hash "'"
    if ("wifi_ssid_hex" in values) {
        print "WIFI_SSID_HEX=" ssid
        print "WIFI_PSK_HEX=" psk
    }
}
