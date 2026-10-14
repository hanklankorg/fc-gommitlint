// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Command releaseplan selects a release subset without changing the declared
// defaults. The same plan supplies GoReleaser, image builds, and musl builds.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type imageBuild struct {
	Platform  string   `json:"platform"`
	BuildArgs []string `json:"build-args"`
}

type muslBuild struct {
	Platform    string `json:"platform"`
	Interpreter string `json:"interpreter"`
	Output      string `json:"output"`
}

type releasePlan struct {
	Images map[string][]imageBuild `json:"images"`
	Musl   []muslBuild             `json:"musl"`
}

var (
	errSelection = errors.New("invalid release platform selection")
	errMatrix    = errors.New("unsupported GoReleaser build matrix")
)

func main() {
	config := flag.String("config", ".goreleaser.yaml", "repository-relative GoReleaser configuration")
	images := flag.String("images", "packaging/release-platforms.json", "repository-relative image and musl declaration")
	platforms := flag.String("platforms", os.Getenv("RELEASE_PLATFORMS"), "comma-separated declared OS/architecture pairs; empty keeps the full release")
	output := flag.String("output", "", "directory for the selected configuration and plan")

	flag.Parse()

	if *output == "" || flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}

	if err := writePlan(*config, *images, *platforms, *output); err != nil {
		fmt.Fprintln(os.Stderr, "release plan:", err)
		os.Exit(1)
	}
}

func writePlan(configPath, imagesPath, platforms, output string) error {
	root, err := os.OpenRoot(".")
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	config, err := root.ReadFile(configPath)
	if err != nil {
		return err
	}

	images, err := root.ReadFile(imagesPath)
	if err != nil {
		return err
	}

	selected, plan, err := selectPlan(config, images, platforms)
	if err != nil {
		return err
	}

	encoded, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(output, 0o700); err != nil {
		return err
	}

	destination, err := os.OpenRoot(output)
	if err != nil {
		return err
	}
	defer func() { _ = destination.Close() }()

	if err := destination.WriteFile("goreleaser.yaml", selected, 0o600); err != nil {
		return err
	}

	return destination.WriteFile("release-plan.json", append(encoded, '\n'), 0o600)
}

func selectPlan(config, images []byte, platforms string) ([]byte, releasePlan, error) {
	var plan releasePlan
	if err := json.Unmarshal(images, &plan); err != nil {
		return nil, plan, err
	}

	if platforms == "" {
		return config, plan, nil
	}

	wanted, err := requestedPlatforms(plan, platforms)
	if err != nil {
		return nil, plan, err
	}

	selected, err := selectGoBuilds(config, wanted)
	if err != nil {
		return nil, plan, err
	}

	for role, builds := range plan.Images {
		plan.Images[role] = slices.DeleteFunc(builds, func(build imageBuild) bool { return !slices.Contains(wanted, build.Platform) })
	}

	plan.Musl = slices.DeleteFunc(plan.Musl, func(build muslBuild) bool { return !slices.Contains(wanted, build.Platform) })

	return selected, plan, nil
}

func requestedPlatforms(plan releasePlan, platforms string) ([]string, error) {
	wanted := strings.Split(platforms, ",")

	for i, platform := range wanted {
		if strings.TrimSpace(platform) != platform || strings.Count(platform, "/") != 1 || slices.Contains(wanted[:i], platform) {
			return nil, fmt.Errorf("platform %q is malformed or duplicated: %w", platform, errSelection)
		}
		// Supported image platforms come from the consumer's declaration,
		// never from a host-architecture guess or a second built-in matrix.
		for role, builds := range plan.Images {
			if !slices.ContainsFunc(builds, func(build imageBuild) bool { return build.Platform == platform }) {
				return nil, fmt.Errorf("platform %q is not declared for image role %s: %w", platform, role, errSelection)
			}
		}

		if !slices.ContainsFunc(plan.Musl, func(build muslBuild) bool { return build.Platform == platform }) {
			return nil, fmt.Errorf("platform %q has no declared musl build: %w", platform, errSelection)
		}
	}

	return wanted, nil
}

func selectGoBuilds(config []byte, wanted []string) ([]byte, error) {
	var document map[string]any

	if err := yaml.Unmarshal(config, &document); err != nil {
		return nil, err
	}

	builds, ok := document["builds"].([]any)
	if !ok || len(builds) == 0 {
		return nil, fmt.Errorf("GoReleaser must declare its builds: %w", errMatrix)
	}

	matched := make(map[string]bool)

	for _, value := range builds {
		build, ok := value.(map[string]any)
		if !ok {
			return nil, errMatrix
		}

		if err := selectGoBuild(build, wanted, matched); err != nil {
			return nil, err
		}
	}

	for _, platform := range wanted {
		if !matched[platform] {
			return nil, fmt.Errorf("platform %q has no enabled GoReleaser build: %w", platform, errSelection)
		}
	}

	return yaml.Marshal(document)
}

func selectGoBuild(build map[string]any, wanted []string, matched map[string]bool) error {
	enabled, err := buildEnabled(build["skip"])
	if err != nil || !enabled {
		return err
	}

	operatingSystems, osOK := matrixStrings(build["goos"])
	architectures, archOK := matrixStrings(build["goarch"])

	if !osOK || !archOK || build["targets"] != nil {
		return fmt.Errorf("selection requires explicit goos/goarch matrices: %w", errMatrix)
	}

	ignored, _ := build["ignore"].([]any)
	kept := false

	for _, goos := range operatingSystems {
		for _, goarch := range architectures {
			if ignoredPair(ignored, goos, goarch) {
				continue
			}

			platform := goos + "/" + goarch
			if slices.Contains(wanted, platform) {
				matched[platform], kept = true, true
			} else {
				ignored = append(ignored, map[string]any{"goos": goos, "goarch": goarch})
			}
		}
	}

	build["ignore"] = ignored
	if !kept {
		build["skip"] = true
	}

	return nil
}

func matrixStrings(value any) ([]string, bool) {
	values, ok := value.([]any)
	if !ok || len(values) == 0 {
		return nil, false
	}

	result := make([]string, 0, len(values))

	for _, value := range values {
		text, ok := value.(string)
		if !ok || text == "" {
			return nil, false
		}

		result = append(result, text)
	}

	return result, true
}

func buildEnabled(skip any) (bool, error) {
	switch value := skip.(type) {
	case nil:
		return true, nil
	case bool:
		return !value, nil
	case string:
		if value == "true" || value == "false" {
			return value == "false", nil
		}
	}

	return false, fmt.Errorf("selection requires a static skip value: %w", errMatrix)
}

func ignoredPair(ignored []any, goos, goarch string) bool {
	return slices.ContainsFunc(ignored, func(value any) bool {
		pair, ok := value.(map[string]any)
		if !ok || pair["goos"] != nil && pair["goos"] != goos || pair["goarch"] != nil && pair["goarch"] != goarch {
			return false
		}
		// A rule for one microarchitecture does not exclude its whole OS/arch
		// pair. Preserve it for GoReleaser to apply to the selected variants.
		for key, value := range pair {
			if key != "goos" && key != "goarch" && value != "" && value != nil {
				return false
			}
		}

		return true
	})
}
