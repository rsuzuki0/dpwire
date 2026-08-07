# Command-line interface

`dp` provides a filesystem-like CLI UI with familiar Unix and FTP commands; it
is designed so experienced Unix and FTP users can apply familiar commands and
path conventions to the device's core capabilities with as little friction as
practical. It does not mount a filesystem. The protocol's `Document` root and HTTP endpoint
names are private implementation details. Every command resolves device paths
from the fixed root `/`; directory position is not retained between commands.
The absolute form `/Documents/paper.pdf` and root-relative form
`Documents/paper.pdf` are both accepted. DPWire output uses the absolute form.
Directory paths accept a trailing slash, as in `dp ls /Documents/`.

The first imported profile becomes the default:

```sh
dp inspect-cert https://127.0.0.1:58443
dp profile import-sony rp1 https://127.0.0.1:58443 VERIFIED_SHA256 /path/to/sony/workspace
dp profile list
dp profile use rp1
dp profile show
dp profile pair rp1-direct digitalpaper.local
```

`import-sony` requires the credential directory to contain the explicitly
selected `deviceid.dat` and `privatekey.dat` pair. It validates the key and
connection settings, copies them into an owner-private profile directory, and
never overwrites an existing profile. `list` and `show` omit credential IDs and
key paths.

Each profile reports `direct` or `relay`. A direct Sony DPT-RP1 USB connection
normally uses `https://digitalpaper.local:8443`; registration discovery is
advertised separately on HTTP port 8080. A loopback address such as
`https://127.0.0.1:58443` is a relay and requires the vendor application to keep
running. Existing profiles without an explicit connection field are inferred
from the address.

`profile pair` performs fresh registration without reading or changing Sony
application credentials. The Sony App need not be installed or running, and no
client ID, key, or certificate file from it is required. `dp` requests the PIN
only after verifying the first authenticated registration transcript, validates
every later transcript HMAC and the X.509 certificate returned by the device,
generates a new RSA-2048 identity, and stores it atomically with owner-only
permissions. Pairing is direct-only; loopback relay addresses are rejected. If
a profile name already exists, the command stops before contacting the device.

A saved profile name is a directory name under the DPWire configuration; its
data file is named `profile.json`. An external profile file may have any name
or extension. A bare `-profile NAME` is rejected as ambiguous when both a saved
profile and a current-directory file named `NAME` are valid profiles. Use
`-profile ./NAME` to select the file explicitly. A same-named file that is not
a valid strict profile is also an error.

```sh
dp ls
dp --strict ls -l /Documents
dp ls -l /Documents
dp ls -lt '/Documents/*.pdf' | head -n 20
dp ls -lt --global / | head -n 20
dp ls -lR /
dp file /Documents/paper.pdf
dp file --id 23
dp stat --id 0x3a71c8
dp get --glob '/Documents/*報告*2026*.pdf'
dp mkdir /Documents/Archive
dp cp /Documents/paper.pdf /Documents/Archive
dp mv /Documents/old.pdf /Documents/new.pdf
dp put local.pdf /Documents
dp get /Documents/paper.pdf
dp rm /Documents/old.pdf
dp rmdir /Documents/Empty
dp open /Documents/paper.pdf 3
```

`cp` and `mv` operate entirely within the device. `put` transfers from the host
to the device, and `get` transfers from the device to the host. When a
destination is an existing folder, the source basename is retained. If the
destination is omitted for `put` or `get`, the source basename is used.

`ls` prints names only and marks folders with `/`. Entries are sorted by their
Unicode-normalized, case-folded device names, with canonical paths as the tie
breaker. `ls -l` prints these columns:

```text
NUMBER  HEX-ID    TYPE  SIZE  MODIFIED  DEVICE-ID  NAME
23      0x3a71c8  -     4821  ...       ...        paper.pdf
```

`NUMBER` is a persistent, profile-local, nonnegative DPWire number. It remains
attached to the same device object when that object is renamed or moved. New
numbers increase monotonically and deleted numbers are not reused. `HEX-ID` is
a shortened SHA-256 reference derived from the device's opaque ID; DPWire
lengthens it when needed to avoid a collision. The full device ID remains in
the listing for diagnostics.

`ls -t` sorts one listing by device modification time, newest first. It works
with either short or long output, so the newest 20 entries in one folder can be
shown with `dp ls -lt /Documents | head -n 20`; use the quoted
`'/Documents/*.pdf'` form to include PDFs only. Entries with a missing or
invalid modification time follow dated entries; equal times use the normal name
order. `-t`, `-l`, and `-R` may be grouped in any order or supplied as separate
options. With `-R`, time ordering applies independently inside each listed
directory rather than globally across the tree.

`--global` provides the global form: it implies recursive traversal, collects
documents only, and prints one flat list using complete device paths. The
newest 20 PDFs anywhere on the device are therefore:

```sh
dp ls -lt --global / | head -n 20
```

An explicit `-R` is accepted, making `dp ls -lRt --global /` equivalent. Unlike
ordinary recursive output, global output has no directory headings. It must
read the complete selected subtree before sorting and printing, so initial
latency and device requests scale with the tree size. The same 10,000-object
safety limit applies, and duplicate object IDs are emitted once.

