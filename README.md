# DPWire

`dpwire` is a Go library and command-line foundation for managing
PDF-oriented digital paper devices. The initial targets are Sony DPT-RP1 /
DPT-CP1 and compatible Fujitsu QUADERNO generations.

The current PDF-focused release supports fresh pairing, named direct or relay
profiles, device status, listing and metadata, upload/download, device-side
copy and move, folder management, guarded deletion, deterministic packaging,
and credential recovery. Fresh pairing and operation without the Sony App are
physically verified on a DPT-RP1. See the release notes for current limits.

## Why another client?

Sony [ended DPT-RP1/DPT-CP1 support and provision of the Digital Paper App and
firmware](https://www.sony.jp/digital-paper/info2/20240628.html) on 2026-03-31.
The community `dpt-rp1-py` project remains important and active; its current
feature set includes sync, FUSE mounting, and Wi-Fi management.

DPWire addresses a different operational need: an embeddable Go library and
dependency-free `dp` binary with verified TLS identity, guarded deletion,
a stateful protocol emulator, explicit hardware evidence, and reproducible
releases. See the full fair comparison and design rationale in
[English](docs/project-rationale-and-comparison.md) or
[日本語](docs/project-rationale-and-comparison.ja.md).

Fresh setup is fully independent of the Sony App. It requires no Sony App
installation or running process and no client ID, private key, or certificate
file previously created or stored by that app. `dp` generates its own client
identity and pins the certificate returned by the device during pairing. Linux
is a supported build target even though Sony documented only Windows and macOS
versions of its desktop app; physical USB pairing on Linux remains to be
validated separately.

## Names

- project: `DPWire`
- Go module: `github.com/rsuzuki0/dpwire`
- public package: `dpwire`
- CLI: `dp`
- simulator: `dp-sim`

## Evaluation

```sh
go run ./tools/eval -mode=ci
```

Evaluation output is written below `artifacts/eval/`. No physical device is
required for the automated protocol and CLI suite.

## Command line

Import an explicitly selected Sony credential directory after verifying the
certificate fingerprint:

```sh
dp inspect-cert https://127.0.0.1:58443
dp profile import-sony rp1 https://127.0.0.1:58443 VERIFIED_SHA256 /path/to/sony/workspace
dp profile list
```

Profiles record whether their address is a direct device connection or a local
vendor-app relay. Loopback imports are classified as `relay`; addresses such as
`https://digitalpaper.local:8443` are classified as `direct`. Legacy profiles
without this field remain compatible and are classified from their address.

For a fresh direct identity, connect the device by USB and run:

```sh
dp profile pair rp1-direct digitalpaper.local
```

The device displays a PIN after the authenticated key exchange begins. Enter it
at the prompt. The new RSA private key and profile are stored owner-only outside
the Sony application directory; an existing profile is never overwritten.
New installations use a `dpwire` configuration directory. Upgrades continue
using an existing legacy `digitalpaper` directory in place; keys are neither
moved nor copied implicitly.

The first imported profile becomes the default. Daily commands therefore need
no profile flag:

```sh
dp auth
dp device
dp ls
dp ls -l Documents
dp file Documents/paper.pdf
dp file --id 23
dp get --glob '*report*2026*.pdf'
dp get Documents/paper.pdf
dp put paper.pdf Documents
dp cp Documents/paper.pdf Documents/copy.pdf
dp mv Documents/copy.pdf Documents/Archive
dp mkdir Documents/New
dp rm Documents/old.pdf
dp rmdir Documents/Empty
```

Use `dp profile use NAME` to change the default or `-profile NAME` for one
command. A legacy profile JSON file remains accepted through `-profile FILE`.
Profile listing and display omit client IDs and private-key paths. Imported
keys and configuration files use owner-only permissions and are not overwritten.

The CLI provides a filesystem-like UI with familiar Unix and FTP commands; it
does not mount a filesystem. Device paths are always relative to the device
root; `.` denotes that root and the protocol-internal `Document/` prefix is
rejected. A trailing `/` is accepted on directory paths. `ls` prints names,
while `ls -l` begins each entry with a persistent nonnegative number and a
`0x` hexadecimal reference, followed by type, byte size, modification time,
the full device ID, and name. A number or hexadecimal reference can be supplied
with `--id`. `--glob` selects a basename, or a complete root-relative path when
the pattern contains `/`; quote the pattern to prevent local shell expansion.
Glob matching is case-insensitive. The verified DPT-RP1 also resolves exact
paths case-insensitively. Exactly one matching object is required. `cp` and `mv` operate within the device;
`put` and `get` transfer between the host and device. Existing destinations are
never overwritten. See `docs/cli.md`.

`rm` deletes exactly one document using the revision just resolved by the CLI.
`rmdir` deletes only an empty folder and always disables the protocol's
recursive force-delete behavior. Neither command is used implicitly by another
operation; deletion is permanent on devices that provide no trash facility.

`inspect-cert` obtains only
untrusted first-contact certificate information and sends no credentials:

```sh
dp inspect-cert 192.0.2.10
```

Existing Sony credential pairs can be enumerated without silently selecting
one:

```sh
dp credentials find "$HOME/Library/Application Support"
```

Maintainers can run the redacted read-only physical-device verification with
`go run ./tools/device-check`. A PDF is downloaded only when an explicit,
user-approved `-download-path` is supplied; see `docs/testing.md`.

## Releases

Tagged releases produce deterministic `dp` archives for macOS and Linux on
arm64 and amd64, a complete tracked-source archive, `release.json`, and
`SHA256SUMS`. Install, upgrade, rollback, and credential recovery are described
in `docs/install.md` and `docs/recovery.md`. Version-specific limits are in
`docs/release-notes.md`.

Maintainers can validate reproducibility without publishing:

```sh
go run ./tools/eval -mode=release
```

An exact version tag is required by the release workflow. The archive builder
refuses a dirty worktree, an existing output directory, and a tag/version
mismatch.

## Status

This repository is pre-release software. Unsupported operations return an
explicit error; future commands are not exposed as successful placeholders.

## License

DPWire is tested for safety as extensively as practical. All use, including
commands that modify or delete device content, is at the user's own risk. The
software is provided without warranty under the MIT License. See `LICENSE` and
`NOTICE`.
