// Package configy is a tiny settings layer: every setting is read from the
// app's own persisted store, unless an environment variable sets it. The
// environment always wins, and a setting says where its value came from, so the
// UI can render an env-set one read-only with the variable's name.
//
//	cfg := configy.New("MYAPP", st.Lookup)
//	level, err := cfg.Get[string]("log_level")
//	s, err := cfg.Resolve[time.Duration]("tv_grace_period") // s.Value, s.Source, s.EnvVar
//
// The env var for a key is <PREFIX>_SETTING_<KEY>, upper-cased with "." turned
// into "_": "media.api_key" under prefix "MYAPP" is
// MYAPP_SETTING_MEDIA_API_KEY. An env var that is set but empty counts as
// unset, so an unset compose variable (FOO=${FOO}) can't silently blank a
// setting.
//
// A value that cannot be read as the requested type is an error, never a
// fallback: an env var the operator set but got wrong must not be quietly
// ignored in favour of an older stored value, and a switch must not quietly turn
// off (or on). The error names the key and the env var, and deliberately not the
// value, which may be a secret that ended up in the wrong place.
package configy

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Lookup returns the persisted value for key, and false if none is stored. It
// is called on every read, so it must read the store's current state.
type Lookup func(key string) (string, bool)

// Value is the set of types a setting can be read as.
type Value interface {
	string | int | bool | time.Duration
}

// Source says where a setting's value came from.
type Source int

const (
	// SourceUnset: no env var and nothing stored. The value is T's zero value.
	SourceUnset Source = iota
	// SourceConfig: the app's persisted store.
	SourceConfig
	// SourceEnv: an environment variable, which overrides the store.
	SourceEnv
)

func (s Source) String() string {
	switch s {
	case SourceConfig:
		return "config"
	case SourceEnv:
		return "env"
	}
	return "unset"
}

// Setting is a resolved setting: its value, where it came from, and the env var
// that would (or does) set it.
type Setting[T Value] struct {
	Value  T
	Source Source
	// EnvVar is the variable that sets this key, whether or not it is set now.
	EnvVar string
}

// ErrInvalid is what a ParseError unwraps to, so a caller can use errors.Is. It
// is deliberately all there is: the parser's own error quotes the value.
var ErrInvalid = errors.New("invalid value")

// ParseError is a value that could not be read as the type asked for. Neither it
// nor anything it wraps contains the value itself.
type ParseError struct {
	Key    string
	Source Source // SourceEnv or SourceConfig
	EnvVar string // the variable, when Source is SourceEnv
	Type   string
}

func (e *ParseError) Error() string {
	if e.Source == SourceEnv {
		return fmt.Sprintf("%s is not a valid %s", e.EnvVar, e.Type)
	}
	return fmt.Sprintf("the stored value of %q is not a valid %s", e.Key, e.Type)
}

func (e *ParseError) Unwrap() error { return ErrInvalid }

// Mask is how much of a secret a UI may show.
type Mask int

const (
	// MaskNone shows nothing about the value but that one is set. The default.
	MaskNone Mask = iota
	// MaskLastFour shows the last four characters, e.g. "••••••1234".
	MaskLastFour
)

// Option configures a Config.
type Option func(*Config)

// WithMask sets how Config.Mask shows a secret. The default is MaskNone.
func WithMask(m Mask) Option { return func(c *Config) { c.mask = m } }

// Config reads settings from env vars, falling back to a persisted store.
type Config struct {
	prefix string
	lookup Lookup
	mask   Mask
}

// New returns a Config whose env vars are <prefix>_SETTING_<KEY>. lookup
// supplies the persisted value for keys no env var sets; it may be nil when
// everything comes from env.
func New(prefix string, lookup Lookup, opts ...Option) *Config {
	c := &Config{
		prefix: strings.ToUpper(strings.TrimRight(prefix, "_")) + "_SETTING_",
		lookup: lookup,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// EnvVar returns the env var that sets key.
func (c *Config) EnvVar(key string) string {
	return c.prefix + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

// Source reports where key's value comes from, without reading it as any type.
// Use it to refuse an API write to a setting the environment owns:
//
//	if cfg.Source("media.url") == configy.SourceEnv { /* read-only */ }
func (c *Config) Source(key string) Source {
	if _, ok := c.env(key); ok {
		return SourceEnv
	}
	if c.lookup != nil {
		if _, ok := c.lookup(key); ok {
			return SourceConfig
		}
	}
	return SourceUnset
}

// Resolve returns key's effective value as T and where it came from. The env var
// wins whenever it is set; otherwise the persisted value; otherwise T's zero value
// with SourceUnset. A value that does not parse as T is a *ParseError, whichever
// source it came from: there is no fallback to the next source.
func (c *Config) Resolve[T Value](key string) (Setting[T], error) {
	s := Setting[T]{EnvVar: c.EnvVar(key)}
	var zero T
	if raw, ok := c.env(key); ok {
		v, err := parse[T](raw)
		if err != nil {
			return Setting[T]{EnvVar: s.EnvVar}, &ParseError{Key: key, Source: SourceEnv, EnvVar: s.EnvVar, Type: typeName(zero)}
		}
		s.Value, s.Source = v, SourceEnv
		return s, nil
	}
	if c.lookup != nil {
		if raw, ok := c.lookup(key); ok {
			v, err := parse[T](raw)
			if err != nil {
				return Setting[T]{EnvVar: s.EnvVar}, &ParseError{Key: key, Source: SourceConfig, Type: typeName(zero)}
			}
			s.Value, s.Source = v, SourceConfig
		}
	}
	return s, nil
}

// Get is Resolve without the source.
func (c *Config) Get[T Value](key string) (T, error) {
	s, err := c.Resolve[T](key)
	return s.Value, err
}

// Mask shows a secret the way this Config was set up to (see WithMask).
func (c *Config) Mask(value string) string { return MaskSecret(value, c.mask) }

// env returns the env var's value for key, treating empty as unset.
func (c *Config) env(key string) (string, bool) {
	v, ok := os.LookupEnv(c.EnvVar(key))
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

func typeName[T Value](zero T) string {
	switch any(zero).(type) {
	case string:
		return "string"
	case int:
		return "whole number"
	case bool:
		return "true/false"
	case time.Duration:
		return "duration"
	}
	return fmt.Sprintf("%T", zero)
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
				return 0, fmt.Errorf("invalid duration: %w", err)
			}
			return time.Duration(f * float64(unit)), nil
		}
	}
	return time.ParseDuration(raw)
}

// MaskSecret shows value according to mode. An empty value stays empty in every
// mode, so "not set" is never confused with "set". MaskNone never reveals the
// length either.
func MaskSecret(value string, mode Mask) string {
	if value == "" {
		return ""
	}
	if mode == MaskLastFour {
		const visible = 4
		n := []rune(value)
		if len(n) <= visible {
			return strings.Repeat("•", len(n))
		}
		return strings.Repeat("•", len(n)-visible) + string(n[len(n)-visible:])
	}
	return "••••••••"
}
