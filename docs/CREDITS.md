# Credits and prior work

- [JBL's Smart Mode support article](https://support.jbl.com/howto/jbl-bar-9-1-lacking-dialogue-too-much-surround-us/000017755.html) documents the manual mute-hold/volume-up toggle and the reset on power-on.
- [almirus / touch MAX's JBL 9.1 repository](https://github.com/almirus/jbl9.1) and [article](https://touch-max.ru/vzlom/zapusk-ssh-na-proshivke-jbl-9-1) published the USB AutoScript approach. That entry mechanism is prior work. This repository does not redistribute its SSH/backup payload or vendor firmware.
- [SimenCodes' Smart Mode change for JBL_Soundbar](https://github.com/MrBearPresident/JBL_Soundbar/pull/6) is prior network/Home Assistant automation, reported tested on Bar 1000. It does not establish Bar 9.1 compatibility.
- REA and Ghidra supported local firmware analysis; BusyBox/QEMU and synthetic host fixtures supported testing. Local model reviews were advisory; shipped behavior follows source checks and hardware evidence.

This project contributes a self-contained Bar 9.1 implementation with mode readback, guarded temporary console access, key-release checks, restoration and persistent startup handling. No world-first or exhaustive novelty claim is made. JBL and related names belong to their respective owners; the project is unaffiliated.
