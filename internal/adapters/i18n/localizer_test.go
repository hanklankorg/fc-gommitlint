// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package i18n

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

const (
	localeDeDE = "de_DE"
	localeEnUS = "en_US"
	localeSvSE = "sv_SE"
)

func TestNormalizeLocale(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty", input: "", expected: ""},
		{name: "whitespace only", input: "   ", expected: ""},
		{name: "simple language", input: "en", expected: "en"},
		{name: "language uppercase", input: "EN", expected: "en"},
		{name: "language region", input: localeEnUS, expected: localeEnUS},
		{name: "region lowercase normalised", input: "en_us", expected: localeEnUS},
		{name: "language uppercase normalised", input: "EN_US", expected: localeEnUS},
		{name: "strip encoding", input: "en_US.UTF-8", expected: localeEnUS},
		{name: "strip variant", input: "en_US@euro", expected: localeEnUS},
		{name: "strip encoding and variant", input: "en_US.UTF-8@euro", expected: localeEnUS},
		{name: "dash separator converted", input: "en-US", expected: localeEnUS},
		{name: "leading whitespace", input: "  sv_SE", expected: localeSvSE},
		{name: "trailing whitespace", input: "sv_SE  ", expected: localeSvSE},
		{name: "swedish", input: "sv", expected: "sv"},
		{name: "swedish region", input: localeSvSE, expected: localeSvSE},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			check.Equal(t, tc.expected, normalizeLocale(tc.input))
		})
	}
}

func TestDetectLocaleFromEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		env      map[string]string
		expected string
	}{
		{
			name:     "all empty returns empty",
			env:      map[string]string{envLCAll: "", envLCMessages: "", envLang: ""},
			expected: "",
		},
		{
			name:     "LC_ALL takes precedence over all",
			env:      map[string]string{envLCAll: localeSvSE, envLCMessages: localeEnUS, envLang: localeDeDE},
			expected: localeSvSE,
		},
		{
			name:     "LC_MESSAGES used when LC_ALL empty",
			env:      map[string]string{envLCAll: "", envLCMessages: localeEnUS, envLang: localeDeDE},
			expected: localeEnUS,
		},
		{
			name:     "LANG as last fallback",
			env:      map[string]string{envLCAll: "", envLCMessages: "", envLang: localeDeDE},
			expected: localeDeDE,
		},
		{
			name:     "whitespace-only values skipped",
			env:      map[string]string{envLCAll: "  ", envLCMessages: "  en_US  ", envLang: ""},
			expected: localeEnUS,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			getenv := func(key string) string { return tc.env[key] }
			check.Equal(t, tc.expected, detectLocaleFromEnv(getenv))
		})
	}
}

func TestLocaleVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		locale   string
		expected []string
	}{
		{name: "simple language has no extra variants", locale: "en", expected: []string{"en"}},
		{name: "region locale includes base", locale: localeEnUS, expected: []string{localeEnUS, "en"}},
		{name: "swedish region includes base", locale: localeSvSE, expected: []string{localeSvSE, "sv"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			check.Equal(t, tc.expected, localeVariants(tc.locale))
		})
	}
}

func TestBaseLocale(t *testing.T) {
	t.Parallel()

	check.Equal(t, "en", baseLocale(localeEnUS))
	check.Equal(t, "sv", baseLocale(localeSvSE))
	check.Equal(t, "en", baseLocale("en"))
	check.Equal(t, "en", baseLocale("EN_US"))
}

func TestXdgDataDirs_DefaultPaths(t *testing.T) {
	t.Setenv("XDG_DATA_DIRS", "")

	dirs := xdgDataDirs()
	check.NotEmpty(t, dirs)
	check.Contains(t, dirs, "/usr/local/share")
	check.Contains(t, dirs, "/usr/share")
}

func TestXdgDataDirs_FromEnv(t *testing.T) {
	t.Setenv("XDG_DATA_DIRS", "/opt/share:/custom/share")

	check.Equal(t, []string{"/opt/share", "/custom/share"}, xdgDataDirs())
}

func TestXdgDataHome_FromEnv(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/custom/data")

	check.Equal(t, "/custom/data", xdgDataHome())
}

func TestXdgDataHome_DefaultFallback(t *testing.T) {
	// HOME is pinned to a temp directory rather than read from the host. The
	// suffix check this replaces was satisfied by whatever home directory the
	// machine happened to have, and would fail outright wherever HOME is unset
	// — os.UserHomeDir errors there and xdgDataHome returns "". Pinning it
	// makes the exact path assertable instead of just its ending.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	check.Equal(t, filepath.Join(home, ".local", "share"), xdgDataHome())
}

