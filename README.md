# configy

A tiny settings layer for Go apps. Each setting is read from the app's own
persisted store, unless an environment variable sets it. **The environment always
wins**, and a setting says where its value came from, so the UI can show an
env-set one read-only.

Requires Go 1.27 or later (generic methods).

```go
cfg := configy.New("MYAPP", st.Lookup)

level, err := cfg.Get[string]("log_level")
delay, err := cfg.Get[int]("next_run_delay_minutes")
grace, err := cfg.Get[time.Duration]("tv_grace_period") // "7d", "2w", "45m"
on, err    := cfg.Get[bool]("daemon_enabled")

s, err := cfg.Resolve[string]("media.url")
// s.Value, s.Source (env | config | unset), s.EnvVar ("MYAPP_SETTING_MEDIA_URL")
```

`Get` and `Resolve` accept `string`, `int`, `bool` and `time.Duration`.

## Where values come from

1. The env var `<PREFIX>_SETTING_<KEY>`, upper-cased with `.` turned into `_`
   (`media.api_key` is `MYAPP_SETTING_MEDIA_API_KEY`). A variable that
   is set but empty counts as unset.
2. The `Lookup` you pass to `New`, `func(key string) (string, bool)`. It is
   called on every read, so it must read the store's current state.
3. The zero value, with source `unset`.

## Invalid values are errors

A value that doesn't parse as the type asked for is a `*ParseError`, from
whichever source it came from. There is **no fallback**: a variable the operator
set but got wrong is not quietly replaced by an older stored value, and a switch
is not quietly turned off (`READONLY=ture` is an error, not `false`). The caller
decides what to do; an app will usually refuse to start.

The error names the key and the env var, and **never the value**, which may be a
secret in the wrong place. It unwraps to `configy.ErrInvalid`.

## For the UI and API

```go
cfg.Source("log_level") == configy.SourceEnv   // refuse API writes; render read-only
cfg.EnvVar("log_level")                        // "MYAPP_SETTING_LOG_LEVEL", for the label
cfg.Mask(apiKey)                               // per WithMask; default shows nothing
```

`Source` never parses the value, so a field whose variable holds garbage is still
reported as env-set and can be shown locked next to the error.

### Showing a secret

`WithMask` sets how much `Mask` reveals: `MaskNone` (the default) shows that a
value is set and nothing else, not even its length; `MaskLastFour` shows
`••••••1234`. An empty value stays empty in both, so "not set" is never confused
with "set".

## Not included

Persistence, validation and clamping stay in the app, and so does any UI markup.
Rules that relate settings to each other (an app that must lock a URL when its
API key is env-set, say) are the app's, built on `Source`.
