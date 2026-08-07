# Digital Paper

`digitalpaper` is a Go library and command-line foundation for managing
PDF-oriented digital paper devices. The initial targets are Sony DPT-RP1 / 
DPT-CP1 and compatible Fujitsu QUADERNO generations.

The project is being built in phases. P0 preserves protocol references and
provides reproducible checks, compatibility catalogs, crypto test vectors, and
a stateful HTTPS simulator. P1 adds authenticated, read-only device access. P2
adds verified folder creation, PDF upload, device-side copy, move/rename, and
viewer control. P3 has added named profile onboarding, guarded deletion, direct
connection, and fresh pairing. Physical pairing validation and release
packaging remain in progress.

P3 targets a complete PDF-only daily-use release: profile onboarding, fresh
pairing, guarded deletion, packaging, installation, and recovery. Document
renderers and Markdown/LaTeX/Tectonic workflows are deliberately deferred until
after a real-use soak period.

## Names

- Go module: `github.com/rsuzuki0/digitalpaper`
- public package: `digitalpaper`
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

The first imported profile becomes the default. Daily commands therefore need
no profile flag:

```sh
dp auth
dp device
dp ls
dp ls -l Codex_dp
dp file Codex_dp/paper.pdf
dp get Codex_dp/paper.pdf
dp put paper.pdf Codex_dp
dp cp Codex_dp/paper.pdf Codex_dp/copy.pdf
dp mv Codex_dp/copy.pdf Codex_dp/Archive
dp mkdir Codex_dp/New
dp rm Codex_dp/old.pdf
dp rmdir Codex_dp/Empty
```

Use `dp profile use NAME` to change the default or `-profile NAME` for one
command. A legacy profile JSON file remains accepted through `-profile FILE`.
Profile listing and display omit client IDs and private-key paths. Imported
keys and configuration files use owner-only permissions and are not overwritten.

The CLI presents a Unix-like virtual filesystem. Device paths are always
relative to its root; `.` denotes the root and the protocol-internal
`Document/` prefix is rejected. `ls` prints names, while `ls -l` includes type,
byte size, modification time, device ID, and name. `cp` and `mv` operate within
the device; `put` and `get` transfer between the host and device. Existing
destinations are never overwritten. See `docs/cli.md`.

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

## Status

This repository is pre-release software. Unsupported operations return an
explicit error; future commands are not exposed as successful placeholders.

## License

MIT. See `LICENSE` and `NOTICE`.
