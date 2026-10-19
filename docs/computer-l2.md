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

The directory contains `runtime`, `store`, `config`, `downloads`, `logs`, and
`evidence`. None may be a symlink. Install the official `lume-v0.6.1` release's
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
Jianying in the Guest only. `vnc` requires `vncdotool==1.4.2` in a separate test
virtual environment; it connects only to the test VM's loopback VNC server.
No Python or application dependency is added to the product.

Run each command with `python3 scripts/lume_live.py --root TEST_ROOT`:

| Phase | Command | Evidence |
| --- | --- | --- |
| Doctor | `doctor` | Pinned Lume version, valid bundle signature, isolated storage and telemetry configuration |
| Download | `download` | Apple macOS 27.0.1 build 26A434 image; committed ranges have SHA-256 receipts, and the complete 26,637,307,067-byte image must match `2f016638293c3e641b8b25391a76fbc16563b3711915a5551cf8aa0f5598a5c1` |
| Provision | `create TEST_ROOT/downloads/UniversalMac_27.0.1_26A434_Restore.ipsw` | Refuses an existing VM; rechecks image digest; new 4-vCPU, 8-GiB, 64-GiB disk VM with unattended login and SSH |
| Launch | `serve` in an owned foreground execution, then `start` | Loopback API on port 17774; server configuration must contain the owned store before any lifecycle request |
| Drive | `guest 'EXPLICIT_TEST_COMMAND'`; `vnc 'JSON_ACTIONS'` | Actual Guest process/application results; VNC handles initial consent within the Guest |
| Observe | `state`; `vnc '[["capture","desktop.png"]]'` | Provider state with VNC URL removed; screenshot confined to `TEST_ROOT/evidence` |
| Cleanup | `stop`, then stop and join the owned foreground server | Requests OS shutdown inside the synthetic Guest through Lume `shutdown`; requires stopped provider state within 90 seconds and never falls back to power-off; inspect resources and owned process handles |

Lume 0.6.1's HTTP `/stop` and process SIGTERM invoke framework power-off; they
are not evidence of graceful Guest shutdown. `poweroff` is an explicit abnormal
recovery action and must never be recorded as a clean checkpoint. The synthetic
unattended Guest uses Lume's built-in test login; do not pass a personal account
or credential to this harness.

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
