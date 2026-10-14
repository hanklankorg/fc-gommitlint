// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"
)

var errUpdateSecondReplacement = errors.New("injected second replacement failure")

const fakeMsgenScript = `#!/bin/sh
set -eu

printf '%s\n' BEGIN "$@" END >> "$MSGEN_ARGS"

output=
input=
after_separator=
for argument in "$@"; do
	if [ -n "$after_separator" ]; then
		input=$argument
		break
	fi
	case "$argument" in
		--output=*) output=${argument#--output=} ;;
		--) after_separator=1 ;;
	esac
done

if [ -z "$input" ]; then
	for argument in "$@"; do
		case "$argument" in
			--*) ;;
			*) input=$argument ; break ;;
		esac
	done
fi
if [ -z "$output" ] || [ -z "$input" ]; then
	printf '%s\n' 'missing output or input' >&2
	exit 90
fi
if [ "$MSGEN_STDERR" = 1 ]; then
	printf 'fake msgen: %s\n' "$input" >&2
fi
if [ "$MSGEN_MODE" = hang ]; then
	: > "$MSGEN_STARTED"
	while :; do :; done
fi
if [ "$MSGEN_MODE" = fail ]; then
	printf '%s\n' 'simulated msgen failure' >&2
	exit 22
fi

printf '%s\n' \
	'msgid ""' \
	'msgstr ""' \
	'"Project-Id-Version: old\\n"' \
	'"Language: old\\n"' \
	'' \
	'msgid "generated English"' \
	'msgstr "generated English"' > "$output"
`

const fakeMsgmergeScript = `#!/bin/sh
set -eu

printf '%s\n' BEGIN "$@" END >> "$MSGMERGE_ARGS"

input=
template=
after_separator=
for argument in "$@"; do
	if [ -n "$after_separator" ]; then
		if [ -z "$input" ]; then input=$argument; else template=$argument; break; fi
		continue
	fi
	case "$argument" in
		--) after_separator=1 ;;
		--*) ;;
		*) if [ -z "$input" ]; then input=$argument; else template=$argument; break; fi ;;
	esac
done

if [ -z "$input" ] || [ -z "$template" ]; then
	printf '%s\n' 'missing input or template' >&2
	exit 90
fi
if [ "$MSGMERGE_STDERR" = 1 ]; then
	printf 'fake msgmerge: %s\n' "$input" >&2
fi
if [ "$MSGMERGE_MODE" = fail-sv ]; then
	case "$input" in
		*/sv/*)
			printf '%s\n' 'simulated sv failure' >&2
			exit 23
			;;
	esac
fi

printf '%s\n' '# merged by fake msgmerge' >> "$input"
`

const fakeMsgattribScript = `#!/bin/sh
set -eu

printf '%s\n' BEGIN "$@" END >> "$MSGATTRIB_ARGS"

output=
input=
after_separator=
for argument in "$@"; do
	if [ -n "$after_separator" ]; then
		input=$argument
		break
	fi
	case "$argument" in
		--output-file=*) output=${argument#--output-file=} ;;
		--) after_separator=1 ;;
	esac
done

if [ -z "$output" ] || [ -z "$input" ]; then
	printf '%s\n' 'missing output or input' >&2
	exit 90
fi

: > "$output"
while IFS= read -r line || [ -n "$line" ]; do
	printf '%s\n' "$line" >> "$output"
done < "$input"
`

const validTranslationPO = `msgid ""
msgstr ""
"Project-Id-Version: old\n"
"PO-Revision-Date: old\n"
"Last-Translator: old\n"
"Language-Team: old\n"
"Language: sv\n"
"Content-Type: text/plain; charset=UTF-8\n"

msgid "existing"
msgstr "befintlig"
`

const oldTranslationPOT = "old template"

type translationUpdateFixture struct {
	rootDir   string
	localeDir string
	argsDir   string
	started   string
	stdout    bytes.Buffer
	stderr    bytes.Buffer
}

