// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"github.com/urfave/cli/v3"
)

const commandNameTranslationsCompile = "compile"

// NewTranslationsCompileCommand wraps the msgfmt loop.
//
// One `msgfmt --check` per locale `.po` produces a `.mo` under
// `generated/locale/<lang>/LC_MESSAGES/` — the canonical POSIX layout
// the localizer expects at runtime when shipping `.mo` files alongside
// the binary in distro packages.
func NewTranslationsCompileCommand(localizer output.Localizer) *cli.Command {
	localizer = output.EnsureLocalizer(localizer)

	return &cli.Command{
		Name:  commandNameTranslationsCompile,
		Usage: localizer.Get("compile .po catalogs to binary .mo files"),
		Description: localizer.Get(`Compiles each locale/<lang>/gommitlint.po to a binary
generated/locale/<lang>/LC_MESSAGES/gommitlint.mo via msgfmt --check.
The --check flag fails on syntax errors and on incomplete plural
forms — caught at build time, never at runtime.

Run-host requirement: msgfmt (GNU gettext-tools).`),

		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "source-dir",
				Usage: localizer.Get("source directory holding locale/<lang>/<domain>.po (default: ./locale)"),
			},
			&cli.StringFlag{
				Name:  "output-dir",
				Usage: localizer.Get("output directory for compiled .mo catalogs (default: ./generated/locale)"),
			},
			&cli.StringFlag{
				Name:  "domain",
				Value: gommitlintKey,
				Usage: localizer.Get("PO domain name (also the PO/MO filename stem)"),
			},
		},

		Action: ExecuteTranslationsCompile,
	}
}

// ExecuteTranslationsCompile runs msgfmt across every locale .po found.
func ExecuteTranslationsCompile(ctx context.Context, cmd *cli.Command) error {
	if err := rejectUnexpectedTranslationArgs(cmd, "gommitlint translations compile"); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	domain := cmd.String("domain")
	if err := validateTranslationDomain(domain, commandLocalizer); err != nil {
		return err
	}

	cfg, err := translationCompileConfigFromCommand(cmd, domain)
	if err != nil {
		return err
	}

	localizer := commandLocalizer()
	if err := cfg.checkPrerequisites(ctx, localizer); err != nil {
		return err
	}

	found, err := cfg.compileCatalogs(ctx, cmd.ErrWriter, localizer)
	if err != nil {
		return err
	}

	if found == 0 {
		return noTranslationSourcesError(cfg.sourceDir, cfg.domain, localizer)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if cmd.Root().Bool(flags.Quiet) {
		return nil
	}

	writeErr := writeCommandOutput(cmd.ErrWriter, localizer.GetN(
		"Compiled %d translation catalog\n",
		"Compiled %d translation catalogs\n",
		found,
		found,
	), localizer)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return errors.Join(writeErr, ctxErr)
	}

	return writeErr
}

type translationCompileConfig struct {
	sourceDir  string
	outputDir  string
	domain     string
	renameFile func(string, string) error
}

func translationCompileConfigFromCommand(cmd *cli.Command, domain string) (translationCompileConfig, error) {
	rootDir, err := repoRoot()
	if err != nil {
		return translationCompileConfig{}, fmt.Errorf("locate repo root: %w", err)
	}

	sourceDir := cmd.String("source-dir")
	if sourceDir == "" {
		sourceDir = filepath.Join(rootDir, "locale")
	}

	outputDir := cmd.String("output-dir")
	if outputDir == "" {
		outputDir = filepath.Join(rootDir, "generated", "locale")
	}

	return translationCompileConfig{sourceDir: sourceDir, outputDir: outputDir, domain: domain}, nil
}

func (c translationCompileConfig) checkPrerequisites(ctx context.Context, localizer output.Localizer) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if _, err := exec.LookPath("msgfmt"); err != nil {
		return missingTranslationDependencyError("msgfmt", err, localizer)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := requireRealTranslationDir(c.sourceDir); err != nil {
		return translationSourceDirError(c.sourceDir, err, localizer)
	}

	return nil
}

