# Testing

P0 and P1 are evaluated without a physical device:

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

The P1 integration test performs the complete nonce/signature/cookie exchange,
forces one-item pagination, resolves a Unicode path, reads device status, and
streams a PDF through the authenticated simulator. This proves internal
consistency, not physical-device compatibility.

Pairing is intentionally unavailable. Registration paths return HTTP 501
with `P0_PAIRING_RESERVED` until P3 implements and tests the complete protocol.

## Physical-device P1 check

`tools/device-check` performs a redacted read-only check. It never selects a
document for download automatically; `-download-path` must name a PDF the user
has explicitly approved. Without that option it authenticates and checks status
and listings but skips download.

```sh
go run ./tools/device-check \
  -address https://127.0.0.1:58443 \
  -fingerprint VERIFIED_SHA256 \
  -client-id-file /path/to/deviceid.dat \
  -private-key-file /path/to/privatekey.dat \
  -download-path 'Document/Codex-P1/safe-test.pdf'
```

The JSON report omits document names, paths, IDs, contents, client IDs, private
keys, and certificate fingerprints. Compatibility observations are reviewed
into `spec/compat/models.json` and `spec/compat/quirks.json`; the raw report is
not committed.
