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

var (
	errCompileSecondReplacement = errors.New("injected second replacement failure")
	errCompileReplacement       = errors.New("injected replacement failure")
	errCompileRollback          = errors.New("injected rollback failure")
)

const fakeMsgfmtScript = `#!/bin/sh
set -eu

printf '%s\n' BEGIN "$@" END >> "$MSGFMT_ARGS"

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
	printf '%s\n' 'missing output or -- separator' >&2
	exit 90
fi
if [ "$MSGFMT_STDERR" = 1 ]; then
	printf 'fake msgfmt: %s\n' "$input" >&2
fi
if [ "$MSGFMT_MODE" = hang ]; then
	: > "$MSGFMT_STARTED"
	while :; do :; done
fi
if [ "$MSGFMT_MODE" = fail-sv ]; then
	case "$input" in
		*/sv/gommitlint.po)
			printf '%s\n' 'simulated sv failure' >&2
			exit 23
			;;
	esac
fi

printf 'compiled %s\n' "$input" > "$output"
`

type translationCompileFixture struct {
	sourceDir string
	outputDir string
	argsLog   string
	started   string
	stdout    bytes.Buffer
	stderr    bytes.Buffer
}

func newTranslationCompileFixture(t *testing.T) *translationCompileFixture {
	t.Helper()

	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	sourceDir := filepath.Join(root, "locale")

	check.NoError(t, os.MkdirAll(binDir, 0o750))
	check.NoError(t, os.MkdirAll(sourceDir, 0o750))
	check.NoError(t, os.WriteFile(filepath.Join(binDir, "msgfmt"), []byte(fakeMsgfmtScript), 0o700))

	fixture := &translationCompileFixture{
		sourceDir: sourceDir,
		outputDir: filepath.Join(root, "generated", "locale"),
		argsLog:   filepath.Join(root, "msgfmt.args"),
		started:   filepath.Join(root, "msgfmt.started"),
	}

	t.Setenv("LC_ALL", "C")
	t.Setenv("PATH", binDir)
	t.Setenv("MSGFMT_ARGS", fixture.argsLog)
	t.Setenv("MSGFMT_STARTED", fixture.started)
	t.Setenv("MSGFMT_MODE", "success")
	t.Setenv("MSGFMT_STDERR", "0")

	return fixture
}

func (f *translationCompileFixture) writeCatalog(t *testing.T, locale string) string {
	t.Helper()

	localeDir := filepath.Join(f.sourceDir, locale)
	check.NoError(t, os.MkdirAll(localeDir, 0o750))

	path := filepath.Join(localeDir, gommitlintKey+".po")
	check.NoError(t, os.WriteFile(path, []byte("test catalog"), 0o600))

	return path
}

