// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package validation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

var (
	// ErrRepositoryRequired indicates the target needs repository access.
	ErrRepositoryRequired = errors.New("repository required for this validation target")
	// ErrRuleBuilderRequired indicates the service has no rule builder dependency.
	ErrRuleBuilderRequired = errors.New("rule builder required")
	// ErrMessageReaderRequired indicates the service has no message reader dependency.
	ErrMessageReaderRequired = errors.New("message reader required")
	// ErrUnknownValidationTarget indicates an unsupported target type.
	ErrUnknownValidationTarget = errors.New("unknown validation target type")
	// ErrCommitCountNotPositive indicates a non-positive count target.
	ErrCommitCountNotPositive = errors.New("commit count must be positive")
	// ErrEmptyCommitMessage indicates a message target contained no commit message.
	ErrEmptyCommitMessage = domain.ErrEmptyCommitMessage
)

// Rules groups the rules built for a validation request.
type Rules struct {
	Commit     []domain.CommitRule
	Repository []domain.RepositoryRule
}

// Request contains all inputs needed to run validation.
type Request struct {
	BaseBranchCandidates []string
	BaseBranchRequired   bool
	Config               settings.Config
	ConfigSources        []string
	ExcludeRules         []string
	IgnoreRepoRules      bool
	RuleFilter           []string
	Target               Target
	WarningDetails       []domain.Warning
}

// OpenRepositoryFunc opens the repository for a validation request.
type OpenRepositoryFunc func(context.Context) (domain.Repository, error)

// RuleBuilder builds commit and repository rules for a request.
type RuleBuilder func(settings.Config, []string, []string, string) (Rules, error)

// MessageReader reads commit message content for message-file validation.
type MessageReader func(string) (string, error)

// MissingRepositoryFunc reports whether an error means no Git repository exists.
type MissingRepositoryFunc func(error) bool

// Service coordinates validation without depending on CLI framework details.
type Service struct {
	BuildRules          RuleBuilder
	IsMissingRepository MissingRepositoryFunc
	Logger              domain.Logger
	Now                 func() time.Time
	OpenRepository      OpenRepositoryFunc
	ReadMessage         MessageReader
}

// Validate runs validation and returns a domain report.
func (s Service) Validate(ctx context.Context, req Request) (domain.Report, error) {
	s = s.withDefaults()

	logger := s.Logger
	logger.Trace("Entering validation service", "target_type", req.Target.Type)

	defer func() {
		logger.Trace("Exiting validation service", "target_type", req.Target.Type)
	}()

	repoResult, err := s.createRepository(ctx, req.Target)
	if err != nil {
		return domain.Report{}, err
	}

	baseBranchRequired := req.BaseBranchRequired && isBaseBranchTarget(req.Target, req.BaseBranchCandidates)

	baseBranch, branchWarnings, err := s.resolveBaseBranch(ctx, req.BaseBranchCandidates, repoResult.branchAnalyzer, baseBranchRequired)
	if err != nil {
		return domain.Report{}, err
	}

	req.WarningDetails = append(req.WarningDetails, branchWarnings...)
	req.Target = rebuildTarget(req.Target, req.BaseBranchCandidates, baseBranch)
	req.ExcludeRules = excludeBranchRulesWhenUnresolved(baseBranch, req.ExcludeRules)

	if s.BuildRules == nil {
		return domain.Report{}, ErrRuleBuilderRequired
	}

	rules, err := s.BuildRules(req.Config, req.RuleFilter, req.ExcludeRules, baseBranch)
	if err != nil {
		return domain.Report{}, err
	}

	if err := ensureRepoRulesCompatible(repoResult.branchAnalyzer, rules.Repository, req.IgnoreRepoRules, req.Target); err != nil {
		return domain.Report{}, err
	}

	logger.Debug("Rules created", "commit_rules", len(rules.Commit), "repo_rules", len(rules.Repository), "filter", req.RuleFilter, "exclude", req.ExcludeRules)

	vc := domain.NewValidationContext(rules.Commit, rules.Repository, repoResult.branchAnalyzer, req.Config, logger, domain.ValidationContextOptions{
		CommitReader:    repoResult.commitReader,
		IgnoreRepoRules: req.IgnoreRepoRules,
	})

	report, err := s.validateTarget(ctx, req.Target, vc, domain.ReportInput{
		ConfigSources:  req.ConfigSources,
		WarningDetails: req.WarningDetails,
		MessageFile:    messageFilePath(req.Target),
	})
	if err != nil {
		return domain.Report{}, err
	}

	logger.Debug("Validation completed", "passed", report.Summary.AllPassed, "commit_count", len(report.Commits), "failed_commits", report.Summary.FailedCommits)

	return report, nil
}

// messageFilePath returns the commit message file path for message-file
// targets. Stdin ("-") has no path a formatter could anchor to.
func messageFilePath(target Target) string {
	if !target.IsMessageFile() || target.Source == "-" {
		return ""
	}

	return target.Source
}

