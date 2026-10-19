# Credits and prior work

## Foundation: touch MAX and almirus/jbl9.1

I built on the work published in **[“Запуск ssh на прошивке JBL 9.1” — touch MAX, 8 January 2024](https://touch-max.ru/vzlom/zapusk-ssh-na-proshivke-jbl-9-1)**. The article links to **[almirus/jbl9.1](https://github.com/almirus/jbl9.1)**, including its [AutoScript folder](https://github.com/almirus/jbl9.1/tree/master/AutoScript).

That work documented USB-triggered AutoScript execution on the Bar 9.1, used it for access/backup work, described persistent storage locations, and explicitly invited the community to investigate keeping Smart Mode off. The article also acknowledges earlier MediaTek/Blu-ray AutoScript discussions. I am crediting the published Bar 9.1 foundation here, not claiming it originated the entire MediaTek mechanism.

**The USB execution entry point is theirs to credit, not my discovery.** The article is published under the site's `admin` byline; I use the public site name and linked GitHub handle rather than infer a personal identity. I do not redistribute their SSH/backup payload, firmware dumps or other repository contents in my installation ZIP.

## Official behavior and related automation

- **[JBL's Smart Mode support article](https://support.jbl.com/howto/jbl-bar-9-1-lacking-dialogue-too-much-surround-us/000017755.html):** I rely on JBL's documented mute-hold/volume-up toggle and the documented reset on power-on. That key sequence is not my invention.
- **[SimenCodes' Smart Mode PR for MrBearPresident/JBL_Soundbar](https://github.com/MrBearPresident/JBL_Soundbar/pull/6):** this earlier Home Assistant/network approach was merged on 11 November 2024, with testing reported on Bar 1000. I credit it as related prior automation, not the USB entry point or evidence that my code supports those models.

## What I added

I put together a self-contained, reversible Bar 9.1 workaround with exact-build checks, current-mode readback, guarded temporary console access, verified key delivery/release, console restoration, and a persistent helper that waits through startup and standby. I also corrected the inherited-socket/descriptor handling needed by my resident watcher. My confirmed scope and remaining limits are in [the validation record](VALIDATION.md).

I used REA and Ghidra for analysis, BusyBox/QEMU and synthetic fixtures for testing, and local-model reviews as advisory input. I base the release claims on source checks and the logs from my bar. I make no world-first claim and have not exhausted every earlier implementation. My MIT license covers the code and documentation I distribute here; it does not relicense linked upstream projects or vendor firmware.

JBL and related names belong to their respective owners. I am not affiliated with JBL or claiming endorsement from the projects credited above.
