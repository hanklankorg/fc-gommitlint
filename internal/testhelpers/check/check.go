// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package check

import (
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"regexp"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// TestingT is the subset of *testing.T this package needs. Allows
// passing fakes from check's own tests; *testing.T satisfies it.
type TestingT interface {
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Helper()
}

// defaultOpts allows diffing unexported fields in tests.
//
// cmp.Exporter that always returns true is unsafe in production code
// but routine in tests — we explicitly opt every test into "I want
// to see internal field diffs".
func defaultOpts() []cmp.Option {
	return []cmp.Option{
		cmp.Exporter(func(reflect.Type) bool { return true }),
	}
}

// msg renders one literal argument or a format string followed by arguments.
func msg(msgAndArgs []any) string {
	switch len(msgAndArgs) {
	case 0:
		return ""
	case 1:
		if asText, ok := msgAndArgs[0].(string); ok {
			return ": " + asText
		}

		return ": " + fmt.Sprintf("%v", msgAndArgs[0])
	default:
		if format, ok := msgAndArgs[0].(string); ok {
			return ": " + fmt.Sprintf(format, msgAndArgs[1:]...)
		}

		return ": " + fmt.Sprintf("%v", msgAndArgs)
	}
}

// Equal fails when expected != actual.
//
// Equality is decided by reflect.DeepEqual, including for pointers and
// third-party types with cycle-prone or non-comparable internals. cmp.Diff only
// render a human-readable (-expected +actual) report when sides
// differ; if cmp panics on the offending types (e.g. *foo with
// non-comparable internals), we fall back to %+v.
func Equal(tb TestingT, expected, actual any, msgAndArgs ...any) {
	tb.Helper()

	if reflect.DeepEqual(expected, actual) {
		return
	}

	tb.Fatalf("mismatch%s:\n%s", msg(msgAndArgs), safeDiff(expected, actual))
}

// Equalf accepts a format string and arguments for failure context.
func Equalf(tb TestingT, expected, actual any, format string, args ...any) {
	tb.Helper()
	Equal(tb, expected, actual, append([]any{format}, args...)...)
}

// NotEqual fails when expected == actual. Equality semantics match
// Equal (reflect.DeepEqual).
func NotEqual(tb TestingT, expected, actual any, msgAndArgs ...any) {
	tb.Helper()

	if reflect.DeepEqual(expected, actual) {
		tb.Fatalf("unexpected equality%s:\n%v", msg(msgAndArgs), actual)
	}
}

// safeDiff renders an expected/actual pair as a cmp.Diff string, or
// (when cmp panics on unwalkable internals) falls back to a verbose
// printf. Only used for failure output — never authoritative for
// equality decisions.
func safeDiff(expected, actual any) string {
	rendered := ""

	defer func() {
		if recovered := recover(); recovered != nil {
			rendered = fmt.Sprintf(
				"(cmp.Diff unavailable: %v)\nexpected: %+v\nactual:   %+v",
				recovered, expected, actual,
			)
		}
	}()

	rendered = cmp.Diff(expected, actual, defaultOpts()...)

	return rendered
}

// NoError fails when err is non-nil.
func NoError(tb TestingT, err error, msgAndArgs ...any) {
	tb.Helper()

	if err != nil {
		tb.Fatalf("unexpected error%s: %v", msg(msgAndArgs), err)
	}
}

// Error fails when err is nil.
func Error(tb TestingT, err error, msgAndArgs ...any) {
	tb.Helper()

	if err == nil {
		tb.Fatalf("expected an error%s, got nil", msg(msgAndArgs))
	}
}

// ErrorIs fails when err is nil OR doesn't wrap target.
func ErrorIs(tb TestingT, err, target error, msgAndArgs ...any) {
	tb.Helper()

	if !errors.Is(err, target) {
		tb.Fatalf("err = %v, want errors.Is(%v)%s", err, target, msg(msgAndArgs))
	}
}

// ErrorAs fails when err is nil OR doesn't unwrap to target's type.
func ErrorAs(tb TestingT, err error, target any, msgAndArgs ...any) {
	tb.Helper()

	if !errors.As(err, target) {
		tb.Fatalf("err = %v, want errors.As(%T)%s", err, target, msg(msgAndArgs))
	}
}

// ErrorContains fails when err is nil OR its message lacks substr.
func ErrorContains(tb TestingT, err error, substr string, msgAndArgs ...any) {
	tb.Helper()

	if err == nil {
		tb.Fatalf("expected error containing %q%s, got nil", substr, msg(msgAndArgs))

		return
	}

	if !strings.Contains(err.Error(), substr) {
		tb.Fatalf("error %q does not contain %q%s", err.Error(), substr, msg(msgAndArgs))
	}
}

// True fails when cond is false.
func True(tb TestingT, cond bool, msgAndArgs ...any) {
	tb.Helper()

	if !cond {
		tb.Fatalf("expected true%s", msg(msgAndArgs))
	}
}

// False fails when cond is true.
func False(tb TestingT, cond bool, msgAndArgs ...any) {
	tb.Helper()

	if cond {
		tb.Fatalf("expected false%s", msg(msgAndArgs))
	}
}

// Nil fails when v is non-nil. Typed nils such as (*T)(nil) count as nil.
func Nil(tb TestingT, value any, msgAndArgs ...any) {
	tb.Helper()

	if !isNil(value) {
		tb.Fatalf("expected nil%s, got: %v", msg(msgAndArgs), value)
	}
}

// NotNil fails when v is nil (including typed nils like (*T)(nil)).
func NotNil(tb TestingT, value any, msgAndArgs ...any) {
	tb.Helper()

	if isNil(value) {
		tb.Fatalf("expected non-nil%s", msg(msgAndArgs))
	}
}

// Contains asserts haystack contains needle. Handles:
//   - string haystack + string needle (substring)
//   - slice / array + element (deep equality via cmp)
//   - map + key (key presence)
func Contains(tb TestingT, haystack, needle any, msgAndArgs ...any) {
	tb.Helper()

	if !contains(haystack, needle) {
		tb.Fatalf("%v does not contain %v%s", haystack, needle, msg(msgAndArgs))
	}
}

// NotContains asserts haystack does NOT contain needle.
func NotContains(tb TestingT, haystack, needle any, msgAndArgs ...any) {
	tb.Helper()

	if contains(haystack, needle) {
		tb.Fatalf("%v unexpectedly contains %v%s", haystack, needle, msg(msgAndArgs))
	}
}

// Len fails when len(collection) != want.
func Len(tb TestingT, collection any, want int, msgAndArgs ...any) {
	tb.Helper()

	got, ok := lenOf(collection)
	if !ok {
		tb.Fatalf("Len: %T has no length%s", collection, msg(msgAndArgs))

		return
	}

	if got != want {
		tb.Fatalf("len = %d, want %d%s", got, want, msg(msgAndArgs))
	}
}

// Empty asserts len(collection) == 0 OR collection is nil.
func Empty(tb TestingT, collection any, msgAndArgs ...any) {
	tb.Helper()

	if isEmpty(collection) {
		return
	}

	tb.Fatalf("expected empty, got %v%s", collection, msg(msgAndArgs))
}

// NotEmpty asserts len(collection) > 0.
func NotEmpty(tb TestingT, collection any, msgAndArgs ...any) {
	tb.Helper()

	if !isEmpty(collection) {
		return
	}

	tb.Fatalf("expected non-empty%s", msg(msgAndArgs))
}

// Zero asserts v is the zero value for its type.
func Zero(tb TestingT, value any, msgAndArgs ...any) {
	tb.Helper()

	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return
	}

	zero := reflect.Zero(rv.Type()).Interface()
	if !cmp.Equal(zero, value, defaultOpts()...) {
		tb.Fatalf("expected zero value%s, got %v", msg(msgAndArgs), value)
	}
}

