# Digital Paper v0.3.0-p3 release notes

This is the first PDF-only daily-use release candidate. It replaces the Sony
Digital Paper App for the verified DPT-RP1 direct-USB workflow while keeping the
Go communication library independently reusable.

## Included

- Fresh PIN pairing with independent RSA credentials and owner-only storage.
- Named direct and Sony-relay profiles with explicit default selection.
- Authenticated model, firmware, battery, and storage status.
- Unix/FTP-style `ls`, `ls -l`, `file`/`stat`, `get`, `put`, `cp`, `mv`,
  `mkdir`, `rm`, `rmdir`, and `open` commands with a user-visible virtual root.
- PDF-only upload and replacement with revision conflict checks.
- Device-side copy and move without unnecessary host round trips.
- Revision-guarded document deletion and empty-only, non-recursive folder
  deletion.
- Deterministic macOS/Linux arm64/amd64 archives, source archive, release
  manifest, SHA-256 checksums, and recovery documentation.
- A stateful authentication, registration, document, and fault-injection
  emulator for automated evaluation without hardware.

## Physical verification

Sony DPT-RP1 firmware `1.6.50.14130` has been physically verified for direct
fresh pairing, authentication after quitting the Sony App, status and listing,
PDF download/upload/replacement, folder creation, device-side copy/move, and
guarded deletion. Exact operation records are in `compatibility.md`.

## Known limits

- DPT-CP1 and every Fujitsu QUADERNO generation remain explicitly unverified by
  this project. Similar names and community reports are not treated as proof.
- Viewer `open` is emulator-tested but not yet physically verified.
- Native note and handwritten-annotation files are preserved as PDFs; behavior
  that differs from ordinary PDFs has not been physically characterized.
- Sync, backup automation, watchers, CUPS, Markdown, LaTeX, Pandoc, Tectonic,
  and other renderers are intentionally deferred until after the P4 soak.
- Profile deletion is intentionally not automated in this release candidate.
- Release binaries are not code-signed or notarized. If local macOS policy
  rejects an unsigned binary, build from the verified source archive.
- A sleeping or disconnected device must be woken or reconnected; automatic
  background reconnection is not part of P3.

## Security boundary

There is no trust-all TLS mode. The DPT-RP1 certificate-without-SAN behavior is
handled by an exact certificate pin established during registration. PINs,
client IDs, keys, fingerprints, document paths, IDs, and contents are excluded
from committed device-test records.
