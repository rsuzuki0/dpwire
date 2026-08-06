# Testing

P0 is evaluated without a physical device:

```sh
go run ./tools/eval -mode=developer
go run ./tools/eval -mode=ci
```

`developer` runs formatting, static analysis with `go vet`, architecture and
spec checks, unit/protocol tests, coverage, and CLI smoke tests. `ci` adds the
race detector and cross-builds. `nightly` additionally fuzzes cryptographic
code. CI also runs a pinned Staticcheck release. Every mode enforces at least
60% aggregate statement coverage; P0 currently exceeds that floor.

The simulator uses an ephemeral TLS certificate. Tests must use the client or
CA material returned by `dptest.Simulator`; disabling TLS verification is not
permitted.

Pairing is intentionally unavailable in P0. Registration paths return HTTP 501
with `P0_PAIRING_RESERVED` until P3 implements and tests the complete protocol.
