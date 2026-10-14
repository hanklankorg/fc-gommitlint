// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"encoding/json"
	"strings"
	"testing"

	sarif "github.com/owenrumney/go-sarif/v3/pkg/report/v210/sarif"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func sarifTestReport(messageFile string) domain.Report {
	report := snapshotReport(2)
	report.Metadata.MessageFile = messageFile

	return report
}

func parseSARIF(t *testing.T, doc string) *sarif.Report {
	t.Helper()

	var parsed sarif.Report

	check.NoError(t, json.Unmarshal([]byte(doc), &parsed), "SARIF output must be valid JSON")
	check.Len(t, parsed.Runs, 1)

	return &parsed
}

// validateSARIF reparses the emitted document and validates it against the
// canonical SARIF 2.1.0 JSON Schema embedded in go-sarif. Unlike parseSARIF,
// which only proves the output round-trips into the library's structs, this
// proves the formatter emits schema-conformant SARIF.
func validateSARIF(t *testing.T, doc string) {
	t.Helper()

	report, err := sarif.FromString(doc)
	check.NoError(t, err, "SARIF output must parse")
	check.NoError(t, report.Validate(), "SARIF output must satisfy the 2.1.0 schema")
}

// TestSARIF_SchemaValid pins the formatter's output against the real SARIF
// 2.1.0 schema in each location mode we support. These are integration-style
// guards: the schema is the authority, not our own assertions.
func TestSARIF_SchemaValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		report domain.Report
	}{
		{"history mode (logical locations)", sarifTestReport("")},
		{"message-file mode (physical locations)", sarifTestReport(".git/COMMIT_EDITMSG")},
		{"clean report (no violations)", domain.Report{}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			validateSARIF(t, SARIF(testCase.report, SARIFOptions{ToolVersion: "1.2.3"}))
		})
	}
}

func TestSARIF_HistoryModeUsesLogicalLocations(t *testing.T) {
	t.Parallel()

	doc := SARIF(sarifTestReport(""), SARIFOptions{ToolVersion: "1.2.3"})
	parsed := parseSARIF(t, doc)

	run := parsed.Runs[0]
	check.Equal(t, "gommitlint", *run.Tool.Driver.Name)
	check.Equal(t, "1.2.3", *run.Tool.Driver.Version)
	check.Len(t, run.Results, 2)

	result := run.Results[0]
	check.Equal(t, "GL305", *result.RuleID)
	check.Equal(t, "error", result.Level)
	check.Equal(t, "Line 1 is 74 characters (max 72)", *result.Message.Text)
	check.NotEmpty(t, result.PartialFingerprints["gommitlint/v1"])

	check.Len(t, result.Locations, 1)
	logical := result.Locations[0].LogicalLocations
	check.Len(t, logical, 1)
	check.Equal(t, "abc1234def", *logical[0].FullyQualifiedName)
	check.Equal(t, "commit", *logical[0].Kind)
	check.Nil(t, result.Locations[0].PhysicalLocation)

	// One rule descriptor, sourced from the GL catalog, despite two results.
	check.Len(t, run.Tool.Driver.Rules, 1)
	rule := run.Tool.Driver.Rules[0]
	check.Equal(t, "GL305", *rule.ID)
	check.Equal(t, "Body lines must fit the configured width", *rule.ShortDescription.Text)
	check.NotNil(t, rule.Help)
}

func TestSARIF_MessageFileModeUsesPhysicalLocations(t *testing.T) {
	t.Parallel()

	doc := SARIF(sarifTestReport(".git/COMMIT_EDITMSG"), SARIFOptions{})
	parsed := parseSARIF(t, doc)

	results := parsed.Runs[0].Results
	check.Len(t, results, 2)

	first := results[0].Locations[0].PhysicalLocation
	check.NotNil(t, first)
	check.Equal(t, ".git/COMMIT_EDITMSG", *first.ArtifactLocation.URI)
	check.Equal(t, 1, *first.Region.StartLine)

	second := results[1].Locations[0].PhysicalLocation
	check.Equal(t, 2, *second.Region.StartLine)
}

func TestSARIF_FingerprintsAreStableAndDistinct(t *testing.T) {
	t.Parallel()

	first := SARIF(sarifTestReport(""), SARIFOptions{})
	second := SARIF(sarifTestReport(""), SARIFOptions{})
	check.Equal(t, first, second, "same report must produce identical SARIF")

	parsed := parseSARIF(t, first)
	results := parsed.Runs[0].Results
	check.NotEqual(t, results[0].PartialFingerprints["gommitlint/v1"],
		results[1].PartialFingerprints["gommitlint/v1"],
		"violations on different lines must have distinct fingerprints")
}

// TestSARIF_FingerprintIncludesTheCommit pins the identity half of the
// fingerprint contract: the same check, reason and line on two commits are
// two findings for an aggregator, and the same commit reported again is one.
func TestSARIF_FingerprintIncludesTheCommit(t *testing.T) {
	t.Parallel()

	tooLong := domain.New(rulecatalog.Subject, diagnostics.ReasonSubjectTooLong, "Subject too long")
	report := newReportBuilder().
		failingCommit("abc1234", "feat: one", rulecatalog.Subject, tooLong).
		failingCommit("def5678", "feat: two", rulecatalog.Subject, tooLong).
		failingCommit("abc1234", "feat: one", rulecatalog.Subject, tooLong).
		build()

	results := parseSARIF(t, SARIF(report, SARIFOptions{})).Runs[0].Results
	check.Len(t, results, 3)

	first := results[0].PartialFingerprints["gommitlint/v1"]
	check.NotEmpty(t, first)
	check.NotEqual(t, first, results[1].PartialFingerprints["gommitlint/v1"],
		"the same violation on another commit is another finding")
	check.Equal(t, first, results[2].PartialFingerprints["gommitlint/v1"],
		"the same commit reported again is the same finding")
}

func TestSARIF_MessagesStayEnglishUnderLocale(t *testing.T) {
	t.Parallel()

	// The formatter takes no localizer at all — this guards the contract
	// that SARIF text is locale-independent.
	doc := SARIF(sarifTestReport(""), SARIFOptions{})
	check.Contains(t, doc, "Line 1 is 74 characters (max 72)")
	check.NotContains(t, doc, "Rad 1")
}

func TestSARIF_DocumentEnvelope(t *testing.T) {
	t.Parallel()

	doc := SARIF(sarifTestReport(""), SARIFOptions{})

	check.Contains(t, doc, `"version": "2.1.0"`)
	check.True(t, strings.Contains(doc, "sarif-schema-2.1.0.json"), "must reference the 2.1.0 schema")
}