func newTranslationUpdateFixture(t *testing.T) *translationUpdateFixture {
	t.Helper()

	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	localeDir := filepath.Join(root, "locale")
	argsDir := filepath.Join(root, "args")

	check.NoError(t, os.MkdirAll(binDir, 0o750))
	check.NoError(t, os.MkdirAll(localeDir, 0o750))
	check.NoError(t, os.MkdirAll(argsDir, 0o750))
	check.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0o750))
	check.NoError(t, os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module codefloe.com/itiquette/gommitlint\n\ngo 1.26.6\n"),
		0o600,
	))
	check.NoError(t, os.WriteFile(
		filepath.Join(root, "messages.go"),
		[]byte("package fixture\n\nfunc message(localizer interface{ Get(string, ...any) string }) {\n\t_ = localizer.Get(\"fixture message\")\n}\n"),
		0o600,
	))
	check.NoError(t, os.WriteFile(filepath.Join(binDir, "msgen"), []byte(fakeMsgenScript), 0o700))
	check.NoError(t, os.WriteFile(filepath.Join(binDir, "msgmerge"), []byte(fakeMsgmergeScript), 0o700))
	check.NoError(t, os.WriteFile(filepath.Join(binDir, "msgattrib"), []byte(fakeMsgattribScript), 0o700))

	fixture := &translationUpdateFixture{
		rootDir:   root,
		localeDir: localeDir,
		argsDir:   argsDir,
		started:   filepath.Join(root, "msgen.started"),
	}

	t.Chdir(root)
	t.Setenv("LC_ALL", "C")
	t.Setenv("PATH", binDir)
	t.Setenv("MSGEN_ARGS", fixture.msgenArgs())
	t.Setenv("MSGEN_MODE", "success")
	t.Setenv("MSGEN_STARTED", fixture.started)
	t.Setenv("MSGEN_STDERR", "0")
	t.Setenv("MSGMERGE_ARGS", fixture.msgmergeArgs())
	t.Setenv("MSGMERGE_MODE", "success")
	t.Setenv("MSGMERGE_STDERR", "0")
	t.Setenv("MSGATTRIB_ARGS", fixture.msgattribArgs())

	return fixture
}

func (f *translationUpdateFixture) writeCatalog(t *testing.T, locale string) {
	t.Helper()

	localeDir := filepath.Join(f.localeDir, locale)
	check.NoError(t, os.MkdirAll(localeDir, 0o750))

	path := filepath.Join(localeDir, gommitlintKey+".po")
	check.NoError(t, os.WriteFile(path, []byte(validTranslationPO), 0o600))
}

func (f *translationUpdateFixture) writePOT(t *testing.T) {
	t.Helper()

	path := filepath.Join(f.localeDir, gommitlintKey+".pot")
	check.NoError(t, os.WriteFile(path, []byte(oldTranslationPOT), 0o600))
}

func (f *translationUpdateFixture) command(stderr io.Writer) *cli.Command {
	if stderr == nil {
		stderr = &f.stderr
	}

	return &cli.Command{
		Name:      gommitlintKey,
		Flags:     cliAdapter.NewGlobalFlags(nil, ""),
		Commands:  []*cli.Command{NewTranslationsCommand(nil)},
		Writer:    &f.stdout,
		ErrWriter: stderr,
	}
}

func (f *translationUpdateFixture) run(ctx context.Context, quiet bool, stderr io.Writer) error {
	args := []string{gommitlintKey}
	if quiet {
		args = append(args, "--quiet")
	}

	args = append(args, "translations", "update", "--locale-dir", f.localeDir)

	return f.command(stderr).Run(ctx, args)
}

func (f *translationUpdateFixture) potPath() string {
	return filepath.Join(f.localeDir, gommitlintKey+".pot")
}

func (f *translationUpdateFixture) poPath(locale string) string {
	return filepath.Join(f.localeDir, locale, gommitlintKey+".po")
}

func (f *translationUpdateFixture) msgenArgs() string {
	return filepath.Join(f.argsDir, "msgen.args")
}

func (f *translationUpdateFixture) msgmergeArgs() string {
	return filepath.Join(f.argsDir, "msgmerge.args")
}

func (f *translationUpdateFixture) msgattribArgs() string {
	return filepath.Join(f.argsDir, "msgattrib.args")
}

