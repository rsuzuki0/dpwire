# Why Digital Paper exists

This document explains why this independent Go implementation was created,
what it does differently, and where the Sony Digital Paper App and
`dpt-rp1-py` remain stronger. A [Japanese version](project-rationale-and-comparison.ja.md)
is available.

Comparison checked: 2026-08-06.

## Short answer

Sony officially ended repair and support for the DPT-RP1 and DPT-CP1 on
2026-03-31. Sony's notice also says that provision of the Digital Paper App and
device firmware ended with that support. Existing hardware can remain useful,
but depending on an unavailable, unsupported desktop application is not a
durable operating plan.

The open-source [`dpt-rp1-py`](https://github.com/janten/dpt-rp1-py) project
already provides a capable Python library and CLI. It was essential pioneering
work and remains active: its inspected head commit from 2026-07-13 fixed an
intermittent registration failure. It also implements features that this
project deliberately does not yet provide, including synchronization, FUSE
mounting, and Wi-Fi configuration.

Digital Paper is therefore not presented as the first community client or as a
feature-for-feature replacement for every existing tool. It is an independent,
narrower implementation for a different operational requirement: a
long-lived, embeddable Go library and dependency-free `dp` binary with strict
TLS identity checks, guarded destructive operations, a hardware-independent
protocol emulator, reproducible releases, and explicit device/firmware
verification records.

It does not wrap, launch, or depend on the Sony App. Fresh pairing requires no
Sony App client ID, private key, or certificate file: `dp` generates a new RSA
client identity, receives the device certificate from the device itself,
validates it, and records an exact pin. This distinction matters on Linux,
where Sony documented Windows and macOS desktop apps but did not provide a
Linux version.

## Fair comparison

| Area | Sony Digital Paper App | `dpt-rp1-py` | Digital Paper / `dp` |
|---|---|---|---|
| Role | Sony's original end-user GUI and supported workflow | Community Python library, CLI, sync tool, and FUSE mount | Independent Go library, Unix/FTP-style CLI, and protocol emulator |
| Current status | Sony support and distribution ended 2026-03-31 | Active community project at the inspected 2026-07-13 revision | Pre-release independent project; P3 PDF core completed and physically tested |
| Source and license | Vendor application; not an open-source community project | MIT-licensed open source | MIT-licensed open source |
| Installation/runtime | Historical vendor installer and desktop runtime; no longer provided by Sony | Python 3 package installed with `pip`; current metadata lists ten runtime dependencies | One statically linked Go binary for supported targets; no Python, package manager, or Sony App at runtime |
| Desktop platforms | Sony's last published App material describes Windows and macOS versions; no Linux App is listed | Upstream states Windows, Linux, and macOS testing | macOS and Linux binaries for arm64 and amd64; automated Linux builds, but physical USB pairing on Linux is not yet recorded |
| Intended devices | Sony DPT-RP1 and DPT-CP1 | Upstream states Sony DPT-RP1/CP1 and Fujitsu QUADERNO support | DPT-RP1 is physically verified; DPT-CP1 and all QUADERNO generations remain explicitly unverified here |
| Registration | Vendor-managed pairing and credential storage | Fresh PIN registration or reuse of Sony credentials | Fresh PIN registration, named profiles, or explicit Sony-credential import; new keys are stored outside Sony's directory |
| Connection model | Vendor-managed USB/network connection | Automatic discovery or explicit address; Wi-Fi, Bluetooth, and USB are described upstream | Explicit named direct or relay profiles; direct USB via `digitalpaper.local` is physically verified |
| Daily PDF work | The original graphical transfer, synchronization, and print workflow | Broad document CLI: list, upload, download, copy, move, delete, display, and sync | `ls`, `file`/`stat`, `get`, `put`, `cp`, `mv`, `mkdir`, guarded `rm`/`rmdir`, and `open` |
| Features beyond the P3 PDF core | Vendor GUI integration and supported sync workflow | Sync, FUSE mount, Wi-Fi management, templates, configuration, screenshot, and firmware commands | None yet by design; sync, backup, rendering, CUPS, and OS integration are deferred until after the soak period |
| Path model | Device UI hides the protocol's `Document/` root | Upstream CLI commonly exposes `Document/...` | Filesystem-like CLI UI rejects and hides `Document/`; users address paths from the device root |
| TLS server identity | Internal vendor behavior; not externally auditable as a maintained open client | The inspected code disables HTTPS certificate verification for its session | No trust-all mode; normal CA/hostname verification or exact leaf SHA-256 pinning for the DPT-RP1 certificate-without-SAN quirk |
| Destructive-operation policy | Interactive GUI behavior controlled by the vendor app | Direct delete calls; upstream sync warns that newer files overwrite older files without another warning | Document deletion requires the revision just resolved; `rmdir` verifies emptiness and explicitly disables recursive force deletion |
| Automated verification | Sony's historical internal testing is not available as a reproducible public suite | The inspected revision has no committed test files or protocol emulator; its GitHub workflow publishes the Python package | Stateful auth/pairing/document emulator, fault injection, unit and integration tests, race testing, architecture/spec checks, and cross-builds |
| Releases and recovery | Vendor-controlled; downloads ended with support | PyPI/source installation under the upstream project | Deterministic binary/source archives, manifest, SHA-256 checksums, owner-only profile storage, installation and recovery guides |
| Best fit today | An already-working legacy installation whose platform still runs it | Users who need its broader mature feature set and accept a Python environment | Users and applications prioritizing a small verified PDF core, Go embedding, explicit safety boundaries, and reproducible deployment |

## What this comparison does not claim

- `dp` is not currently a superset of `dpt-rp1-py`. In particular, it has no
  sync, FUSE mount, or Wi-Fi-management command.
- A DPT-RP1 hardware result does not establish DPT-CP1 or QUADERNO
  compatibility. Those targets remain separately classified.
- A single binary does not by itself make software safer. The relevant
  differences are the TLS policy, mutation guards, bounded protocol handling,
  automated tests, and recoverable release process.
- This project does not claim official Sony or Fujitsu endorsement and does not
  replace hardware service, firmware maintenance, or vendor support.
- The comparison describes inspected published code and documented behavior;
  it is not a judgment about the diligence of previous maintainers.

## How the independent implementation arose

### 1. The vendor dependency became a continuity risk

Digital Paper hardware is useful precisely because it is simple, focused, and
durable. Its usable life can exceed that of a desktop application tied to old
OS releases, installers, certificate behavior, and vendor distribution. Sony's
support termination made this mismatch concrete: the official application and
firmware are no longer provided, even though functioning devices remain.

### 2. Community work established that replacement was possible

`dpt-rp1-py` demonstrated direct registration, authentication, document
operations, synchronization, discovery, and mounting. The Java
[`DigitalPaperApp`](https://github.com/DPT-RP1/DigitalPaperApp) project expanded
the explored surface and preserved a translation of the Polaris 2.0 endpoint
material. These projects are references and predecessors, not competitors to
erase from the history.

The current `dpt-rp1-py` registration fix is a concrete example of that value:
it preserves the device's 256/257-byte Java `BigInteger` representation in the
authenticated transcript. This Go implementation independently encodes and
tests the same required wire behavior.

### 3. The production requirement was different

The intended operator does not otherwise maintain Python applications and did
not want the device workflow to depend on Python, `pip`, virtual environments,
or a multi-package runtime. The requested foundation also had to be reusable by
future native applications rather than remain coupled to one CLI.

That led to a new implementation rather than a line-by-line port. Official
documentation, the preserved Polaris endpoint description, community code, and
physical observations are compared, but the public API, package boundaries,
error model, path model, credential store, emulator, and release process are
designed for this project.

## Why the work matters

### Hardware preservation

An unsupported desktop application should not turn functioning reading and
writing hardware into e-waste. A documented protocol client gives owners a
recoverable way to keep using their devices.

### Operational independence

Fresh pairing and direct authentication work after the Sony App is quit. The
new identity is stored independently, so the vendor application's private
workspace is neither required nor modified. The Sony App does not need to be
installed, launched, or used to manufacture credential files. `dp` generates
the client key itself; the device supplies the certificate used to establish
the exact TLS pin.

### A reusable protocol boundary

The public Go package is separate from the CLI, profiles, future renderers, and
OS integration. A macOS application, print adapter, or other tool can reuse the
same authenticated client without invoking a command-line subprocess.

### Evidence instead of assumed compatibility

Reference documentation, emulator behavior, and physical hardware results are
different evidence levels. Operations are promoted to `device-verified` only
with a recorded model, firmware, date, transport, and redacted result. Firmware
quirks are documented rather than silently generalized to every device.

### Safer long-term maintenance

Destructive commands are narrow, errors are explicit, responses are bounded,
TLS trust cannot be globally disabled, credentials are written atomically with
owner-only permissions, and unsupported future commands do not pretend to
succeed. Reproducible archives and a source bundle reduce dependence on the
original development machine.

## Design requirements in brief

1. **Deploy as one binary.** Support macOS and Linux on arm64 and amd64 without
   a production language runtime or CGO dependency.
2. **Keep the Go library independent.** The protocol client must not depend on
   CLI parsing, profile storage, synchronization, renderers, or OS integration.
3. **Authenticate directly.** Support fresh PIN pairing, imported credentials,
   named devices, and operation without the Sony App.
4. **Verify TLS identity.** Use standard CA/hostname validation where possible
   and exact certificate pinning for documented legacy-device constraints.
5. **Make familiar operations unsurprising.** Use `ls`, `cp`, `mv`, `mkdir`,
   `get`, and `put`; hide the protocol-only `Document/` prefix.
6. **Guard irreversible mutations.** Refuse overwrite by default, require a
   current revision for PDF deletion, reject root deletion, and allow only
   empty non-recursive folder deletion.
7. **Test without hardware.** Emulate registration, authentication, PDF
   operations, pagination, failures, and conflicts; separately record physical
   verification.
8. **Preserve protocol evidence.** Pin reference revisions and checksums, keep
   compatibility catalogs machine-readable, and document deviations.
9. **Release reproducibly.** Cross-build, race-test, checksum, bundle source,
   record the toolchain and commit, and document install, rollback, and
   credential recovery.
10. **Expand in controlled phases.** Stabilize the PDF core in real use before
    adding sync, backup automation, CUPS, Markdown, LaTeX, or Tectonic.

## Current boundary

P3 is a complete PDF-only release candidate, not a finished universal device
manager. On Sony DPT-RP1 firmware `1.6.50.14130`, fresh pairing, independent
direct authentication, status, listing, download, upload/replacement,
device-side copy/move, folder creation, and guarded deletion have been
physically verified. The device was connected by USB throughout. Fresh pairing
and independent authentication used USB Ethernet directly; the earlier
document-operation tests used the Sony App loopback relay. Viewer open is
emulator-tested but not physically verified. Other models and the deferred
features remain clearly marked.

## Sources and credit

- [Sony: Digital Paper and Digital Paper App support termination](https://www.sony.jp/digital-paper/info2/20240628.html)
- [Sony Help Guide: transferring a document from a computer](https://helpguide.sony.net/dpt/rp1/v1/en/contents/TP0001178322.html)
- [Sony's last Digital Paper App update: Windows and macOS variants](https://www.sony.jp/digital-paper/update/#20240628)
- [`dpt-rp1-py` README at the inspected revision](https://github.com/janten/dpt-rp1-py/blob/9dda9d9a16c20477bd19374866e2095705765f96/README.md)
- [`dpt-rp1-py` package metadata and dependencies](https://github.com/janten/dpt-rp1-py/blob/9dda9d9a16c20477bd19374866e2095705765f96/setup.json)
- [`dpt-rp1-py` TLS session configuration](https://github.com/janten/dpt-rp1-py/blob/9dda9d9a16c20477bd19374866e2095705765f96/dptrp1/dptrp1.py#L163-L164)
- [`dpt-rp1-py` 256/257-byte registration fix](https://github.com/janten/dpt-rp1-py/commit/9dda9d9a16c20477bd19374866e2095705765f96)
- [Java DigitalPaperApp and preserved endpoint translation](https://github.com/DPT-RP1/DigitalPaperApp)
- Local protocol provenance: `spec/references/provenance.json`
- Local physical verification records: `spec/compat/models.json`

`dpt-rp1-py` and the Java DigitalPaperApp are MIT-licensed projects by their
respective contributors. Their work is credited here and in the repository's
provenance records. Digital Paper is independent and is not affiliated with,
endorsed by, or sponsored by Sony or Fujitsu.