This traversal is a sequence of ordinary remote directory reads rather than a
point-in-time snapshot. Content changed during the scan may appear according to
different moments. The command is read-only; rerun it when one contemporaneous
view matters.

`--strict` is a global option and therefore precedes the command:

```sh
dp --strict ls -lt --global /
```

It adds canonical metadata checks for timestamps, byte sizes, and agreement
between an entry name and its path. Safe legacy metadata remains usable without
the option. Unsafe identifiers and paths, invalid UTF-8 in identity fields, and
control characters in operational and listing fields are rejected in every
mode.

Commands that accept an existing object accept any one of these forms:

```sh
dp file /Documents/paper.pdf
dp file Documents/paper.pdf
dp file --id 23
dp file --id 0x3a71c8
dp file '/Documents/*.pdf'
dp ls -l '/Documents/2026-*.pdf'
dp ls -l '*'
dp ls -l '*/'
dp get --glob '/Documents/*報告*2026*.pdf'
dp mv --glob '/Documents/Inbox/*draft*.pdf' /Documents/Archive/
```

A glob is expanded one path segment at a time from the fixed device root, like
a shell pathname. `E*` examines only entries directly under the device root;
`/Documents/E*` examines only direct children of `/Documents`; and `*/E*`
examines direct children of each matching root folder. There is no implicit
recursive search, and `**` has no special recursive meaning. Matching uses Unix
glob syntax after Unicode NFC normalization and case folding. Exact paths and glob patterns are
case-insensitive on the verified DPT-RP1; `--glob` applies case-insensitive
matching consistently. Other device families require separate path-resolution
verification. Quote the pattern so the host shell does not expand it. Exactly
Zero matches stop with an error. For a command requiring one object, multiple
matches stop and list each persistent number, hexadecimal reference, and exact
path. No matching object is modified in either case. Expansion stops at a
10,000-object safety limit.

`ls`, `file`, and `stat` accept a quoted glob directly, without `--glob`.
DPWire first attempts an exact path lookup, so an existing literal filename
containing `*`, `?`, or `[` remains addressable. Glob expansion occurs only
after that exact lookup reports no object. `--glob` remains available to force
glob interpretation.

A trailing `/` restricts a glob to folders. At the fixed root, `'*'` matches
all direct entries and `'*/'` matches direct folders only. Quotes are required:
an unquoted `*` is expanded against the host working directory by the shell
before `dp` receives its arguments. When that expansion supplies multiple host
paths, `dp` stops with a quoting hint.

An explicit `--glob` value containing none of `*`, `?`, or `[` is suspicious:
the host shell may have expanded an unquoted pattern to exactly one pathname.
DPWire displays the value and asks `y/[N]`; EOF, an empty answer, or anything
other than `y` cancels. A one-item shell expansion passed as an ordinary path
cannot be distinguished from a path typed literally, so glob patterns must
still be quoted.

For a glob, `ls` prints all matching entries themselves and does not enter a
matching folder. `file` and `stat` return a JSON array for every glob request,
including one match. Exact paths and `--id` return the existing single JSON
object. Commands that transfer, open, copy, move, or remove one object require
explicit `--glob` and exactly one compatible match.

`ls -R /Documents` recursively lists the contents of `/Documents` and every
folder below it. Add the long columns with `ls -lR`; `-lR`, `-Rl`, `-l -R`,
and `-R -l` are equivalent. Time sorting composes with them, for example
`ls -ltR` or `ls -l -t -R`. Each folder requires one logical, automatically
paginated device listing. Traversal stops at 10,000 observed entries and skips
an already visited folder ID, preventing an anomalous cycle from running
indefinitely. `-R` belongs to `ls`; `file` and `stat` remain metadata commands
for their selected object or glob results.

The reference map is stored owner-only in the active DPWire configuration
directory. It contains device object IDs and types, but no filenames or paths.
Its namespace is derived from the connection address, client ID, and device
certificate fingerprint. Changing any of those values starts a separate number
sequence; restore the matching reference map to preserve old numbers. During a
recursive long listing, the loaded map is memoized within the command while
atomic persistence and interprocess locking remain in effect.
`file` and `stat` are identical and return complete metadata for one or more
entries.

`get` verifies the `%PDF-` file signature before writing response bytes to the
new local file. A mismatch is an error and the CLI removes that local file.
`open` accepts either no page or one strictly parsed positive decimal integer;
trailing characters are rejected.

Existing destinations are protected from overwrite. `rm` is an explicit request
to delete exactly one document: the CLI resolves its current revision and the
device rejects the deletion if that revision changes. `rmdir` rejects the
device root and non-empty folders. It both checks for children beforehand and
sends `force_delete_flag: "false"`, so a child created concurrently also stops
the operation. There is no recursive or force option.

No command silently invokes `rm` or `rmdir`. A successful command reports the
removed absolute device path only after a metadata lookup confirms that the entry
is absent. Deletion is permanent on devices that provide no trash facility.
