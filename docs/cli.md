# Command-line interface

`dp` presents the device as a small Unix-like virtual filesystem. The
protocol's `Document` root and HTTP endpoint names are private implementation
details. Device paths are always root-relative; `.` means the device root.

The first imported profile becomes the default:

```sh
dp inspect-cert https://127.0.0.1:58443
dp profile import-sony rp1 https://127.0.0.1:58443 VERIFIED_SHA256 /path/to/sony/workspace
dp profile list
dp profile use rp1
dp profile show
```

`import-sony` requires the credential directory to contain the explicitly
selected `deviceid.dat` and `privatekey.dat` pair. It validates the key and
connection settings, copies them into an owner-private profile directory, and
never overwrites an existing profile. `list` and `show` omit credential IDs and
key paths.

```sh
dp ls
dp ls -l Codex_dp
dp file Codex_dp/paper.pdf
dp mkdir Codex_dp/Archive
dp cp Codex_dp/paper.pdf Codex_dp/Archive
dp mv Codex_dp/old.pdf Codex_dp/new.pdf
dp put local.pdf Codex_dp
dp get Codex_dp/paper.pdf
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

P2 never overwrites an existing destination. It exposes no `rm` or `rmdir`
command. Destructive commands will only be added with explicit confirmation,
conflict checks, and recoverability rules.
