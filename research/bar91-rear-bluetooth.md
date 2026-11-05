# Bar 9.1 detachable speakers: independent Bluetooth investigation

Investigated 5 November 2026 using REA 6.7.0 and Ghidra 12.1.2. **No
independent Bluetooth mode or working firmware unlock was established. The
packaged rear receiver is a Microchip/SMSC DARR83 wireless-audio implementation,
separate from the main bar's MediaTek Bluetooth A2DP stack.** The evidence points
to needing additional Bluetooth hardware for direct phone-to-rear playback.
That last statement is an engineering inference; this investigation did not
inspect the physical rear PCB or exhaustively decompile the radio firmware.

All work was offline. No soundbar connection, firmware upload, device setting
change, or executable patch was performed. Extracted vendor binaries and full
REA evidence remain in Git-ignored research directories.

## Pinned artifacts

The original `23.06.41.80` ZIP contains documentation and `JBL_BAR_9_1.bin`.
The image is **121,130,715 bytes**, SHA-256
`028e5ea80971bfe6019efa3415fb5aeeb46a025589e0dd0f6560f09ecc6de892`.
The main `bdpprog` is SHA-256
`86ec83472a10dd4278285123f8ecbfb96bdd97444dd98c3a56d5d87441c90b63`.
Its extracted bytes were also checked directly against the original image's
SquashFS member. The older `firmware-workspace/analysis/rootfs` has a different
executable and was excluded from the conclusions below.

Main-bar addresses below are virtual addresses for this exact `bdpprog`.
Rear MCU addresses use the inferred mapping documented in the controller
follow-up section. Module offsets are file offsets in the original update
image.

## The update contains the rear firmware

The rear images are appended after the main MediaTek image, outside its Linux
root filesystem. Finding no rear-named file in the rootfs would miss them.

REA decompilation established the container layout:

- `0x729a98` reads the final **52 bytes** of `JBL_BAR_9_1.bin`.
- `0x729974` decodes four big-endian triplets of **byte-sum checksum, length,
  version**, followed by one big-endian main version word.
- `0x7dd18c` reads MCU, TX and RX sections at cumulative offsets derived from
  those lengths and checks each section's byte sum.

Supplementary Python extraction using that REA-derived layout produced:

| Section | Offset | Length | Footer version |
| --- | ---: | ---: | ---: |
| Main MediaTek image | 0 | 120,826,116 | 182 |
| Rear controller MCU | 120,826,116 | 42,403 | 46 |
| Wireless transmitter, TX | 120,868,519 | 131,072 | 18 |
| Wireless receiver, RX | 120,999,591 | 131,072 | 17 |
| Footer | 121,130,663 | 52 | Main: 182 |

All four computed unsigned 32-bit byte sums matched the footer, and the RX
section ends exactly at the footer. These checks validate the extraction
boundaries; they do not establish that a device would accept modified images.
The footer's numeric versions are module values, not replacements for JBL's
public software-version string.

| Extracted section | SHA-256 |
| --- | --- |
| MCU | `d5741e10c2e85b605ed799988a942e1992ccbf3e0477c449375219fa79a98be5` |
| TX | `0e0fa36e9404ec40b6640431f77836c0f54af5f7d20f46725bc1796f364be36f` |
| RX | `cbb20bd4f8ff197ac29cdda6f26d8a0242b18f779fbdf9a582dc0d2e03acbd4c` |

## Wireless audio and Bluetooth are separate paths

The extracted TX and RX sections contain DARR83 SDK identifiers and distinct
Harman TX/RX version labels. Their radio-band diagnostics name **5.2 GHz and
5.8 GHz**. The main bar's RX update routine at `0x7f10a8` explicitly identifies
its target as **DWHP83** and transfers receiver firmware in 16-byte units.

