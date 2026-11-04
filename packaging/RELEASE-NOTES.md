Single-USB JBL Bar Toolkit with automatic hardware selection.

1. Download **jbl-bar-toolkit-…-usb.zip** and its matching **-recovery.zip**.
2. Extract the USB ZIP to FAT32 and open **SETUP.html**.
3. Create the local config, replace `JBLToolkit/config.txt`, and run **Check**.
4. Read `STATUS.txt` and `HARDWARE.txt`; install only after checks pass.

The current package contains Bar 9.1 only, selected by exact executable identity.
Bar 800/1000/1300/1300X and MK2 installation is not implemented. Their USB startup
routes still need research. Model-specific ZIPs are also provided by CI.

Full combined installation, USB-free restart and recovery qualification remains
pending. Recovery is independent of the remote and config. Keep an existing
installation's original USB/recovery. Choose **Install / update everything**
to install, migrate a recognized standalone setup, or update a toolkit. Toolkit
updates retain installed settings, SSH credentials/host keys and sound settings.
Unknown or modified installations stop before replacement. This updates toolkit
software, not JBL firmware.
Public ZIPs contain no owner credentials. Configured USBs must stay private.
