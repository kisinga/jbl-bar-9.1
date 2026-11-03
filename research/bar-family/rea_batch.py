#!/usr/bin/env python3
"""Run an explicit offline REA plan without changing MCP/client configuration.

Plan: JSON array of {name, arguments, label}. One server retains its analysis
between calls. Full results stay in the output directory; stdout is a ledger.
No firmware entry point is executed by the runner itself.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("plan", type=Path)
    parser.add_argument("--rea", required=True, type=Path)
    parser.add_argument("--ghidra", type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    plan = json.loads(args.plan.read_text())
    # Deliberately exclude tools that execute target code or contact devices.
    allowed = {"open_binary", "close_binary", "search_strings", "search_procedures",
               "procedure_pseudo_code", "procedure_assembly", "xrefs",
               "resolve_containing_procedure", "read_bytes", "list_segments",
               "export_evidence_bundle", "inspect_artifact", "inspect_firmware_regions"}
    for step in plan:
        if step["name"] not in allowed:
            raise ValueError(f"Not an offline analysis operation: {step['name']}")
        if Path(step["label"]).name != step["label"]:
            raise ValueError("Labels must be filenames")
    out = args.output.resolve()
    out.mkdir(parents=True, exist_ok=False)
    env = os.environ.copy()
    if args.ghidra:
        env["GHIDRA_INSTALL_DIR"] = str(args.ghidra.resolve())
    sequence = 0
    with (out / "server.log").open("w") as log:
        proc = subprocess.Popen([str(args.rea.resolve()), "--mcp"], env=env,
                                stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                stderr=log, text=True, bufsize=1)

        def rpc(method, params):
            nonlocal sequence
            sequence += 1
            proc.stdin.write(json.dumps({"jsonrpc": "2.0", "id": sequence,
                                         "method": method, "params": params}) + "\n")
            proc.stdin.flush()
            for line in proc.stdout:
                message = json.loads(line)
                if message.get("id") == sequence:
                    return message
            raise RuntimeError("REA server ended; see server.log")

        try:
            rpc("initialize", {"protocolVersion": "2025-03-26", "capabilities": {},
                               "clientInfo": {"name": "jbl-bar-family-offline", "version": "1"}})
            proc.stdin.write(json.dumps({"jsonrpc": "2.0", "method": "notifications/initialized"}) + "\n")
            proc.stdin.flush()
            for step in plan:
                result = rpc("tools/call", {"name": step["name"], "arguments": step.get("arguments", {})})
                path = out / (step["label"] + ".json")
                path.write_text(json.dumps(result, indent=2) + "\n")
                failed = "error" in result or result.get("result", {}).get("isError", False)
                print(json.dumps({"label": step["label"], "bytes": path.stat().st_size,
                                  "failed": failed}), flush=True)
                if failed:
                    raise RuntimeError(f"REA operation failed: {path}")
        finally:
            if proc.poll() is None:
                try:
                    rpc("tools/call", {"name": "close_binary", "arguments": {}})
                finally:
                    proc.terminate()
                    proc.wait(timeout=20)


if __name__ == "__main__":
    main()