func (f *translationCompileFixture) command(stderr io.Writer) *cli.Command {
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

func (f *translationCompileFixture) run(ctx context.Context, quiet bool, stderr io.Writer) error {
	args := []string{gommitlintKey}
	if quiet {
		args = append(args, "--quiet")
	}

	args = append(args,
		"translations", "compile",
		"--source-dir", f.sourceDir,
		"--output-dir", f.outputDir,
	)

	return f.command(stderr).Run(ctx, args)
}

func (f *translationCompileFixture) outputPath(locale string) string {
	return filepath.Join(f.outputDir, locale, "LC_MESSAGES", gommitlintKey+".mo")
}

func TestExecuteTranslationsCompile_RoutesOutputAndSeparatesInput(t *testing.T) {
	fixture := newTranslationCompileFixture(t)
	poFile := fixture.writeCatalog(t, "en")
	t.Setenv("MSGFMT_STDERR", "1")

	err := fixture.run(t.Context(), false, nil)

	check.NoError(t, err)
	check.Empty(t, fixture.stdout.String())
	check.Contains(t, fixture.stderr.String(), "fake msgfmt: "+poFile)

	args, err := os.ReadFile(fixture.argsLog)
	check.NoError(t, err)
	check.Contains(t, string(args), "\n--\n"+poFile+"\nEND\n")

	compiled, err := os.ReadFile(fixture.outputPath("en"))
	check.NoError(t, err)
	check.Equal(t, "compiled "+poFile+"\n", string(compiled))
	check.Empty(t, translationCompileTemps(t, fixture.outputDir))
}

func TestExecuteTranslationsCompile_SuccessNoticeUsesSingular(t *testing.T) {
	fixture := newTranslationCompileFixture(t)
	fixture.writeCatalog(t, "en")

	err := fixture.run(t.Context(), false, nil)

	check.NoError(t, err)
	check.Equal(t, "Compiled 1 translation catalog\n", fixture.stderr.String())
}

func TestExecuteTranslationsCompile_SuccessNoticeUsesPlural(t *testing.T) {
	fixture := newTranslationCompileFixture(t)
	fixture.writeCatalog(t, "en")
	fixture.writeCatalog(t, "sv")

	err := fixture.run(t.Context(), false, nil)

	check.NoError(t, err)
	check.Equal(t, "Compiled 2 translation catalogs\n", fixture.stderr.String())
}

func TestExecuteTranslationsCompile_QuietSuppressesSuccessNotice(t *testing.T) {
	fixture := newTranslationCompileFixture(t)
	poFile := fixture.writeCatalog(t, "en")
	t.Setenv("MSGFMT_STDERR", "1")

	err := fixture.run(t.Context(), true, nil)

	check.NoError(t, err)
	check.Equal(t, "fake msgfmt: "+poFile+"\n", fixture.stderr.String())
}

func TestExecuteTranslationsCompile_ReportsOutputFailures(t *testing.T) {
	t.Run("success_notice", func(t *testing.T) {
		fixture := newTranslationCompileFixture(t)
		fixture.writeCatalog(t, "en")

		err := fixture.run(t.Context(), false, failingWriter{err: errMockWrite})

		check.ErrorIs(t, err, ErrOutputWrite)
		check.ErrorIs(t, err, errMockWrite)
	})

	t.Run("subprocess_stderr", func(t *testing.T) {
		fixture := newTranslationCompileFixture(t)
		fixture.writeCatalog(t, "en")
		t.Setenv("MSGFMT_STDERR", "1")

		err := fixture.run(t.Context(), false, failingWriter{err: errMockWrite})

		check.ErrorIs(t, err, ErrOutputWrite)
		check.ErrorIs(t, err, errMockWrite)

		_, statErr := os.Stat(fixture.outputPath("en"))
		check.ErrorIs(t, statErr, os.ErrNotExist)
		check.Empty(t, translationCompileTemps(t, fixture.outputDir))
	})
}

func TestExecuteTranslationsCompile_PreCancelledContextDoesNotMutateOutput(t *testing.T) {
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
			fixture := newTranslationCompileFixture(t)
			fixture.writeCatalog(t, "en")

			err := fixture.run(testCase.ctx(), false, nil)

			check.ErrorIs(t, err, testCase.want)

			_, statErr := os.Lstat(fixture.outputDir)
			check.ErrorIs(t, statErr, os.ErrNotExist)
			_, statErr = os.Stat(fixture.argsLog)
			check.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

type triggeredTranslationContext struct {
	done chan struct{}
	err  error
}

func (*triggeredTranslationContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (c *triggeredTranslationContext) Done() <-chan struct{} {
	return c.done
}

func (c *triggeredTranslationContext) Err() error {
	select {
	case <-c.done:
		return c.err
	default:
		return nil
	}
}

func (*triggeredTranslationContext) Value(any) any {
	return nil
}

func TestExecuteTranslationsCompile_MidCommandContextCleansTemporaryOutput(t *testing.T) {
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(strings.ReplaceAll(want.Error(), " ", "_"), func(t *testing.T) {
			fixture := newTranslationCompileFixture(t)
			fixture.writeCatalog(t, "en")
			finalPath := fixture.outputPath("en")
			check.NoError(t, os.MkdirAll(filepath.Dir(finalPath), 0o750))
			check.NoError(t, os.WriteFile(finalPath, []byte("old catalog"), 0o600))
			t.Setenv("MSGFMT_MODE", "hang")

			ctx := &triggeredTranslationContext{
				done: make(chan struct{}),
				err:  want,
			}
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
				t.Fatal("translation compile did not stop after context cancellation")
			}

			content, err := os.ReadFile(finalPath)
			check.NoError(t, err)
			check.Equal(t, "old catalog", string(content))
			check.Empty(t, translationCompileTemps(t, fixture.outputDir))
		})
	}
}

func TestExecuteTranslationsCompile_StagesAllCatalogsBeforeCommit(t *testing.T) {
	fixture := newTranslationCompileFixture(t)
	fixture.writeCatalog(t, "en")
	fixture.writeCatalog(t, "sv")
	t.Setenv("MSGFMT_MODE", "fail-sv")

	for _, locale := range []string{"en", "sv"} {
		path := fixture.outputPath(locale)
		check.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		check.NoError(t, os.WriteFile(path, []byte("old "+locale), 0o600))
	}

	err := fixture.run(t.Context(), false, nil)

	check.Error(t, err)

	for _, locale := range []string{"en", "sv"} {
		content, readErr := os.ReadFile(fixture.outputPath(locale))
		check.NoError(t, readErr)
		check.Equal(t, "old "+locale, string(content))
	}

	check.Empty(t, translationCompileTemps(t, fixture.outputDir))
}

func TestTranslationCompileConfig_SecondReplacementFailureRollsBack(t *testing.T) {
	fixture := newTranslationCompileFixture(t)
	fixture.writeCatalog(t, "en")
	fixture.writeCatalog(t, "sv")

	modes := map[string]os.FileMode{"en": 0o600, "sv": 0o640}
	beforeModes := make(map[string]os.FileMode, len(modes))

	for locale, mode := range modes {
		path := fixture.outputPath(locale)
		check.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		check.NoError(t, os.WriteFile(path, []byte("old "+locale), mode))
		check.NoError(t, os.Chmod(path, mode))

		info, err := os.Lstat(path)
		check.NoError(t, err)

		beforeModes[locale] = info.Mode()
	}

	failed := false
	cfg := translationCompileConfig{
		sourceDir: fixture.sourceDir,
		outputDir: fixture.outputDir,
		domain:    gommitlintKey,
		renameFile: func(oldPath, newPath string) error {
			if !failed && newPath == fixture.outputPath("sv") {
				failed = true

				return errCompileSecondReplacement
			}

			return os.Rename(oldPath, newPath)
		},
	}

	found, err := cfg.compileCatalogs(t.Context(), &fixture.stderr, nil)

	check.ErrorIs(t, err, errCompileSecondReplacement)
	check.Equal(t, 0, found)
	check.True(t, failed, "the injected second replacement must be reached")

	for _, locale := range []string{"en", "sv"} {
		path := fixture.outputPath(locale)
		content, readErr := os.ReadFile(path)
		check.NoError(t, readErr)
		check.Equal(t, "old "+locale, string(content))

		info, statErr := os.Lstat(path)
		check.NoError(t, statErr)
		check.Equal(t, beforeModes[locale], info.Mode())
	}

	check.Empty(t, translationCompileTemps(t, fixture.outputDir))
}

func TestCommitTranslationCatalogs_SurfacesRollbackFailure(t *testing.T) {
	dir := t.TempDir()
	catalogs := []translationCatalog{
		{target: filepath.Join(dir, "first.mo"), temp: filepath.Join(dir, "first.tmp")},
		{target: filepath.Join(dir, "second.mo"), temp: filepath.Join(dir, "second.tmp")},
	}

	for _, catalog := range catalogs {
		check.NoError(t, os.WriteFile(catalog.target, []byte("old"), 0o600))
		check.NoError(t, os.WriteFile(catalog.temp, []byte("new"), 0o600))
	}

	replacementFailed := false

	err := commitTranslationCatalogs(t.Context(), catalogs, func(oldPath, newPath string) error {
		if oldPath == catalogs[1].temp && newPath == catalogs[1].target {
			replacementFailed = true

			return errCompileReplacement
		}

		if replacementFailed && newPath == catalogs[0].target {
			return errCompileRollback
		}

		return os.Rename(oldPath, newPath)
	})

	check.ErrorIs(t, err, errCompileReplacement)
	check.ErrorIs(t, err, errCompileRollback)
}

func TestExecuteTranslationsCompile_RequiresRealCatalogSources(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available to unprivileged Windows tests")
	}

	t.Run("locale_symlink", func(t *testing.T) {
		fixture := newTranslationCompileFixture(t)
		outside := t.TempDir()
		check.NoError(t, os.WriteFile(filepath.Join(outside, gommitlintKey+".po"), []byte("outside"), 0o600))
		check.NoError(t, os.Symlink(outside, filepath.Join(fixture.sourceDir, "en")))

		err := fixture.run(t.Context(), false, nil)

		check.ErrorContains(t, err, "symlink")
		assertFakeMsgfmtNotRun(t, fixture.argsLog)
	})

	t.Run("catalog_symlink", func(t *testing.T) {
		fixture := newTranslationCompileFixture(t)
		outside := filepath.Join(t.TempDir(), "outside.po")
		check.NoError(t, os.WriteFile(outside, []byte("outside"), 0o600))

		localeDir := filepath.Join(fixture.sourceDir, "en")
		check.NoError(t, os.Mkdir(localeDir, 0o750))
		check.NoError(t, os.Symlink(outside, filepath.Join(localeDir, gommitlintKey+".po")))

		err := fixture.run(t.Context(), false, nil)

		check.ErrorContains(t, err, "regular file")
		assertFakeMsgfmtNotRun(t, fixture.argsLog)
	})

	t.Run("catalog_directory", func(t *testing.T) {
		fixture := newTranslationCompileFixture(t)
		path := filepath.Join(fixture.sourceDir, "en", gommitlintKey+".po")
		check.NoError(t, os.MkdirAll(path, 0o750))

		err := fixture.run(t.Context(), false, nil)

		check.ErrorContains(t, err, "regular file")
		assertFakeMsgfmtNotRun(t, fixture.argsLog)
	})
}