// NotZero asserts v is not the zero value for its type.
func NotZero(tb TestingT, value any, msgAndArgs ...any) {
	tb.Helper()

	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		tb.Fatalf("expected non-zero value, got untyped nil%s", msg(msgAndArgs))
	}

	zero := reflect.Zero(rv.Type()).Interface()
	if cmp.Equal(zero, value, defaultOpts()...) {
		tb.Fatalf("expected non-zero value%s, got %v", msg(msgAndArgs), value)
	}
}

// Greater asserts got > want (numeric, string).
func Greater[T numericOrString](tb TestingT, got, want T, msgAndArgs ...any) {
	tb.Helper()

	if got <= want {
		tb.Fatalf("got %v, want > %v%s", got, want, msg(msgAndArgs))
	}
}

// GreaterOrEqual asserts got >= want.
func GreaterOrEqual[T numericOrString](tb TestingT, got, want T, msgAndArgs ...any) {
	tb.Helper()

	if got < want {
		tb.Fatalf("got %v, want >= %v%s", got, want, msg(msgAndArgs))
	}
}

// Less asserts got < want.
func Less[T numericOrString](tb TestingT, got, want T, msgAndArgs ...any) {
	tb.Helper()

	if got >= want {
		tb.Fatalf("got %v, want < %v%s", got, want, msg(msgAndArgs))
	}
}