func TestExecuteTranslationsUpdate_RoutesOutputStagesCatalogsAndSeparatesInputs(t *testing.T) {
	fixture := newTranslationUpdateFixture(t)
	fixture.writePOT(t)
	fixture.writeCatalog(t, "en")
	fixture.writeCatalog(t, "sv")
	t.Setenv("MSGEN_STDERR", "1")
	t.Setenv("MSGMERGE_STDERR", "1")

	err := fixture.run(t.Context(), false, nil)

	check.NoError(t, err)
	check.Empty(t, fixture.stdout.String())
	check.Contains(t, fixture.stderr.String(), "fake msgen:")
	check.Contains(t, fixture.stderr.String(), "fake msgmerge:")
	check.Contains(t, fixture.stderr.String(), "Translation catalogs updated")

	pot, err := os.ReadFile(fixture.potPath())
	check.NoError(t, err)
	check.Contains(t, string(pot), `msgid "fixture message"`)

	enPO, err := os.ReadFile(fixture.poPath("en"))
	check.NoError(t, err)
	check.Contains(t, string(enPO), `"Language: en\n"`)

	svPO, err := os.ReadFile(fixture.poPath("sv"))
	check.NoError(t, err)
	check.Contains(t, string(svPO), "# merged by fake msgmerge")

	msgenArgs, err := os.ReadFile(fixture.msgenArgs())
	check.NoError(t, err)
	check.Contains(t, string(msgenArgs), "\n--\n")
	check.NotContains(t, string(msgenArgs), "--output="+fixture.poPath("en"))

	msgmergeArgs, err := os.ReadFile(fixture.msgmergeArgs())
	check.NoError(t, err)
	check.Contains(t, string(msgmergeArgs), "\n--\n")
	check.NotContains(t, string(msgmergeArgs), "\n--no-obsolete\n")
	check.NotContains(t, string(msgmergeArgs), "\n"+fixture.poPath("sv")+"\n")

	msgattribArgs, err := os.ReadFile(fixture.msgattribArgs())
	check.NoError(t, err)
	check.Contains(t, string(msgattribArgs), "\n--no-obsolete\n")
	check.Contains(t, string(msgattribArgs), "\n--\n")
	check.Empty(t, translationUpdateTemps(t, fixture.localeDir))
}

func TestExecuteTranslationsUpdate_QuietSuppressesSuccessNotice(t *testing.T) {
	fixture := newTranslationUpdateFixture(t)
	fixture.writeCatalog(t, "en")
	t.Setenv("MSGEN_STDERR", "1")

	err := fixture.run(t.Context(), true, nil)

	check.NoError(t, err)
	check.Contains(t, fixture.stderr.String(), "fake msgen:")
	check.NotContains(t, fixture.stderr.String(), "Translation catalogs updated")
}

func TestExecuteTranslationsUpdate_ReportsOutputFailures(t *testing.T) {
	t.Run("success_notice", func(t *testing.T) {
		fixture := newTranslationUpdateFixture(t)
		fixture.writeCatalog(t, "en")

		err := fixture.run(t.Context(), false, failingWriter{err: errMockWrite})

		check.ErrorIs(t, err, ErrOutputWrite)
		check.ErrorIs(t, err, errMockWrite)
	})

	t.Run("subprocess_stderr", func(t *testing.T) {
		fixture := newTranslationUpdateFixture(t)
		fixture.writePOT(t)
		fixture.writeCatalog(t, "en")
		t.Setenv("MSGEN_STDERR", "1")

		err := fixture.run(t.Context(), false, failingWriter{err: errMockWrite})

		check.ErrorIs(t, err, ErrOutputWrite)
		check.ErrorIs(t, err, errMockWrite)
		assertTranslationFileContent(t, fixture.potPath(), oldTranslationPOT)
		assertTranslationFileContent(t, fixture.poPath("en"), validTranslationPO)
		check.Empty(t, translationUpdateTemps(t, fixture.localeDir))
	})
}

