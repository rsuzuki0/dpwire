# Compatibility

Machine-readable model status is stored in `spec/compat/models.json`.

- Sony DPT-RP1 and DPT-CP1 are initial targets.
- Fujitsu QUADERNO Gen.1 is an initial compatibility target.
- QUADERNO Gen.2 is a priority validation target based on community reports.
- QUADERNO Gen.3C is research-only until its protocol is probed safely.

`documented`, `emulated`, and `device-verified` are separate states. Emulator
success never promotes an operation to device-verified.
