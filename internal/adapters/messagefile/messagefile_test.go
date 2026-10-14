// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package messagefile

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
)

func TestReadWithFS_ReadsFromFileOrStdin(t *testing.T) {
	env := testhelpers.NewTestEnv(t)

	tests := []struct {
		name        string
		setupTest   func() (string, io.Reader)
		isTerminal  bool
		wantErr     error
		wantMessage string
	}{
		{
			name: "read_from_regular_file",
			setupTest: func() (string, io.Reader) {
				filePath := filepath.Join(env.RepoPath, "test-message.txt")
				content := "Test commit message\n\nDetailed description"
				env.FS.WriteFile(filePath, []byte(content))

				return filePath, nil
			},
			wantMessage: "Test commit message\n\nDetailed description",
		},
		{
			name: "non_existent_file",
			setupTest: func() (string, io.Reader) {
				return filepath.Join(env.RepoPath, "non-existent.txt"), nil
			},
			wantErr: os.ErrNotExist,
		},
		{
			name: "stdin_from_plain_reader",
			setupTest: func() (string, io.Reader) {
				return "-", strings.NewReader("feat: test message from pipe")
			},
			wantMessage: "feat: test message from pipe",
		},
		{
			// A real pipe is an *os.File, the shape `echo msg | gommitlint
			// validate -f -` hands over; only the terminal detector may refuse it.
			name: "stdin_from_os_pipe",
			setupTest: func() (string, io.Reader) {
				reader, writer, err := os.Pipe()
				check.NoError(t, err)
				t.Cleanup(func() { _ = reader.Close() })

				_, err = io.WriteString(writer, "feat: message from an os pipe")
				check.NoError(t, err)
				check.NoError(t, writer.Close())

				return "-", reader
			},
			wantMessage: "feat: message from an os pipe",
		},
		{
			name: "stdin_exceeds_size_limit",
			setupTest: func() (string, io.Reader) {
				overlarge := strings.Repeat("x", domain.MaxCommitMessageSize+1)

				return "-", strings.NewReader(overlarge)
			},
			wantErr: ErrCommitMessageTooLarge,
		},
		{
			name: "large_file_exceeds_size_limit",
			setupTest: func() (string, io.Reader) {
				filePath := filepath.Join(env.RepoPath, "large.txt")

				largeContent := make([]byte, domain.MaxCommitMessageSize+1)
				for i := range largeContent {
					largeContent[i] = 'x'
				}

				env.FS.WriteFile(filePath, largeContent)

				return filePath, nil
			},
			wantErr: filesystem.ErrFileTooLarge,
		},
		{
			name: "empty_file",
			setupTest: func() (string, io.Reader) {
				filePath := filepath.Join(env.RepoPath, "empty.txt")
				env.FS.WriteFile(filePath, []byte(""))

				return filePath, nil
			},
			wantMessage: "",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			filePath, stdin := testCase.setupTest()
			isTerminal := func(fd int) bool { return testCase.isTerminal }

			var (
				got string
				err error
			)

			if stdin != nil {
				got, err = ReadWithFS(filePath, isTerminal, stdin, nil)
			} else {
				got, err = ReadWithFS(filePath, isTerminal, nil, env.SigningFS)
			}

			if testCase.wantErr != nil {
				check.ErrorIs(t, err, testCase.wantErr)

				return
			}

			check.NoError(t, err)
			check.Equal(t, testCase.wantMessage, got)
		})
	}
}

func TestReadWithStdin_RejectsTerminalInput(t *testing.T) {
	stdin := os.Stdin
	isTerminal := func(fd int) bool { return true }

	_, err := ReadWithFS("-", isTerminal, stdin, nil)

	check.ErrorIs(t, err, ErrStdinTerminalMode)
}