func TestExecuteTranslationsUpdate_PreCancelledContextDoesNotMutateCatalogs(t *testing.T) {
	testCases := []struct {
		name string
		ctx  func() context.Context
		want error
	}{
		{
			name: "cancelled",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()

				return ctx
			},
			want: context.Canceled,
		},
		{
			name: "deadline_exceeded",
			ctx: func() context.Context {
				ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
				t.Cleanup(cancel)

				return ctx
			},
			want: context.DeadlineExceeded,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newTranslationUpdateFixture(t)
			fixture.writePOT(t)
			fixture.writeCatalog(t, "en")

			err := fixture.run(testCase.ctx(), false, nil)

			check.ErrorIs(t, err, testCase.want)
			assertTranslationFileContent(t, fixture.potPath(), oldTranslationPOT)
			assertTranslationFileContent(t, fixture.poPath("en"), validTranslationPO)
			assertTranslationToolNotRun(t, fixture.msgenArgs())
		})
	}
}

func TestExecuteTranslationsUpdate_MidCommandCancellationPreservesCatalogs(t *testing.T) {
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(strings.ReplaceAll(want.Error(), " ", "_"), func(t *testing.T) {
			fixture := newTranslationUpdateFixture(t)
			fixture.writePOT(t)
			fixture.writeCatalog(t, "en")
			fixture.writeCatalog(t, "sv")
			t.Setenv("MSGEN_MODE", "hang")

			ctx := &triggeredTranslationContext{done: make(chan struct{}), err: want}
			result := make(chan error, 1)

			go func() {
				result <- fixture.run(ctx, false, nil)
			}()

			waitForTranslationFile(t, fixture.started)
			close(ctx.done)

			select {
			case err := <-result:
				check.ErrorIs(t, err, want)
			case <-time.After(5 * time.Second):
				t.Fatal("translation update did not stop after context cancellation")
			}

			assertTranslationFileContent(t, fixture.potPath(), oldTranslationPOT)
			assertTranslationFileContent(t, fixture.poPath("en"), validTranslationPO)
			assertTranslationFileContent(t, fixture.poPath("sv"), validTranslationPO)
			check.Empty(t, translationUpdateTemps(t, fixture.localeDir))
		})
	}
}

func TestExecuteTranslationsUpdate_StagesAllCatalogsBeforeCommit(t *testing.T) {
	fixture := newTranslationUpdateFixture(t)
	fixture.writePOT(t)
	fixture.writeCatalog(t, "en")
	fixture.writeCatalog(t, "sv")
	t.Setenv("MSGMERGE_MODE", "fail-sv")

	err := fixture.run(t.Context(), false, nil)

	check.Error(t, err)
	assertTranslationRecovery(t, err, errTranslationUpdate)

	var exitErr *exec.ExitError
	check.ErrorAs(t, err, &exitErr)
	assertTranslationFileContent(t, fixture.potPath(), oldTranslationPOT)
	assertTranslationFileContent(t, fixture.poPath("en"), validTranslationPO)
	assertTranslationFileContent(t, fixture.poPath("sv"), validTranslationPO)
	check.Empty(t, translationUpdateTemps(t, fixture.localeDir))
}

func TestTranslationUpdateConfig_SecondReplacementFailureRollsBack(t *testing.T) {
	fixture := newTranslationUpdateFixture(t)
	fixture.writePOT(t)
	fixture.writeCatalog(t, "en")

	targets := map[string]struct {
		content string
		mode    os.FileMode
	}{
		fixture.potPath():    {content: oldTranslationPOT, mode: 0o640},
		fixture.poPath("en"): {content: validTranslationPO, mode: 0o604},
	}
	beforeModes := make(map[string]os.FileMode, len(targets))

	for path, target := range targets {
		check.NoError(t, os.Chmod(path, target.mode))

		info, err := os.Lstat(path)
		check.NoError(t, err)

		beforeModes[path] = info.Mode()
	}

	failed := false
	cfg := translationUpdateConfig{
		rootDir:   fixture.rootDir,
		localeDir: fixture.localeDir,
		domain:    gommitlintKey,
		potFile:   fixture.potPath(),
		renameFile: func(oldPath, newPath string) error {
			if !failed && newPath == fixture.poPath("en") {
				failed = true

				return errUpdateSecondReplacement
			}

			return os.Rename(oldPath, newPath)
		},
	}

	updates, err := cfg.findCatalogUpdates(t.Context())
	check.NoError(t, err)
	check.NoError(t, cfg.stageCatalogUpdates(t.Context(), updates, &fixture.stderr, nil))

	err = commitTranslationUpdates(t.Context(), updates, cfg.renameFile)
	removeStagedTranslationUpdates(updates)

	check.ErrorIs(t, err, errUpdateSecondReplacement)
	check.True(t, failed, "the injected second replacement must be reached")

	for path, target := range targets {
		assertTranslationFileContent(t, path, target.content)

		info, statErr := os.Lstat(path)
		check.NoError(t, statErr)
		check.Equal(t, beforeModes[path], info.Mode())
	}

	check.Empty(t, translationUpdateTemps(t, fixture.localeDir))
}

