# Compatibility

Machine-readable model status is stored in `spec/compat/models.json`.

- Sony DPT-RP1 and DPT-CP1 are initial targets.
- Fujitsu QUADERNO Gen.1 is an initial compatibility target.
- QUADERNO Gen.2 is a priority validation target based on community reports.
- QUADERNO Gen.3C is research-only until its protocol is probed safely.

These targets do not imply confirmed compatibility. P1 is implemented and
emulator-tested against the preserved Polaris interface, but no operation is
marked `device-verified` until its model, firmware, result, and safe test record
are captured from hardware. In particular, current Fujitsu generations must
not be assumed protocol-compatible solely because they are called digital
paper or digital note-taking devices.

`documented`, `emulated`, and `device-verified` are separate states. Emulator
success never promotes an operation to device-verified.

## Verified hardware

Sony DPT-RP1 firmware `1.6.50.14130` completed the P1 read-only verification on
2026-08-06: RSA nonce authentication, firmware/battery/storage, pagination,
folder listing, ID metadata, Unicode path resolution, and streamed PDF download
with ETag and revision. The test used the Sony application's loopback relay and
did not record document metadata or credential material.

The same device and firmware completed P2 safe-write verification on 2026-08-06:
folder creation, device-side PDF copy, document rename and move, folder rename,
whole-PDF upload without the optional unspecified `file_hash`, rejection of an
incorrect target revision, revision-guarded replacement, and byte-for-byte
download verification. The approved source PDF remained unchanged. Delete and
viewer-open were not tested, and native-note behavior remains specification-only
until a note fixture is explicitly approved.

P3 guarded document and empty-folder deletion are implemented and
emulator-tested, but remain unverified on physical hardware. No retained P2
artifact has been deleted by the P3 implementation work.
