// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Package hooks provides embedded git hook scripts.
//
// The scripts in this package are embedded into the gommitlint binary
// at build time using go:embed directives. This allows the hooks to be
// version controlled, linted with shellcheck, and formatted with shfmt
// while still being bundled into the binary with zero runtime dependencies.
package hooks

import (
	_ "embed"
	"strings"
)

// CommitMsgHook contains the embedded commit-msg hook script for local repositories.
//
//go:embed commit-msg.sh
var CommitMsgHook string

// PrePushHook contains the embedded pre-push hook script for local repositories.
//
//go:embed pre-push.sh
var PrePushHook string

// CommitMsgPlugin contains the embedded commit-msg plugin script for global hooks.
//
//go:embed commit-msg-plugin.sh
var CommitMsgPlugin string

// PrePushPlugin contains the embedded pre-push plugin script for global hooks.
//
//go:embed pre-push-plugin.sh
var PrePushPlugin string

// DispatcherTemplate contains the embedded dispatcher script template.
// The string HOOK_TYPE should be replaced with the actual hook type at runtime.
//
//go:embed dispatcher.sh
var DispatcherTemplate string

// InstalledBinaryPlaceholder marks where a generated hook records the absolute
// path of the gommitlint binary that installed it. Hooks try PATH first and
// fall back to this path, so a shell without the developer's PATH still
// validates instead of silently passing.
const InstalledBinaryPlaceholder = "@GOMMITLINT_INSTALLED_BIN@"

// WithInstalledBinary substitutes the installed-binary placeholder. The path
// lands inside a double-quoted POSIX shell string, so the characters that are
// special there are escaped; an empty path leaves the fallback disabled and
// the hook fails closed when PATH has no gommitlint.
func WithInstalledBinary(script, path string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "`", "\\`")

	return strings.ReplaceAll(script, InstalledBinaryPlaceholder, replacer.Replace(path))
}
