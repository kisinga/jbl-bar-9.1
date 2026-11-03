#!/usr/bin/env python3
"""Replay pinned OTA1 ARM Smart Mode code with explicit service stubs.

Requires unicorn==2.1.4. Emulates native instructions, not a Python rewrite.
No OS syscall implementation, device connection, firmware boot or audio DSP.
The publisher and optional DSP subscriber are replayed through return. The
toggle stops after its state write, before virtualization handling.
"""
import argparse
import hashlib
import json
from pathlib import Path
import struct

from unicorn import Uc, UC_ARCH_ARM, UC_MODE_ARM, UC_HOOK_CODE
from unicorn.arm_const import (UC_ARM_REG_R0, UC_ARM_REG_R1, UC_ARM_REG_R2,
                               UC_ARM_REG_R3, UC_ARM_REG_SP, UC_ARM_REG_LR,
                               UC_ARM_REG_PC, UC_ARM_REG_C1_C0_2, UC_ARM_REG_FPEXC)

BINARY_SHA256 = "13ca77508d7b3f93707c55edf7dd95fe8b51b8253f32934aef3c2f9da4efc93c"
DSP_SHA256 = "07c1df0ccb82fc26da2ea7ceebdec282c3ba933a5d61be399da7acd4bfa3a011"
OBJECT, STACK, STOP = 0x50000000, 0x40000000, 0x60000000


