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
	"regexp"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/tools/i18nextract"
	"github.com/urfave/cli/v3"
)

const commandNameTranslationsUpdate = "update"

// NewTranslationsUpdateCommand wraps the extraction and gettext pipeline.
func NewTranslationsUpdateCommand(localizer output.Localizer) *cli.Command {
	localizer = output.EnsureLocalizer(localizer)

	return &cli.Command{
		Name:  commandNameTranslationsUpdate,
		Usage: localizer.Get("refresh .po catalogs from source"),
		Description: localizer.Get(`Re-extracts translatable strings from Go source with the built-in
extractor (it understands the project's Localizer API and Msg
constructors), then folds the new template into each locale's existing
.po with msgmerge. Translator-edited msgstr entries are preserved;
obsolete msgids are removed; new untranslated msgids land with empty
msgstr.

Run-host requirements (build-time only — not needed by end users):
  msgmerge  GNU gettext-tools
  msgen     GNU gettext-tools
  msgattrib GNU gettext-tools`),

		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "locale-dir",
				Usage: localizer.Get("locale source directory (default: ./locale)"),
			},
			&cli.StringFlag{
				Name:  "domain",
				Value: gommitlintKey,
				Usage: localizer.Get("PO domain name (also the PO/MO filename stem)"),
			},
		},

		Action: ExecuteTranslationsUpdate,
	}
}

// ExecuteTranslationsUpdate runs the update pipeline.
func ExecuteTranslationsUpdate(ctx context.Context, cmd *cli.Command) error {
	if err := rejectUnexpectedTranslationArgs(cmd, "gommitlint translations update"); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	domain := cmd.String("domain")
	if err := validateTranslationDomain(domain, commandLocalizer); err != nil {
		return err
	}

	rootDir, err := repoRoot()
	if err != nil {
		return translationUpdateError(fmt.Errorf("locate repo root: %w", err), commandLocalizer())
	}

	localeDir := cmd.String("locale-dir")
	if localeDir == "" {
		localeDir = filepath.Join(rootDir, "locale")
	}

	cfg := translationUpdateConfig{
		rootDir:   rootDir,
		localeDir: localeDir,
		domain:    domain,
		potFile:   filepath.Join(localeDir, domain+".pot"),
	}

	localizer := commandLocalizer()
	if err := cfg.checkPrerequisites(ctx, localizer); err != nil {
		return err
	}

	updates, err := cfg.findCatalogUpdates(ctx)
	if err != nil {
		return translationUpdateError(err, localizer)
	}
	defer removeStagedTranslationUpdates(updates)

	if err := cfg.stageCatalogUpdates(ctx, updates, cmd.ErrWriter, localizer); err != nil {
		return translationUpdateError(err, localizer)
	}

	if err := commitTranslationUpdates(ctx, updates, cfg.renameFile); err != nil {
		return translationUpdateError(err, localizer)
	}

	if cmd.Root().Bool(flags.Quiet) {
		return nil
	}

	writeErr := writeCommandOutput(cmd.ErrWriter, "Translation catalogs updated\n", localizer)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return errors.Join(writeErr, ctxErr)
	}

	return writeErr
}

type translationUpdateConfig struct {
	rootDir    string
	localeDir  string
	domain     string
	potFile    string
	renameFile func(string, string) error
}

