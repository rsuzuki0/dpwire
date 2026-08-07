# Testing

The implemented P0 through P3 surface is evaluated without a physical device:

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

The P2 integration test covers root and folder resolution, duplicate names,
folder creation and rename, device-side copy, document move/rename, multipart
PDF upload without the unspecified `file_hash` query, local SHA-256, guarded
replacement, viewer open, capacity errors, malformed and incomplete success
responses, server errors, partial failure, and revision conflict. The simulator
supports deterministic wildcard fault injection. Automated success means
internal consistency, not physical-device compatibility.

The 2026-08-06 physical P2 check used one explicitly approved source PDF and a
new child folder in an approved namespace. It exercised create, copy, rename,
move, upload, conflict rejection, guarded replacement, and download verification.
It performed no delete and no viewer-open operation. Test copies were retained
on the device so the run was recoverable and inspectable.

P3 profile tests use generated RSA keys and temporary configuration roots. They
cover first-import default selection, multiple profiles, explicit selection,
refusal to overwrite, owner-only permissions, safe display redaction, legacy
profile-file compatibility, and a complete authenticated CLI operation without
`-profile`.

P3 deletion tests cover mandatory document revisions, stale-revision rejection,
post-delete absence verification, non-empty-folder rejection, root protection,
and the exact `force_delete_flag: "false"` folder request. CLI tests prove that
`rm` removes one document and `rmdir` removes an empty folder only. These are
simulator results; physical deletion remains unverified until an expendable
device fixture is separately approved.

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
