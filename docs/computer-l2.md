# Computer Live Shadow acceptance

The baseline §9.4 requires actual Jianying compatibility acceptance before the
production Lume ComputerProvider is implemented. Fake Computer and coding sandbox
evidence does not establish this compatibility. ERII-24 remains incomplete until
the live acceptance and the production lifecycle slice are both verified.

`scripts/lume_live.py` owns the opt-in local mechanics. It is separate from
deterministic CI and does not install a LaunchAgent. Obtain authorization for the
new test VM, downloads, applications and Guest consent before provisioning.
Never point this harness at an existing personal VM.

Use a dedicated private directory below the current user's home. Its
`erii24-owned.json` marker must contain exactly:

```json
{"issue_id":"01a14029-4eab-7782-afba-3980c6c98a7c","vm":"erii24-base"}
```

The directory contains `runtime`, `store`, `config`, `downloads`, `logs`, `cache`,
`fixtures`, and `evidence`. None may be a symlink. Install the official `lume-v0.6.1` release's
`lume.app` into `runtime` without executing the general installer. Put the
following configuration in `config/lume/config.yaml`, substituting the dedicated
directory's absolute path for `TEST_ROOT`:

```yaml
defaultLocationName: "erii24"
cacheDirectory: "TEST_ROOT/cache"
cachingEnabled: false
telemetryEnabled: false
vmLocations:
  - name: "erii24"
    path: "TEST_ROOT/store"
```

The harness supplies `XDG_CONFIG_HOME` and disables telemetry. It runs Lume under
Seatbelt with host-user file access denied outside the owned directory. It does
not share host directories or synchronize the clipboard. Install CuaDriver and
Jianying in the Guest only. `vnc-session` requires `vncdotool==1.4.2` in a separate test
virtual environment; it connects only to the test VM's loopback VNC server.
No Python or application dependency is added to the product.

Run each command with `python3 scripts/lume_live.py --root TEST_ROOT`:

| Phase | Command | Evidence |
| --- | --- | --- |
| Doctor | `doctor` | Pinned Lume version, valid bundle signature, isolated storage and telemetry configuration |
| Download | `download` | Apple macOS 27.0.1 build 26A434 image; committed ranges have SHA-256 receipts, and the complete 26,637,307,067-byte image must match `2f016638293c3e641b8b25391a76fbc16563b3711915a5551cf8aa0f5598a5c1` |
| Tested seed | `pull-tested-image` | Official `macos-tahoe-cua:26.5.2` (150-GB sparse disk); refuses an existing base slot. The public seed has Guest SIP disabled; record the actual state. Host SIP is never changed |
| Provision | `create TEST_ROOT/downloads/UniversalMac_27.0.1_26A434_Restore.ipsw` | Refuses an existing VM; rechecks image digest; new 4-vCPU, 8-GiB, 64-GiB disk VM with unattended login and SSH |
| Synthetic media | `fixtures` in an empty owned fixtures directory; `media inspect NAME`; `media verify-export export.mov`; `media audio export.mp3` | Builds the project-local Swift helper under Seatbelt; two three-second H.264 clips, six-second 440-Hz WAV, two-cue SRT; exact hashes; decodes exported frames and PCM without online accounts |
| Fault injection | `crash`, collect/join `serve`, then launch again | Kills only the exact owned Lume process after comparing the Multica daemon PID and full command; explicitly abnormal, never a clean checkpoint |
| Launch | `serve` in an owned foreground execution, then `start`, `wait-ready` | Loopback API on port 17774; server configuration must contain the owned store before any lifecycle request |
| Drive | `guest 'EXPLICIT_TEST_COMMAND'`; `guest-script FIXTURE.sh`; `put FIXTURE`; `driver TOOL JSON_OBJECT`; `driver-click X Y [COUNT]` | Actual Guest process/application results; uploads use owned synthetic fixtures; Driver response metadata and screenshots are recorded in owned evidence |
| Initial consent | `vnc-session control` in the dedicated test venv, then `vnc-control 'JSON_ACTIONS'` from another foreground execution | One persistent connection; each batch must acknowledge completion; socket restricted to the private test root; `vnc-control quit` joins it |
| Observe | `state`; `driver get_desktop_state '{"session":"erii24-live"}'`; `get FIXTURE` | Provider state with VNC URL removed; Driver screenshot bytes and digest, capture metadata; bounded Guest evidence transfer checks both size and hash |
| Cleanup | `stop`, then stop and join the owned foreground server | Requests OS shutdown inside the synthetic Guest through Lume `shutdown`; requires native config lock release, removed session marker and closed VNC listener before reconciling the HTTP stopped state; never falls back to power-off |

