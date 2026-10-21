# SSH Management

AIAI's SSH manager is local-first. It stores server metadata and access history
on the current computer and uses the computer's installed OpenSSH client for
connections. SSH management never requires GitHub authentication or an AIAI
online session.

## Profiles

Create, inspect, partially update, and remove profiles with:

```bash
aiai server add production --host 192.168.1.100 --user ubuntu
aiai server add ipv6-host --host 2001:db8::10 --user deploy --port 2222
aiai server list
aiai server show production
aiai server update production --port 2222
aiai server remove production
```

Aliases are case-normalized and must contain only letters, digits, `.`, `_`, or
`-`. Hosts accept ASCII DNS names, IPv4, and IPv6. Ports must be between 1 and
65535. Usernames reject whitespace, leading option characters, and shell or SSH
destination separators.

Removal asks for confirmation on an interactive terminal. Non-interactive use
must pass `--yes`. Removing a profile does not remove its historical logs.

## Key references

```bash
aiai server key set production ~/.ssh/id_ed25519
aiai server key show production
aiai server key clear production
```

Absolute paths and `~/` expansion are supported. The path must point to a
readable regular file when it is configured and is revalidated before each
connection. Only the normalized path is stored. Private key bytes are never
copied, printed, logged, uploaded, or persisted by AIAI.

A profile without a key reference uses normal OpenSSH behavior, including
ssh-agent identities and keys selected by the user's SSH configuration.

## Managed keys and discovery

```bash
aiai key generate laptop
aiai key generate legacy-rsa --type rsa --bits 4096
aiai key list
aiai key find laptop
aiai key show laptop
```

Managed key pairs are stored in the `keys` directory beside `ssh.json`. The
directory is restricted to the current user; private files use mode `0600` and
public files use `0644` on Unix platforms. Private key bytes are never placed in
JSON, logs, errors, or normal command output.

Generation invokes the installed `ssh-keygen` directly with an argument array.
In a terminal, `ssh-keygen` prompts for a passphrase normally. Scripts must opt
in explicitly to an unencrypted key with `--no-passphrase`; AIAI does not accept
a passphrase flag because command-line passphrases can leak through process
listings and shell history.

Key discovery scans AIAI's managed directory and the user's standard `~/.ssh`
directory. It identifies pairs through their public `.pub` files and derives
type, comment, and SHA-256 fingerprint from public data only. Symlinks and
unpaired or malformed files are skipped.

## Connections

```bash
aiai ssh production
```

AIAI starts `ssh` directly without a shell wrapper and attaches the terminal's
standard input, output, and error streams. The configured port, username, and
optional identity path are passed as separate arguments. OpenSSH continues to
apply its standard configuration, proxy behavior, authentication prompts, and
known-host policy. AIAI never disables host-key checking or accepts an unknown
host automatically.

The CLI returns the OpenSSH process's exit code. Ctrl+C and context cancellation
are recorded as canceled sessions where the operating system exposes them. A
missing `ssh` executable produces an actionable error.

Interactive SSH sessions do not support `--json`, because mixing a machine JSON
document with an attached terminal session would be misleading.

From the interactive AIAI home screen, use `/servers` to browse saved profiles,
`/ssh` to select a destination, `/logs` to inspect recent history, `/keys` or
`/keys <query>` to find keys, and `/keygen <name>` to generate a new Ed25519 key.
Select a key in `/keys`, then press `c` to copy its public key. AIAI-managed
keys also offer `d` to delete the private/public pair after confirmation;
keys discovered in `~/.ssh` are never deleted by the TUI.
Press `/` on the server or key list to focus search. While search is
focused, every printable key is treated as text. Enter keeps the filter and
returns to list navigation; Esc clears it. A subsequent Esc returns.
Server profiles use a single-column list; Enter opens the selected profile's
details. Selecting a destination from `/ssh` closes the active Bubble Tea screen before OpenSSH is
started. `/keygen` similarly closes the TUI before `ssh-keygen` prompts for a
passphrase. Both tools therefore receive normal terminal input and output safely.

## Access history

```bash
aiai log list
aiai log list --server production
aiai log list --server production --limit 100 --json
aiai server logs production
```

History is sorted newest first. Each entry snapshots the alias, host, and user,
along with UTC start/end timestamps, duration, status, and an exit code when one
is available. Human output converts timestamps to local time. Exit code zero is
reported as success; AIAI does not separately claim that authentication was
successful. Failed starts and cancellations are also retained. Entries left
pending by an abruptly terminated AIAI process are recovered as canceled after
the owning process is no longer running.

Logs never include passwords, private key contents, authentication tokens, or
remote command contents.

## Storage and platform requirements

The versioned data file is named `ssh.json` under the platform's AIAI config
directory:

- Linux: `${XDG_CONFIG_HOME:-~/.config}/aiai/ssh.json`
- macOS: `~/Library/Application Support/aiai/ssh.json`
- Windows: `%AppData%\aiai\ssh.json`

Managed keys use the sibling `keys` directory, for example
`~/.config/aiai/keys` on Linux and `~/Library/Application Support/aiai/keys` on
macOS.

Set `AIAI_CONFIG_DIR` to keep both `ssh.json` and the managed `keys` directory
somewhere else. This is useful in sandboxed terminals, for example:

```sh
AIAI_CONFIG_DIR="$PWD/.aiai" aiai
```

This file is separate from `credentials.json`. The directory and file are
restricted to the current user where the platform supports Unix-style modes.
Writes use a temporary sibling and atomic replacement. A short-lived lock file
serializes concurrent writers. A symlink at the AIAI storage directory, data
file, or lock file is refused.

OpenSSH must be installed separately:

- Linux: install the distribution's `openssh-client` package.
- macOS: `/usr/bin/ssh` is normally included with the operating system.
- Windows: enable the Microsoft OpenSSH Client optional feature or install a
  compatible `ssh.exe` available through `PATH`.

Use `--json` with profile, key-reference, and log commands to receive the normal
`schema_version: 1` AIAI output envelope. JSON contains key paths when requested,
but never key contents.
