# ADR 0011: Persistent object references and unique glob selection

## Decision

Commands that operate on an existing device object accept its exact absolute
or root-relative path, `--id NUMBER|0xHEX`, or `--glob PATTERN`.

`dp ls -l` assigns each observed object a profile-local, persistent,
nonnegative integer. Numbers increase monotonically and are never reused. A
short hexadecimal reference begins with `0x` and is derived from SHA-256 of the
opaque device ID. Displayed prefixes start at six digits and lengthen when a
collision requires it. Rename and move operations preserve both references
because the device ID remains unchanged.

The owner-only reference file records the number, opaque device ID, and object
type. It records no filename or path. Updates use an interprocess lock and
atomic replacement.

`--glob` uses Unix pathname glob syntax and Unicode NFC normalization. Matching
starts at the device root and advances one directory level per `/`. A pattern
without `/` examines direct children of the root; a single leading `/` makes
the fixed root explicit. There is no implicit recursive search, and `**` has no
special recursive meaning. Exact path resolution and glob matching are
case-insensitive on the verified DPT-RP1. DPWire applies the same case folding
to glob matching on every device; exact path resolution remains a device API
behavior to verify on other families. Commands that require one object proceed
only when exactly one compatible object matches. Ambiguity then reports the
number, hexadecimal reference, and exact path for every match. Traversal has a
10,000-object safety limit.

Read-only `ls`, `file`, and `stat` infer glob intent from metacharacters after
an exact-path lookup fails. They permit multiple matches. `ls` lists matching
entries without entering matched folders; `file` and `stat` return a JSON array
for inferred or explicit globs. Other object commands retain explicit `--glob`
and require exactly one compatible match.

A trailing `/` restricts glob results to folders. Documentation quotes every
glob so the host shell passes the pattern unchanged; multiple positional paths
produce a quoting hint rather than being interpreted as device operands.
An explicit `--glob` without metacharacters requires `y/[N]` confirmation
because it may be a one-item host-shell expansion. An ordinary positional path
cannot reveal whether the shell produced it, so quoting remains part of the
command contract.

Recursive traversal is an explicit `ls -R` operation, independent of glob
matching. `ls -lR`, `ls -Rl`, `ls -l -R`, and `ls -R -l` are equivalent.
Traversal lists each folder once by device ID and shares the 10,000-entry
safety limit. `file` and `stat` do not accept a recursive option.

Recursive long listing emits the first folder without waiting for the complete
tree. The reference store memoizes its validated state for the lifetime of the
command, invalidates that state when another process atomically replaces the
store, and preserves interprocess locking. Hexadecimal reference calculation
sorts digests and compares adjacent values rather than comparing every pair.

## Consequences

Paths containing spaces, non-Western text, or other hard-to-enter characters
can be selected from a short value printed by `ls -l`. Exact paths retain their
literal meaning, including glob metacharacters in filenames. The explicit
`--glob` option also keeps host-shell expansion distinct from device-side
matching; examples quote patterns for this reason. Glob scope follows familiar
shell pathname expansion and does not unexpectedly search unrelated subtrees.

The persistent integer is DPWire metadata rather than an inode or vendor ID.
Its scope is one configured device identity. Restoring the reference file
preserves its numbering; losing the file causes later listings to assign new
numbers without affecting credentials or device content.
The store namespace includes connection address, client ID, and certificate
fingerprint; a change to any component selects another namespace.