func TestExecuteTranslationsUpdate_RejectsRedirectedAndNonregularManagedPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available to unprivileged Windows tests")
	}

	testCases := []struct {
		name  string
		setup func(*testing.T, *translationUpdateFixture) string
	}{
		{
			name: "locale_directory_symlink",
			setup: func(t *testing.T, fixture *translationUpdateFixture) string {
				t.Helper()

				outside := t.TempDir()
				check.NoError(t, os.Remove(fixture.localeDir))
				check.NoError(t, os.Symlink(outside, fixture.localeDir))

				return ""
			},
		},
		{
			name: "pot_symlink",
			setup: func(t *testing.T, fixture *translationUpdateFixture) string {
				t.Helper()

				outside := filepath.Join(t.TempDir(), "outside.pot")
				check.NoError(t, os.WriteFile(outside, []byte("outside"), 0o600))
				check.NoError(t, os.Symlink(outside, fixture.potPath()))

				return outside
			},
		},
		{
			name: "locale_entry_symlink",
			setup: func(t *testing.T, fixture *translationUpdateFixture) string {
				t.Helper()

				outside := t.TempDir()
				outsidePO := filepath.Join(outside, gommitlintKey+".po")
				check.NoError(t, os.WriteFile(outsidePO, []byte("outside"), 0o600))
				check.NoError(t, os.Symlink(outside, filepath.Join(fixture.localeDir, "sv")))

				return outsidePO
			},
		},
		{
			name: "po_symlink",
			setup: func(t *testing.T, fixture *translationUpdateFixture) string {
				t.Helper()

				outside := filepath.Join(t.TempDir(), "outside.po")
				check.NoError(t, os.WriteFile(outside, []byte("outside"), 0o600))
				check.NoError(t, os.Mkdir(filepath.Join(fixture.localeDir, "sv"), 0o750))
				check.NoError(t, os.Symlink(outside, fixture.poPath("sv")))

				return outside
			},
		},
		{
			name: "po_directory",
			setup: func(t *testing.T, fixture *translationUpdateFixture) string {
				t.Helper()

				check.NoError(t, os.MkdirAll(fixture.poPath("sv"), 0o750))

				return ""
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newTranslationUpdateFixture(t)
			managedPath := testCase.setup(t, fixture)

			var before []byte

			if managedPath != "" {
				var readErr error

				before, readErr = os.ReadFile(managedPath)
				check.NoError(t, readErr)
			}

			err := fixture.run(t.Context(), false, nil)

			check.Error(t, err)

			var recoveryErr RecoveryError
			check.ErrorAs(t, err, &recoveryErr)
			check.True(
				t,
				errors.Is(err, errTranslationUpdate) || errors.Is(err, errLocaleDirNotFound),
				"managed-path failures must retain an update or locale sentinel",
			)

			if managedPath != "" {
				after, afterErr := os.ReadFile(managedPath)
				check.NoError(t, afterErr)
				check.Equal(t, string(before), string(after))
			}

			assertTranslationToolNotRun(t, fixture.msgenArgs())
			assertTranslationToolNotRun(t, fixture.msgmergeArgs())
		})
	}
}

