// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

var (
	// ErrCommitNotFound indicates a commit was not found in git output.
	ErrCommitNotFound = errors.New("commit not found")
	// ErrUnexpectedGitLogFormat indicates the git log output is not parseable.
	ErrUnexpectedGitLogFormat = errors.New("unexpected git log format")
)

// gitLogFormat is the --format string used for all commit log reads.
// Fields are separated by US (0x1f) and records by RS (0x1e).
// Order: hash, parents, author name, author email, author date, body.
const (
	gitLogFormat    = "--format=%H%x1f%P%x1f%an%x1f%ae%x1f%ad%x1f%B%x1e"
	gitEndOfOptions = "--end-of-options"
)

// CommitReader reads commit data using the native git CLI.
type CommitReader struct {
	repoPath string
}

// NewCommitReader creates a CLI-backed commit reader.
func NewCommitReader(repoPath string) *CommitReader {
	return &CommitReader{repoPath: repoPath}
}

// DetectObjectFormat returns the repository object format.
func DetectObjectFormat(ctx context.Context, repoPath string) (string, error) {
	output, err := runGitCommand(ctx, repoPath, "rev-parse", "--show-object-format")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// GetCommit retrieves a single commit by reference.
func (r CommitReader) GetCommit(ctx context.Context, ref string) (domain.Commit, error) {
	if err := validateGitRefSafety(ref); err != nil {
		return domain.Commit{}, fmt.Errorf("invalid git reference: %w", err)
	}

	exists, err := runGitCommandQuiet(ctx, r.repoPath, "rev-parse", "--verify", "--quiet", gitEndOfOptions, ref+"^{commit}")
	if err != nil {
		return domain.Commit{}, err
	}

	if !exists {
		return domain.Commit{}, fmt.Errorf("%w: %s", domain.ErrReferenceNotFound, ref)
	}

	output, err := runGitCommand(
		ctx,
		r.repoPath,
		"show",
		"-s",
		"--date=iso-strict",
		gitLogFormat,
		gitEndOfOptions,
		ref,
	)
	if err != nil {
		return domain.Commit{}, err
	}

	commits, err := parseGitLogOutput(output)
	if err != nil {
		return domain.Commit{}, err
	}

	if len(commits) == 0 {
		return domain.Commit{}, fmt.Errorf("%w: %s", ErrCommitNotFound, ref)
	}

	return r.attachSignatureData(ctx, commits[0])
}

// GetHeadCommits retrieves the latest commits from HEAD.
func (r CommitReader) GetHeadCommits(ctx context.Context, count int) ([]domain.Commit, error) {
	output, err := runGitCommand(
		ctx,
		r.repoPath,
		"log",
		"--max-count",
		strconv.Itoa(count),
		"--date=iso-strict",
		gitLogFormat,
		gitEndOfOptions,
		"HEAD",
	)
	if err != nil {
		return nil, err
	}

	commits, err := parseGitLogOutput(output)
	if err != nil {
		return nil, err
	}

	return r.attachSignatureDataToCommits(ctx, commits)
}

// GetCommitRange retrieves commits in a range.
func (r CommitReader) GetCommitRange(ctx context.Context, fromRef, toRef string) ([]domain.Commit, error) {
	if err := validateGitRefSafety(fromRef); err != nil {
		return nil, fmt.Errorf("invalid git reference: %w", err)
	}

	if err := validateGitRefSafety(toRef); err != nil {
		return nil, fmt.Errorf("invalid git reference: %w", err)
	}

	for _, ref := range []string{fromRef, toRef} {
		exists, err := runGitCommandQuiet(ctx, r.repoPath, "rev-parse", "--verify", "--quiet", gitEndOfOptions, ref+"^{commit}")
		if err != nil {
			return nil, err
		}

		if !exists {
			return nil, fmt.Errorf("%w: %s", domain.ErrReferenceNotFound, ref)
		}
	}

	output, err := runGitCommand(
		ctx,
		r.repoPath,
		"log",
		"--date=iso-strict",
		gitLogFormat,
		gitEndOfOptions,
		fmt.Sprintf("%s..%s", fromRef, toRef),
	)
	if err != nil {
		return nil, err
	}

	commits, err := parseGitLogOutput(output)
	if err != nil {
		return nil, err
	}

	slices.Reverse(commits)

	return r.attachSignatureDataToCommits(ctx, commits)
}

func (r CommitReader) attachSignatureDataToCommits(ctx context.Context, commits []domain.Commit) ([]domain.Commit, error) {
	for commitIndex := range commits {
		commit, err := r.attachSignatureData(ctx, commits[commitIndex])
		if err != nil {
			return nil, err
		}

		commits[commitIndex] = commit
	}

	return commits, nil
}

func (r CommitReader) attachSignatureData(ctx context.Context, commit domain.Commit) (domain.Commit, error) {
	if commit.Hash == "" {
		return commit, nil
	}

	rawCommit, err := runGitCommand(ctx, r.repoPath, "cat-file", "commit", commit.Hash)
	if err != nil {
		return domain.Commit{}, err
	}

	signature, payload := extractCommitSignatureData(rawCommit)
	if signature == "" {
		return commit, nil
	}

	commit.Signature = signature
	commit.SignaturePayload = payload

	return commit, nil
}

func runGitCommand(ctx context.Context, repoPath string, args ...string) ([]byte, error) {
	if repoPath == "" {
		return nil, domain.ErrNotGitRepository
	}

	cmdArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...) //nolint:gosec
	cmd.Env = Environment()

	// Output, not CombinedOutput: this return value is parsed as git data --
	// log records split on 0x1e, and the raw commit whose bytes become
	// SignaturePayload. git can write a warning to stderr while still exiting
	// 0, and CombinedOutput would splice that line into the first record, or
	// into the payload a signature is checked against.
	output, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if exitErr := new(exec.ExitError); errors.As(err, &exitErr) {
			stderr = exitErr.Stderr
		}

		trimmedOutput := strings.TrimSpace(string(stderr))
		if trimmedOutput == "" {
			return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}

		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, trimmedOutput)
	}

	return output, nil
}

