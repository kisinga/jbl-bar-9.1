# Community discussions and sharing notes

I checked these links on **19 October 2026**. The dates below are the latest relevant activity I could verify, not a claim that people are replying today. I have not posted to these communities or contacted the upstream authors.

## Where I would share this

| Discussion | Activity I could verify | Why it fits |
| --- | --- | --- |
| [AVForums: JBL Bar 9.1 owners' thread, page 190](https://www.avforums.com/threads/jbl-bar-9-1-wireless-dolby-atmos-soundbar.2287796/page-190) | Replies through **17 July 2026**; **16 July** explicitly discusses the Bar 9.1 resetting Smart Mode after standby | The closest model-specific audience; the latest discussion also distinguishes newer models' app behavior. |
| [r/Soundbars: Making JBL's Smart Mode stay off](https://www.reddit.com/r/Soundbars/comments/1gochb1/making_jbls_smart_mode_stay_off/) | Indexed replies through **8 February 2026**, including a **13 January** question about a solution without Home Assistant | Relevant existing automation discussion. I would clearly label my solution **Bar 9.1 only**, not general JBL One support. |
| [r/Soundbars: JBL 9.1 smart mode permanently off](https://www.reddit.com/r/Soundbars/comments/p8uy76/jbl_91_smart_mode_permanently_off/) | Indexed replies through **10 February 2026** | Exact problem and model in the title. The original post is from 2021; I would reply only if comments remain open. |
| [r/Soundbars: JBL Bar 9.1, turn off Smart Mode!](https://www.reddit.com/r/Soundbars/comments/vzqf3w/jbl_bar_91_turn_off_smart_mode/) | Indexed replies through **12 July 2025** | Older but directly relevant and discoverable. I would not call this a recently active thread. |

For broader reach, I would make **one useful new post in [r/Soundbars](https://www.reddit.com/r/Soundbars/)**, then add a short, relevant follow-up in an existing thread if permitted. The community still has 2026 soundbar discussions; I would avoid dropping the same link into unrelated purchase/advice threads.

I could not verify Reddit's current subreddit rules or the posts' locked/archived flags: its JSON endpoints returned 403 and the rules page did not expose rule text. I would check these while logged in before posting. A recent search crawl is not evidence that a thread is still open.

**AVForums has a specific restriction:** [its rules](https://www.avforums.com/pages/rules/) prohibit posts substantially created by AI and restrict commercial/competing-site promotion. I would write an AVForums contribution personally from my actual experience, rather than paste the draft below. I am sharing a free, independent project and would state my involvement openly; that does not itself guarantee moderator approval.

## My draft for a community that permits this kind of post

**Title: JBL Bar 9.1: I made a USB workaround that turns Smart Mode off after startup**

I got tired of manually disabling Smart Mode whenever I turned on my JBL Bar 9.1, so I made a reversible USB-installed workaround.

Install once, remove USB, and the helper waits indefinitely in standby before selecting Standard Mode after wake. I confirmed a mains restart followed by delayed power-on on my bar; it took about 35 seconds after detecting power-on.

I built on [touch MAX's AutoScript article](https://touch-max.ru/vzlom/zapusk-ssh-na-proshivke-jbl-9-1) and [almirus/jbl9.1](https://github.com/almirus/jbl9.1). Credit for the USB entry point belongs to that earlier work.

Source, install/uninstall ZIPs and instructions: https://github.com/kisinga/jbl-bar9-standard-mode

This is experimental and checks one exact Bar 9.1 firmware build. It installs a persistent helper and temporarily changes console fields in RAM; it does not flash firmware. My first install needed a retry, and an earlier development version stalled startup, so keep the uninstall ZIP. Repeated standby/CEC wakes remain unverified. USB removal alone does not uninstall it.

If you try it, I would appreciate your firmware label and cold-boot/standby results.

## What I would link

I would link the repository or [confirmed v0.1.0 release](https://github.com/kisinga/jbl-bar9-standard-mode/releases/tag/v0.1.0), not the untested working branch as a finished fix. I would describe it as automatic Standard Mode after startup, not a universal firmware patch or instant permanent change to JBL's own setting.
