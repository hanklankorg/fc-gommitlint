// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

/*
Package internal contains the private application code for gommitlint.

Production code is organized into domain (core business logic), app (use-case
coordination), and adapters (external integrations) following hexagonal architecture.
Package tests stay beside their code; cross-package integration scenarios and
repository-policy tests live under test/integration and test/policy.

See docs/modules/ROOT/pages/architecture.adoc for architectural details.
*/
package internal