type translationCatalog struct {
	source string
	target string
	temp   string
}

func (c translationCompileConfig) compileCatalogs(
	ctx context.Context,
	stderr io.Writer,
	localizer output.Localizer,
) (int, error) {
	catalogs, err := c.findCatalogs(ctx)
	if err != nil {
		return 0, err
	}

	if err := ctx.Err(); err != nil {
		return 0, err
	}

	if len(catalogs) == 0 {
		return 0, nil
	}

	defer func() {
		for _, catalog := range catalogs {
			if catalog.temp != "" {
				_ = os.Remove(catalog.temp)
			}
		}
	}()

	if err := stageTranslationCatalogs(ctx, catalogs, stderr, localizer); err != nil {
		return 0, err
	}

	if err := commitTranslationCatalogs(ctx, catalogs, c.renameFile); err != nil {
		return 0, err
	}

	return len(catalogs), nil
}

func stageTranslationCatalogs(
	ctx context.Context,
	catalogs []translationCatalog,
	stderr io.Writer,
	localizer output.Localizer,
) error {
	for index := range catalogs {
		if err := ctx.Err(); err != nil {
			return err
		}

		temp, err := stageTranslationCatalog(ctx, catalogs[index], stderr, localizer)
		if err != nil {
			return err
		}

		catalogs[index].temp = temp
	}

	return ctx.Err()
}

func commitTranslationCatalogs(
	ctx context.Context,
	catalogs []translationCatalog,
	renameFile func(string, string) error,
) error {
	return commitTranslationCatalogBatch(ctx, catalogs, renameFile)
}

type translationCatalogSnapshot struct {
	target  string
	backup  string
	existed bool
}

// A batch can span directories, so it cannot be crash-atomic. Snapshots provide
// command-return atomicity by rolling back ordinary failures before return.
func commitTranslationCatalogBatch(
	ctx context.Context,
	catalogs []translationCatalog,
	renameFile func(string, string) error,
) error {
	if renameFile == nil {
		renameFile = os.Rename
	}

	for _, catalog := range catalogs {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := validateTranslationCatalogCommit(catalog); err != nil {
			return err
		}
	}

	snapshots, err := snapshotTranslationCatalogs(catalogs)
	if err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		removeTranslationCatalogBackups(snapshots)

		return err
	}

	replaced := 0

	for index := range catalogs {
		if err := ctx.Err(); err != nil {
			return rollbackTranslationCatalogBatch(err, snapshots, replaced, renameFile)
		}

		if err := replaceTranslationCatalog(catalogs[index], renameFile); err != nil {
			return rollbackTranslationCatalogBatch(err, snapshots, replaced, renameFile)
		}

		catalogs[index].temp = ""
		replaced++
	}

	if err := ctx.Err(); err != nil {
		return rollbackTranslationCatalogBatch(err, snapshots, replaced, renameFile)
	}

	removeTranslationCatalogBackups(snapshots)

	return nil
}

func snapshotTranslationCatalogs(catalogs []translationCatalog) ([]translationCatalogSnapshot, error) {
	snapshots := make([]translationCatalogSnapshot, 0, len(catalogs))

	for _, catalog := range catalogs {
		info, err := os.Lstat(catalog.target)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				snapshots = append(snapshots, translationCatalogSnapshot{target: catalog.target})

				continue
			}

			removeTranslationCatalogBackups(snapshots)

			return nil, fmt.Errorf("snapshot translation catalog %s: %w", catalog.target, err)
		}

		backup, err := copyTranslationToTemp(catalog.target, catalog.target)
		if err != nil {
			removeTranslationCatalogBackups(snapshots)

			return nil, fmt.Errorf("snapshot translation catalog %s: %w", catalog.target, err)
		}

		if err := os.Chmod(backup, info.Mode()); err != nil {
			_ = os.Remove(backup)

			removeTranslationCatalogBackups(snapshots)

			return nil, fmt.Errorf("preserve translation catalog mode %s: %w", catalog.target, err)
		}

		snapshots = append(snapshots, translationCatalogSnapshot{
			target:  catalog.target,
			backup:  backup,
			existed: true,
		})
	}

	return snapshots, nil
}

