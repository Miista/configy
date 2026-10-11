# CLAUDE.md

## What this is

`configy` is a tiny, dependency-free Go module (`github.com/miista/configy`) shared by
several apps. Each setting is read from the app's own
persisted store unless an env var sets it; the env always wins, and every setting carries
its source (env, config or unset) so the UI can show an env-set one read-only. See
`README.md` for usage.

## Commands

- `go test ./...`
- `go vet ./...`
- `gofmt -l .` (should print nothing)

Requires Go 1.27 or later: `Config.Get[T]` is a generic method.

## Design rules (decided, don't relitigate)

- The public surface stays small: `New`, `Get[T]`, `Resolve[T]`, `Source`, `EnvVar`, `Mask`,
  `WithMask`, `MaskSecret`, the `Source`/`Setting`/`ParseError`/`Mask` types and
  `ErrInvalid`. `Get[T]` and `Resolve[T]` are limited to `string | int | bool |
  time.Duration`.
- Env var is `<PREFIX>_SETTING_<KEY>`, upper-cased, `.` becomes `_`. An empty value counts
  as unset, so an unset compose variable can't blank a setting.
- The env always has the highest priority. The persisted `Lookup` is called on every
  read. No caching, no stale reads.
- A value that doesn't parse is an error (`*ParseError`), from the env or the store alike.
  **No fallback to another source and no silent default**, because a typo in a switch
  must not quietly mean "off". The error never contains the value (it may be a secret).
- `Source` reports env / config / unset without parsing, so a UI can lock a field whose
  variable holds garbage. It replaces `IsManaged` and `Managed`.
- Masking is a mode, `MaskNone` (default: nothing, not even the length) or `MaskLastFour`.
- The module owns no persistence, validation, clamping or UI markup. Those stay in each
  app (e.g. an app's minimum value for a setting).
- No dependencies beyond the standard library.

## Consumers

Not migrated yet. Each app currently has its own copy under `internal/settings`
(`Resolve`, `IsManaged`, `MaskSecret`). Migrate one app at a time on a branch, keeping that
app's existing settings tests as the regression check. MyApp's `go.mod` is on Go 1.26.5
and must be bumped to 1.27 first (check its Dockerfile and CI image too).
Behaviour changes for the apps' own copies: an empty env var used to lock the field to
`""` and now counts as unset; an unusable env value used to fall back or be ignored and is
now an error; `Get` returns `(T, error)`; `Managed`/`IsManaged` become `Source`.
