# CLAUDE.md

## What this is

`configy` is a tiny, dependency-free Go module (`github.com/miista/configy`) shared by
several apps. Each setting is read from the app's own
persisted store unless an env var pins it; a pinned setting wins and is reported as
managed so the UI can show it read-only. See `README.md` for usage.

## Commands

- `go test ./...`
- `go vet ./...`
- `gofmt -l .` (should print nothing)

Requires Go 1.27 or later: `Config.Get[T]` is a generic method.

## Design rules (decided, don't relitigate)

- The public surface stays small: `New`, `Get[T]`, `IsManaged`, `Managed`, `EnvVar`,
  `MaskSecret`, `WithWarn`. `Get[T]` is limited to `string | int | bool | time.Duration`.
- Env var is `<PREFIX>_SETTING_<KEY>`, upper-cased, `.` becomes `_`. An empty value counts
  as unset, so an unset compose variable can't blank a setting.
- The persisted `Lookup` is called on every `Get`. No caching, no stale reads.
- Invalid env value: warn, then `int` and `time.Duration` fall back to the persisted value
  and `bool` falls to `false` (fail safe). The setting still counts as managed.
- `Managed` takes the keys to check. Dotted keys can't be recovered from env var names.
- The module owns no persistence, validation, clamping or UI markup. Those stay in each
  app (e.g. an app's minimum value for a setting).
- No dependencies beyond the standard library.

## Consumers

Not migrated yet. Each app currently has its own copy under `internal/settings`
(`Resolve`, `IsManaged`, `MaskSecret`). Migrate one app at a time on a branch, keeping that
app's existing settings tests as the regression check. MyApp's `go.mod` is on Go 1.26.5
and must be bumped to 1.27 first (check its Dockerfile and CI image too).
Behaviour change for the apps' own copies: an empty env var used to lock the field to
`""` and now counts as unset.