func TestExecuteTranslationsCompile_RejectsSymlinkedOutputComponents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available to unprivileged Windows tests")
	}

	for _, component := range []string{"output_dir", "locale", "messages", "catalog"} {
		t.Run(component, func(t *testing.T) {
			fixture := newTranslationCompileFixture(t)
			fixture.writeCatalog(t, "en")
			outsideDir := t.TempDir()
			outsideFile := filepath.Join(outsideDir, "outside.mo")
			check.NoError(t, os.WriteFile(outsideFile, []byte("outside"), 0o600))

			switch component {
			case "output_dir":
				check.NoError(t, os.MkdirAll(filepath.Dir(fixture.outputDir), 0o750))
				check.NoError(t, os.Symlink(outsideDir, fixture.outputDir))
			case "locale":
				check.NoError(t, os.MkdirAll(fixture.outputDir, 0o750))
				check.NoError(t, os.Symlink(outsideDir, filepath.Join(fixture.outputDir, "en")))
			case "messages":
				check.NoError(t, os.MkdirAll(filepath.Join(fixture.outputDir, "en"), 0o750))
				check.NoError(t, os.Symlink(outsideDir, filepath.Join(fixture.outputDir, "en", "LC_MESSAGES")))
			case "catalog":
				path := fixture.outputPath("en")
				check.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
				check.NoError(t, os.Symlink(outsideFile, path))
			}

			err := fixture.run(t.Context(), false, nil)

			check.ErrorContains(t, err, "symlink")

			content, readErr := os.ReadFile(outsideFile)
			check.NoError(t, readErr)
			check.Equal(t, "outside", string(content))
			assertFakeMsgfmtNotRun(t, fixture.argsLog)
		})
	}
}

