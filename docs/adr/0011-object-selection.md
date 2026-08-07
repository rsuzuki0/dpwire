# ADR 0011: Persistent object references and unique glob selection

## Decision

Commands that operate on an existing device object accept its exact
root-relative path, `--id NUMBER|0xHEX`, or `--glob PATTERN`.

`dp ls -l` assigns each observed object a profile-local, persistent,
nonnegative integer. Numbers increase monotonically and are never reused. A
short hexadecimal reference begins with `0x` and is derived from SHA-256 of the
opaque device ID. Displayed prefixes start at six digits and lengthen when a
collision requires it. Rename and move operations preserve both references
because the device ID remains unchanged.

The owner-only reference file records the number, opaque device ID, and object
type. It records no filename or path. Updates use an interprocess lock and
atomic replacement.

`--glob` uses Unix glob syntax and Unicode NFC normalization. Patterns without
`/` match basenames throughout the device; patterns with `/` match complete
root-relative paths. Exact path resolution and glob matching are
case-insensitive on the verified DPT-RP1. DPWire applies the same case folding
to glob matching on every device; exact path resolution remains a device API
behavior to verify on other families. A command proceeds only when exactly one compatible
object matches. Ambiguity reports the number, hexadecimal reference, and exact
path for every match. Traversal has a 10,000-object safety limit.

## Consequences

Paths containing spaces, non-Western text, or other hard-to-enter characters
can be selected from a short value printed by `ls -l`. Exact paths retain their
literal meaning, including glob metacharacters in filenames. The explicit
`--glob` option also keeps host-shell expansion distinct from device-side
matching; examples quote patterns for this reason.

The persistent integer is DPWire metadata rather than an inode or vendor ID.
Its scope is one configured device identity. Restoring the reference file
preserves its numbering; losing the file causes later listings to assign new
numbers without affecting credentials or device content.
