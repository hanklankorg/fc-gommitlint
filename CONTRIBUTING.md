# Contributing

[![Conventional Commits](https://img.shields.io/badge/Conventional%20Commits-1.0.0-%23FE5196?style=for-the-badge&logo=conventionalcommits&logoColor=white)](https://conventionalcommits.org)
[![Code of Conduct](https://img.shields.io/badge/Simple%20Contributor%20Code%20of%20Conduct-1.0-4baaaa.svg?style=for-the-badge)](CODE_OF_CONDUCT.md)
[![DCO - developer certificate of origin](https://img.shields.io/badge/DCO-Developer%20Certificate%20of%20Origin-lightyellow?style=for-the-badge)](https://developercertificate.org/)

Use this guide when you contribute code, tests, documentation, or feedback.

## Ways to Contribute

You can:

- Fix or report a bug.

- Suggest enhancements to code, tests and documentation.

- Report or fix problems found during installation or in development environments.

- Identify missing information or functionality.

## Community Guideline

Be nice and respectful to each other.

We follow the [Simple Contributor Code of Conduct](CODE_OF_CONDUCT.md).

## File an Issue

Before creating an issue, check for an existing issue about the same topic.
If one exists, add relevant information there instead of creating a duplicate.
For suspected vulnerabilities, follow [SECURITY.md](SECURITY.md) instead of opening a public issue.

### Report a bug

Open an issue that summarizes the bug.
Include the affected version, reproduction steps, and expected and actual results.
Remove credentials and private repository information from attached diagnostics.
If you can set labels, select "bug".

### Suggest a feature

To request a new feature, summarize the desired functionality and its use case.
If you can set labels, select "feature" or "enhancement".

## Contribute Code, Documentation and more

Before contributing code, documentation, or other improvements:

- Discuss ideas before contributing to align with project goals.

- Check open issues and coordinate the work with the maintainers.
  Use an existing issue where possible.

- Follow the [Development Guide](docs/contributing/development.md) and the repository's code, test, and documentation conventions.

- Keep the change consistent with the project goals, architecture, and contribution guidelines.
  Maintainers can decline changes that do not meet those requirements.

- Read the [Pull Request Lifecycle](#pull-request-lifecycle).

- Submit contributions under the project's applicable licenses, following the **inbound=outbound** norm.
  Code is dual-licensed under **EUPL-1.2 OR GPL-3.0-or-later**, at the recipient's choice.
  Documentation and configuration use **CC0-1.0**.
  Your DCO sign-off acknowledges these contribution terms.

- Follow both signoff and cryptographic-signing requirements in the [Commit Guideline](#commit-guideline).

## Issues and Pull Request Feedback

**This project is actively maintained.**
Maintainers review and respond to issues and pull requests as time permits.
Feedback can be a question, a requested change, acceptance, or an explanation of why a proposal does not fit.

## Note

For substantial changes, discuss the proposal before implementation.
Agree on scope, review effort, and maintenance responsibility with the project.
Keep each issue or pull request focused on one topic.
Complete the requested fields and include relevant evidence.

## Pull Request Lifecycle

Use the standard Fork-and-Pull workflow:

1. Fork the repository.
2. Create a feature branch.
3. Make your changes with the required commit messages, signoffs, and signatures.
4. Submit a pull request to the main project.
5. Address review feedback.

## AI-Assisted Contributions

AI-assisted contributions must meet the same guidelines as other contributions.
You remain responsible for the submission:

- Review the complete change before submission.
- Keep commits properly rebased and consistent with the commit guidelines.
- Meet the project's quality standards.
- Be able to explain and defend the changes.
- Report which checks ran, which failed, and which were unavailable.

The DCO sign-off certifies that *you* have the right to submit the contribution, regardless of how it was authored.

[AGENTS.md](AGENTS.md) requires explicit user approval for commits, pushes, rebases, merges, resets, and tags.
These contribution guidelines do not grant that approval to an agent.

## Commit Guideline

### DCO - Signoff and Signing a Commit

**Two commit requirements:**

1. **DCO Sign-off** (`--signoff`): Record your certification in a `Signed-off-by` trailer.
2. **Cryptographic Signing** (`--gpg-sign`): Sign the commit with your configured signing key.

After reading the DCO, configuring your signing key, and reviewing the staged changes:

```bash
git commit --signoff --gpg-sign -m "fix: describe the change"
```

Signing-key setup and the distinction between sign-off and signature are explained in the optional [Git commit signing and DCO](https://codefloe.com/itiquette/docs/src/branch/main/development/notes/git-commit-signing.md) background note.

### Commit Standard

Gommitlint requires [Conventional Commits](https://www.conventionalcommits.org) and both [commit requirements](#commit-guideline).
Group related changes, keep unrelated work separate, and use an imperative subject describing what applying the commit changes.

## Reporting security issues

Report suspected vulnerabilities privately through [SECURITY.md](SECURITY.md).
Do not open a public issue before disclosure has been coordinated, including for findings reported by vulnerability scanners.
These contribution guidelines do not create an exception to that policy.

## Development Guidelines

For contributor setup and daily development, see the
[Development Guide](docs/contributing/development.md).
Maintainers preparing production releases must also follow the
[Maintainer Release Guide](docs/contributing/releasing.md).

`just verify` is the non-fixing gate: lint, tagged-harness compilation, and in-process tests.
`just verify-fix` applies lint fixes first, then runs the same gate with lint in allow-dirty mode.
Neither is a read-only filesystem sandbox or permission to run every documented workflow.
Black-box and external execution remain separate guarded local tiers under [AGENTS.md](AGENTS.md).
Compilation alone does not prove a passing external-binary test.

## Writing style and Translations

Keep prose brief and direct.
Link to long procedures instead of repeating them.
Use [one-sentence-per-line](https://sembr.org/) in Markdown and AsciiDoc.
English is primary. Translations are best effort.
Make the English source correct before updating translations.

### Translation contributions

Edit the intended `locale/<lang>/gommitlint.po`, preserving message IDs and placeholders.
The local [Development Guide: Translations](docs/contributing/development.md#translations) owns the complete add/update-language procedure, gettext prerequisites, generation commands, side effects, and checks.

## Config JSON Schema

Change settings types and their doc comments rather than hand-editing `schemas/gommitlint.schema.json`.
Follow the local [schema-generation procedure](docs/contributing/development.md#config-json-schema), including its embedded-binary and editor-version limits.

### Publishing to SchemaStore

The local [SchemaStore section](docs/contributing/development.md#publishing-to-schemastore) retains the complete catalog-entry example, submission location, and availability/version caveats.

## FLOSS Standards

This project aims to comply with the principles outlined in the following standards:

- License compliance with the [REUSE specification](https://reuse.software/)
- Commits in the [Conventional Commits format](https://www.conventionalcommits.org/en/v1.0.0/)
- Changelog in the [Keep-A-Changelog format](https://keepachangelog.com/en/1.1.0/)