// LessOrEqual asserts got <= want.
func LessOrEqual[T numericOrString](tb TestingT, got, want T, msgAndArgs ...any) {
	tb.Helper()

	if got > want {
		tb.Fatalf("got %v, want <= %v%s", got, want, msg(msgAndArgs))
	}
}

// Positive asserts v > 0.
func Positive[T numeric](tb TestingT, value T, msgAndArgs ...any) {
	tb.Helper()

	if value <= 0 {
		tb.Fatalf("expected positive, got %v%s", value, msg(msgAndArgs))
	}
}

// InDelta asserts abs(expected - actual) <= delta. Accepts any
// numeric type, including float64 values from decoded JSON.
func InDelta(tb TestingT, expected, actual any, delta float64, msgAndArgs ...any) {
	tb.Helper()

	expectedF, ok := asFloat64(expected)
	if !ok {
		tb.Fatalf("InDelta: expected %T not numeric%s", expected, msg(msgAndArgs))

		return
	}

	actualF, ok := asFloat64(actual)
	if !ok {
		tb.Fatalf("InDelta: actual %T not numeric%s", actual, msg(msgAndArgs))

		return
	}

	if math.Abs(expectedF-actualF) > delta {
		tb.Fatalf("|%v - %v| > %v%s", expectedF, actualF, delta, msg(msgAndArgs))
	}
}

// asFloat64 coerces any numeric value (including float-typed JSON
// numbers received as any) into float64. Returns false for
// non-numeric inputs.
func asFloat64(value any) (float64, bool) {
	switch num := value.(type) {
	case float64:
		return num, true
	case float32:
		return float64(num), true
	case int:
		return float64(num), true
	case int8:
		return float64(num), true
	case int16:
		return float64(num), true
	case int32:
		return float64(num), true
	case int64:
		return float64(num), true
	case uint:
		return float64(num), true
	case uint8:
		return float64(num), true
	case uint16:
		return float64(num), true
	case uint32:
		return float64(num), true
	case uint64:
		return float64(num), true
	}

	return 0, false
}

// Regexp asserts s matches pattern (compiled regular expression).
func Regexp(tb TestingT, pattern, str string, msgAndArgs ...any) {
	tb.Helper()

	re, err := regexp.Compile(pattern)
	if err != nil {
		tb.Fatalf("invalid regexp %q: %v%s", pattern, err, msg(msgAndArgs))

		return
	}

	if !re.MatchString(str) {
		tb.Fatalf("%q does not match %q%s", str, pattern, msg(msgAndArgs))
	}
}

// ElementsMatch asserts both slices contain the same elements,
// regardless of order. Uses cmp.Diff with cmpopts.SortSlices for
// readable failure output.
func ElementsMatch(tb TestingT, listA, listB any, msgAndArgs ...any) {
	tb.Helper()

	opts := append(defaultOpts(),
		cmpopts.SortSlices(func(x, y any) bool {
			return fmt.Sprintf("%v", x) < fmt.Sprintf("%v", y)
		}),
	)
	if diff := cmp.Diff(listA, listB, opts...); diff != "" {
		tb.Fatalf("element-sets differ%s:\n%s", msg(msgAndArgs), diff)
	}
}

// Same asserts two pointers reference the same object.
func Same(tb TestingT, expected, actual any, msgAndArgs ...any) {
	tb.Helper()

	if !samePointer(expected, actual) {
		tb.Fatalf("pointers differ%s: %p vs %p", msg(msgAndArgs), expected, actual)
	}
}

// FileExists asserts the path exists and is a regular file.
func FileExists(tb TestingT, path string, msgAndArgs ...any) {
	tb.Helper()

	info, err := os.Stat(path)
	if err != nil {
		tb.Fatalf("FileExists(%s)%s: %v", path, msg(msgAndArgs), err)

		return
	}

	if info.IsDir() {
		tb.Fatalf("FileExists(%s)%s: is a directory", path, msg(msgAndArgs))
	}
}

// NoFileExists asserts the path either doesn't exist or isn't a regular file.
func NoFileExists(tb TestingT, path string, msgAndArgs ...any) {
	tb.Helper()

	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		tb.Fatalf("NoFileExists(%s)%s: file exists", path, msg(msgAndArgs))
	}
}

