// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	sarif "github.com/owenrumney/go-sarif/v3/pkg/report/v210/sarif"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
)

// FuzzOutputStability verifies equivalent JSON APIs, field preservation, and
// schema-valid deterministic SARIF with stable finding fingerprints.
func FuzzOutputStability(f *testing.F) {
	f.Add("abc123", "feat: add thing", "subject is too long", uint16(1), false)
	f.Add("", "message from stdin", "failure\nwith control\ttext", uint16(0), true)

	f.Fuzz(func(t *testing.T, hash, subject, message string, line uint16, messageFile bool) {
		if len(hash)+len(subject)+len(message) > 64*1024 || !utf8.ValidString(hash) || !utf8.ValidString(subject) || !utf8.ValidString(message) {
			t.Skip()
		}

		buildReport := func(hash, message string) domain.Report {
			validationError := domain.New(rulecatalog.Subject, diagnostics.ReasonSubjectTooLong, message).
				WithContextMap(diagnostics.Context{diagnostics.ContextLine: strconv.Itoa(int(line))})

			report := domain.Report{
				Metadata: domain.ReportMetadata{Timestamp: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)},
				Summary: domain.ReportSummary{
					AllPassed: false, TotalCommits: 1, ValidatedCommits: 1, FailedCommits: 1,
					FailedRules: map[string]int{rulecatalog.Subject: 1},
				},
				Commits: []domain.CommitReport{{
					Commit: domain.Commit{Hash: hash, Subject: subject},
					RuleResults: []domain.RuleReport{{
						Name: rulecatalog.Subject, Status: domain.StatusFailed, Errors: []domain.ValidationError{validationError},
					}},
				}},
			}
			if messageFile {
				report.Metadata.MessageFile = ".git/COMMIT_EDITMSG"
			}

			return report
		}

		report := buildReport(hash, message)

		jsonString := JSON(report, nil)
		if repeated := JSON(report, nil); repeated != jsonString {
			t.Fatal("JSON output is not deterministic")
		}

		var streamed bytes.Buffer
		if err := JSONTo(&streamed, report, nil); err != nil {
			t.Fatalf("stream JSON report: %v", err)
		}

		if strings.TrimSuffix(streamed.String(), "\n") != jsonString {
			t.Fatal("JSON and JSONTo produced different documents")
		}

		var parsedJSON jsonReport
		if err := json.Unmarshal([]byte(jsonString), &parsedJSON); err != nil {
			t.Fatalf("parse JSON output: %v", err)
		}

		if len(parsedJSON.Commits) != 1 || parsedJSON.Commits[0].Hash != hash || parsedJSON.Commits[0].Subject != subject {
			t.Fatalf("commit fields changed in JSON output: %#v", parsedJSON.Commits)
		}

		if len(parsedJSON.Commits[0].Rules) != 1 || len(parsedJSON.Commits[0].Rules[0].Errors) != 1 {
			t.Fatalf("diagnostic structure changed in JSON output: %#v", parsedJSON.Commits[0].Rules)
		}

		gotError := parsedJSON.Commits[0].Rules[0].Errors[0]
		if gotError.Message != message || gotError.Context[diagnostics.ContextLine] != strconv.Itoa(int(line)) {
			t.Fatalf("diagnostic fields changed in JSON output: %#v", gotError)
		}

		firstSARIF := SARIF(report, SARIFOptions{ToolVersion: "fuzz"})

		secondSARIF := SARIF(report, SARIFOptions{ToolVersion: "fuzz"})
		if firstSARIF != secondSARIF {
			t.Fatal("SARIF output is not deterministic")
		}

		parsedSARIF, err := sarif.FromString(firstSARIF)
		if err != nil {
			t.Fatalf("parse SARIF output: %v", err)
		}

		if err := parsedSARIF.Validate(); err != nil {
			t.Fatalf("validate SARIF schema: %v", err)
		}

		if len(parsedSARIF.Runs) != 1 || len(parsedSARIF.Runs[0].Results) != 1 {
			t.Fatalf("SARIF finding count changed: %#v", parsedSARIF.Runs)
		}

		fingerprint := parsedSARIF.Runs[0].Results[0].PartialFingerprints[sarifFingerprintKey]

		decodedFingerprint, err := hex.DecodeString(fingerprint)
		if err != nil || len(decodedFingerprint) != 16 {
			t.Fatalf("invalid SARIF fingerprint %q: %v", fingerprint, err)
		}

		// The fingerprint is the finding's identity across runs: a reworded
		// message or a new tool version must not move it, and the commit it
		// is about must.
		reworded := SARIF(buildReport(hash, message+" reworded"), SARIFOptions{ToolVersion: "fuzz-next"})
		if got := parseSARIF(t, reworded).Runs[0].Results[0].PartialFingerprints[sarifFingerprintKey]; got != fingerprint {
			t.Fatalf("SARIF fingerprint depends on message or version: got %q want %q", got, fingerprint)
		}

		otherCommit := SARIF(buildReport(hash+"1", message), SARIFOptions{ToolVersion: "fuzz"})
		if got := parseSARIF(t, otherCommit).Runs[0].Results[0].PartialFingerprints[sarifFingerprintKey]; got == fingerprint {
			t.Fatalf("SARIF fingerprint ignores the commit: %q for both %q and %q", got, hash, hash+"1")
		}
	})
}
