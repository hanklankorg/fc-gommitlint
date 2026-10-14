// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	sarif "github.com/owenrumney/go-sarif/v3/pkg/report/v210/sarif"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

const (
	sarifToolName       = "gommitlint"
	sarifInformationURI = "https://codefloe.com/itiquette/gommitlint"
	sarifRulesHelpURI   = "https://codefloe.com/itiquette/gommitlint/src/branch/main/docs/modules/ROOT/pages/reference/rules.adoc"
	// sarifFingerprintKey names the fingerprint scheme so consumers can
	// detect scheme changes; bump the suffix if the hash input changes.
	sarifFingerprintKey = "gommitlint/v1"
)

// SARIFOptions configures SARIF rendering.
type SARIFOptions struct {
	// ToolVersion is the gommitlint version reported as tool.driver.version.
	ToolVersion string
}

// SARIF formats a report as a SARIF 2.1.0 document.
//
// Messages are always English regardless of locale: SARIF feeds dashboards
// and aggregation pipelines where result text and fingerprints must be
// stable across machines. Locations are physical (file plus line) when the
// report came from --message-file, logical (commit SHA) otherwise — commit
// messages are not files in the repository tree.
func SARIF(report domain.Report, options SARIFOptions) string {
	var builder strings.Builder

	_ = SARIFTo(&builder, report, options)

	return builder.String()
}

// SARIFTo writes a SARIF 2.1.0 document to w. The SARIF library materialises
// the formatted document before writing it; this is not incremental streaming.
func SARIFTo(writer io.Writer, report domain.Report, options SARIFOptions) error {
	doc := sarif.NewReport()
	run := sarif.NewRunWithInformationURI(sarifToolName, sarifInformationURI)

	if options.ToolVersion != "" {
		run.Tool.Driver.WithVersion(options.ToolVersion)
	}

	for _, commitReport := range report.Commits {
		for _, rule := range commitReport.RuleResults {
			for _, err := range rule.Errors {
				addSARIFResult(run, commitReport.Commit, report.Metadata.MessageFile, rule.Name, err)
			}
		}
	}

	for _, rule := range report.Repository.RuleResults {
		for _, err := range rule.Errors {
			addSARIFResult(run, domain.Commit{}, "", rule.Name, err)
		}
	}

	doc.AddRun(run)

	return doc.PrettyWrite(writer)
}

func addSARIFResult(run *sarif.Run, commit domain.Commit, messageFile, ruleName string, err domain.ValidationError) {
	ruleID := string(err.Diagnostic.ID)
	if ruleID == "" {
		ruleID = ruleName
	}

	ensureSARIFRule(run, ruleID, err)

	// Findings render with the neutral localizer on purpose; see SARIF.
	result := run.CreateResultForRule(ruleID).
		WithLevel("error").
		WithMessage(sarif.NewTextMessage(convertToUserFriendlyMessage(err, nil))).
		WithPartialFingerprints(map[string]string{
			sarifFingerprintKey: sarifFingerprint(commit.Hash, ruleID, err),
		})

	if location := sarifLocation(commit, messageFile, err); location != nil {
		result.AddLocation(location)
	}

	if commit.Hash != "" {
		bag := sarif.NewPropertyBag()
		bag.Add("commitSha", commit.Hash)
		bag.Add("commitSubject", commit.Subject)
		result.WithProperties(bag)
	}
}

// ensureSARIFRule registers the reporting descriptor for a check the first
// time it is referenced, sourced from the public GL diagnostic catalog.
func ensureSARIFRule(run *sarif.Run, ruleID string, err domain.ValidationError) {
	rule := run.AddRule(ruleID)
	if rule.ShortDescription != nil {
		return // already populated on a previous result
	}

	summary := string(err.Diagnostic.Reason)
	if definition, ok := diagnostics.ByID(ruleID); ok {
		summary = definition.Summary
	}

	rule.WithDescription(summary).WithHelpURI(sarifRulesHelpURI)

	if helpMsg, ok := LookupHelpMessage(err.Diagnostic.Reason, err.Context); ok {
		rule.WithHelp(sarif.NewMultiformatMessageString().WithText(helpMsg.String()))
	}
}

// sarifLocation builds the best available location for a finding: a physical
// file location with a line region in --message-file mode, a logical commit
// location otherwise, or nil when neither is known (stdin input).
func sarifLocation(commit domain.Commit, messageFile string, err domain.ValidationError) *sarif.Location {
	if messageFile != "" {
		physical := sarif.NewPhysicalLocation().
			WithArtifactLocation(sarif.NewSimpleArtifactLocation(filepath.ToSlash(messageFile)))

		if line := sarifContextLine(err); line > 0 {
			physical.WithRegion(sarif.NewSimpleRegion(line, line))
		}

		return sarif.NewLocation().WithPhysicalLocation(physical)
	}

	if commit.Hash == "" {
		return nil
	}

	logical := sarif.NewLogicalLocation().
		WithKind("commit").
		WithName(truncateSHA(commit.Hash)).
		WithFullyQualifiedName(commit.Hash)

	return sarif.NewLocation().WithLogicalLocations([]*sarif.LogicalLocation{logical})
}

// sarifContextLine extracts the 1-indexed message line a finding points at,
// or 0 when the finding has no line-level context.
func sarifContextLine(err domain.ValidationError) int {
	line, convErr := strconv.Atoi(err.Context[diagnostics.ContextLine])
	if convErr != nil || line < 1 {
		return 0
	}

	return line
}

// sarifFingerprint gives each finding a stable identity across runs so
// aggregators can track it: same commit, check, line, and reason hash to
// the same fingerprint regardless of message wording or tool version.
func sarifFingerprint(commitHash, ruleID string, err domain.ValidationError) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		commitHash,
		ruleID,
		string(err.Diagnostic.Reason),
		err.Context[diagnostics.ContextLine],
	}, "\x00")))

	return hex.EncodeToString(sum[:16])
}
