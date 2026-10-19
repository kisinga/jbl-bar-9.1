# Design and research findings

`AutoScript/AutoScript` invokes the USB dispatcher. `setup.sh` verifies the payload, executable identity, storage and absence of conflicting hooks. It runs a bounded trial before installing `/var/JBLStandard` and atomically activating `/var/AutoScript.TSS`.

At boot, `run.sh` closes inherited sockets and starts a read-only watcher. It polls every five seconds, waits indefinitely for three consistent awake samples, then runs `once.sh`. The bounded runner performs its own identity/readiness checks and delegates to `controller.sh` and `helper.sh`. Current Standard Mode is a no-op. A failure can disable subsequent attempts rather than repeatedly sending keys.

The controller temporarily changes two command-console fields in process memory, verifies access, invokes the firmware's normal key-event path and restores the fields. The helper holds mute, confirms the long-press state, sends volume-up, checks mode changed from 3 to 0, and verifies command completion and released input state. No instruction bytes are patched. It installs no SSH server or network listener.

After success, the watcher marks that observed awake period handled. A later observed non-awake state or a changed process identity re-arms it. Transitions shorter than the polling interval may be missed. Some CEC/STR behavior is still unvalidated. Manual Smart Mode selection during an already handled awake period is left alone.

## Why closing sockets matters

An earlier resident watcher coincided with prolonged LOAD. Native analysis found differing client/daemon IPC transfer sizes (6168 versus 4220 bytes) and a shell child capable of retaining the accepted socket. A modeled AF_UNIX exchange with the extracted ARM shell reproduced a caller waiting while the child held that socket. Closing the inherited socket released the modeled caller. The exact live client-library hash was not captured, so the full native causal chain is not proven solely by these observations.

Revision 4 closed sockets but could exit before starting the watcher. Enumerating `/proc/$$/fd/*` can include the shell's own temporary directory descriptor. After low sockets close, that entry may have disappeared by the time `readlink` examines it. Revision 5 skips absent entries, while refusing to continue if an existing descriptor cannot be inspected. It preserves regular file descriptors, including the open script. This bug was reproduced under the extracted ARM shell; revision 5 subsequently booted and performed the action on hardware.

## What came from earlier work versus this investigation

The USB AutoScript entry point is prior published work, credited separately. Reverse engineering established the build-specific mode/readiness fields, CLI gate behavior and key-event sequence used by this implementation. Runtime checks and iteration added key-release confirmation, restoration, persistent storage/hook validation, indefinite standby waiting, and the socket-startup correction.

This is privileged local code execution through a USB mechanism, with persistent local automation. It is not an OTA/signature bypass. No credentials, remote exploit or full vendor firmware are required in the public package.
