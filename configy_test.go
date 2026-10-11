package configy

import (
	"testing"
	"time"
)

func store(m map[string]string) Lookup {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func quiet() Option { return WithWarn(func(string, ...any) {}) }

func TestEnvVarNaming(t *testing.T) {
	c := New("myapp", nil)
	if got := c.EnvVar("tv_grace_period"); got != "MYAPP_SETTING_TV_GRACE_PERIOD" {
		t.Errorf("got %s", got)
	}
	if got := c.EnvVar("media.api_key"); got != "MYAPP_SETTING_MEDIA_API_KEY" {
		t.Errorf("got %s", got)
	}
	if got := New("APP_", nil).EnvVar("x"); got != "APP_SETTING_X" {
		t.Errorf("trailing underscore in prefix not trimmed: %s", got)
	}
}

func TestPersistedUsedWhenNoEnv(t *testing.T) {
	c := New("T", store(map[string]string{"name": "disk", "n": "7", "on": "true", "d": "2d"}))
	if got := c.Get[string]("name"); got != "disk" {
		t.Errorf("string: %q", got)
	}
	if got := c.Get[int]("n"); got != 7 {
		t.Errorf("int: %d", got)
	}
	if got := c.Get[bool]("on"); !got {
		t.Error("bool: want true")
	}
	if got := c.Get[time.Duration]("d"); got != 48*time.Hour {
		t.Errorf("duration: %v", got)
	}
	if c.IsManaged("name") {
		t.Error("managed with no env var set")
	}
}

func TestEnvWinsAndIsManaged(t *testing.T) {
	t.Setenv("T_SETTING_NAME", "env")
	t.Setenv("T_SETTING_MEDIA_URL", "http://j")
	c := New("T", store(map[string]string{"name": "disk", "media.url": "disk", "other": "x"}))

	if got := c.Get[string]("name"); got != "env" {
		t.Errorf("got %q", got)
	}
	if got := c.Get[string]("media.url"); got != "http://j" {
		t.Errorf("dotted key: got %q", got)
	}
	if !c.IsManaged("name") || c.IsManaged("other") {
		t.Error("IsManaged wrong")
	}
	m := c.Managed("name", "media.url", "other")
	if len(m) != 2 || m["name"] != "T_SETTING_NAME" || m["media.url"] != "T_SETTING_MEDIA_URL" {
		t.Errorf("Managed = %v", m)
	}
}

func TestEmptyEnvIsUnset(t *testing.T) {
	t.Setenv("T_SETTING_NAME", "")
	c := New("T", store(map[string]string{"name": "disk"}))
	if got := c.Get[string]("name"); got != "disk" {
		t.Errorf("got %q, want persisted", got)
	}
	if c.IsManaged("name") {
		t.Error("empty env var reported as managed")
	}
}

func TestInvalidEnvFallsBackByType(t *testing.T) {
	t.Setenv("T_SETTING_N", "abc")
	t.Setenv("T_SETTING_D", "soon")
	t.Setenv("T_SETTING_ON", "maybe")
	c := New("T", store(map[string]string{"n": "5", "d": "1h", "on": "true"}), quiet())

	if got := c.Get[int]("n"); got != 5 {
		t.Errorf("int: got %d, want persisted 5", got)
	}
	if got := c.Get[time.Duration]("d"); got != time.Hour {
		t.Errorf("duration: got %v, want persisted 1h", got)
	}
	if got := c.Get[bool]("on"); got {
		t.Error("bool: garbled env must fail safe to false, not fall back to persisted true")
	}
	if !c.IsManaged("n") {
		t.Error("an invalid env value is still managed, so the UI shows it locked")
	}
}

func TestInvalidEnvWarns(t *testing.T) {
	t.Setenv("T_SETTING_N", "abc")
	var warned int
	c := New("T", nil, WithWarn(func(string, ...any) { warned++ }))
	c.Get[int]("n")
	if warned != 1 {
		t.Errorf("warned %d times, want 1", warned)
	}
}

func TestMissingAndInvalidPersistedGiveZero(t *testing.T) {
	c := New("T", store(map[string]string{"n": "oops"}))
	if got := c.Get[int]("n"); got != 0 {
		t.Errorf("invalid persisted: got %d", got)
	}
	if got := c.Get[string]("absent"); got != "" {
		t.Errorf("absent: got %q", got)
	}
	if got := New("T", nil).Get[int]("n"); got != 0 {
		t.Errorf("nil lookup: got %d", got)
	}
}

func TestLookupReadAnewEveryGet(t *testing.T) {
	m := map[string]string{"n": "1"}
	c := New("T", store(m))
	if got := c.Get[int]("n"); got != 1 {
		t.Fatalf("got %d", got)
	}
	m["n"] = "2"
	if got := c.Get[int]("n"); got != 2 {
		t.Errorf("stale read: got %d, want 2", got)
	}
}

func TestDurationForms(t *testing.T) {
	cases := map[string]time.Duration{
		"7d":    7 * 24 * time.Hour,
		"2w":    14 * 24 * time.Hour,
		"1.5d":  36 * time.Hour,
		"45m":   45 * time.Minute,
		"1h30m": 90 * time.Minute,
	}
	for in, want := range cases {
		t.Setenv("T_SETTING_D", in)
		if got := New("T", nil).Get[time.Duration]("d"); got != want {
			t.Errorf("%q: got %v, want %v", in, got, want)
		}
	}
}

func TestMaskSecret(t *testing.T) {
	cases := map[string]string{
		"":           "",
		"ab":         "••",
		"abcd":       "••••",
		"abcdef1234": "••••••1234",
	}
	for in, want := range cases {
		if got := MaskSecret(in); got != want {
			t.Errorf("MaskSecret(%q) = %q, want %q", in, got, want)
		}
	}
}
