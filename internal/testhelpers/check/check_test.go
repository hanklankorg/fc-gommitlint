// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package check

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// fakeT records a single failure for assertion. Fatalf also records that
// the failure was fatal: production *testing.T's Fatalf would call
// FailNow, but in tests we want to observe the failure and continue
// inspecting state.
type fakeT struct {
	failed bool
	fatal  bool
	msg    string
}

func (fake *fakeT) Errorf(format string, args ...any) {
	fake.failed = true
	fake.msg = fmt.Sprintf(format, args...)
}

func (fake *fakeT) Fatalf(format string, args ...any) {
	fake.failed = true
	fake.fatal = true
	fake.msg = fmt.Sprintf(format, args...)
}

func (*fakeT) Helper() {}

// wantPass / wantFail are the two states we ever assert on a fake.
func wantPass(t *testing.T, fake *fakeT) {
	t.Helper()

	if fake.failed {
		t.Fatalf("expected pass, fake failed with: %s", fake.msg)
	}
}

// wantFail requires a fatal failure: every helper stops the test, so a
// caller never runs on past a failed precondition.
func wantFail(t *testing.T, fake *fakeT, substr string) {
	t.Helper()

	if !fake.failed {
		t.Fatal("expected fail, fake passed")
	}

	if !fake.fatal {
		t.Fatalf("expected a fatal failure, fake recorded Errorf: %s", fake.msg)
	}

	if substr != "" && !containsSubstring(fake.msg, substr) {
		t.Fatalf("expected fail message to contain %q, got: %s", substr, fake.msg)
	}
}

func containsSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}

	return sub == ""
}

// Sentinel errors used by the test suite. Declared at the package
// level to satisfy err113, which forbids ad-hoc errors.New in tests.
var (
	errBoom              = errors.New("boom")
	errSentinel          = errors.New("sentinel")
	errOther             = errors.New("other")
	errPlain             = errors.New("plain")
	errConfigInvalidYAML = errors.New("config: invalid yaml")
	errConfigInvalid     = errors.New("config: invalid")
)

// ─── Equal / NotEqual / Equalf ────────────────────────────────────────────

func TestEqual(t *testing.T) {
	t.Parallel()

	t.Run("equal primitives pass", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Equal(fake, 42, 42)
		wantPass(t, fake)
	})

	t.Run("unequal primitives fail with diff", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Equal(fake, 42, 7)
		wantFail(t, fake, "mismatch")
	})

	t.Run("equal structs with unexported fields pass via cmp.Exporter", func(t *testing.T) {
		t.Parallel()

		type s struct {
			pub  string
			priv int
		}

		fake := &fakeT{}
		Equal(fake, s{pub: "a", priv: 1}, s{pub: "a", priv: 1})
		wantPass(t, fake)
	})

	t.Run("nil and empty slice considered different", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}

		var nilSlice []int
		Equal(fake, nilSlice, []int{})
		wantFail(t, fake, "mismatch")
	})

	t.Run("nil and empty map considered different", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}

		var nilMap map[string]int
		Equal(fake, nilMap, map[string]int{})
		wantFail(t, fake, "mismatch")
	})

	t.Run("msgAndArgs string appended on failure", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Equal(fake, 1, 2, "rule must equal expected")
		wantFail(t, fake, "rule must equal expected")
	})

	t.Run("msgAndArgs format+args formatted", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Equal(fake, 1, 2, "rule %s expected %d", "Subject", 42)
		wantFail(t, fake, "rule Subject expected 42")
	})
}

func TestEqualf(t *testing.T) {
	t.Parallel()

	fake := &fakeT{}
	Equalf(fake, 1, 2, "expected %d, got something else", 1)
	wantFail(t, fake, "expected 1, got something else")
}

func TestNotEqual(t *testing.T) {
	t.Parallel()

	t.Run("different values pass", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		NotEqual(fake, 1, 2)
		wantPass(t, fake)
	})

	t.Run("same values fail", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		NotEqual(fake, 1, 1)
		wantFail(t, fake, "unexpected equality")
	})
}

// ─── NoError / Error / ErrorIs / ErrorAs / ErrorContains ────────────────