// DirExists asserts the path exists and is a directory.
func DirExists(tb TestingT, path string, msgAndArgs ...any) {
	tb.Helper()

	info, err := os.Stat(path)
	if err != nil {
		tb.Fatalf("DirExists(%s)%s: %v", path, msg(msgAndArgs), err)

		return
	}

	if !info.IsDir() {
		tb.Fatalf("DirExists(%s)%s: not a directory", path, msg(msgAndArgs))
	}
}

// Panics asserts fn panics when called.
func Panics(tb TestingT, fn func(), msgAndArgs ...any) {
	tb.Helper()

	defer func() {
		if recovered := recover(); recovered == nil {
			tb.Fatalf("expected panic%s, got none", msg(msgAndArgs))
		}
	}()

	fn()
}

// NotPanics asserts fn does not panic when called.
func NotPanics(tb TestingT, fn func(), msgAndArgs ...any) {
	tb.Helper()

	defer func() {
		if recovered := recover(); recovered != nil {
			tb.Fatalf("unexpected panic%s: %v", msg(msgAndArgs), recovered)
		}
	}()

	fn()
}

// Failf fails immediately with a failure label followed by formatted context.
func Failf(tb TestingT, failureMessage, format string, args ...any) {
	tb.Helper()
	tb.Fatalf("%s: "+format, append([]any{failureMessage}, args...)...)
}

// FailNow fails the test immediately with an optional message.
func FailNow(tb TestingT, msgAndArgs ...any) {
	tb.Helper()
	tb.Fatalf("test failed%s", msg(msgAndArgs))
}

type numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

type numericOrString interface {
	numeric | ~string
}

// isNil reports whether v is nil, including typed nils (channel,
// func, interface, map, pointer, slice) which == comparison misses.
func isNil(value any) bool {
	if value == nil {
		return true
	}

	rv := reflect.ValueOf(value)

	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface,
		reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

// lenOf returns len(v) for strings, byte slices, and any kind that
// supports len() via reflect. Second return is false otherwise.
func lenOf(value any) (int, bool) {
	switch typed := value.(type) {
	case string:
		return len(typed), true
	case []byte:
		return len(typed), true
	}

	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return 0, false
	}

	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array, reflect.Chan, reflect.String:
		return rv.Len(), true
	default:
		return 0, false
	}
}

// isEmpty treats nil, zero-length values, and basic zero values as empty.
func isEmpty(value any) bool {
	if isNil(value) {
		return true
	}

	if length, ok := lenOf(value); ok {
		return length == 0
	}

	rv := reflect.ValueOf(value)
	zero := reflect.Zero(rv.Type()).Interface()

	return cmp.Equal(value, zero, defaultOpts()...)
}

// contains for strings (substring), slices/arrays (element equality
// via cmp), and maps (key presence).
//
// String-kind values cover both plain `string` and defined types like
// `type diagnostics.Reason string` — these are widely used in this
// project, and a naive needle.(string) type-assertion would miss them.
func contains(haystack, needle any) bool {
	haystackStr, haystackIsString := asString(haystack)
	if haystackIsString {
		if sub, ok := asString(needle); ok {
			return strings.Contains(haystackStr, sub)
		}
	}

	rv := reflect.ValueOf(haystack)
	if !rv.IsValid() {
		return false
	}

	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := range rv.Len() {
			if cmp.Equal(rv.Index(i).Interface(), needle, defaultOpts()...) {
				return true
			}
		}

		return false
	case reflect.Map:
		for _, key := range rv.MapKeys() {
			if cmp.Equal(key.Interface(), needle, defaultOpts()...) {
				return true
			}
		}

		return false
	default:
		return false
	}
}

// asString coerces any string-kind value (plain string or a defined
// type like `type ID string`) to a Go string. Returns false for
// anything else.
func asString(value any) (string, bool) {
	if str, ok := value.(string); ok {
		return str, true
	}

	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return "", false
	}

	if rv.Kind() == reflect.String {
		return rv.String(), true
	}

	return "", false
}

// samePointer reports whether two values reference the same memory
// location. Both must be pointers; either being non-pointer returns
// false.
func samePointer(lhs, rhs any) bool {
	rvL := reflect.ValueOf(lhs)
	rvR := reflect.ValueOf(rhs)

	if rvL.Kind() != reflect.Pointer || rvR.Kind() != reflect.Pointer {
		return false
	}

	return rvL.Pointer() == rvR.Pointer()
}
