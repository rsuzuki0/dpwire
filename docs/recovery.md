# Credential backup and recovery

Digital Paper profile storage contains RSA private keys that authorize access
to the device. Treat every backup as a secret. Never commit it, attach it to an
issue, put it in ordinary cloud storage, or send it with a diagnostic report.

## Locations and contents

The default macOS directory is:

```text
~/Library/Application Support/digitalpaper/
```

On Linux it is normally:

```text
~/.config/digitalpaper/
```

`config.json` selects the default profile. Each directory below `profiles/`
contains a redaction-safe `profile.json` and a secret `privatekey.pem`. Fresh
pairing stores its identity here; it neither references nor changes the Sony
Digital Paper App directory.

## Preferred recovery order

1. Pair a new direct profile when the device is available. This creates a new
   independent identity and avoids transporting an old private key.
2. Restore a protected backup when re-pairing is unavailable or continuity of
   the existing identity is required.
3. Explicitly import a known Sony credential pair only as a compatibility
   fallback.

A lost private key cannot be reconstructed from the client ID or device
certificate. Create a new pairing instead.

## Backup

First confirm which profile is active:

```sh
dp profile list
dp profile show
```

Copy the entire `digitalpaper` directory to an encrypted volume or an encrypted
backup. Preserve it as one unit; copying only `privatekey.pem` omits the client
ID, certificate pin, address, and default selection. The live directory and
every profile directory must be owner-only (`0700`), and every contained file
must be owner-only (`0600`). An unencrypted archive is sensitive even if its
filesystem permissions are restrictive.

## Restore on a new machine

Install and verify the same or a newer `dp` binary first. If a Digital Paper
configuration directory already exists, do not merge or overwrite it. Move it
aside as a recoverable backup, restore the saved directory in its place, and
then enforce owner-only permissions. On macOS:

```sh
chmod 700 "$HOME/Library/Application Support/digitalpaper"
find "$HOME/Library/Application Support/digitalpaper" -type d -exec chmod 700 {} \;
find "$HOME/Library/Application Support/digitalpaper" -type f -exec chmod 600 {} \;
dp profile list
dp auth
```

The direct device must be connected and awake. If `digitalpaper.local` does not
resolve, check the USB or network connection before changing credentials. A TLS
fingerprint mismatch is a stop condition; do not bypass it with insecure TLS.

## Recover without a backup

Connect and wake the device and use a new, unused profile name:

```sh
dp profile pair recovered-direct digitalpaper.local
dp profile use recovered-direct
dp auth
```

After successful authentication, retain any old profile until ordinary PDF
operations have been checked. Profile removal is intentionally not automated
in the P3 CLI; this prevents a recovery attempt from deleting the last usable
credential.

## Diagnostic boundary

Safe reports may include the profile name, connection mode, device model,
firmware, and an error code. They must omit PINs, client IDs, private keys,
certificate fingerprints, document paths, document IDs, and document content.
