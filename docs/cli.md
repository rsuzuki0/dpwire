# Command-line interface

`dp` provides a filesystem-like CLI UI with familiar Unix and FTP commands; it
does not mount a filesystem. The protocol's `Document` root and HTTP endpoint
names are private implementation details. Device paths are always
root-relative; `.` means the device root.

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
dp ls -l Codex_dp
dp file Codex_dp/paper.pdf
dp mkdir Codex_dp/Archive
dp cp Codex_dp/paper.pdf Codex_dp/Archive
dp mv Codex_dp/old.pdf Codex_dp/new.pdf
dp put local.pdf Codex_dp
dp get Codex_dp/paper.pdf
dp rm Codex_dp/old.pdf
dp rmdir Codex_dp/Empty
dp open Codex_dp/paper.pdf 3
```

`cp` and `mv` operate entirely within the device. `put` transfers from the host
to the device, and `get` transfers from the device to the host. When a
destination is an existing folder, the source basename is retained. If the
destination is omitted for `put` or `get`, the source basename is used.

`ls` prints names only and marks folders with `/`. `ls -l` prints type, byte
size, modification time, device ID, and name. `file` and `stat` are identical
and return the complete metadata for one entry. IDs are diagnostic information
and are never required as command arguments.

Existing destinations are protected from overwrite. `rm` is an explicit request
to delete exactly one document: the CLI resolves its current revision and the
device rejects the deletion if that revision changes. `rmdir` rejects the
device root and non-empty folders. It both checks for children beforehand and
sends `force_delete_flag: "false"`, so a child created concurrently also stops
the operation. There is no recursive or force option.

No command silently invokes `rm` or `rmdir`. A successful command reports the
removed root-relative path only after a metadata lookup confirms that the entry
is absent. Deletion is permanent on devices that provide no trash facility.
