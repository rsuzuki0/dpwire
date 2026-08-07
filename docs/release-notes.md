# DPWire v0.3.0 release notes

This is the first PDF-only daily-use release candidate. It provides a verified
DPT-RP1 direct-USB workflow and an independently reusable Go communication
library.

## Downloads

- `dpwire-v0.3.0-darwin-amd64.tar.gz`: Intel Mac, macOS 12 Monterey or newer.
- `dpwire-v0.3.0-darwin-arm64.tar.gz`: Apple silicon, macOS 12 or newer.
- `dpwire-v0.3.0-linux-amd64.tar.gz`: Intel/AMD 64-bit Linux.
- `dpwire-v0.3.0-linux-arm64.tar.gz`: 64-bit ARM Linux.

The Intel Mac binary uses the baseline x86-64 target (`GOAMD64=v1`).

## Included

- Fresh PIN pairing with independent RSA credentials and owner-only storage.
- Fresh pairing connects directly to the device, generates its own client
  identity, and validates and pins the certificate supplied by the device.
- Named direct and Sony-relay profiles with explicit default selection.
- Authenticated model, firmware, battery, and storage status.
- Unix/FTP-style `ls`, `ls -l`, `file`/`stat`, `get`, `put`, `cp`, `mv`,
  `mkdir`, `rm`, `rmdir`, and `open` commands in a filesystem-like CLI UI.
- Persistent nonnegative object numbers, collision-safe `0x` references, and
  bounded Unix glob selection for paths that are difficult to type.
- Direct quoted glob arguments and multiple-match output for read-only `ls`,
  `file`, and `stat`, with exact literal paths taking precedence.
- Folder-only globs with a trailing `/`, shell-quoting diagnostics, and explicit
  bounded recursive listings through `ls -R` and `ls -lR`.
- Unix-style `ls -t` modification-time sorting, including combined short,
  long, and recursive option forms.
- Flat, documents-only `ls --global` traversal for globally sorted newest-file
  listings with complete device paths.
- Confirmation for suspicious metacharacter-free `--glob` values, sorted
  listings, memoized recursive reference assignment, strict page parsing, and
  case-insensitive rejection of the private protocol root.
- Downloaded PDF signature verification before local output is accepted.
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
PDF download/upload/replacement, folder creation, device-side copy/move,
guarded deletion, recursive listing, and viewer open. Decimal and `0x`
persistent selectors and case-insensitive
unique glob selection were also verified read-only. The device was connected by USB throughout. Fresh pairing and
independent authentication used the USB Ethernet endpoint directly; earlier
document-operation checks used the Sony App loopback relay. Exact operation
records are in `compatibility.md`.

A later strict global traversal over direct USB also verified canonical
metadata, flattened newest-first ordering, absolute CLI paths, and duplicate-ID
suppression across the complete device tree without retaining document names,
paths, or IDs.

## Known limits

- DPT-CP1 and every Fujitsu QUADERNO generation require separate physical
  verification.
- Linux arm64/amd64 binaries are built and automated tests are portable to
  Linux, but physical USB pairing on Linux has not yet been recorded.
- Native note and handwritten-annotation files are preserved as PDFs; behavior
  that differs from ordinary PDFs has not been physically characterized.
- Sync, backup automation, watchers, CUPS, Markdown, LaTeX, Pandoc, Tectonic,
  and other renderers are planned after the PDF-core soak period.
- Profile deletion remains a manual recovery operation in this release.
- Release binaries are not code-signed or notarized. If local macOS policy
  rejects an unsigned binary, build from the verified source archive.
- A sleeping or disconnected device must be woken or reconnected; background
  reconnection is planned for later work.

## Security boundary

TLS identity uses standard verification or an exact certificate pin established
during registration. This handles the DPT-RP1 certificate-without-SAN behavior.
PINs, client IDs, keys, fingerprints, document paths, IDs, and contents are excluded
from committed device-test records.