func TestExecuteTranslationsCompile_ExpectedFailuresAreRecoveryErrors(t *testing.T) {
	t.Run("missing_dependency", func(t *testing.T) {
		fixture := newTranslationCompileFixture(t)
		fixture.writeCatalog(t, "en")
		t.Setenv("PATH", t.TempDir())

		err := fixture.run(t.Context(), false, nil)

		assertTranslationRecovery(t, err, errMissingTranslationDependency)
		check.ErrorIs(t, err, exec.ErrNotFound)
	})

	t.Run("missing_source", func(t *testing.T) {
		fixture := newTranslationCompileFixture(t)
		fixture.sourceDir = filepath.Join(t.TempDir(), "missing")

		err := fixture.run(t.Context(), false, nil)

		assertTranslationRecovery(t, err, errSourceDirNotFound)
		check.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("no_catalogs", func(t *testing.T) {
		fixture := newTranslationCompileFixture(t)

		err := fixture.run(t.Context(), false, nil)

		assertTranslationRecovery(t, err, errNoTranslationSources)

		_, statErr := os.Lstat(fixture.outputDir)
		check.ErrorIs(t, statErr, os.ErrNotExist)
	})

	t.Run("msgfmt_failure", func(t *testing.T) {
		fixture := newTranslationCompileFixture(t)
		fixture.writeCatalog(t, "sv")
		t.Setenv("MSGFMT_MODE", "fail-sv")

		err := fixture.run(t.Context(), false, nil)

		assertTranslationRecovery(t, err, errTranslationCompile)

		var exitErr *exec.ExitError
		check.ErrorAs(t, err, &exitErr)
		check.Empty(t, translationCompileTemps(t, fixture.outputDir))
	})
}

func assertTranslationRecovery(t *testing.T, err error, sentinel error) {
	t.Helper()

	check.ErrorIs(t, err, sentinel)

	var recoveryErr RecoveryError
	check.ErrorAs(t, err, &recoveryErr)
	check.NotEmpty(t, recoveryErr.Suggestions)
}

func assertFakeMsgfmtNotRun(t *testing.T, argsLog string) {
	t.Helper()

	_, err := os.Stat(argsLog)
	check.ErrorIs(t, err, os.ErrNotExist)
}

func waitForTranslationFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for {
		_, err := os.Stat(path)
		if err == nil {
			return
		}

		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("wait for %s: %v", path, err)
		}

		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func translationCompileTemps(t *testing.T, root string) []string {
	t.Helper()

	var paths []string

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}

			return walkErr
		}

		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "."+gommitlintKey+".mo-") {
			paths = append(paths, path)
		}

		return nil
	})
	check.NoError(t, err)

	return paths
}