func TestNoError(t *testing.T) {
	t.Parallel()

	t.Run("nil err passes", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		NoError(fake, nil)
		wantPass(t, fake)
	})

	t.Run("non-nil err fails", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		NoError(fake, errBoom)
		wantFail(t, fake, "boom")
	})
}

func TestError(t *testing.T) {
	t.Parallel()

	t.Run("non-nil err passes", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Error(fake, errBoom)
		wantPass(t, fake)
	})

	t.Run("nil err fails", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Error(fake, nil)
		wantFail(t, fake, "expected an error")
	})
}

func TestErrorIs(t *testing.T) {
	t.Parallel()

	t.Run("wrapped errSentinel passes", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		ErrorIs(fake, fmt.Errorf("context: %w", errSentinel), errSentinel)
		wantPass(t, fake)
	})

	t.Run("unrelated error fails", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		ErrorIs(fake, errOther, errSentinel)
		wantFail(t, fake, "errors.Is")
	})

	t.Run("nil err fails", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		ErrorIs(fake, nil, errSentinel)
		wantFail(t, fake, "errors.Is")
	})
}

type customError struct{ msg string }

func (e *customError) Error() string { return e.msg }

func TestErrorAs(t *testing.T) {
	t.Parallel()

	t.Run("matching type passes", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}

		var target *customError
		ErrorAs(fake, fmt.Errorf("wrap: %w", &customError{msg: "boom"}), &target)
		wantPass(t, fake)
	})

	t.Run("non-matching type fails", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}

		var target *customError
		ErrorAs(fake, errPlain, &target)
		wantFail(t, fake, "errors.As")
	})
}

func TestErrorContains(t *testing.T) {
	t.Parallel()

	t.Run("substring present passes", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		ErrorContains(fake, errConfigInvalidYAML, "invalid yaml")
		wantPass(t, fake)
	})

	t.Run("substring absent fails", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		ErrorContains(fake, errConfigInvalid, "missing")
		wantFail(t, fake, "does not contain")
	})

	t.Run("nil err fails", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		ErrorContains(fake, nil, "x")
		wantFail(t, fake, "got nil")
	})
}

// ─── True / False ─────────────────────────────────────────────────────────

func TestTrue(t *testing.T) {
	t.Parallel()

	fake := &fakeT{}
	True(fake, true)
	wantPass(t, fake)

	fake = &fakeT{}
	True(fake, false)
	wantFail(t, fake, "expected true")
}

func TestFalse(t *testing.T) {
	t.Parallel()

	fake := &fakeT{}
	False(fake, false)
	wantPass(t, fake)

	fake = &fakeT{}
	False(fake, true)
	wantFail(t, fake, "expected false")
}

// ─── Nil / NotNil — covers the typed-nil edge case ──────────────────────

func TestNil(t *testing.T) {
	t.Parallel()

	t.Run("untyped nil passes", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Nil(fake, nil)
		wantPass(t, fake)
	})

	t.Run("typed nil pointer passes", func(t *testing.T) {
		t.Parallel()

		var p *int

		fake := &fakeT{}
		Nil(fake, p)
		wantPass(t, fake)
	})

	t.Run("typed nil slice passes", func(t *testing.T) {
		t.Parallel()

		var s []int

		fake := &fakeT{}
		Nil(fake, s)
		wantPass(t, fake)
	})

	t.Run("non-nil pointer fails", func(t *testing.T) {
		t.Parallel()

		value := 1
		fake := &fakeT{}
		Nil(fake, &value)
		wantFail(t, fake, "expected nil")
	})

	t.Run("non-nil scalar fails", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Nil(fake, 1)
		wantFail(t, fake, "expected nil")
	})
}

func TestNotNil(t *testing.T) {
	t.Parallel()

	value := 1
	fake := &fakeT{}
	NotNil(fake, &value)
	wantPass(t, fake)

	fake = &fakeT{}

	var p *int
	NotNil(fake, p) // typed nil pointer
	wantFail(t, fake, "expected non-nil")
}

// ─── Contains / NotContains / ContainsString ────────────────────────────

