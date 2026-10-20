# Archived Ghidra research helpers

Copied unchanged from the original firmware spike. These are optional research tools, not part of installation, release archives or the 102 host checks. They were used with the existing Ghidra 12.1.x analysis environment. Their current behavior has not been requalified as part of packaging.

- `DecompileMulti.java`: decompile functions containing supplied hexadecimal offsets; adds **0x10000** to each input for the original import layout.
- `FuncRange.java`: list function sizes in a hexadecimal range, using the same **0x10000** adjustment.
- `FindMovwRefs.java`: heuristic scan for MOVW/MOVT constant construction and decompile containing functions. Matching immediates alone do not prove data flow or register identity.
- `FindPCRelString.java`: exploratory ARM/Thumb PC-relative reference search, with assumptions about import base and instruction mode. Treat hits as candidates for manual inspection, not proof.

Use Ghidra's Script Manager or headless `-scriptPath` / `-postScript` with an already imported, analyzed program. Read each script before supplying arguments; adapt the import-base assumptions for your own project. No firmware or decompiled vendor code is bundled here. Full original projects remain in the ignored local archive described in `research/README.md`.