// checkPrerequisites verifies the external tools are on PATH and
// the locale directory exists. Errors are actionable: each missing tool
// names the install command rather than just saying "not found".
func (c translationUpdateConfig) checkPrerequisites(ctx context.Context, localizer output.Localizer) error {
	for _, tool := range []string{"msgmerge", "msgen", "msgattrib"} {
		if err := ctx.Err(); err != nil {
			return err
		}

		if _, err := exec.LookPath(tool); err != nil {
			return missingTranslationDependencyError(tool, err, localizer)
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := requireRealTranslationDir(c.localeDir); err != nil {
		return translationLocaleDirError(c.localeDir, err, localizer)
	}

	return nil
}

type translationUpdateKind uint8

const (
	translationUpdatePOT translationUpdateKind = iota
	translationUpdateEnglish
	translationUpdateLocale
)

type translationUpdateCatalog struct {
	kind   translationUpdateKind
	source string
	target string
	temp   string
}

func (c translationUpdateConfig) findCatalogUpdates(ctx context.Context) ([]translationUpdateCatalog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := rejectTranslationSymlinkComponents(c.potFile); err != nil {
		return nil, fmt.Errorf("inspect POT path %s: %w", c.potFile, err)
	}

	if err := requireRegularTranslationTarget(c.potFile); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(c.localeDir)
	if err != nil {
		return nil, fmt.Errorf("read locale directory %s: %w", c.localeDir, err)
	}

	updates := []translationUpdateCatalog{{kind: translationUpdatePOT, target: c.potFile}}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		update, managed, err := c.catalogUpdateForEntry(entry)
		if err != nil {
			return nil, err
		}

		if managed {
			updates = append(updates, update)
		}
	}

	return updates, ctx.Err()
}

func (c translationUpdateConfig) catalogUpdateForEntry(
	entry os.DirEntry,
) (translationUpdateCatalog, bool, error) {
	entryPath := filepath.Join(c.localeDir, entry.Name())

	info, err := entry.Info()
	if err != nil {
		return translationUpdateCatalog{}, false, fmt.Errorf("inspect locale entry %s: %w", entryPath, err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return translationUpdateCatalog{}, false, translationSymlinkError(entryPath)
	}

	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return translationUpdateCatalog{}, false, fmt.Errorf("%w: %s", errTranslationPathNotRegular, entryPath)
		}

		return translationUpdateCatalog{}, false, nil
	}

	if err := requireRealTranslationDir(entryPath); err != nil {
		return translationUpdateCatalog{}, false, fmt.Errorf("inspect locale directory %s: %w", entryPath, err)
	}

	poFile := filepath.Join(entryPath, c.domain+".po")
	if entry.Name() == "en" {
		if err := requireRegularTranslationTarget(poFile); err != nil {
			return translationUpdateCatalog{}, false, err
		}

		return translationUpdateCatalog{kind: translationUpdateEnglish, target: poFile}, true, nil
	}

	if err := requireRegularTranslationFile(poFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return translationUpdateCatalog{}, false, nil
		}

		return translationUpdateCatalog{}, false, fmt.Errorf("inspect translation catalog %s: %w", poFile, err)
	}

	return translationUpdateCatalog{
		kind:   translationUpdateLocale,
		source: poFile,
		target: poFile,
	}, true, nil
}

func (c translationUpdateConfig) stageCatalogUpdates(
	ctx context.Context,
	updates []translationUpdateCatalog,
	stderr io.Writer,
	localizer output.Localizer,
) error {
	for index := range updates {
		if err := ctx.Err(); err != nil {
			return err
		}

		var (
			temp string
			err  error
		)

		switch updates[index].kind {
		case translationUpdatePOT:
			temp, err = c.stagePOT(ctx)
		case translationUpdateEnglish:
			temp, err = c.stageEnglishTranslation(
				ctx,
				updates[index].target,
				updates[0].temp,
				stderr,
				localizer,
			)
		case translationUpdateLocale:
			temp, err = c.stageLocaleTranslation(
				ctx,
				updates[index],
				updates[0].temp,
				stderr,
				localizer,
			)
		}

		if err != nil {
			return err
		}

		updates[index].temp = temp
	}

	return ctx.Err()
}