Microchip identifies DWHP83 as a DARR83 module in its
[KleerNet overview](https://ww1.microchip.com/downloads/en/DeviceDoc/en562499.pdf).
The [DARR83 data brief](https://ww1.microchip.com/downloads/en/DeviceDoc/darr83db.pdf)
describes a dedicated wireless audio system with I²S/S/PDIF interfaces and a
control channel. Its Bluetooth references concern radio coexistence, not a
Bluetooth A2DP receiver.

The JBL radio images also contain diagnostics for a non-KleerNet mode. Thus
the platform identification does **not** prove compatibility with an arbitrary
KleerNet USB transmitter, another JBL model, or a different soundbar.

The main bar independently initializes Bluetooth in `0x7a3fd8`, whose own
diagnostic identifies `_bta2dp_comp_init`; it calls
`a_mtkapi_a2dp_sink_enable(1)`. Those Bluetooth APIs run in the Linux bar's
MediaTek stack. Their presence in the soundbar firmware does not give the
detached rear controller a Bluetooth stack.

## What the inspected firmware implements

| Area | Static evidence | Interpretation and limit |
| --- | --- | --- |
| Rear wireless pairing | Main CLI table at `0xcc21dc` points to `0x7d3e44`, then `0x7f1520`. Registration at `0x7f0398` installs table `0xcc2408`; slot `+0x44` points to `0x7f0308`. | The wireless-pair command uses the Microchip implementation. REA instruction inspection follows `0x7f0308` to an I²C byte write at `0x7f4e5c`, with arguments `0x80`, `0xc2`, `1`. This is not a phone Bluetooth discoverability command. |
| RF configuration and identities | `0x7eff28` constructs RF-band control data and calls the I²C transport at `0x7f4cbc`; `0x7f0018` constructs receiver-ID data. | Firmware primitives exist for its own radio network. No alternate Bluetooth transport was established. |
| Rear update selection | `0x7296a0` checks dock status, UART communication and version before selecting left/right MCU and RX updates. | Rear updates are supported by vendor code; a generic toolkit flasher or custom-image acceptance was not established. |
| Rear controller features | The extracted MCU contains diagnostics for Microchip initialization, pairing/connected/idle states, amplifier controls, battery/charging, standby and firmware updates. | These identify implemented subsystems; strings alone do not prove every reachable command or physical component. |
| Rear transport mode | The rear MCU's power-button handler at `0x0800c164` reaches `0x080056a4`, which sets a flag and calls `0x08006fd8`. The latter constructs a wireless register `0xd8` write with bytes `ff ff ff fd`. | The traced operation clears the receiver pairing identity. No alternate Bluetooth audio source was established in this path. |
| Bluetooth audio | The main bar's native A2DP initializer calls the MediaTek sink API. | Confirms a separate main-bar Bluetooth path, not independent rear pairing. |
| A `standalone` string in the main executable | Cross-references lead to `0x3c63e4`, parsing `version`, `encoding`, and `standalone` with yes/no values. | It is XML declaration parsing, not a standalone-speaker feature. |

No CLI, I²C, pairing or update operation in this table was sent to hardware.
These are code paths to investigate, not instructions to call arbitrary native
addresses from a shell.

## Rear controller code follow-up

The MCU payload has a Cortex-M vector table. An inferred application mapping
of **`0x08002000`**, with Thumb reset entry **`0x0800c389`**, aligns exception
stubs and the reset routine. REA decompilation of that routine sets MSP to
`0x20000b30` and enters initialization code. This mapping suggests an 8 KiB
resident bootloader; the bootloader itself was not recovered or tested.

For analysis, a derived ELF wrapper preserves the entire MCU payload in one
load segment at that address. The wrapper is SHA-256
`8efc99fe446a6bc9b0b9e37eb05f63352b9aba4d7e059de1ec46ea81c0865315`;
it is an analyst-created artifact, not a vendor-supplied ELF. MCU addresses
in this section and the transport-mode row depend on this inferred mapping.
An earlier provisional raw import at `0x0800c000` was used only for string
inventory; its addresses are not the corrected code addresses.

The connection-state routine at `0x08006a18` reads a GPIO input and reports
wireless connected/disconnected state. Together with the transport-mode trace,
this supplies code evidence beyond string names. Neither traced handler
establishes a Bluetooth pairing mechanism. A REA defined-string search of the
corrected MCU image for Bluetooth, A2DP, AVRCP, GATT, HCI, and bounded BT/BLE
names returned no matches. The RX/TX images were inspected with supplementary
byte-offset string inventories, not ARM decompilation.

## Answer to the unlock question

The inspected release supplies firmware for the rears, so further controller
and receiver research is possible. However, it provides **no demonstrated
switch that turns them into standard Bluetooth speakers**. The positive
evidence identifies the DARR83 radio platform and a separate main-bar Bluetooth
stack. A software-only Bluetooth conversion is therefore unsupported by the
evidence and appears unlikely.

A direct phone connection would likely require adding a Bluetooth audio
receiver to the rear's audio path. Designing that modification requires PCB,
audio-interface, power and amplifier-routing evidence not collected here.
Emulating a compatible transmitter is another research question; this pass
does not establish an off-the-shelf compatible transmitter or a working
replacement protocol.

## Coverage and remaining questions

- **Answered:** main-bar versus rear radio separation; appended module
  boundaries/checksums; vendor rear update selection and docking/UART guards.
- **Partially answered:** rear controller feature coverage and radio identity.
  Hardware identification still needs a board inspection, and the DARR83 TX/RX
  firmware has not been fully decompiled. REA's advertised raw architecture
  choices do not include its 8052 core.
- **Unresolved:** any additional radio on the physical rear board; a usable
  alternate audio input; custom image acceptance and recovery; interoperability
  with another transmitter; acoustic and runtime behavior.

The main executable's REA defined-string search for combinations of
rear/surround and Bluetooth/A2DP returned no matches. This is a bounded negative
result over Ghidra-defined strings, not proof that all firmware lacks such code.

## Evidence and reuse

Complete main-bar results, plans, raw bytes, the evidence bundle and an analysis
snapshot are under `research/private/rear-bluetooth/`. Container inventories,
module extraction script, exact module identities, supplementary string offsets
and rear-controller REA results are under
`research/private/rear-bluetooth-payloads/`. Do not publish the extracted vendor
binaries or full decompiled bodies.

| Finding | REA Evidence ID |
| --- | --- |
| Footer read at `0x729a98` | `ev_5f3676da983798163bcee7b310c414df5a22ec8837890990d95e83fc65054500` |
| Footer fields at `0x729974` | `ev_4b13faf027fd14dbe655dc73112c3ca98fe4d3a89226cca8c455df54c45a7985` |
| Section loader at `0x7dd18c` | `ev_96d90ff83b29f62f17b69cf2546761654b094ef4826d94875b3fd67ce2b53dc5` |
| DWHP83 update routine | `ev_7ff4bfb6d3e9abec2c9ada6c5e05296b47d72d10f9452a9c1a456be609ede4f0` |
| Dock/UART/version update guards | `ev_3fcad4bb9f9897482ca58b90432df9f004153bdf72ed3daecc4c9514cf28f494` |
| Wireless vtable registration | `ev_5a1b9524b5b21a8c74f7198ecdf6d6deb6af4e3d9a2432a6e83575715098ee8f` |
| Wireless vtable bytes | `ev_3e412373d7ab9ce3ba6aeb3f84ae02216b1929234bd3574a45fa7c2650368f4d` |
| Pair implementation tail branch | `ev_bb91ce44b6f8d03dccd92c5fede16e727214a94b6d79f03d4adbb718312fac30` |
| Pair I²C transport | `ev_e0ce81faddd2a5b1fec2628e2cf852622872b55aaee93ffd68d975cdbb0b2093` |
| Main Bluetooth A2DP initializer | `ev_c374910da74af95287ca84ecc4171cea6efc6673d73ff29b36f829552f20ff2d` |
| Unrelated XML `standalone` parser | `ev_89f779762cf60a4a62efa506f8d8d0dd4235da05e20c70a8db30f4c81357e523` |
| Bounded main-bar rear/Bluetooth string search | `ev_76b8ffd2994a1d0070937f56e9fd78fe8af891418e11e65583758bd84c9954b2` |
| Derived MCU reset routine | `ev_b2460fca96c66ed219acd334a6bc37ccf796ef7f8e4f440c227ed8af44e68dc6` |
| Derived MCU connection-state handler | `ev_400b0ec5d35ad0d7b8b589350104db5bcdc751ff0d8e08157f6d470ecbf7d004` |
| Derived MCU transport-mode handler | `ev_2c7497c1715d08d0d0fd8a517037c80dec461c1278625b6df2c9efe89d48f0d1` |
| Derived MCU receiver pairing-ID clear | `ev_6c66916151b17703a00618066fa31df1f9dc8a67010d85fe139c43424dfa6137` |
| Bounded derived-MCU Bluetooth string search | `ev_d94fab98753689f26ad447692e2d1196965a0394a304f9a53f3dbcf4d6630874` |

Ghidra did not create function objects for several small tail-call wrappers.
Where necessary, REA instruction inspection and table-byte reads were used
instead. Some attempted numeric pseudocode selectors fell back to file-offset
resolution and selected unrelated functions; those results were excluded.
Static code inspection does not establish that the code ran on a device.
