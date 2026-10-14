// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestDefaultPreservesTheDeclaredRelease(t *testing.T) {
	t.Parallel()

	config, err := os.ReadFile("../../.goreleaser.yaml")
	check.NoError(t, err)

	images, err := os.ReadFile("../../packaging/release-platforms.json")
	check.NoError(t, err)

	selected, plan, err := selectPlan(config, images, "")
	check.NoError(t, err)
	check.Equal(t, config, selected)
	check.Equal(t, 2, len(plan.Musl))
	check.Equal(t, 2, len(plan.Images["alpine"]))
	check.Equal(t, 2, len(plan.Images["distroless"]))
}

func TestSelectionKeepsArtifactsAndImagesOnTheSamePlatforms(t *testing.T) {
	t.Parallel()

	config := []byte("builds:\n  - id: linux\n    goos: [linux]\n    goarch: [amd64, arm64]\n  - id: mac\n    goos: [darwin]\n    goarch: [arm64]\narchives:\n  - id: archive\n")
	images := []byte(`{"images":{"app":[{"platform":"linux/amd64","build-args":["ARCH=amd64"]},{"platform":"linux/arm64","build-args":["ARCH=arm64"]}]},"musl":[{"platform":"linux/amd64","interpreter":"amd-loader","output":"amd-bin"},{"platform":"linux/arm64","interpreter":"arm-loader","output":"arm-bin"}]}`)

	for _, architecture := range []string{"amd64", "arm64"} {
		t.Run(architecture, func(t *testing.T) {
			t.Parallel()

			platform := "linux/" + architecture
			selected, plan, err := selectPlan(config, images, platform)
			check.NoError(t, err)
			check.Equal(t, 1, len(plan.Images["app"]))
			check.Equal(t, platform, plan.Images["app"][0].Platform)
			check.Equal(t, 1, len(plan.Musl))
			check.Equal(t, platform, plan.Musl[0].Platform)

			var decoded struct {
				Builds []struct {
					Skip   bool                `yaml:"skip"`
					Ignore []map[string]string `yaml:"ignore"`
				} `yaml:"builds"`
				Archives []map[string]string `yaml:"archives"`
			}

			check.NoError(t, yaml.Unmarshal(selected, &decoded))

			other := "arm64"
			if architecture == "arm64" {
				other = "amd64"
			}

			check.False(t, decoded.Builds[0].Skip)
			check.True(t, decoded.Builds[1].Skip)
			check.Equal(t, []map[string]string{{"goos": "linux", "goarch": other}}, decoded.Builds[0].Ignore)
			check.Equal(t, []map[string]string{{"id": "archive"}}, decoded.Archives)
		})
	}

	for _, selection := range []string{"linux/s390x", "linux/amd64,linux/amd64", "linux/amd64,", " linux/amd64", "amd64"} {
		_, _, err := selectPlan(config, images, selection)
		check.ErrorIs(t, err, errSelection, selection)
	}
}

func TestSelectionRefusesAnIgnoredBinaryPlatform(t *testing.T) {
	t.Parallel()

	config := []byte("builds:\n  - goos: [linux]\n    goarch: [arm64]\n    ignore: [{goos: linux, goarch: arm64}]\n")
	images := []byte(`{"images":{"app":[{"platform":"linux/arm64"}]},"musl":[{"platform":"linux/arm64"}]}`)

	_, _, err := selectPlan(config, images, "linux/arm64")
	check.ErrorIs(t, err, errSelection)
}

func TestSelectionPreservesVariantIgnoresAndRejectsDisabledBuilds(t *testing.T) {
	t.Parallel()

	config := []byte("builds:\n  - goos: [linux]\n    goarch: [amd64]\n    goamd64: [v1, v2]\n    ignore: [{goos: linux, goarch: amd64, goamd64: v2}]\n")
	selected, err := selectGoBuilds(config, []string{"linux/amd64"})
	check.NoError(t, err)
	check.Contains(t, string(selected), "goamd64: v2")

	for _, skip := range []string{"true", "'true'"} {
		config := []byte("builds:\n  - goos: [linux]\n    goarch: [amd64]\n    skip: " + skip + "\n")
		_, err := selectGoBuilds(config, []string{"linux/amd64"})
		check.ErrorIs(t, err, errSelection)
	}
}

func TestInvalidSelectionWritesNothing(t *testing.T) {
	t.Chdir(t.TempDir())
	check.NoError(t, os.WriteFile("config.yaml", []byte("builds: []\n"), 0o600))
	check.NoError(t, os.WriteFile("images.json", []byte(`{"images":{},"musl":[]}`), 0o600))

	output := filepath.Join(t.TempDir(), "plan")
	check.ErrorIs(t, writePlan("config.yaml", "images.json", "not-a-platform", output), errSelection)
	_, err := os.Stat(output)
	check.True(t, os.IsNotExist(err))
}

func TestSelectionRejectsUnsupportedMatrices(t *testing.T) {
	t.Parallel()

	for _, build := range []string{
		"goos: [linux]\n    goarch: [7]",
		"goos: [linux]\n    goarch: [amd64]\n    targets: [linux_amd64_v1]",
		"goos: [linux]\n    goarch: [amd64]\n    skip: '{{ .Env.SKIP }}'",
	} {
		_, err := selectGoBuilds([]byte("builds:\n  - "+build+"\n"), []string{"linux/amd64"})
		check.ErrorIs(t, err, errMatrix)
	}
}