type repositoryResult struct {
	branchAnalyzer domain.BranchAnalyzer
	commitReader   domain.CommitReader
}

func (s Service) withDefaults() Service {
	if s.Now == nil {
		s.Now = time.Now
	}

	if s.IsMissingRepository == nil {
		s.IsMissingRepository = func(err error) bool {
			return errors.Is(err, domain.ErrNotGitRepository)
		}
	}

	return s
}

func (s Service) createRepository(ctx context.Context, target Target) (repositoryResult, error) {
	if s.OpenRepository == nil {
		return repositoryResult{}, nil
	}

	repo, err := s.OpenRepository(ctx)
	if err == nil {
		return repositoryResult{branchAnalyzer: repo, commitReader: repo}, nil
	}

	if target.IsMessageFile() && s.IsMissingRepository(err) {
		s.Logger.Debug("Skipping repository for message-file validation", "error", err)

		return repositoryResult{}, nil
	}

	return repositoryResult{}, err
}

func (s Service) resolveBaseBranch(ctx context.Context, candidates []string, repo domain.BranchAnalyzer, required bool) (string, []domain.Warning, error) {
	if len(candidates) == 0 {
		return "", nil, nil
	}

	if repo == nil {
		s.Logger.Debug("Skipping base branch resolution without repository", "candidates", strings.Join(candidates, ", "))

		return "", nil, nil
	}

	for idx, candidate := range candidates {
		exists, err := repo.BranchExists(ctx, candidate)
		if err != nil {
			return "", nil, fmt.Errorf("resolve base branch %q: %w", candidate, err)
		}

		if exists {
			if idx > 0 {
				tried := strings.Join(candidates[:idx], ", ")
				s.Logger.Info("Base branch fallback", "tried", tried, "resolved", candidate)

				return candidate, []domain.Warning{{
					Code: domain.WarnBaseBranchFallback,
					Context: map[string]string{
						domain.WarningContextResolved: candidate,
						domain.WarningContextTried:    tried,
					},
				}}, nil
			}

			s.Logger.Debug("Base branch resolved", "branch", candidate)

			return candidate, nil, nil
		}

		s.Logger.Debug("Base branch candidate not found", "candidate", candidate)
	}

	allCandidates := strings.Join(candidates, ", ")
	if required {
		return "", nil, baseBranchNotFoundError(candidates)
	}

	// Inline the candidate list into the human-readable message; zerolog's
	// ConsoleWriter appends key=value pairs to the message without a separator,
	// producing fragments like "skipping branch validation candidates=banana"
	// that read like a typo. The structured field is still emitted at debug
	// level for grep-ability.
	s.Logger.Warn(fmt.Sprintf("No base branch found (tried: %s), skipping branch validation", allCandidates))
	s.Logger.Debug("Base branch candidates exhausted", "candidates", allCandidates)

	return "", []domain.Warning{{
		Code: domain.WarnBaseBranchNotFound,
		Context: map[string]string{
			domain.WarningContextCandidates: allCandidates,
		},
	}}, nil
}

func rebuildTarget(target Target, candidates []string, resolvedBranch string) Target {
	if !isBaseBranchTarget(target, candidates) {
		return target
	}

	firstCandidate := candidates[0]
	switch {
	case resolvedBranch == firstCandidate:
		return target
	case resolvedBranch != "":
		return BaseBranchTarget(resolvedBranch)
	default:
		return DefaultTarget()
	}
}

func isBaseBranchTarget(target Target, candidates []string) bool {
	if len(candidates) == 0 {
		return false
	}

	return target.Type == TargetTypeRange && target.Source == candidates[0] && target.Target == refHead
}

func baseBranchNotFoundError(candidates []string) domain.ValidationError {
	allCandidates := strings.Join(candidates, ", ")

	return domain.New(diagnostics.SourceIDValidation, diagnostics.ReasonRangeNotFound,
		"base branch not found: "+allCandidates).
		WithContextMap(diagnostics.Context{
			diagnostics.ContextRef:      allCandidates,
			diagnostics.ContextExpected: "existing base branch",
		})
}

func excludeBranchRulesWhenUnresolved(baseBranch string, excludeRules []string) []string {
	if baseBranch != "" {
		return excludeRules
	}

	for _, excluded := range excludeRules {
		if strings.EqualFold(excluded, rulecatalog.BranchAhead) {
			return excludeRules
		}
	}

	return append(excludeRules, rulecatalog.BranchAhead)
}

func ensureRepoRulesCompatible(repo domain.BranchAnalyzer, repoRules []domain.RepositoryRule, ignoreRepoRules bool, target Target) error {
	if repo == nil && len(repoRules) > 0 && !ignoreRepoRules && !target.IsMessageFile() {
		return domain.New(diagnostics.SourceIDValidation, diagnostics.ReasonRepoRuleNoRepo,
			"repository rules configured but not in a git repository (use --ignore-repo-rules to continue anyway)")
	}

	return nil
}

