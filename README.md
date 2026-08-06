# Digital Paper

`digitalpaper` is a Go library and command-line foundation for managing
PDF-oriented digital paper devices. The initial targets are Sony DPT-RP1 / 
DPT-CP1 and compatible Fujitsu QUADERNO generations.

The project is being built in phases. P0 preserves protocol references and
provides reproducible checks, compatibility catalogs, crypto test vectors, and
a stateful HTTPS simulator. It intentionally does not implement real device
authentication, pairing, or document writes.

## Names

- Go module: `github.com/rsuzuki0/digitalpaper`
- public package: `digitalpaper`
- CLI: `dp`
- simulator: `dp-sim`

## P0 evaluation

```sh
go run ./tools/eval -mode=ci
```

Evaluation output is written below `artifacts/eval/`. No physical device is
required in P0.

## Status

This repository is pre-release software. Unsupported operations return an
explicit error; future commands are not exposed as successful placeholders.

## License

MIT. See `LICENSE` and `NOTICE`.
