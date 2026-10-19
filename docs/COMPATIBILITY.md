# Compatibility

| Item | Supported / observed |
| --- | --- |
| Model | JBL Bar 9.1 True Wireless Surround |
| Recorded firmware label | 23.06.41.80; display label not independently verified |
| `bdpprog` SHA-256 | `86ec83472a10dd4278285123f8ecbfb96bdd97444dd98c3a56d5d87441c90b63` |
| `/bin/cli` SHA-256 | `8910fb852d2f994a519615fede404fbb036e3a8f5264cc69cdff57038af3c78f` |
| USB path expected by device | `/mnt/sda1`, FAT32 |
| Persistent storage | `/var` resolves to writable UBIFS `/mnt/ubi_boot/var` |
| Confirmed hardware | My Bar 9.1, revision 5 scripts, 2026-10-19 |

The installer checks identity and prerequisites itself. Ordinary users do not need to extract firmware or calculate device hashes. A version label alone is insufficient: region/build differences may matter. Unknown executables, conflicting hooks and modified payloads cause refusal. **Do not remove these checks or reuse fixed addresses for another build.**

Fresh installations and reinsertion of this exact release are supported. The installer also recognizes the exact previous development revision 4 for backed-up replacement. It does not automatically migrate arbitrary old scripts, other AutoScript projects or modified installations.

No claim of compatibility with Bar 1000/1300, other JBL models or all MediaTek devices is made. A future firmware update may invalidate the hashes or remove the installed hook. Do not downgrade/update firmware just to try this project without separately establishing the proper vendor procedure.

I used a 512 MiB FAT32 partition. I have not systematically tested other sizes/layouts. Formatting is unnecessary if a suitable FAT32 drive already works. The helper has no USB serial-number restriction or owner credentials.