func (s Service) validateTarget(ctx context.Context, target Target, vc *domain.ValidationContext, input domain.ReportInput) (domain.Report, error) {
	s.Logger.Trace("Entering ValidateTarget", "target_type", target.Type, "source", target.Source)

	defer func() {
		s.Logger.Trace("Exiting ValidateTarget", "target_type", target.Type)
	}()

	switch target.Type {
	case TargetTypeCommit, TargetTypeCommits, TargetTypeRange, TargetTypeCount:
		commits, err := fetchTargetCommits(ctx, target, vc)
		if err != nil {
			return domain.Report{}, err
		}

		return s.buildMultipleCommitReport(ctx, commits, vc, input)
	case TargetTypeMessage:
		return s.validateMessage(ctx, target.Source, vc, input)
	default:
		return domain.Report{}, fmt.Errorf("%w: %s", ErrUnknownValidationTarget, target.Type)
	}
}

func fetchTargetCommits(ctx context.Context, target Target, vc *domain.ValidationContext) ([]domain.Commit, error) {
	reader, err := requireCommitReader(vc)
	if err != nil {
		return nil, err
	}

	if target.Type == TargetTypeCount {
		if err := ValidateCommitCount(target.Count); err != nil {
			return nil, err
		}
	}

	if err := checkCancelled(ctx); err != nil {
		return nil, err
	}

	switch target.Type {
	case TargetTypeCommit:
		commit, err := reader.GetCommit(ctx, target.Source)
		if err != nil {
			return nil, err
		}

		return []domain.Commit{commit}, nil
	case TargetTypeCommits:
		commits := make([]domain.Commit, 0, len(target.Refs))
		for _, ref := range target.Refs {
			if err := checkCancelled(ctx); err != nil {
				return nil, err
			}

			commit, err := reader.GetCommit(ctx, ref)
			if err != nil {
				return nil, err
			}

			commits = append(commits, commit)
		}

		return commits, nil
	case TargetTypeRange:
		return reader.GetCommitRange(ctx, target.Source, target.Target)
	case TargetTypeCount:
		return reader.GetHeadCommits(ctx, target.Count)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownValidationTarget, target.Type)
	}
}

func requireCommitReader(vc *domain.ValidationContext) (domain.CommitReader, error) {
	if vc.CommitReader == nil {
		return nil, ErrRepositoryRequired
	}

	return vc.CommitReader, nil
}

func checkCancelled(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (s Service) validateMessage(ctx context.Context, filePath string, vc *domain.ValidationContext, input domain.ReportInput) (domain.Report, error) {
	if s.ReadMessage == nil {
		return domain.Report{}, ErrMessageReaderRequired
	}

	message, err := s.ReadMessage(filePath)
	if err != nil {
		return domain.Report{}, err
	}

	message = strings.TrimSpace(message)
	if message == "" {
		return domain.Report{}, ErrEmptyCommitMessage
	}

	commit := domain.ParseCommitMessage(message)

	return s.buildSingleCommitReport(ctx, commit, vc, input)
}

func (s Service) buildSingleCommitReport(ctx context.Context, commit domain.Commit, vc *domain.ValidationContext, input domain.ReportInput) (domain.Report, error) {
	return s.buildMultipleCommitReport(ctx, []domain.Commit{commit}, vc, input)
}

func (s Service) buildMultipleCommitReport(ctx context.Context, commits []domain.Commit, vc *domain.ValidationContext, input domain.ReportInput) (domain.Report, error) {
	validationResults, ignoreInfo, err := domain.ValidateCommits(ctx, commits, vc)
	if err != nil {
		// A cancelled run leaves un-validated commits as zero-value results,
		// which have no failures and would render as "passed". Propagate the
		// cancellation instead of building a false-pass report; the CLI maps
		// context.Canceled to a non-zero exit.
		return domain.Report{}, err
	}

	input, repoErrors := s.prepareReportInput(ctx, vc, input)

	return domain.BuildReport(
		validationResults,
		repoErrors,
		vc.ReportBuildContext(),
		&ignoreInfo,
		input,
	), nil
}

func (s Service) prepareReportInput(ctx context.Context, vc *domain.ValidationContext, input domain.ReportInput) (domain.ReportInput, []domain.ValidationError) {
	var (
		repoErrors   []domain.ValidationError
		repoRuleInfo map[string]string
	)

	if !vc.IgnoreRepoRules && vc.BranchAnalyzer != nil {
		repoErrors, repoRuleInfo = domain.ValidateRepositoryWithInfo(ctx, vc.RepoRules, vc.BranchAnalyzer, vc.Logger)
	}

	input.Timestamp = s.Now()
	input.RepoRuleInfo = repoRuleInfo

	return input, repoErrors
}