func TestExecuteTranslationsUpdate_ExpectedFailuresAreRecoveryErrors(t *testing.T) {
	t.Run("missing_dependency", func(t *testing.T) {
		fixture := newTranslationUpdateFixture(t)
		t.Setenv("PATH", t.TempDir())

		err := fixture.run(t.Context(), false, nil)

		assertTranslationRecovery(t, err, errMissingTranslationDependency)
		check.ErrorIs(t, err, exec.ErrNotFound)
	})

	t.Run("missing_locale_directory", func(t *testing.T) {
		fixture := newTranslationUpdateFixture(t)
		check.NoError(t, os.Remove(fixture.localeDir))

		err := fixture.run(t.Context(), false, nil)

		assertTranslationRecovery(t, err, errLocaleDirNotFound)
		check.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("msgen_failure", func(t *testing.T) {
		fixture := newTranslationUpdateFixture(t)
		fixture.writeCatalog(t, "en")
		t.Setenv("MSGEN_MODE", "fail")

		err := fixture.run(t.Context(), false, nil)

		assertTranslationRecovery(t, err, errTranslationUpdate)

		var exitErr *exec.ExitError
		check.ErrorAs(t, err, &exitErr)
	})
}

func TestPatchPOHeader_RejectsMalformedStructureWithoutMutation(t *testing.T) {
	testCases := []struct {
		name    string
		content string
		want    error
	}{
		{name: "missing_msgid", content: "msgstr \"\"\n", want: errNoMsgIDHeader},
		{name: "missing_msgstr", content: "msgid \"\"\n", want: errMalformedPOHeader},
		{name: "wrong_msgstr", content: "msgid \"\"\nmsgstr \"value\"\n", want: errMalformedPOHeader},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "catalog.po")
			check.NoError(t, os.WriteFile(path, []byte(testCase.content), 0o600))

			err := patchPOHeader(path, "sv")

			check.ErrorIs(t, err, testCase.want)
			assertTranslationFileContent(t, path, testCase.content)
			check.Empty(t, translationUpdateTemps(t, filepath.Dir(path)))
		})
	}
}

func TestPatchPOHeader_AtomicallyReplacesRegularCatalog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.po")
	check.NoError(t, os.WriteFile(path, []byte(validTranslationPO), 0o600))

	err := patchPOHeader(path, "de")

	check.NoError(t, err)
	content, err := os.ReadFile(path)
	check.NoError(t, err)
	check.Contains(t, string(content), `"Project-Id-Version: gommitlint\n"`)
	check.Contains(t, string(content), `"Language: de\n"`)
	check.NotContains(t, string(content), `"Language: sv\n"`)
	check.Empty(t, translationUpdateTemps(t, dir))
}

func TestRepoRoot_RequiresGommitlintModuleMarker(t *testing.T) {
	t.Run("rejects_arbitrary_git_checkout", func(t *testing.T) {
		root := t.TempDir()
		check.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0o750))
		check.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/other\n"), 0o600))
		t.Chdir(root)

		found, err := repoRoot()

		check.ErrorIs(t, err, errGommitlintModuleNotFound)
		check.Empty(t, found)
	})

	t.Run("finds_marker_from_nested_directory", func(t *testing.T) {
		root := t.TempDir()
		nested := filepath.Join(root, "one", "two")
		check.NoError(t, os.MkdirAll(nested, 0o750))
		check.NoError(t, os.WriteFile(
			filepath.Join(root, "go.mod"),
			[]byte("module codefloe.com/itiquette/gommitlint\n"),
			0o600,
		))
		t.Chdir(nested)

		found, err := repoRoot()

		check.NoError(t, err)
		check.Equal(t, root, found)
	})
}

func assertTranslationFileContent(t *testing.T, path, want string) {
	t.Helper()

	content, err := os.ReadFile(path)
	check.NoError(t, err)
	check.Equal(t, want, string(content))
}

func assertTranslationToolNotRun(t *testing.T, logPath string) {
	t.Helper()

	_, err := os.Stat(logPath)
	check.ErrorIs(t, err, os.ErrNotExist)
}

func translationUpdateTemps(t *testing.T, root string) []string {
	t.Helper()

	var paths []string

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}

			return walkErr
		}

		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "."+gommitlintKey+".") {
			paths = append(paths, path)
		}

		return nil
	})
	check.NoError(t, err)

	return paths
}
