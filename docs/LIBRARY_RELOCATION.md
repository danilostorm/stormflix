# Library source-root relocation

StormFlix treats a change of physical mount root as a storage relocation, not automatically as a new media catalog.

When a library edit replaces old source roots with new roots in an unambiguous one-for-one mapping, existing `media.path` values are rewritten using their relative paths while keeping each `media.id` unchanged. Episodic `media_series_identity.source_root` follows the new physical root.

Keeping `media.id` stable preserves metadata, selected artwork, subtitles, watch progress and other media-owned state. A following normal library scan therefore updates the existing media rows at their new paths instead of recreating them. Normal non-refresh metadata scans continue to skip already matched titles.

Safety rules:

- merely reordering existing source roots is not treated as relocation;
- ambiguous source add/remove counts are not guessed;
- a target path already owned by another media row is not destructively merged;
- files and folders on the mounted source are never renamed, moved or deleted by this operation.

For an ordinary Drive/rclone mount-path change, edit the existing StormFlix library and replace the old source path with the new source path, save it, then run the normal library scan. Do not delete/recreate the library if the goal is to retain its catalog identity.