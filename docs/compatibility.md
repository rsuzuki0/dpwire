# Compatibility

Machine-readable model status is stored in `spec/compat/models.json`.

- Sony DPT-RP1 and DPT-CP1 are initial targets.
- Fujitsu QUADERNO Gen.1 is an initial compatibility target.
- QUADERNO Gen.2 is a priority validation target based on community reports.
- QUADERNO Gen.3C is research-only until its protocol is probed safely.

Each target has a separate compatibility state. An operation becomes
`device-verified` when its model, firmware, result, and safe test record are
captured from hardware. Current Fujitsu generations remain validation targets.

`documented`, `emulated`, and `device-verified` are separate states. Emulator
success never promotes an operation to device-verified.

## Verified hardware

Sony DPT-RP1 firmware `1.6.50.14130` completed read-only verification on
2026-08-06: RSA nonce authentication, firmware/battery/storage, pagination,
folder listing, ID metadata, Unicode path resolution, and streamed PDF download
with ETag and revision. The test used the Sony application's loopback relay and
the device was connected by USB. It did not record document metadata or
credential material.

The same device and firmware completed safe-write verification on 2026-08-06:
folder creation, device-side PDF copy, document rename and move, folder rename,
whole-PDF upload without the optional unspecified `file_hash`, rejection of an
incorrect target revision, revision-guarded replacement, and byte-for-byte
download verification. The approved source PDF remained unchanged. Delete and
viewer-open were not tested, and native-note behavior remains specification-only
until a note fixture is explicitly approved.

The same device and firmware completed guarded-deletion verification on
2026-08-06 using a newly generated UUID-named folder tree and UUID-named PDF
copy at the device root. Non-empty `rmdir` was refused locally, revision-guarded
document deletion succeeded, and empty-folder deletion with
`force_delete_flag: "false"` succeeded for both generated folders. Post-delete
lookups confirmed absence. The approved source PDF and retained write-test artifacts
were read-only verified afterward and remained present.

The same USB-connected device was also discovered on interface-scoped mDNS as
`Digital Paper DPT-RP1._digitalpaper._tcp.local`, with registration at
`digitalpaper.local:8080`. Existing credentials authenticated directly against
`https://digitalpaper.local:8443`, and firmware, battery, and storage requests
succeeded without using the loopback relay. The direct TLS certificate
fingerprint matched the relay-visible device certificate.

The same device then completed fresh client registration directly on port 8080
using the eight-digit PIN displayed by the device. The newly generated RSA
identity authenticated on port 8443 immediately. After the Sony Digital Paper
App was quit, the new profile still authenticated and read firmware, battery,
storage, and the root listing. This verifies that freshly paired credentials
are stored and used independently of the vendor application. No credential or
PIN values were retained in the repository.