// isolateLocaleSearch pins the XDG variables the candidate-path builder reads,
// so the list under test comes from this fixture rather than from whatever the
// host has installed. Without it the search roots are the machine's real
// /usr/share and $HOME, which makes the assertions below depend on the box.
func isolateLocaleSearch(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data-home"))
	t.Setenv("XDG_DATA_DIRS", filepath.Join(root, "data-dir"))
}

func TestLocaleCandidatePaths_NoDuplicates(t *testing.T) {
	isolateLocaleSearch(t)

	paths := localeCandidatePaths("en")

	// Without this the test is vacuous: a builder returning nothing at all has
	// no duplicates either, and would satisfy the loop below perfectly.
	check.NotEmpty(t, paths, "no candidate paths would make the duplicate check meaningless")

	seen := make(map[string]struct{})

	for _, p := range paths {
		_, exists := seen[p]
		check.False(t, exists, "duplicate path: %s", p)
		seen[p] = struct{}{}
	}
}

// TestLocaleCandidatePaths_TriesTheRegionBeforeTheBaseLocale pins the ordering,
// not just the membership. The previous form checked that en_US paths and en
// paths were both present, which a builder that searched the generic locale
// FIRST would also satisfy — and that builder would load an "en" translation
// for a user who asked for "en_US" whenever both are installed, silently
// preferring the less specific one.
//
// The guarantee comes from localeVariants returning the region before the base
// and localeCandidatePaths iterating variants in order, so every en_US path
// must precede every en path.
func TestLocaleCandidatePaths_TriesTheRegionBeforeTheBaseLocale(t *testing.T) {
	isolateLocaleSearch(t)

	paths := localeCandidatePaths(localeEnUS)
	check.NotEmpty(t, paths)

	lastRegion, firstBase := -1, -1

	for index, path := range paths {
		switch {
		case strings.Contains(path, "/"+localeEnUS+"/"):
			lastRegion = index
		case strings.Contains(path, "/en/"):
			if firstBase == -1 {
				firstBase = index
			}
		}
	}

	check.NotEqual(t, -1, lastRegion, "expected paths containing /en_US/")
	check.NotEqual(t, -1, firstBase, "expected paths containing /en/ (base locale fallback)")
	check.True(t, lastRegion < firstBase,
		"every %s path must be searched before any base-locale path (last region %d, first base %d)",
		localeEnUS, lastRegion, firstBase)
}

func TestTranslatorFromData_UnknownExtension(t *testing.T) {
	t.Parallel()

	check.Nil(t, translatorFromData("file.txt", []byte("content")))
	check.Nil(t, translatorFromData("file", []byte("content")))
}

func TestTranslatorFromData_EmptyPO(t *testing.T) {
	t.Parallel()

	// gotext handles empty PO data gracefully — returns a usable (empty) translator
	check.NotNil(t, translatorFromData("file.po", []byte("")))
}

func TestTranslatorFromData_EmptyMO(t *testing.T) {
	t.Parallel()

	check.NotNil(t, translatorFromData("file.mo", []byte("")))
}

func TestNewLocalizer_EmptyLocale(t *testing.T) {
	t.Parallel()

	l := NewLocalizer("")
	check.NotNil(t, l)
	check.Equal(t, "hello", l.Get("hello"))
	check.Equal(t, "hello world", l.Get("hello %s", "world"))
}

func TestNewLocalizer_UnknownLocale_PassthroughFallback(t *testing.T) {
	t.Parallel()

	l := NewLocalizer("zz_ZZ")
	check.NotNil(t, l)
	check.Equal(t, "hello", l.Get("hello"))
	check.Equal(t, "one item", l.GetN("one item", "%d items", 1))
	check.Equal(t, "2 items", l.GetN("one item", "%d items", 2, 2))
}

func TestNewLocalizer_DoesNotLoadCatalogFromWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	localeDir := filepath.Join(root, localeDirName, "sv")
	check.NoError(t, os.MkdirAll(localeDir, 0o750))

	po := "msgid \"\"\nmsgstr \"Language: sv\\n\"\n\nmsgid \"hello\"\nmsgstr \"repository supplied\"\n"
	check.NoError(t, os.WriteFile(filepath.Join(localeDir, "gommitlint.po"), []byte(po), 0o600))
	t.Chdir(root)
	isolateLocaleSearch(t)

	localizer := NewLocalizer("sv")

	check.Equal(t, "hello", localizer.Get("hello"))
}