func runGitCommandQuiet(ctx context.Context, repoPath string, args ...string) (bool, error) {
	if repoPath == "" {
		return false, domain.ErrNotGitRepository
	}

	cmdArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...) //nolint:gosec
	cmd.Env = Environment()

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}

		return false, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}

	return true, nil
}

func parseGitLogOutput(output []byte) ([]domain.Commit, error) {
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		return []domain.Commit{}, nil
	}

	records := bytes.Split(trimmed, []byte{0x1e})
	commits := make([]domain.Commit, 0, len(records))

	for _, record := range records {
		if len(record) == 0 {
			continue
		}

		fields := bytes.SplitN(record, []byte{0x1f}, 6)
		if len(fields) != 6 {
			return nil, fmt.Errorf("%w: %d fields", ErrUnexpectedGitLogFormat, len(fields))
		}

		hash := strings.TrimSpace(string(fields[0]))
		parents := strings.Fields(strings.TrimSpace(string(fields[1])))
		author := strings.TrimSpace(string(fields[2]))
		authorEmail := strings.TrimSpace(string(fields[3]))
		commitDate := strings.TrimSpace(string(fields[4]))
		message := strings.TrimRight(string(fields[5]), "\n")

		isMerge := len(parents) > 1
		commit := domain.NewCommit(hash, message, author, authorEmail, commitDate, "", isMerge)
		commits = append(commits, commit)
	}

	return commits, nil
}

// Commit object headers carrying a signature: "gpgsig" covers the SHA-1
// content of a commit, "gpgsig-sha256" its SHA-256 content. A commit signed
// for both formats carries both.
const (
	signatureHeaderSHA1   = "gpgsig"
	signatureHeaderSHA256 = "gpgsig-sha256"
)

// extractCommitSignatureData splits a raw commit object into its signature
// and the payload the signature covers. Signature headers are only recognised
// in the header section, which the first empty line ends: a message body line
// that happens to start with "gpgsig " is commit text, not a signature.
//
// git reads only the header for the repository's object format and hands the
// verifier the payload with both headers removed. This adapter serves SHA-256
// repositories, so when a commit is signed for both formats the SHA-256
// signature is the one returned.
func extractCommitSignatureData(rawCommit []byte) (string, []byte) {
	signatures := make(map[string][]string)
	payload := make([]byte, 0, len(rawCommit))
	inHeaders := true
	header := ""

	for len(rawCommit) > 0 {
		line, rest := nextCommitObjectLine(rawCommit)
		rawCommit = rest

		if inHeaders && len(bytes.TrimSuffix(line, []byte("\n"))) == 0 {
			inHeaders = false
			header = ""
		}

		if inHeaders && header != "" && len(line) > 0 && line[0] == ' ' {
			signatures[header] = append(signatures[header], trimCommitObjectLine(string(line[1:])))

			continue
		}

		header = ""

		if inHeaders {
			if name, signatureValue, ok := commitSignatureHeaderValue(line); ok {
				signatures[name] = append(signatures[name], signatureValue)
				header = name

				continue
			}
		}

		payload = append(payload, line...)
	}

	for _, name := range []string{signatureHeaderSHA256, signatureHeaderSHA1} {
		if lines := signatures[name]; len(lines) > 0 {
			return strings.Join(lines, "\n"), payload
		}
	}

	return "", nil
}

func nextCommitObjectLine(rawCommit []byte) ([]byte, []byte) {
	lineEnd := bytes.IndexByte(rawCommit, '\n')
	if lineEnd == -1 {
		return rawCommit, nil
	}

	lineEnd++

	return rawCommit[:lineEnd], rawCommit[lineEnd:]
}

// commitSignatureHeaderValue returns the header name and the first signature
// line when line is a signature header.
func commitSignatureHeaderValue(line []byte) (string, string, bool) {
	trimmedLine := bytes.TrimSuffix(line, []byte("\n"))

	for _, header := range []string{signatureHeaderSHA1, signatureHeaderSHA256} {
		if value, ok := bytes.CutPrefix(trimmedLine, []byte(header+" ")); ok {
			return header, string(value), true
		}
	}

	return "", "", false
}

func trimCommitObjectLine(line string) string {
	return strings.TrimSuffix(line, "\n")
}

func validateGitRefSafety(ref string) error {
	return domain.DefaultSecurityPolicy().ValidateGitReference(ref)
}
