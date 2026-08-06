# Command-line interface

`dp` presents the device as a small Unix-like virtual filesystem. The
protocol's `Document` root and HTTP endpoint names are private implementation
details. Device paths are always root-relative; `.` means the device root.

```sh
dp -profile device.json ls
dp -profile device.json ls -l Codex_dp
dp -profile device.json file Codex_dp/paper.pdf
dp -profile device.json mkdir Codex_dp/Archive
dp -profile device.json cp Codex_dp/paper.pdf Codex_dp/Archive
dp -profile device.json mv Codex_dp/old.pdf Codex_dp/new.pdf
dp -profile device.json put local.pdf Codex_dp
dp -profile device.json get Codex_dp/paper.pdf
dp -profile device.json open Codex_dp/paper.pdf 3
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