def replay(binary, mode, toggle=False, dsp=False, local=False, argument_type=2):
    emulator = Uc(UC_ARCH_ARM, UC_MODE_ARM)
    emulator.mem_map(0x10000, 0x300000)
    phoff = struct.unpack_from("<I", binary, 28)[0]
    entsize, count = struct.unpack_from("<HH", binary, 42)
    for i in range(count):
        kind, offset, address, _, size, memsize, _, _ = struct.unpack_from("<8I", binary, phoff + i * entsize)
        if kind == 1:
            if not 0x10000 <= address <= address + memsize <= 0x310000:
                raise ValueError("ELF segment outside pinned replay layout")
            emulator.mem_write(address, binary[offset:offset + size])
    for address, size in ((OBJECT, 0x4000), (STACK, 0x10000), (STOP, 0x1000)):
        emulator.mem_map(address, size)

    def word(address):
        return struct.unpack("<I", emulator.mem_read(address, 4))[0]

    def cstring(address):
        data = bytearray()
        for offset in range(512):
            byte = emulator.mem_read(address + offset, 1)
            if byte == b"\0":
                return data.decode("ascii")
            data += byte
        raise ValueError("Unbounded replay string")

    def string(address):
        pointer, length = struct.unpack("<II", emulator.mem_read(address, 8))
        if length > 512:
            raise ValueError("Unbounded C++ string")
        return bytes(emulator.mem_read(pointer, length)).decode("ascii")

    emulator.mem_write(OBJECT + 0x100, struct.pack("<I", OBJECT + 0x1000))
    emulator.mem_write(OBJECT + 0xf8, struct.pack("<I", 0xfeed))
    emulator.mem_write(OBJECT + 0xc4, struct.pack("<i", mode))
    if dsp:
        # Synthetic this/vtables and one MsgPack argument; native code reads them.
        for address, value in ((OBJECT, OBJECT + 0x1000),
                               (OBJECT + 0x1000, STOP + 0x100),
                               (OBJECT + 4, OBJECT + 0x2000),
                               (OBJECT + 0x2000, OBJECT + 0x2100),
                               (OBJECT + 0x2104, STOP + 0x200),
                               (OBJECT + 0x300, OBJECT + 0x400),
                               (OBJECT + 0x420, 6), (OBJECT + 0x428, 1),
                               (OBJECT + 0x42c, OBJECT + 0x500),
                               (OBJECT + 0x500, argument_type),
                               (OBJECT + 0x508, mode), (OBJECT + 0x50c, 0),
                               (OBJECT + 0x6c, 99),
                               (OBJECT + 0x70, OBJECT + 0x800), (OBJECT + 0x74, 4)):
            emulator.mem_write(address, struct.pack("<I", value))
        emulator.mem_write(OBJECT + 0x800, b"TEST")
    emulator.reg_write(UC_ARM_REG_SP, STACK + 0xf000)
    emulator.reg_write(UC_ARM_REG_LR, STOP)
    emulator.reg_write(UC_ARM_REG_R0, OBJECT)
    emulator.reg_write(UC_ARM_REG_R1, OBJECT + 0x300 if dsp else mode)
    emulator.reg_write(UC_ARM_REG_C1_C0_2, 0xf << 20)
    emulator.reg_write(UC_ARM_REG_FPEXC, 1 << 30)
    events, logs = [], []
    stopped = False
    start, end = (0xccd7c, 0xccdf8) if toggle else (0xcc8d4, 0xccab0)
    if dsp:
        start, end = 0xb2fd4, 0xb33c0

    def hook(uc, address, size, _):
        nonlocal stopped
        if address == (end if toggle else STOP):
            stopped = True
            uc.emu_stop()
            return
        if start <= address < end:
            return
        r0, r1, r2, r3 = (uc.reg_read(reg) for reg in
                          (UC_ARM_REG_R0, UC_ARM_REG_R1, UC_ARM_REG_R2, UC_ARM_REG_R3))
        if dsp and address == 0x74a9c:
            uc.reg_write(UC_ARM_REG_R0, int(local))
        elif dsp and address == STOP + 0x100:
            events.append({"stub": "set_hmpp_eq_mode", "value": r1})
        elif dsp and address == STOP + 0x200:
            events.append({"stub": "DAP operation", "arguments": [r1, r2, r3]})
        elif dsp and address == 0x4e638:
            events.append({"stub": "exception allocation", "rejected": True})
            stopped = True
            uc.emu_stop()
            return
        elif address == (0x5bbe0 if dsp else 0x84be4):  # Borrow immutable string bytes.
            value = cstring(r1)
            uc.mem_write(r0, struct.pack("<II", r1, len(value)))
        elif address == 0x14f82c:
            events.append({"stub": "volume_fade_on_action", "owner": string(r1), "action": string(r2)})
        elif address == (0xed62c if dsp else 0x96d44):
            events.append({"stub": "sleep_for", "milliseconds": struct.unpack("<q", uc.mem_read(r0, 8))[0]})
        elif address in ((0x112cd0,) if dsp else (0xbb2d4, 0xcc510)):
            topic = string(r2)
            event = {"stub": "WAMP", "topic": topic}
            if not dsp:
                event["value"] = struct.unpack("<i", uc.mem_read(r3, 4))[0]
            events.append(event)
            uc.mem_write(r0, bytes(8))  # Synthetic successful future, no service.
        elif address == (0x4ed70 if dsp else 0x62130):
            logs.append(bytes(uc.mem_read(r1, r2)).decode("ascii"))
        elif dsp and address == 0x4eadc:
            logs.append(cstring(r1))
        elif address in ((0x4e7dc, 0x4f16c, 0x4e62c, 0x4ea7c) if dsp else (0x61e18, 0x619f8)):
            pass
        else:
            raise ValueError(f"Unstubbed native target at {address:#x}")
        uc.reg_write(UC_ARM_REG_PC, uc.reg_read(UC_ARM_REG_LR))

    emulator.hook_add(UC_HOOK_CODE, hook)
    emulator.emu_start(start, STOP + 4, count=5000)
    if not stopped:
        raise ValueError("Replay instruction budget exhausted")
    return {"input": mode, "stored_mode": word(OBJECT + (0x6c if dsp else 0xc4)),
            "events": events, "log": "".join(logs)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--dsp-binary", type=Path, help="Also replay the native DSP-service subscriber")
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    binary = args.binary.read_bytes()
    if hashlib.sha256(binary).hexdigest() != BINARY_SHA256:
        raise ValueError("Unqualified audio-ui binary identity")
    toggles = [replay(binary, mode, toggle=True) for mode in (0, 1)]
    publishes = [replay(binary, mode) for mode in (0, 1)]
    for row in toggles:
        assert row["stored_mode"] == 1 - row["input"]
    for row in publishes:
        assert row["stored_mode"] == row["input"]
        assert row["events"] == [
            {"stub": "volume_fade_on_action", "owner": "com.harman.aui", "action": "playing"},
            {"stub": "sleep_for", "milliseconds": 200},
            {"stub": "WAMP", "topic": "com.harman.mcu-cli.GPIO.mute", "value": 1200},
            {"stub": "WAMP", "topic": "com.harman.audio.audmodeChanged", "value": row["input"]}]
        assert ("on smart mode" if row["input"] == 1 else "off smart mode") in row["log"]
    report = {"binary_sha256": BINARY_SHA256, "authority": "controlled-replay",
              "engine": "Unicorn", "toggle_prefix": toggles, "publisher": publishes,
              "limitations": ["Service calls, C++ strings/logging and sleep are explicit stubs.",
                              "Toggle replay stops before virtualization and display handling.",
                              "No WAMP transport, DSP subscriber, acoustic effect or wake timing validated."]}
    if args.dsp_binary:
        dsp_binary = args.dsp_binary.read_bytes()
        if hashlib.sha256(dsp_binary).hexdigest() != DSP_SHA256:
            raise ValueError("Unqualified dsp-client binary identity")
        subscriber = []
        for local in (False, True):
            for mode in (0, 1):
                row = replay(dsp_binary, mode, dsp=True, local=local)
                expected = []
                if local:
                    expected += [{"stub": "WAMP", "topic": "com.harman.music.pause"},
                                 {"stub": "sleep_for", "milliseconds": 400}]
                expected.append({"stub": "set_hmpp_eq_mode", "value": (2 if local else 1) if mode else 0})
                if not mode:
                    expected.append({"stub": "DAP operation", "arguments": [0, 2, 1]})
                if local:
                    expected += [{"stub": "sleep_for", "milliseconds": 100},
                                 {"stub": "WAMP", "topic": "com.harman.music.resume"}]
                assert row["stored_mode"] == mode and row["events"] == expected
                subscriber.append({"local_source": local, **row})
        rejected = replay(dsp_binary, 1, dsp=True, argument_type=1)
        assert rejected["stored_mode"] == 99
        assert rejected["events"] == [{"stub": "exception allocation", "rejected": True}]
        report["dsp_subscriber"] = {"binary_sha256": DSP_SHA256, "cases": subscriber,
                                     "rejected_argument_type": rejected}
        report["limitations"][-1] = "DSP subscriber instructions replayed; EQ/DAP calls are stubs. No WAMP delivery, acoustic effect or wake timing validated."
    with args.output.open("x") as stream:
        stream.write(json.dumps(report, indent=2) + "\n")
    print(json.dumps({"toggle_cases": len(toggles), "publisher_cases": len(publishes),
                      "subscriber_cases": 5 if args.dsp_binary else 0, "passed": True}))


if __name__ == "__main__":
    main()
