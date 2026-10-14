// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"github.com/leonelquinteros/gotext"
)

const (
	localeDomain  = "gommitlint"
	localePoFile  = "gommitlint.po"
	localeMoFile  = "gommitlint.mo"
	localeDirName = "locale"
	localeLcName  = "LC_MESSAGES"
	envLCAll      = "LC_ALL"
	envLCMessages = "LC_MESSAGES"
	envLang       = "LANG"
)

// NewLocalizerFromEnv creates a localizer using LC_ALL, LC_MESSAGES, or LANG.
func NewLocalizerFromEnv() output.Localizer {
	locale := detectLocaleFromEnv(getEnv)

	return NewLocalizer(locale)
}

// NewLocalizer creates a localizer for the provided locale string.
func NewLocalizer(locale string) output.Localizer {
	return newLocalizer(locale, loadTranslator)
}

// NewLocalizerFromDir creates a localizer from an explicitly trusted locale directory.
func NewLocalizerFromDir(locale string, localeDir string) output.Localizer {
	return newLocalizer(locale, func(normalized string) gotext.Translator {
		return loadTranslatorFromPaths(explicitLocalePaths(localeDir, normalized))
	})
}

func newLocalizer(locale string, loader func(string) gotext.Translator) output.Localizer {
	normalized := normalizeLocale(locale)
	localizer := gotext.NewLocale("", normalized)

	if normalized == "" {
		return localizer
	}

	translator := loader(normalized)
	if translator != nil {
		localizer.AddTranslator(localeDomain, translator)
	}

	return localizer
}

func detectLocaleFromEnv(getenv func(string) string) string {
	for _, key := range []string{envLCAll, envLCMessages, envLang} {
		if value := strings.TrimSpace(getenv(key)); value != "" {
			return value
		}
	}

	return ""
}

func normalizeLocale(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return ""
	}

	locale = strings.Split(locale, ".")[0]
	locale = strings.Split(locale, "@")[0]
	locale = strings.ReplaceAll(locale, "-", "_")

	parts := strings.Split(locale, "_")
	if len(parts) == 1 {
		return strings.ToLower(parts[0])
	}

	language := strings.ToLower(parts[0])
	region := strings.ToUpper(parts[1])

	return language + "_" + region
}

func loadTranslator(locale string) gotext.Translator {
	if locale == "" {
		return nil
	}

	return loadTranslatorFromPaths(localeCandidatePaths(locale))
}

func loadTranslatorFromPaths(paths []string) gotext.Translator {
	rootFS := os.DirFS(string(filepath.Separator))
	for _, path := range paths {
		relPath := strings.TrimPrefix(filepath.Clean(path), string(filepath.Separator))
		if !fs.ValidPath(relPath) {
			continue
		}

		data, err := fs.ReadFile(rootFS, filepath.ToSlash(relPath))
		if err != nil {
			continue
		}

		translator := translatorFromData(path, data)
		if translator != nil {
			return translator
		}
	}

	return nil
}

func explicitLocalePaths(localeDir string, locale string) []string {
	localeDir, err := filepath.Abs(localeDir)
	if err != nil {
		return nil
	}

	var paths []string
	for _, variant := range localeVariants(locale) {
		paths = append(paths,
			filepath.Join(localeDir, variant, localePoFile),
			filepath.Join(localeDir, variant, localeLcName, localeMoFile),
		)
	}

	return paths
}

func translatorFromData(path string, data []byte) gotext.Translator {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mo":
		mo := gotext.NewMo()
		mo.Parse(data)

		return mo
	case ".po":
		po := gotext.NewPo()
		po.Parse(data)

		return po
	default:
		return nil
	}
}

func localeCandidatePaths(locale string) []string {
	variants := localeVariants(locale)
	paths := make([]string, 0, len(variants)*4)
	seen := make(map[string]struct{})

	addPath := func(path string) {
		if path == "" {
			return
		}

		if _, ok := seen[path]; ok {
			return
		}

		seen[path] = struct{}{}
		paths = append(paths, path)
	}

	for _, variant := range variants {
		for _, path := range xdgDataHomeLocalePaths(variant) {
			addPath(path)
		}

		for _, path := range xdgDataDirLocalePaths(variant) {
			addPath(path)
		}
	}

	return paths
}

func localeVariants(locale string) []string {
	variants := []string{locale}

	base := baseLocale(locale)
	if base != "" && base != locale {
		variants = append(variants, base)
	}

	return variants
}

func xdgDataHomeLocalePaths(locale string) []string {
	dataHome := xdgDataHome()
	if dataHome == "" {
		return nil
	}

	return []string{
		filepath.Join(dataHome, localeDirName, locale, localeLcName, localeMoFile),
	}
}

func xdgDataDirLocalePaths(locale string) []string {
	var paths []string

	for _, dir := range xdgDataDirs() {
		if dir == "" {
			continue
		}

		paths = append(paths, filepath.Join(dir, localeDirName, locale, localeLcName, localeMoFile))
	}

	return paths
}

func xdgDataHome() string {
	if value := getEnv("XDG_DATA_HOME"); value != "" {
		return value
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".local", "share")
}

func xdgDataDirs() []string {
	if value := getEnv("XDG_DATA_DIRS"); value != "" {
		return strings.Split(value, ":")
	}

	return []string{"/usr/local/share", "/usr/share"}
}

func baseLocale(locale string) string {
	parts := strings.Split(locale, "_")
	if len(parts) == 0 {
		return ""
	}

	return strings.ToLower(parts[0])
}

func getEnv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}