// stagePOT extracts calls through the project's Localizer API into a staged
// template in the destination directory.
func (c translationUpdateConfig) stagePOT(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	excludes := []string{".git", "tools", filepath.Join("internal", "adapters", "i18n")}

	catalog, err := i18nextract.Extract(c.rootDir, excludes)
	if err != nil {
		return "", fmt.Errorf("extract translatable strings: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	if err := requireRealTranslationDir(filepath.Dir(c.potFile)); err != nil {
		return "", fmt.Errorf("inspect POT directory: %w", err)
	}

	if err := requireRegularTranslationTarget(c.potFile); err != nil {
		return "", err
	}

	potFile, err := os.CreateTemp(filepath.Dir(c.potFile), "."+filepath.Base(c.potFile)+"-")
	if err != nil {
		return "", fmt.Errorf("create temporary POT: %w", err)
	}

	tempPath := potFile.Name()
	keepTemp := false

	defer func() {
		if !keepTemp {
			_ = os.Remove(tempPath)
		}
	}()

	modeErr := potFile.Chmod(0o644)

	writeErr := error(nil)
	if modeErr == nil {
		writeErr = catalog.WritePOT(potFile)
	}

	closeErr := potFile.Close()

	if err := errors.Join(modeErr, writeErr, closeErr); err != nil {
		return "", fmt.Errorf("write temporary POT: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	if err := requireRegularTranslationFile(tempPath); err != nil {
		return "", fmt.Errorf("inspect temporary POT: %w", err)
	}

	if err := patchPOHeader(tempPath, ""); err != nil {
		return "", fmt.Errorf("patch POT header: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	keepTemp = true

	return tempPath, nil
}

func (c translationUpdateConfig) stageEnglishTranslation(
	ctx context.Context,
	target string,
	potFile string,
	stderr io.Writer,
	localizer output.Localizer,
) (string, error) {
	if err := requireRealTranslationDir(filepath.Dir(target)); err != nil {
		return "", fmt.Errorf("inspect English locale directory: %w", err)
	}

	if err := requireRegularTranslationTarget(target); err != nil {
		return "", err
	}

	tempPath, err := createTranslationTemp(target)
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

	if err := runTranslationCommand(
		ctx,
		"msgen",
		[]string{"--no-wrap", "--output=" + tempPath, "--", potFile},
		stderr,
		localizer,
	); err != nil {
		return "", err
	}

	if err := requireRegularTranslationFile(tempPath); err != nil {
		return "", fmt.Errorf("inspect staged English catalog: %w", err)
	}

	if err := patchPOHeader(tempPath, "en"); err != nil {
		return "", fmt.Errorf("patch English header: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	keepTemp = true

	return tempPath, nil
}

func (c translationUpdateConfig) stageLocaleTranslation(
	ctx context.Context,
	update translationUpdateCatalog,
	potFile string,
	stderr io.Writer,
	localizer output.Localizer,
) (string, error) {
	tempPath, err := copyTranslationToTemp(update.source, update.target)
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

	if err := runTranslationCommand(
		ctx,
		"msgmerge",
		[]string{
			"--update",
			"--backup=none",
			"--no-fuzzy-matching",
			"--no-wrap",
			"--",
			tempPath,
			potFile,
		},
		stderr,
		localizer,
	); err != nil {
		return "", fmt.Errorf("msgmerge %s: %w", update.source, err)
	}

	if err := requireRegularTranslationFile(tempPath); err != nil {
		return "", fmt.Errorf("inspect staged translation catalog: %w", err)
	}

	filteredPath, err := createTranslationTemp(update.target)
	if err != nil {
		return "", err
	}

	if err := runTranslationCommand(
		ctx,
		"msgattrib",
		[]string{"--no-obsolete", "--output-file=" + filteredPath, "--", tempPath},
		stderr,
		localizer,
	); err != nil {
		_ = os.Remove(filteredPath)

		return "", fmt.Errorf("msgattrib %s: %w", update.source, err)
	}

	if err := requireRegularTranslationFile(filteredPath); err != nil {
		_ = os.Remove(filteredPath)

		return "", fmt.Errorf("inspect filtered translation catalog: %w", err)
	}

	if err := os.Remove(tempPath); err != nil {
		_ = os.Remove(filteredPath)

		return "", fmt.Errorf("remove unfiltered translation catalog: %w", err)
	}

	tempPath = filteredPath

	keepTemp = true

	return tempPath, nil
}

func copyTranslationToTemp(source, target string) (string, error) {
	if err := requireRealTranslationDir(filepath.Dir(source)); err != nil {
		return "", fmt.Errorf("inspect locale directory for %s: %w", source, err)
	}

	if err := requireRegularTranslationFile(source); err != nil {
		return "", fmt.Errorf("inspect translation catalog %s: %w", source, err)
	}

	if err := requireRegularTranslationTarget(target); err != nil {
		return "", err
	}

	sourceFile, err := os.Open(source) //nolint:gosec // The source is validated as a managed regular catalog above.
	if err != nil {
		return "", fmt.Errorf("open translation catalog %s: %w", source, err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+"-")
	if err != nil {
		closeErr := sourceFile.Close()

		return "", fmt.Errorf("create temporary translation catalog: %w", errors.Join(err, closeErr))
	}

	tempPath := tempFile.Name()
	keepTemp := false

	defer func() {
		if !keepTemp {
			_ = os.Remove(tempPath)
		}
	}()

	sourceInfo, statErr := sourceFile.Stat()

	modeErr := error(nil)
	if statErr == nil {
		modeErr = tempFile.Chmod(sourceInfo.Mode().Perm())
	}

	copyErr := error(nil)
	if statErr == nil && modeErr == nil {
		_, copyErr = io.Copy(tempFile, sourceFile)
	}

	tempCloseErr := tempFile.Close()
	sourceCloseErr := sourceFile.Close()

	if err := errors.Join(statErr, modeErr, copyErr, tempCloseErr, sourceCloseErr); err != nil {
		return "", fmt.Errorf("copy translation catalog %s: %w", source, err)
	}

	if err := requireRegularTranslationFile(tempPath); err != nil {
		return "", fmt.Errorf("inspect copied translation catalog: %w", err)
	}

	keepTemp = true

	return tempPath, nil
}

func runTranslationCommand(
	ctx context.Context,
	name string,
	args []string,
	stderr io.Writer,
	localizer output.Localizer,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	errorWriter := &translationCommandErrorWriter{writer: stderr}
	// Every caller passes a string literal: "msgen", "msgmerge" or "msgattrib".
	// Arguments reach the tool directly, never through a shell.
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	command := exec.CommandContext(ctx, name, args...) //nolint:gosec // Command names are fixed and arguments are not interpreted by a shell.
	command.Stderr = errorWriter
	runErr := command.Run()

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
		return fmt.Errorf("%s: %w", name, runErr)
	}

	return nil
}

func commitTranslationUpdates(
	ctx context.Context,
	updates []translationUpdateCatalog,
	renameFile func(string, string) error,
) error {
	catalogs := make([]translationCatalog, len(updates))
	for index, update := range updates {
		catalogs[index] = translationCatalog{target: update.target, temp: update.temp}
	}

	err := commitTranslationCatalogBatch(ctx, catalogs, renameFile)

	for index := range updates {
		updates[index].temp = catalogs[index].temp
	}

	return err
}

func removeStagedTranslationUpdates(updates []translationUpdateCatalog) {
	for _, update := range updates {
		if update.temp != "" {
			_ = os.Remove(update.temp)
		}
	}
}

// repoRoot walks up from CWD until it finds Gommitlint's module directive.
func repoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := cwd
	for {
		modulePath := filepath.Join(dir, "go.mod")

		info, statErr := os.Lstat(modulePath)
		if statErr == nil {
			if !info.Mode().IsRegular() {
				return "", fmt.Errorf("inspect module marker %s: %w", modulePath, errTranslationPathNotRegular)
			}

			data, readErr := os.ReadFile(modulePath) //nolint:gosec // Repository discovery reads candidate module manifests only.
			if readErr != nil {
				return "", fmt.Errorf("read module marker %s: %w", modulePath, readErr)
			}

			if hasGommitlintModuleDirective(data) {
				return dir, nil
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("inspect module marker %s: %w", modulePath, statErr)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%w from %s", errGommitlintModuleNotFound, cwd)
		}

		dir = parent
	}
}

func hasGommitlintModuleDirective(data []byte) bool {
	const directive = "module codefloe.com/itiquette/gommitlint"

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "//", 2)[0])
		if line == directive {
			return true
		}
	}

	return false
}

// poHeaderFieldRE matches any of the field-name prefixes we replace.
// `^"<Field>:` is the natural delimiter inside a PO header block.
var poHeaderFieldRE = regexp.MustCompile(
	`^"(Project-Id-Version|PO-Revision-Date|Last-Translator|Language-Team|Language):`,
)

// patchPOHeader rewrites the project-managed metadata fields in a .po
// file. PO header lines sit between the empty `msgid ""`/`msgstr ""`
// pair and the next blank line. We strip the five fields we own from
// wherever they currently sit and prepend a canonical block at the top
// of the header, so freshly-updated .po files remain stable across runs.
func patchPOHeader(path, language string) error {
	if err := rejectTranslationSymlinkComponents(path); err != nil {
		return err
	}

	info, err := os.Lstat(path)
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", errTranslationPathNotRegular, path)
	}

	data, err := os.ReadFile(path) //nolint:gosec // Translation paths are project-controlled catalog files.
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")

	headerStart, headerEnd, err := poHeaderBounds(lines, path)
	if err != nil {
		return err
	}

	managed := []string{
		`"Project-Id-Version: gommitlint\n"`,
		`"PO-Revision-Date: \n"`,
		`"Last-Translator: \n"`,
		`"Language-Team: \n"`,
		fmt.Sprintf(`"Language: %s\n"`, language),
	}

	kept := append([]string{}, managed...)

	for _, line := range lines[headerStart:headerEnd] {
		if !poHeaderFieldRE.MatchString(line) {
			kept = append(kept, line)
		}
	}

	rewritten := append([]string{}, lines[:headerStart]...)
	rewritten = append(rewritten, kept...)
	rewritten = append(rewritten, lines[headerEnd:]...)

	return replaceTranslationFile(path, []byte(strings.Join(rewritten, "\n")), info.Mode().Perm())
}

func poHeaderBounds(lines []string, path string) (int, int, error) {
	headerStart := -1

	for i, line := range lines {
		if line == `msgid ""` {
			headerStart = i + 2 // skip msgid "" + msgstr "" lines

			break
		}
	}

	if headerStart < 0 {
		return 0, 0, fmt.Errorf("%w in %s", errNoMsgIDHeader, path)
	}

	if headerStart > len(lines) || lines[headerStart-1] != `msgstr ""` {
		return 0, 0, fmt.Errorf(
			"%w in %s: msgid header must be followed by msgstr \"\"",
			errMalformedPOHeader,
			path,
		)
	}

	headerEnd := headerStart
	for headerEnd < len(lines) && lines[headerEnd] != "" {
		if !strings.HasPrefix(lines[headerEnd], `"`) || !strings.HasSuffix(lines[headerEnd], `"`) {
			return 0, 0, fmt.Errorf("%w in %s: invalid header field", errMalformedPOHeader, path)
		}

		headerEnd++
	}

	if headerEnd == headerStart {
		return 0, 0, fmt.Errorf("%w in %s: empty header block", errMalformedPOHeader, path)
	}

	return headerStart, headerEnd, nil
}

func replaceTranslationFile(path string, data []byte, mode os.FileMode) error {
	tempFile, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-")
	if err != nil {
		return fmt.Errorf("create temporary header replacement: %w", err)
	}

	tempPath := tempFile.Name()
	keepTemp := false

	defer func() {
		if !keepTemp {
			_ = os.Remove(tempPath)
		}
	}()

	modeErr := tempFile.Chmod(mode)

	written, writeErr := tempFile.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = io.ErrShortWrite
	}

	closeErr := tempFile.Close()

	if err := errors.Join(modeErr, writeErr, closeErr); err != nil {
		return fmt.Errorf("write temporary header replacement: %w", err)
	}

	if err := rejectTranslationSymlinkComponents(path); err != nil {
		return err
	}

	if err := requireRegularTranslationFile(path); err != nil {
		return fmt.Errorf("inspect header replacement target: %w", err)
	}

	if err := requireRegularTranslationFile(tempPath); err != nil {
		return fmt.Errorf("inspect temporary header replacement: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace translation header %s: %w", path, err)
	}

	keepTemp = true

	return nil
}
