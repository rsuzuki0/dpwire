# Installation and upgrade

Digital Paper releases are dependency-free `dp` binaries. The user archive
contains `dp`, the license and notices, and the CLI, compatibility, installation,
and recovery documents. `dp-sim` is a development tool and is not installed.

## Select and verify an archive

Use `uname -m` to select `darwin-arm64` on Apple silicon or `darwin-amd64` on
an Intel Mac. Linux archives use the same `arm64` and `amd64` architecture
names. Download the selected archive and `SHA256SUMS` into one directory.

Verify exactly the selected file before extracting it. For example:

```sh
archive=digitalpaper-v0.3.0-p3-darwin-arm64.tar.gz
awk -v file="$archive" '$2 == file {print}' SHA256SUMS > selected.sha256
test "$(wc -l < selected.sha256)" -eq 1
shasum -a 256 -c selected.sha256
```

The command must print the archive name followed by `OK`. An absent checksum,
more than one matching line, or any mismatch is a stop condition. Releases also
include `release.json`, which records the source commit, Go version, targets,
and individual archive hashes.

## Install without administrator access

Extract the verified archive and install `dp` into a private executable
directory:

```sh
tar -xzf digitalpaper-v0.3.0-p3-darwin-arm64.tar.gz
mkdir -p "$HOME/.local/bin"
install -m 0755 digitalpaper-v0.3.0-p3-darwin-arm64/dp "$HOME/.local/bin/dp"
"$HOME/.local/bin/dp" version
```

Add `$HOME/.local/bin` to `PATH` in the shell configuration if necessary. The
binary does not require Python, Ruby, Homebrew libraries, or the Sony Digital
Paper App. Device profiles remain under the operating system's user
configuration directory and are not stored beside the binary.

## First direct setup

Connect and wake the device, then create a new direct identity:

```sh
dp profile pair rp1-direct digitalpaper.local
dp profile use rp1-direct
dp auth
dp device
```

Enter the PIN displayed by the device only when prompted. Do not put it on the
command line. Existing profile names are never overwritten.

## Upgrade and rollback

Verify the new archive before changing the installed binary. Keep the prior
binary until the new one authenticates successfully:

```sh
cp -p "$HOME/.local/bin/dp" "$HOME/.local/bin/dp.previous"
install -m 0755 digitalpaper-vNEW-darwin-arm64/dp "$HOME/.local/bin/dp.new"
"$HOME/.local/bin/dp.new" version
mv "$HOME/.local/bin/dp.new" "$HOME/.local/bin/dp"
dp auth
```

An upgrade does not rewrite profiles or private keys. If validation fails,
restore the retained binary with:

```sh
mv "$HOME/.local/bin/dp.previous" "$HOME/.local/bin/dp"
dp version
dp auth
```

Do not delete or merge configuration directories as part of a binary upgrade.
See `recovery.md` before moving credentials to another machine.

## Build from reviewed source

The source archive is checksummed with the binaries. After verification:

```sh
tar -xzf digitalpaper-v0.3.0-p3-source.tar.gz
cd digitalpaper-v0.3.0-p3-source
go test ./...
go build -trimpath -buildvcs=false -o dp ./cmd/dp
```

Go 1.24 or a newer supported Go release is required only for a source build.