func TestContains(t *testing.T) {
	t.Parallel()

	t.Run("string substring", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Contains(fake, "the quick brown fox", "brown")
		wantPass(t, fake)
	})

	t.Run("string substring absent", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Contains(fake, "the quick brown fox", "purple")
		wantFail(t, fake, "does not contain")
	})

	t.Run("slice element", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Contains(fake, []int{1, 2, 3}, 2)
		wantPass(t, fake)
	})

	t.Run("slice element absent", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Contains(fake, []string{"a", "b"}, "c")
		wantFail(t, fake, "does not contain")
	})

	t.Run("map key", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Contains(fake, map[string]int{"a": 1, "b": 2}, "a")
		wantPass(t, fake)
	})

	t.Run("map key absent", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Contains(fake, map[string]int{"a": 1}, "z")
		wantFail(t, fake, "does not contain")
	})
}

func TestNotContains(t *testing.T) {
	t.Parallel()

	fake := &fakeT{}
	NotContains(fake, "abc", "z")
	wantPass(t, fake)

	fake = &fakeT{}
	NotContains(fake, "abc", "b")
	wantFail(t, fake, "unexpectedly contains")
}

const helloStr = "hello"

// ─── Len / Empty / NotEmpty ─────────────────────────────────────────────

func TestLen(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		collection any
		want       int
		shouldPass bool
	}{
		{"string", helloStr, 5, true},
		{"string wrong len", helloStr, 4, false},
		{"slice", []int{1, 2, 3}, 3, true},
		{"empty slice", []int{}, 0, true},
		{"nil slice (len 0)", []int(nil), 0, true},
		{"map", map[string]int{"a": 1}, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeT{}
			Len(fake, tc.collection, tc.want)

			if tc.shouldPass {
				wantPass(t, fake)
			} else {
				wantFail(t, fake, "len")
			}
		})
	}
}

func TestEmpty(t *testing.T) {
	t.Parallel()

	t.Run("nil passes", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Empty(fake, nil)
		wantPass(t, fake)
	})

	t.Run("empty string passes", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Empty(fake, "")
		wantPass(t, fake)
	})

	t.Run("empty slice passes", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Empty(fake, []int{})
		wantPass(t, fake)
	})

	t.Run("non-empty fails", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Empty(fake, "x")
		wantFail(t, fake, "expected empty")
	})
}

func TestNotEmpty(t *testing.T) {
	t.Parallel()

	fake := &fakeT{}
	NotEmpty(fake, "x")
	wantPass(t, fake)

	fake = &fakeT{}
	NotEmpty(fake, "")
	wantFail(t, fake, "expected non-empty")
}

// ─── Zero / NotZero ─────────────────────────────────────────────────────

func TestZero(t *testing.T) {
	t.Parallel()

	t.Run("int zero passes", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Zero(fake, 0)
		wantPass(t, fake)
	})

	t.Run("string zero passes", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Zero(fake, "")
		wantPass(t, fake)
	})

	t.Run("untyped nil passes (no type to zero against)", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Zero(fake, nil)
		wantPass(t, fake)
	})

	t.Run("non-zero fails", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Zero(fake, 7)
		wantFail(t, fake, "expected zero")
	})
}

func TestNotZero(t *testing.T) {
	t.Parallel()

	fake := &fakeT{}
	NotZero(fake, 7)
	wantPass(t, fake)

	fake = &fakeT{}
	NotZero(fake, 0)
	wantFail(t, fake, "expected non-zero")
}

// ─── Greater / GreaterOrEqual / Less / LessOrEqual / Positive ──────────

func TestGreater(t *testing.T) {
	t.Parallel()

	fake := &fakeT{}
	Greater(fake, 5, 3)
	wantPass(t, fake)

	fake = &fakeT{}
	Greater(fake, 3, 5)
	wantFail(t, fake, "want >")

	fake = &fakeT{}
	Greater(fake, 5, 5)
	wantFail(t, fake, "want >")
}

