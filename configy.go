// Package configy is a tiny settings layer: every setting is read from the
// app's own persisted store, unless an environment variable pins it. A pinned
// setting wins over the stored value and is reported as managed, so the UI can
// render it read-only with the env var's name.
//
//	cfg := configy.New("MYAPP", st.Lookup)
//	level := cfg.Get[string]("log_level")
//	delay := cfg.Get[time.Duration]("tv_grace_period")
//
// The env var for a key is <PREFIX>_SETTING_<KEY>, upper-cased with "." turned
// into "_": "media.api_key" under prefix "MYAPP" is
// MYAPP_SETTING_MEDIA_API_KEY. An env var that is set but empty counts as
// unset, so an unset compose variable (FOO=${FOO}) can't silently blank a
// setting.
package configy

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Lookup returns the persisted value for key, and false if none is stored. It
// is called on every Get, so it must read the store's current state.
type Lookup func(key string) (string, bool)

// Value is the set of types a setting can be read as.
type Value interface {
	string | int | bool | time.Duration
}

// Option configures a Config.
type Option func(*Config)

// WithWarn routes warnings (an unparseable env value) to logf. The default
// writes to standard error.
func WithWarn(logf func(format string, args ...any)) Option {
	return func(c *Config) { c.warn = logf }
}

// Config reads settings from env vars, falling back to a persisted store.
type Config struct {
	prefix string
	lookup Lookup
	warn   func(format string, args ...any)
}

// New returns a Config whose env vars are <prefix>_SETTING_<KEY>. lookup
// supplies the persisted value for keys no env var pins; it may be nil when
// everything comes from env.
func New(prefix string, lookup Lookup, opts ...Option) *Config {
	c := &Config{
		prefix: strings.ToUpper(strings.TrimRight(prefix, "_")) + "_SETTING_",
		lookup: lookup,
		warn: func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "configy: "+format+"\n", args...)
		},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// EnvVar returns the env var that pins key.
func (c *Config) EnvVar(key string) string {
	return c.prefix + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

// IsManaged reports whether key is currently pinned by its env var. Use it to
// refuse API writes to a pinned setting.
func (c *Config) IsManaged(key string) bool {
	_, ok := c.env(key)
	return ok
}

// Managed returns, for each of keys that is currently pinned, the env var
// that pins it: the "env_managed" map a UI needs. Keys must be passed in
// because a dotted key ("media.url") can't be recovered from its env var.
func (c *Config) Managed(keys ...string) map[string]string {
	m := map[string]string{}
	for _, k := range keys {
		if c.IsManaged(k) {
			m[k] = c.EnvVar(k)
		}
	}
	return m
}

// Get returns the effective value of key as T: the env var if it pins the
// key, otherwise the persisted value, otherwise T's zero value.
//
// An env value that doesn't parse as T is reported through the warn hook and
// then handled by type: int and Duration fall back to the persisted value, and
// bool falls to false, so a garbled switch fails safe rather than on. A
// persisted value that doesn't parse yields the zero value.
func (c *Config) Get[T Value](key string) T {
	var zero T
	if raw, ok := c.env(key); ok {
		v, err := parse[T](raw)
		if err == nil {
			return v
		}
		c.warn("%s=%q is not a valid %T; ignoring it", c.EnvVar(key), raw, zero)
		if _, isBool := any(zero).(bool); isBool {
			return zero
		}
	}
	if c.lookup != nil {
		if raw, ok := c.lookup(key); ok {
			if v, err := parse[T](raw); err == nil {
				return v
			}
		}
	}
	return zero
}

// env returns the pinning env var's value for key, treating empty as unset.
func (c *Config) env(key string) (string, bool) {
	v, ok := os.LookupEnv(c.EnvVar(key))
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

func parse[T Value](raw string) (T, error) {
	var zero T
	switch any(zero).(type) {
	case string:
		return any(raw).(T), nil
	case int:
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		return any(n).(T), err
	case bool:
		b, err := strconv.ParseBool(strings.TrimSpace(raw))
		return any(b).(T), err
	case time.Duration:
		d, err := parseDuration(strings.TrimSpace(raw))
		return any(d).(T), err
	}
	return zero, fmt.Errorf("unsupported type %T", zero)
}

// parseDuration extends time.ParseDuration with "d" (days) and "w" (weeks)
// suffixes, e.g. "7d" or "2w". Anything else goes to time.ParseDuration.
func parseDuration(raw string) (time.Duration, error) {
	for suffix, unit := range map[string]time.Duration{"d": 24 * time.Hour, "w": 7 * 24 * time.Hour} {
		if n, ok := strings.CutSuffix(raw, suffix); ok {
			f, err := strconv.ParseFloat(n, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid duration %q: %w", raw, err)
			}
			return time.Duration(f * float64(unit)), nil
		}
	}
	return time.ParseDuration(raw)
}

// MaskSecret returns value with all but the last 4 characters replaced by
// bullets, e.g. "••••••1234". An empty value stays empty.
func MaskSecret(value string) string {
	if value == "" {
		return ""
	}
	const visible = 4
	n := []rune(value)
	if len(n) <= visible {
		return strings.Repeat("•", len(n))
	}
	return strings.Repeat("•", len(n)-visible) + string(n[len(n)-visible:])
}
