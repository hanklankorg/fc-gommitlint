// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package gitcli

import (
	"os"
	"slices"
	"strings"
)

// Environment returns the process environment for a product git call.
//
// git reads a few variables to locate a repository instead of discovering it
// from the directory it runs in, and it exports them to its hooks: GIT_DIR
// whenever the repository was not found as ./.git (a subdirectory, git -C, a
// linked worktree), GIT_INDEX_FILE for a partial commit. A product call that
// kept them would act on the hook's repository instead of the one it was given,
// so they are dropped. Configuration variables such as GIT_CONFIG_GLOBAL stay:
// the tool honours a user's git configuration.
func Environment() []string {
	relocating := [...]string{
		"GIT_DIR",
		"GIT_WORK_TREE",
		"GIT_COMMON_DIR",
		"GIT_INDEX_FILE",
		"GIT_OBJECT_DIRECTORY",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	}

	return slices.DeleteFunc(os.Environ(), func(entry string) bool {
		name, _, _ := strings.Cut(entry, "=")

		return slices.Contains(relocating[:], name)
	})
}
