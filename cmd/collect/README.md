# Fixture collector (`cmd/collect`)

Captures everything needed to author and test the AOS-S scrapligo platform
definition and the TextFSM templates against a live switch:

- `session.log` - full transcript of every command sent and its verbatim
  output (the primary artifact; everything else is a subset of this)
- `prompts.log` - the raw prompt string observed in each mode
  (`privileged_exec`, `exec`, `configuration`)
- `errors.log` - verbatim output of deliberately bad commands, in exec and
  config mode (used to build `failure_indicators`)
- `show_version.txt` - firmware/version identity of the switch
- `running_config.cfg` - full `show running-config`
- `shows/*.txt` - one file per `show` command (vlan, interfaces, lldp, arp,
  route, etc.), used to write TextFSM templates
- `summary.txt` - run status and file listing

The collector sends `no page` in `on_open_instructions` so long output
(e.g. `show running-config`) streams as a single block instead of
pausing at `-- MORE --`. A `-- MORE --` stop desynchronizes the session
and corrupts every subsequent command, so this matters. If an operation
times out anyway, the collector sends Ctrl-C (the pager's documented quit
key) before continuing and notes the resync in `session.log`.

## Prerequisites

- SSH access to the switch with a user that can enter configuration mode
  and run `write memory`.
- scrapligo is linked natively (FFI); the collector runs on any platform
  scrapligo supports (linux, darwin, windows; amd64/arm64).

## Expected starting configuration

**Recommended: an isolated lab switch in a fully configured "loaded" state.**
Rationale per option:

- **Factory-blank switch: not recommended.** Blank switches produce empty or
  header-only `show` output for most features (no VLANs beyond 1, no LACP,
  no IP interfaces, no LLDP neighbors, no ARP). Empty-state fixtures are
  valuable for parser edge cases, but a blank box gives you *only* empty
  state, which is the worse starting point. Run the collector on a blank
  box *second*, after the loaded run, to collect empty-state deltas.
- **Production switch: avoid.** The collector deliberately sends malformed
  commands (harmless, but logged) and re-runs `write memory`. It changes
  nothing, but it needs a maintenance window's worth of trust and a user
  with config privileges.
- **Isolated lab switch, loaded: recommended.** Before running, configure a
  realistic sample so every `show` has rows to parse:
  - hostname set to something recognizable (it appears in every prompt)
  - 2-3 VLANs with names (one access, one trunk-capable)
  - at least one interface in each state: default, VLAN access, trunk,
    shutdown, and one with a description
  - one IP interface (management) and a default route, so `show ip route`
    and `show ip interface` are non-empty
  - LLDP enabled, with a second LLDP-speaking device cabled in if possible
  - one static ARP entry or a host pinged recently
  - one LACP/802.3ad bundle if the chassis supports it (optional but
    valuable)
  - `show running-config` should be at least a few screens long so paging
    behavior is exercised and captured

If the lab switch is already in "something in between" state, that is
acceptable - just make sure the features above are present, and note the
gaps in the run's `session.log` by adding a header comment. Empty-state
fixtures for missing features can come from the separate blank-box run.

## Usage

```sh
export AOSS_HOST=10.0.0.10
export AOSS_USERNAME=admin
export AOSS_PASSWORD=secret

# optional
export AOSS_PORT=22
export AOSS_INTERFACE=1        # interface to exercise (default 1, always present)
export AOSS_VLAN=1             # VLAN to exercise (default 1, always present)
export AOSS_TRANSPORT=bin      # "bin" (default) or "ssh2", see below
export AOSS_OUT=fixtures/aoss_$(AOSS version)

go run ./cmd/collect
```

The same environment variables the provider uses (`AOSS_HOST`,
`AOSS_USERNAME`, `AOSS_PASSWORD`) are required. After the run, keep the
output directory per AOS-S firmware version:

```
fixtures/
  aoss_4.4.0.12/   # one directory per firmware version tested
  aoss_5.0.0.3/
```

Firmware drift in headers, column layout, and default-value display is
exactly what the definition and templates must absorb, so per-version
directories are the unit of comparison.

### Transport

- `bin` (default) - shells out to the local `ssh` client, so it inherits
  your `~/.ssh/config` and known_hosts, and works with password or key
  auth exactly like interactive ssh. Use this when `ssh user@host` works
  by hand.