func rollbackTranslationCatalogBatch(
	cause error,
	snapshots []translationCatalogSnapshot,
	replaced int,
	renameFile func(string, string) error,
) error {
	rollbackErr := rollbackTranslationCatalogs(snapshots[:replaced], renameFile)
	removeTranslationCatalogBackups(snapshots[replaced:])

	return errors.Join(cause, rollbackErr)
}

func rollbackTranslationCatalogs(
	snapshots []translationCatalogSnapshot,
	renameFile func(string, string) error,
) error {
	var rollbackErr error

	for _, snapshot := range slices.Backward(snapshots) {
		if err := os.Remove(snapshot.target); err != nil && !errors.Is(err, os.ErrNotExist) {
			rollbackErr = errors.Join(
				rollbackErr,
				fmt.Errorf("remove replacement translation catalog %s: %w", snapshot.target, err),
			)
		}

		if !snapshot.existed {
			continue
		}

		if err := renameFile(snapshot.backup, snapshot.target); err != nil {
			rollbackErr = errors.Join(
				rollbackErr,
				fmt.Errorf("restore translation catalog %s from %s: %w", snapshot.target, snapshot.backup, err),
			)
		}
	}

	return rollbackErr
}

func removeTranslationCatalogBackups(snapshots []translationCatalogSnapshot) {
	for _, snapshot := range snapshots {
		if snapshot.backup != "" {
			_ = os.Remove(snapshot.backup)
		}
	}
}

func (c translationCompileConfig) findCatalogs(ctx context.Context) ([]translationCatalog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(c.sourceDir)
	if err != nil {
		return nil, fmt.Errorf("read source dir: %w", err)
	}

	catalogs := make([]translationCatalog, 0, len(entries))

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("inspect locale entry %s: %w", entry.Name(), err)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return nil, translationSymlinkError(filepath.Join(c.sourceDir, entry.Name()))
		}

		if !info.IsDir() {
			continue
		}

		locale := entry.Name()
		localeDir := filepath.Join(c.sourceDir, locale)

		if err := requireRealTranslationDir(localeDir); err != nil {
			return nil, fmt.Errorf("inspect locale directory %s: %w", localeDir, err)
		}

		poFile := filepath.Join(localeDir, c.domain+".po")

		poInfo, err := os.Lstat(poFile)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			return nil, fmt.Errorf("inspect translation catalog %s: %w", poFile, err)
		}

		if !poInfo.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: %s", errTranslationPathNotRegular, poFile)
		}

		catalogs = append(catalogs, translationCatalog{
			source: poFile,
			target: filepath.Join(c.outputDir, locale, "LC_MESSAGES", c.domain+".mo"),
		})
	}

	return catalogs, nil
}

func stageTranslationCatalog(
	ctx context.Context,
	catalog translationCatalog,
	stderr io.Writer,
	localizer output.Localizer,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if err := requireRealTranslationDir(filepath.Dir(catalog.source)); err != nil {
		return "", fmt.Errorf("inspect locale directory for %s: %w", catalog.source, err)
	}

	if err := requireRegularTranslationFile(catalog.source); err != nil {
		return "", fmt.Errorf("inspect translation catalog %s: %w", catalog.source, err)
	}

	outputDir := filepath.Dir(catalog.target)
	if err := ensureRealTranslationDir(outputDir); err != nil {
		return "", fmt.Errorf("create translation output directory %s: %w", outputDir, err)
	}

	if err := requireRegularTranslationTarget(catalog.target); err != nil {
		return "", err
	}

	tempPath, err := createTranslationTemp(catalog.target)
	if err != nil {
		return "", err
	}

	keepTemp := false

	defer func() {
		if !keepTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if err := ctx.Err(); err != nil {
		return "", err
	}

	if err := runTranslationMsgfmt(ctx, catalog.source, tempPath, stderr, localizer); err != nil {
		return "", err
	}

	if err := requireRegularTranslationFile(tempPath); err != nil {
		return "", fmt.Errorf("inspect compiled translation catalog: %w", err)
	}

	keepTemp = true

	return tempPath, nil
}

func createTranslationTemp(target string) (string, error) {
	tempFile, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+"-")
	if err != nil {
		return "", fmt.Errorf("create temporary translation catalog: %w", err)
	}

	tempPath := tempFile.Name()

	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)

		return "", fmt.Errorf("close temporary translation catalog: %w", err)
	}

	if err := os.Chmod(tempPath, 0o644); err != nil { //nolint:gosec // Compiled catalogs are package data and intentionally world-readable.
		_ = os.Remove(tempPath)

		return "", fmt.Errorf("set temporary translation catalog permissions: %w", err)
	}

	return tempPath, nil
}

