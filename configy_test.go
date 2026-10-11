package configy

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func store(m map[string]string) Lookup {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func mustGet[T Value](t *testing.T, c *Config, key string) T {
	t.Helper()
	v, err := c.Get[T](key)
	if err != nil {
		t.Fatalf("Get(%q): %v", key, err)
	}
	return v
}

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
	if got := mustGet[string](t, c, "name"); got != "disk" {
		t.Errorf("string: %q", got)
	}
	if got := mustGet[int](t, c, "n"); got != 7 {
		t.Errorf("int: %d", got)
	}
	if got := mustGet[bool](t, c, "on"); !got {
		t.Error("bool: want true")
	}
	if got := mustGet[time.Duration](t, c, "d"); got != 48*time.Hour {
		t.Errorf("duration: %v", got)
	}
	if got := c.Source("name"); got != SourceConfig {
		t.Errorf("source = %v, want config", got)
	}
}

func TestEnvWinsOverTheStoreForEveryType(t *testing.T) {
	t.Setenv("T_SETTING_NAME", "env")
	t.Setenv("T_SETTING_N", "9")
	t.Setenv("T_SETTING_ON", "false")
	t.Setenv("T_SETTING_D", "1h")
	t.Setenv("T_SETTING_MEDIA_URL", "http://j")
	c := New("T", store(map[string]string{"name": "disk", "n": "7", "on": "true", "d": "2d", "media.url": "disk"}))

	if got := mustGet[string](t, c, "name"); got != "env" {
		t.Errorf("string: %q", got)
	}
	if got := mustGet[int](t, c, "n"); got != 9 {
		t.Errorf("int: %d", got)
	}
	if got := mustGet[bool](t, c, "on"); got {
		t.Error("bool: the env's false must beat the store's true")
	}
	if got := mustGet[time.Duration](t, c, "d"); got != time.Hour {
		t.Errorf("duration: %v", got)
	}
	if got := mustGet[string](t, c, "media.url"); got != "http://j" {
		t.Errorf("dotted key: %q", got)
	}
}

func TestResolveCarriesTheSourceAndTheEnvVar(t *testing.T) {
	t.Setenv("T_SETTING_FROM_ENV", "e")
	c := New("T", store(map[string]string{"from_env": "d", "from_config": "c"}))
	for key, want := range map[string]struct {
		value string
		src   Source
		env   string
	}{
		"from_env":    {"e", SourceEnv, "T_SETTING_FROM_ENV"},
		"from_config": {"c", SourceConfig, "T_SETTING_FROM_CONFIG"},
		"nowhere":     {"", SourceUnset, "T_SETTING_NOWHERE"},
	} {
		s, err := c.Resolve[string](key)
		if err != nil || s.Value != want.value || s.Source != want.src || s.EnvVar != want.env {
			t.Errorf("%s: %+v, %v; want %+v", key, s, err, want)
		}
	}
	if SourceEnv.String() != "env" || SourceConfig.String() != "config" || SourceUnset.String() != "unset" {
		t.Error("source names")
	}
}

func TestEmptyEnvIsUnset(t *testing.T) {
	t.Setenv("T_SETTING_NAME", "")
	c := New("T", store(map[string]string{"name": "disk"}))
	if got := mustGet[string](t, c, "name"); got != "disk" {
		t.Errorf("got %q, want the stored value", got)
	}
	if c.Source("name") != SourceConfig {
		t.Error("an empty env var must not read as the env setting it")
	}
}

func TestAnInvalidEnvValueIsAnErrorWithNoFallback(t *testing.T) {
	const secretish = "s3cr3t-NOT-A-NUMBER"
	// Each has a perfectly good stored value that must NOT be used instead.
	t.Setenv("T_SETTING_N", secretish)
	t.Setenv("T_SETTING_D", secretish)
	t.Setenv("T_SETTING_ON", secretish)
	c := New("T", store(map[string]string{"n": "5", "d": "1h", "on": "true"}))

	check := func(name string, err error) {
		t.Helper()
		var pe *ParseError
		if !errors.As(err, &pe) || pe.Source != SourceEnv || !strings.HasPrefix(pe.EnvVar, "T_SETTING_") {
			t.Errorf("%s: err = %v, want a *ParseError from the env", name, err)
			return
		}
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: errors.Is(err, ErrInvalid) = false", name)
		}
		// Neither the message nor anything wrapped may contain the value.
		for e := error(err); e != nil; e = errors.Unwrap(e) {
			if strings.Contains(e.Error(), secretish) {
				t.Errorf("%s: the value leaked into %q", name, e.Error())
			}
		}
		if !strings.Contains(err.Error(), pe.EnvVar) {
			t.Errorf("%s: the message should name the variable: %q", name, err.Error())
		}
	}
	n, err := c.Get[int]("n")
	check("int", err)
	if n != 0 {
		t.Errorf("a failed read must return the zero value, got %d", n)
	}
	d, err := c.Get[time.Duration]("d")
	check("duration", err)
	if d != 0 {
		t.Errorf("got %v", d)
	}
	on, err := c.Get[bool]("on")
	check("bool", err)
	if on {
		t.Error("a failed read must not return true")
	}
	if s, err := c.Resolve[int]("n"); err == nil || s.Source != SourceUnset || s.EnvVar != "T_SETTING_N" {
		t.Errorf("Resolve on a bad value: %+v, %v", s, err)
	}
}

