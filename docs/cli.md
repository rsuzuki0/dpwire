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

```sh
dp ls
dp ls -l /Documents
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

`ls` prints names only and marks folders with `/`. `ls -l` prints these columns:

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

Commands that accept an existing object accept any one of these forms:

```sh
dp file /Documents/paper.pdf
dp file Documents/paper.pdf
dp file --id 23
dp file --id 0x3a71c8
dp file '/Documents/*.pdf'
dp ls -l '/Documents/2026-*.pdf'
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

For a glob, `ls` prints all matching entries themselves and does not enter a
matching folder. `file` and `stat` return a JSON array for every glob request,
including one match. Exact paths and `--id` return the existing single JSON
object. Commands that transfer, open, copy, move, or remove one object require
explicit `--glob` and exactly one compatible match.

The reference map is stored owner-only in the active DPWire configuration
directory. It contains device object IDs and types, but no filenames or paths.
`file` and `stat` are identical and return complete metadata for one or more
entries.

Existing destinations are protected from overwrite. `rm` is an explicit request
to delete exactly one document: the CLI resolves its current revision and the
device rejects the deletion if that revision changes. `rmdir` rejects the
device root and non-empty folders. It both checks for children beforehand and
sends `force_delete_flag: "false"`, so a child created concurrently also stops
the operation. There is no recursive or force option.

No command silently invokes `rm` or `rmdir`. A successful command reports the
removed absolute device path only after a metadata lookup confirms that the entry
is absent. Deletion is permanent on devices that provide no trash facility.