func TestGreaterOrEqualInclusive(t *testing.T) {
	t.Parallel()

	fake := &fakeT{}
	GreaterOrEqual(fake, 5, 5)
	wantPass(t, fake)

	fake = &fakeT{}
	GreaterOrEqual(fake, 4, 5)
	wantFail(t, fake, "want >=")
}

func TestLess(t *testing.T) {
	t.Parallel()

	fake := &fakeT{}
	Less(fake, 3, 5)
	wantPass(t, fake)

	fake = &fakeT{}
	Less(fake, 5, 3)
	wantFail(t, fake, "want <")
}

func TestLessOrEqual(t *testing.T) {
	t.Parallel()

	fake := &fakeT{}
	LessOrEqual(fake, 5, 5)
	wantPass(t, fake)

	fake = &fakeT{}
	LessOrEqual(fake, 6, 5)
	wantFail(t, fake, "want <=")
}

func TestPositive(t *testing.T) {
	t.Parallel()

	fake := &fakeT{}
	Positive(fake, 1)
	wantPass(t, fake)

	fake = &fakeT{}
	Positive(fake, 0)
	wantFail(t, fake, "expected positive")
}

// ─── InDelta — covers the any-typed numeric coercion ───────────────────

func TestInDelta(t *testing.T) {
	t.Parallel()

	t.Run("float64 within delta", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		InDelta(fake, 1.0, 1.005, 0.01)
		wantPass(t, fake)
	})

	t.Run("float64 outside delta", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		InDelta(fake, 1.0, 1.5, 0.01)
		wantFail(t, fake, ">")
	})

	t.Run("int coerced to float64", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		InDelta(fake, 1, 1, 0.01)
		wantPass(t, fake)
	})

	t.Run("expected from any (JSON-decoded number)", func(t *testing.T) {
		t.Parallel()

		var jsonish any = float64(42)

		fake := &fakeT{}
		InDelta(fake, 42, jsonish, 0.01)
		wantPass(t, fake)
	})

	t.Run("non-numeric fails clearly", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		InDelta(fake, "not a number", 1, 0.01)
		wantFail(t, fake, "not numeric")
	})
}

// ─── Regexp / ElementsMatch / Same ──────────────────────────────────────

func TestRegexp(t *testing.T) {
	t.Parallel()

	fake := &fakeT{}
	Regexp(fake, `^foo[0-9]+$`, "foo42")
	wantPass(t, fake)

	fake = &fakeT{}
	Regexp(fake, `^foo[0-9]+$`, "bar")
	wantFail(t, fake, "does not match")

	fake = &fakeT{}
	Regexp(fake, `(`, "anything") // invalid regex
	wantFail(t, fake, "invalid regexp")
}

func TestElementsMatch(t *testing.T) {
	t.Parallel()

	t.Run("same elements different order pass", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		ElementsMatch(fake, []int{1, 2, 3}, []int{3, 1, 2})
		wantPass(t, fake)
	})

	t.Run("missing element fails", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		ElementsMatch(fake, []int{1, 2, 3}, []int{1, 2})
		wantFail(t, fake, "element-sets differ")
	})
}

func TestSame(t *testing.T) {
	t.Parallel()

	first := 1
	fake := &fakeT{}
	Same(fake, &first, &first)
	wantPass(t, fake)

	second := 1
	fake = &fakeT{}
	Same(fake, &first, &second)
	wantFail(t, fake, "pointers differ")

	// Non-pointer arguments fail same() short-circuit
	fake = &fakeT{}
	Same(fake, 1, 1)
	wantFail(t, fake, "pointers differ")
}

// ─── FileExists / NoFileExists / DirExists ─────────────────────────────

func TestFileExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	regular := filepath.Join(dir, "file.txt")

	if err := os.WriteFile(regular, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	fake := &fakeT{}
	FileExists(fake, regular)
	wantPass(t, fake)

	fake = &fakeT{}
	FileExists(fake, filepath.Join(dir, "nope"))
	wantFail(t, fake, "")

	// Directory should fail FileExists
	fake = &fakeT{}
	FileExists(fake, dir)
	wantFail(t, fake, "directory")
}

func TestNoFileExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fake := &fakeT{}
	NoFileExists(fake, filepath.Join(dir, "absent"))
	wantPass(t, fake)

	present := filepath.Join(dir, "present")
	if err := os.WriteFile(present, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	fake = &fakeT{}
	NoFileExists(fake, present)
	wantFail(t, fake, "file exists")
}

func TestDirExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fake := &fakeT{}
	DirExists(fake, dir)
	wantPass(t, fake)

	// A regular file should fail DirExists.
	regular := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(regular, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	fake = &fakeT{}
	DirExists(fake, regular)
	wantFail(t, fake, "not a directory")
}

// ─── Panics / NotPanics ─────────────────────────────────────────────────

func TestPanics(t *testing.T) {
	t.Parallel()

	t.Run("panicking fn passes Panics", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Panics(fake, func() { panic("boom") })
		wantPass(t, fake)
	})

	t.Run("non-panicking fn fails Panics", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		Panics(fake, func() {})
		wantFail(t, fake, "expected panic")
	})

	t.Run("non-panicking fn passes NotPanics", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		NotPanics(fake, func() {})
		wantPass(t, fake)
	})

	t.Run("panicking fn fails NotPanics", func(t *testing.T) {
		t.Parallel()

		fake := &fakeT{}
		NotPanics(fake, func() { panic("boom") })
		wantFail(t, fake, "unexpected panic")
	})
}

func TestFailf(t *testing.T) {
	t.Parallel()

	fake := &fakeT{}
	Failf(fake, "missing", "rule %s missing", "Subject")
	wantFail(t, fake, "missing: rule Subject missing")
}

func TestFailNow(t *testing.T) {
	t.Parallel()

	fake := &fakeT{}
	FailNow(fake, "deliberate")
	wantFail(t, fake, "deliberate")
}

// ─── msg() helper — covers single-string vs format+args paths ──────────

func TestMsg(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   []any
		want string
	}{
		{"empty", nil, ""},
		{"single string", []any{"hello"}, ": hello"},
		{"single non-string", []any{42}, ": 42"},
		{"format + args", []any{"rule %s id %d", "Subject", 7}, ": rule Subject id 7"},
		{"non-string first defaults to %v", []any{42, "ignored"}, ": [42 ignored]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := msg(tc.in)
			if got != tc.want {
				t.Errorf("msg(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ─── isNil() helper — covers the typed-nil corner ──────────────────────

func TestIsNil(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		v    any
		want bool
	}{
		{"untyped nil", nil, true},
		{"typed nil pointer", (*int)(nil), true},
		{"typed nil slice", []int(nil), true},
		{"typed nil map", map[string]int(nil), true},
		{"typed nil channel", chan int(nil), true},
		{"typed nil func", (func())(nil), true},
		{"non-nil pointer", new(int), false},
		{"int zero", 0, false},
		{"empty string", "", false},
		{"non-nil interface", errBoom, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isNil(tc.v); got != tc.want {
				t.Errorf("isNil(%v) = %v, want %v", tc.v, got, tc.want)
			}
		})
	}
}

// ─── asFloat64() helper — covers all numeric coercions ────────────────

func TestAsFloat64(t *testing.T) {
	t.Parallel()

	cases := []struct {
		v    any
		want float64
		ok   bool
	}{
		{float64(1.5), 1.5, true},
		{float32(1.5), 1.5, true},
		{42, 42, true},
		{int64(42), 42, true},
		{uint(7), 7, true},
		{uint64(7), 7, true},
		{"not a number", 0, false},
		{nil, 0, false},
		{struct{}{}, 0, false},
	}
	for _, tc := range cases {
		got, ok := asFloat64(tc.v)
		if ok != tc.ok || got != tc.want {
			t.Errorf("asFloat64(%v) = (%v, %v), want (%v, %v)", tc.v, got, ok, tc.want, tc.ok)
		}
	}
}

// ─── Sanity: real *testing.T satisfies TestingT ────────────────────────

func TestRealTestingTSatisfiesTestingT(t *testing.T) {
	t.Parallel()

	var _ TestingT = t
	// Smoke: a passing call on the real t shouldn't fail the suite.
	Equal(t, 1, 1)
	NoError(t, nil)
}