- `ssh2` - built-in libssh2 transport. Useful for testing the same path
  the provider will take in production. libssh2 requires the switch's host
  key to already be in a known_hosts file for password auth; run
  `ssh-keyscan -p <port> <host> >> ~/.ssh/known_hosts` (or point
  `AOSS_KNOWN_HOSTS` at a custom file) before using it.

## Custom platform definition

By default the collector drives the switch with the same embedded AOS-S
platform definition (`internal/client/hp_aoss.yaml`) the provider uses, so
a fresh capture replays cleanly against the fixtures. If you have a
candidate `hp_aoss.yaml` you are iterating on, point the collector at it -
then the fixtures also validate your definition's prompt patterns and mode
transitions:

```sh
export AOSS_DEF_FILE=internal/client/hp_aoss.yaml
# or a stock platform name:
export AOSS_DEF_NAME=hp_comware
go run ./cmd/collect
```

## Troubleshooting

**`authenticate: operation timeout exceeded`** - with the `bin` transport,
login is done "in session": the library watches the terminal stream and
only writes when it sees a username-like prompt (default `user(name):` or
`login:`) or a password-like prompt (default `password:`), then waits for
the platform prompt pattern to declare auth complete.

AOS-S login looks like this:

```text
We'd like to keep you up to date about:
  * Software feature updates
  * New product announcements
  * Special events
Please register your products now at:  www.hpe.com/networking/register


(admin@172.16.34.76) admin@172.16.34.76's password:
```

...the password is accepted, and **then** a restricted-rights legend
appears ending in `Press any key to continue`, waiting for a keypress
before the first prompt. The library has no banner handling: after
sending the password it keeps reading, the legend matches no pattern, and
the prompt never appears, so the auth loop hits the operation timeout.

**Where exactly did it stop?** The collector always records:

- `<AOSS_OUT>/raw_session.bin` - the raw terminal stream, login banner
  included. On an open failure the last 2KB are also printed to stderr -
  if the password prompt is in there but no prompt follows, the banner is
  the blocker; if the password prompt itself is missing, the problem is
  earlier (host key, port, auth method).
- `<AOSS_OUT>/scrapli.log` - full libscrapli trace (level set via
  `AOSS_LOG_LEVEL`, default `trace`), including each auth-stage event
  ("password prompt seen", "username sent", etc.).

Workarounds, in order of preference:

1. **Use the `ssh2` transport** (`export AOSS_TRANSPORT=ssh2`). libssh2
   authenticates at the SSH protocol layer, so the in-session
   prompt-matching loop never runs - the banner simply appears after
   login and the library's normal "send a return before reading the
   prompt" step dismisses it. The host key must be trusted first:
   `ssh-keyscan -p $AOSS_PORT $AOSS_HOST >> ~/.ssh/known_hosts`.
2. **Trick the in-session auth loop with a custom username pattern.**
   Point `AOSS_USERNAME_PATTERN` at the banner text so the loop treats
   the banner as the username prompt; sending the username is a keypress,
   which is exactly what the banner wants:
   ```sh
   export AOSS_TRANSPORT=bin
   export AOSS_USERNAME_PATTERN='press any key to continue'
   ```
   Copy the banner wording verbatim (case-insensitive match). Note the
   whole username string is sent as one write; if the switch consumes
   only the first key and leaves the rest in the input buffer, one
   spurious command will appear at the first prompt - harmless, but
   visible in `session.log`.

**Other causes, if the above does not apply:** host key verification
failing on `ssh2` (use `ssh-keyscan`), the account being restricted to
key auth (use `bin`, which reuses your local ssh setup), or a wrong
`AOSS_PORT`.

## Safety

- The collector sends deliberately invalid commands (`show badcommand123`,
  `vlan 5 name <64 x's>`, `interface bogus0/0`). These are no-ops that the
  switch rejects; confirm on a production-like device that this is
  acceptable (audit logging) before running there.
- It enters config mode and visits `vlan <N>` and `interface <I>` submodes
  without issuing any configuration commands, then exits with `end` and
  runs `write memory` twice (once after capture, once at the end). It does
  not add, remove, or modify configuration.
- If `AOSS_VLAN` is overridden to an ID that does not exist, `vlan <N>`
  in config mode may create the VLAN object. The default (1) always exists
  and keeps the run a strict no-op.
- Each section is isolated: a failing section is logged to `session.log`
  and the run continues, so partial output is always useful.
