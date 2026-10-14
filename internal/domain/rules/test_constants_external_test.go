// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules_test

const (
	testBaseFormEndingED       = "Base form ending with 'ed'"
	testBaseFormEndingS        = "Base form ending with 's'"
	testCommitHashABC1234      = "abc1234"
	testCommitHashDEF5678      = "def5678"
	testCompanyWildcard        = "*@company.com"
	testConventionalAPIScope   = "api"
	testConventionalFeatAdd    = "feat: add feature"
	testConventionalFeatColon  = "feat:"
	testConventionalFixType    = "fix"
	testEmailJane              = "jane@example.com"
	testEmailJohn              = "john@example.com"
	testJiraBodyDescription    = "This is a description"
	testJiraDescription        = "Description"
	testJiraProjectPROJ        = "PROJ"
	testJiraProjectTEAM        = "TEAM"
	testJiraSubjectFeatPROJ    = "feat: add feature PROJ-123"
	testMissingRequiredSignoff = "Missing required sign-off"
	testMultiSignoffMessage    = `Fix bug
Signed-off-by: Laval Lion <laval.lion@cavora.org>
Signed-off-by: Cragger Crocodile <cragger@svamp.org>`
	testSubjectAddNewFeature = "Add new feature"
	testSubjectFeatAPIScope  = "feat(api): add endpoint"
	testSubjectFeatUIAPI     = "feat(ui,api): add feature"
	testSubjectFeatUpper     = "feat: Add new feature"
	testSubjectTestCommit    = "Test commit"
	testTempTicketPattern    = "^TEMP-.*"
	testUseEmailOnlyMessage  = "use email only"
	testWordAdd              = "add"
	testWordAdded            = "added"
	testWordCreat            = "creat"
	testWordCreating         = "creating"
	testWordFix              = "fix"
	testWordFixed            = "fixed"
	testWordImplemented      = "implemented"
	testWordImplement        = "implement"
	testWordRecieve          = "recie" + "ve" // split so the misspell linter ignores the fixture
	testWordTeh              = "t" + "eh"
	testWordThe              = "the"
	testWordUpdat            = "updat"
)
