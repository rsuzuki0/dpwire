# Atomic file helper

`WriteNew` refuses replacement and cleans up incomplete new files. `Replace`
writes and syncs a same-directory temporary before rename. Both require
owner-only permissions and are used for profile and credential persistence.