func runTranslationMsgfmt(
	ctx context.Context,
	source string,
	target string,
	stderr io.Writer,
	localizer output.Localizer,
) error {
	errorWriter := &translationCommandErrorWriter{writer: stderr}
	mfCmd := exec.CommandContext( //nolint:gosec // Fixed command; paths are passed as arguments, not through a shell.
		ctx,
		"msgfmt",
		"--check",
		"--output-file="+target,
		"--",
		source,
	)
	mfCmd.Stderr = errorWriter
	runErr := mfCmd.Run()

	if errorWriter.err != nil {
		cause := errorWriter.err
		if ctxErr := ctx.Err(); ctxErr != nil {
			cause = errors.Join(cause, ctxErr)
		}

		return translationOutputWriteError(cause, localizer)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if runErr != nil {
		return translationCompileError(fmt.Errorf("msgfmt %s: %w", source, runErr), localizer)
	}

	return nil
}

type translationCommandErrorWriter struct {
	writer io.Writer
	err    error
}

func (w *translationCommandErrorWriter) Write(data []byte) (int, error) {
	written, err := w.writer.Write(data)
	if err == nil && written != len(data) {
		err = io.ErrShortWrite
	}

	if err != nil && w.err == nil {
		w.err = err
	}

	return written, err
}

func replaceTranslationCatalog(catalog translationCatalog, renameFile func(string, string) error) error {
	if err := validateTranslationCatalogCommit(catalog); err != nil {
		return err
	}

	if err := renameFile(catalog.temp, catalog.target); err != nil {
		return fmt.Errorf("replace translation catalog %s: %w", catalog.target, err)
	}

	return nil
}

func validateTranslationCatalogCommit(catalog translationCatalog) error {
	if err := rejectTranslationSymlinkComponents(catalog.target); err != nil {
		return err
	}

	if err := requireRegularTranslationTarget(catalog.target); err != nil {
		return err
	}

	if err := requireRegularTranslationFile(catalog.temp); err != nil {
		return fmt.Errorf("inspect staged translation catalog: %w", err)
	}

	return nil
}

func ensureRealTranslationDir(path string) error {
	if err := rejectTranslationSymlinkComponents(path); err != nil {
		return err
	}

	if err := os.MkdirAll(path, 0o750); err != nil {
		return err
	}

	return requireRealTranslationDir(path)
}

func requireRealTranslationDir(path string) error {
	if err := rejectTranslationSymlinkComponents(path); err != nil {
		return err
	}

	info, err := os.Lstat(path)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("%w: %s", errTranslationPathNotDirectory, path)
	}

	return nil
}

func requireRegularTranslationTarget(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("inspect translation output %s: %w", path, err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return translationSymlinkError(path)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", errTranslationPathNotRegular, path)
	}

	return nil
}

func requireRegularTranslationFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", errTranslationPathNotRegular, path)
	}

	return nil
}

func rejectTranslationSymlinkComponents(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	for current := filepath.Clean(absolute); ; current = filepath.Dir(current) {
		info, statErr := os.Lstat(current)
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return translationSymlinkError(current)
		}

		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("inspect translation path %s: %w", current, statErr)
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}

	return nil
}