Lume 0.6.1's HTTP `/stop` and process SIGTERM invoke framework power-off ([pinned implementation](https://github.com/trycua/cua/blob/lume-v0.6.1/libs/lume/src/VM/VM.swift#L749)); they
are not evidence of graceful Guest shutdown. `poweroff` is an explicit abnormal
recovery action and must never be recorded as a clean checkpoint.

Lume 0.6.1's same-process `SharedVM` cache can still report running after the
Guest's native teardown has completed. The harness first proves that the config
run lock, session marker and VNC listener are released. Only then may it reconcile
the cached API record through `/stop` with `force:false`; it accepts the exact
400 `Virtual machine not running: erii24-base` response and independently requires
a subsequent `stopped` response. It never sends that potentially destructive
request while the native resources remain held. `evidence/stop.json` records each
proof and the reconciliation result. A timeout is a failure with no checkpoint.
The native resource proof and strict error normalization are covered by offline
regressions.

The synthetic unattended Guest uses Lume's built-in test login; do not pass a personal account
or credential to this harness.

Use `vnc-session control` through the dedicated venv's Python. It accepts the
same action arrays as stdin mode: `capture`, `click`, `key`, `type`, and `pause`.
Capture paths are filenames within owned evidence. The client corrects the
vncdotool 1.4.2 DesktopSize request dimensions so subsequent screenshots cover
the entire current framebuffer. Keep one connection until Guest shutdown;
reconnection stability is itself a live compatibility check.

Driver sessions may expire between calls. Inspect actual errors and establish a
new session with the pinned Driver's documented session command; never blindly
retry an action whose result is unknown. Driver acknowledgements are not effect
verification: inspect a new screenshot, app state or exported file after action.

The script is an acceptance harness, not the product ComputerProvider or a new
Tool execution path. Application actions in the product must still use the shared
ToolExecutor and Security boundary.

Record the host and Guest macOS builds, Lume version, CuaDriver and Jianying
versions, application signing requirements, fixture hashes, operation results,
screenshots and cleanup result. Keep VNC URLs, passwords and authentication
fields out of the exported evidence. Use only synthetic media and project data.

Every live run must report each of the following independently as `passed`,
`failed`, or `inconclusive`, with inspectable evidence:

1. Jianying installation and launch.
2. Synthetic audio/video import.
3. Timeline edits.
4. Subtitles and transitions.
5. Audio/video export with output readback.
6. Repeated Computer Use observations/actions without stale references.
7. Graceful stop, restart, login, Driver reconnect and document reopening.
8. Abnormal process termination and recovery with file/hash readback.

An absent or unexecuted step is inconclusive. A launch screenshot is not proof of
import, editing, export or recovery. Do not switch to Parallels or change the
baseline based on an inferred compatibility failure; report the observed gap and
obtain the reviewed Provider decision if necessary.

Offline ownership, redaction and macOS Seatbelt checks:

```sh
python3 -m unittest discover -s scripts -p test_lume_live.py -v
```

Upstream sources: [Lume setup](https://cua.ai/docs/lume/quickstart),
[HTTP lifecycle API](https://cua.ai/docs/lume/reference/http-api), and
[CuaDriver in a Guest](https://cua.ai/docs/cua-driver/guides/vms-and-remote).

## Observed compatibility result

The 2026-10-19 L2 run passed the eight Jianying exercises on Guest macOS 26.5.2
(build 25F84), Host macOS 27.0.1 (26A434), Lume 0.6.1, signed CuaDriver 0.36.0,
and signed Jianying 11.6.0. The six-second MOV includes video and audio; a separate
MP3 was exported. Decoded frames show both subtitle cues and the red/blue dissolve.
All four input hashes and both export hashes matched after normal cold restart
and after SIGKILL of the owned Lume process. The editor reopened the saved project
and continued seeking to the transition and subtitle frames. Old Driver captures
were refused after restart. See `computer-l2-result.json` for exact evidence digests.

This result is scoped to that version combination and a 320×180 synthetic project.
It is not a performance claim or a production lifecycle-provider pass. Guest 27's
unattended setup did not complete reliably across restart. Repeated VNC connections
on this Host caused native SIGTRAP exits; initial consent used one persistent
connection, and subsequent application control used CuaDriver. The cache reconciliation
above is required for 0.6.1's HTTP state. The public 26.5.2 seed has Guest SIP disabled;
Host SIP and GPU preferences were untouched. The acceptance VM contains synthetic
project data and is not a clean production base image.
