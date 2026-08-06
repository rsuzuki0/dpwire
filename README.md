# Digital Paper

`digitalpaper` is a Go library and command-line foundation for managing
PDF-oriented digital paper devices. The initial targets are Sony DPT-RP1 / 
DPT-CP1 and compatible Fujitsu QUADERNO generations.

The project is being built in phases. P0 preserves protocol references and
provides reproducible checks, compatibility catalogs, crypto test vectors, and
a stateful HTTPS simulator. P1 adds imported credentials, verified TLS,
nonce-signature authentication, status queries, paginated listings, metadata
lookup, and streaming PDF downloads. Pairing and document writes remain
deliberately unavailable.

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
required for P0 or the automated P1 suite.

## P1 command line

```sh
dp -profile device.json auth
dp -profile device.json device
dp -profile device.json ls
dp -profile device.json stat 'Document/Inbox/paper.pdf'
dp -profile device.json get 'Document/Inbox/paper.pdf' paper.pdf
```

The output file of `get` must not already exist. `inspect-cert` obtains only
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
