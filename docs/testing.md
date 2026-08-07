# Testing

The implemented authentication, pairing, PDF, profile, and deletion surface is
evaluated without a physical device:

```sh
go run ./tools/eval -mode=developer
go run ./tools/eval -mode=ci
```

`developer` runs formatting, static analysis with `go vet`, architecture and
spec checks, unit/protocol tests, coverage, and CLI smoke tests. `ci` adds the
race detector and cross-builds. `nightly` additionally fuzzes cryptographic
code. `release` adds two complete deterministic archive builds and requires
their files to be byte-identical. CI and the release gate also run pinned
Staticcheck and `govulncheck` releases.
Every mode enforces at least 60% aggregate statement coverage.

`tools/release` creates four user binary archives, a tracked-source archive,
`release.json`, and `SHA256SUMS`. Production generation refuses a dirty
worktree, an existing output directory, or an exact tag/version mismatch. The
binary archives normalize timestamps, ownership, file order, and permissions;
the linker omits paths, VCS metadata, and a variable build ID.

The simulator uses an ephemeral TLS certificate. Tests must use the client or
CA material returned by `dptest.Simulator`; disabling TLS verification is not
permitted.

The read integration test performs the complete nonce/signature/cookie exchange,
forces one-item pagination, resolves a Unicode path, reads device status, and
streams a PDF through the authenticated simulator. This proves internal
consistency, not physical-device compatibility.

The pairing emulator implements the complete registration server state
machine on a separate HTTP endpoint. Tests cover RFC 3526 DH agreement,
PBKDF2-HMAC-SHA256, the custom AES-CBC wrap integrity check, raw 256/257-byte
Java BigInteger transcripts, wrong PIN, transcript corruption, interruption,
cleanup, repeated registration, fresh RSA identity generation, and immediate
nonce-signature authentication with the new identity. Emulator success does
not promote registration to physical-device verified.

The 2026-08-06 physical pairing check registered a fresh RSA identity on a Sony
DPT-RP1 running firmware `1.6.50.14130`. The device displayed an eight-digit
PIN. All registration messages and cleanup succeeded, and the resulting
owner-private `rp1-direct` profile authenticated immediately. After the Sony
Digital Paper App was quit, the new profile again authenticated directly and
read firmware, battery, storage, and the root folder listing. No PIN, client ID,
private key, certificate fingerprint, or document metadata was recorded.

The write integration test covers root and folder resolution, duplicate names,
folder creation and rename, device-side copy, document move/rename, multipart
PDF upload without the unspecified `file_hash` query, local SHA-256, guarded
replacement, viewer open, capacity errors, malformed and incomplete success
responses, server errors, partial failure, and revision conflict. The simulator
supports deterministic wildcard fault injection. Automated success means
internal consistency, not physical-device compatibility.

The 2026-08-06 physical write check used a USB-connected device through the Sony
application's loopback relay, one explicitly approved source PDF, and a new
child folder in an approved namespace. It exercised create, copy, rename, move,
upload, conflict rejection, guarded replacement, and download verification. It
performed no delete and no viewer-open operation. Test copies were retained on
the device so the run was recoverable and inspectable.

Profile tests use generated RSA keys and temporary configuration roots. They
cover first-import default selection, multiple profiles, explicit selection,
refusal to overwrite, owner-only permissions, safe display redaction, legacy
profile-file compatibility, and a complete authenticated CLI operation without
`-profile`.

Deletion tests cover mandatory document revisions, stale-revision rejection,
post-delete absence verification, non-empty-folder rejection, root protection,
and the exact `force_delete_flag: "false"` folder request. CLI tests prove that
`rm` removes one document and `rmdir` removes an empty folder only.

The 2026-08-06 physical deletion check created a new UUID-named root folder,
a UUID-named child folder, and a UUID-named PDF copy. It confirmed local
non-empty rejection, revision-guarded PDF deletion, and empty-only deletion of
both generated folders. All deletion targets were created specifically for the
test. The approved source PDF and retained write-test artifacts were verified present
afterward.

The installed CLI also completed a direct-USB recursive listing of a
multi-level subtree, including an empty folder, and opened page 1 of a
previously approved PDF. The committed record contains only model, firmware,
transport, operations, and outcome; it omits the selected path and object ID.

On 2026-08-07 a direct-USB `--strict` global listing was streamed into a
redacted aggregate checker. It verified canonical timestamps and byte sizes,
name/path agreement, documents-only output, absolute CLI paths, descending
modification times, and unique device IDs across the complete result. The
checker emitted counts only and did not retain entry names, paths, or IDs.

## Physical-device read-only check

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
  -download-path 'Document/Approved/safe-test.pdf'
```

The JSON report omits document names, paths, IDs, contents, client IDs, private
keys, and certificate fingerprints. Compatibility observations are reviewed
into `spec/compat/models.json` and `spec/compat/quirks.json`; the raw report is
not committed.
