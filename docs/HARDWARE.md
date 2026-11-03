# Choose your hardware

The default toolkit ZIP selects its hardware implementation automatically.
Currently it includes **Bar 9.1 only**. Check the printed model label; do not
select an installer by how similar the speakers look.

| Label / observation | Use |
| --- | --- |
| Bar 9.1 True Wireless Surround | Default toolkit ZIP; let its compatibility check decide |
| Bar 800, 1000, 1300 or 1300X, original generation | Research tools only; no USB installer |
| A label containing MK2 | Separate generation; no USB installer |
| Uncertain | Copy the model code and firmware first; there is no force-model option |

For JBL One bars, read the software version in **JBL One → Product Information**.
A 1300 owned in 2023 is original-generation; 1300 versus 1300X still needs its
label. Detachable rear speakers alone do not distinguish generations.

The dispatcher checks exact supported executable hashes and writes
`JBLToolkit/HARDWARE.txt`. Its selected installer repeats native/storage checks.
An unknown identity stops before installation. USB script execution on newer bars
is not established, so their lack of a USB report proves nothing about support.
Recovery can select an existing toolkit by its ownership marker, independently
of running firmware; its recovery script checks the store and startup hook.

[Full model status](../models/README.md) ·
[Bar 1300 test instructions](../research/bar-family/bar1300/README.md)