func TestAGarbledSwitchIsAnErrorNotFalse(t *testing.T) {
	// "ture" for a lock switch must not quietly mean "off".
	t.Setenv("T_SETTING_READONLY", "ture")
	v, err := New("T", nil).Get[bool]("readonly")
	if err == nil {
		t.Fatalf("got %v with no error: the caller cannot tell a typo from false", v)
	}
}

func TestAnInvalidStoredValueIsAnErrorToo(t *testing.T) {
	c := New("T", store(map[string]string{"n": "oops"}))
	_, err := c.Get[int]("n")
	var pe *ParseError
	if !errors.As(err, &pe) || pe.Source != SourceConfig || pe.EnvVar != "" {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "oops") || !strings.Contains(err.Error(), `"n"`) {
		t.Errorf("message %q should name the key and not the value", err.Error())
	}
}

func TestEnvStillWinsOverAnInvalidStoredValue(t *testing.T) {
	t.Setenv("T_SETTING_N", "3")
	c := New("T", store(map[string]string{"n": "oops"}))
	if got := mustGet[int](t, c, "n"); got != 3 {
		t.Errorf("got %d: a bad stored value is beside the point when the env sets it", got)
	}
}

func TestSourceNeverParses(t *testing.T) {
	t.Setenv("T_SETTING_N", "abc")
	c := New("T", store(map[string]string{"m": "oops"}))
	if c.Source("n") != SourceEnv || c.Source("m") != SourceConfig {
		t.Error("Source reports where a value is, even one that would not parse, so a UI can lock the field")
	}
}

func TestMissingGivesTheZeroValueAndNoError(t *testing.T) {
	c := New("T", store(map[string]string{}))
	if got := mustGet[int](t, c, "absent"); got != 0 {
		t.Errorf("absent: got %d", got)
	}
	if got := mustGet[string](t, New("T", nil), "n"); got != "" {
		t.Errorf("nil lookup: got %q", got)
	}
}

func TestLookupReadAnewEveryGet(t *testing.T) {
	m := map[string]string{"n": "1"}
	c := New("T", store(m))
	if got := mustGet[int](t, c, "n"); got != 1 {
		t.Fatalf("got %d", got)
	}
	m["n"] = "2"
	if got := mustGet[int](t, c, "n"); got != 2 {
		t.Errorf("stale read: got %d, want 2", got)
	}
}

func TestWhitespaceIsTrimmedForNumbersButNotAValue(t *testing.T) {
	t.Setenv("T_SETTING_N", " 5 ")
	if got := mustGet[int](t, New("T", nil), "n"); got != 5 {
		t.Errorf("got %d", got)
	}
	t.Setenv("T_SETTING_N", "   ")
	if _, err := New("T", nil).Get[int]("n"); err == nil {
		t.Error("only spaces is set (not empty) and is not a number")
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
		if got := mustGet[time.Duration](t, New("T", nil), "d"); got != want {
			t.Errorf("%q: got %v, want %v", in, got, want)
		}
	}
	for _, bad := range []string{"d", "xd", "1x", "w", "--1h", "1 d"} {
		t.Setenv("T_SETTING_D", bad)
		if _, err := New("T", nil).Get[time.Duration]("d"); err == nil {
			t.Errorf("%q should not parse", bad)
		}
	}
}

func TestMaskNoneRevealsNothingNotEvenTheLength(t *testing.T) {
	if got := MaskSecret("", MaskNone); got != "" {
		t.Errorf("empty must stay empty, got %q", got)
	}
	a, b := MaskSecret("k", MaskNone), MaskSecret("a-much-longer-api-key-0123456789", MaskNone)
	if a == "" || a != b {
		t.Errorf("%q vs %q: a set secret must look the same whatever its length", a, b)
	}
	if strings.Contains(b, "6789") {
		t.Error("the tail leaked")
	}
}

func TestMaskLastFour(t *testing.T) {
	for in, want := range map[string]string{
		"":           "",
		"ab":         "••",
		"abcd":       "••••",
		"abcdef1234": "••••••1234",
	} {
		if got := MaskSecret(in, MaskLastFour); got != want {
			t.Errorf("MaskSecret(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConfigMaskFollowsTheOptionAndDefaultsToNone(t *testing.T) {
	if got := New("T", nil).Mask("abcdef1234"); strings.Contains(got, "1234") || got == "" {
		t.Errorf("the default must reveal nothing, got %q", got)
	}
	if got := New("T", nil, WithMask(MaskLastFour)).Mask("abcdef1234"); got != "••••••1234" {
		t.Errorf("got %q", got)
	}
	if got := New("T", nil, WithMask(MaskNone)).Mask(""); got != "" {
		t.Errorf("got %q", got)
	}
}
