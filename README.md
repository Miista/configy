# configy

A tiny settings layer for Go apps. Each setting is read from the app's own
persisted store unless an environment variable pins it. A pinned setting wins
and is reported as managed, so the UI can show it read-only.

Requires Go 1.27 or later (generic methods).

```go
cfg := configy.New("MYAPP", st.Lookup)

level := cfg.Get[string]("log_level")
delay := cfg.Get[int]("next_run_delay_minutes")
grace := cfg.Get[time.Duration]("tv_grace_period") // "7d", "2w", "45m"
on    := cfg.Get[bool]("daemon_enabled")
```

`Get` accepts `string`, `int`, `bool` and `time.Duration`.

## Where values come from

1. The env var `<PREFIX>_SETTING_<KEY>`, upper-cased with `.` turned into `_`
   (`media.api_key` is `MYAPP_SETTING_MEDIA_API_KEY`). A variable that
   is set but empty counts as unset.
2. The `Lookup` you pass to `New`, `func(key string) (string, bool)`. It is
   called on every `Get`, so it must read the store's current state.
3. The zero value.

## Invalid values

An env value that doesn't parse is logged (see `WithWarn`) and then handled by
type: `int` and `time.Duration` fall back to the persisted value, `bool` falls
to `false` so a garbled switch fails safe. The setting still counts as managed.

## For the UI and API

```go
cfg.IsManaged("log_level")                      // refuse API writes to pinned settings
cfg.Managed("log_level", "media.url")        // {"log_level": "MYAPP_SETTING_LOG_LEVEL"}
configy.MaskSecret(apiKey)                      // "••••••1234"
```

`Managed` takes the keys to check because a dotted key can't be recovered from
its env var name.

## Not included

Persistence, validation and clamping stay in the app. So does any UI markup.
