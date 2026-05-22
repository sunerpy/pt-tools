# Release Announcement Overrides

Files in this directory override the auto-generated GitHub Release body for
the Telegram release announcement workflow only. The GitHub Release body and
the project `CHANGELOG.md` are not affected.

## Naming

- `v<X.Y.Z>.md` — matches the git tag exactly (`v0.32.1.md`)
- `<X.Y.Z>.md` — fallback without the `v` prefix (`0.32.1.md`)

## Workflow

1. Curate a user-facing announcement in Markdown (≤ 600 Chinese characters
   per `pt-tools-workflow` skill rules).
2. Commit the file to `dev` alongside the release commits.
3. Merge the release PR. `telegram-release-announce.yml` reads this file
   instead of the noisy `release.body` and renders it via
   `tg_release_announce.py`.

When no override file exists the workflow falls back to `release.body`.
