# Digital Paper

`digitalpaper` is a Go library and command-line foundation for managing
PDF-oriented digital paper devices. The initial targets are Sony DPT-RP1 / 
DPT-CP1 and compatible Fujitsu QUADERNO generations.

The project is being built in phases. P0 preserves protocol references and
provides reproducible checks, compatibility catalogs, crypto test vectors, and
a stateful HTTPS simulator. P1 adds authenticated, read-only device access. P2
adds verified folder creation, PDF upload, device-side copy, move/rename, and
viewer control. Pairing and destructive deletion remain deliberately
unavailable.

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
required for P0 through the automated P2 suite.

## Command line

```sh
dp -profile device.json auth
dp -profile device.json device
dp -profile device.json ls
dp -profile device.json ls -l Codex_dp
dp -profile device.json file Codex_dp/paper.pdf
dp -profile device.json get Codex_dp/paper.pdf
dp -profile device.json put paper.pdf Codex_dp
dp -profile device.json cp Codex_dp/paper.pdf Codex_dp/copy.pdf
dp -profile device.json mv Codex_dp/copy.pdf Codex_dp/Archive
dp -profile device.json mkdir Codex_dp/New
```

The CLI presents a Unix-like virtual filesystem. Device paths are always
relative to its root; `.` denotes the root and the protocol-internal
`Document/` prefix is rejected. `ls` prints names, while `ls -l` includes type,
byte size, modification time, device ID, and name. `cp` and `mv` operate within
the device; `put` and `get` transfer between the host and device. Existing
destinations are never overwritten. See `docs/cli.md`.

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
