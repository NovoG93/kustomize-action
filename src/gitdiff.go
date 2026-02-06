package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func getChangedFilesLastCommit(startDir string) ([]string, error) {
	return getChangedFilesLastCommitWithExclusions(startDir, []string{})
}

func getChangedFilesLastCommitWithExclusions(startDir string, exclusions []string) ([]string, error) {
	repoRoot, err := gitRepoRoot(startDir)
	if err != nil {
		return nil, err
	}
	if err := verifyHasParentCommit(repoRoot); err != nil {
		return nil, err
	}

	out, err := gitOutput(repoRoot, "diff", "--name-only", "--diff-filter=ACMRD", "HEAD~1..HEAD")
	if err != nil {
		return nil, err
	}

	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}

	lines := strings.Split(out, "\n")
	paths := make([]string, 0, len(lines))
	seen := make(map[string]bool, len(lines))
	for _, l := range lines {
		p := normalizeRepoRelativePath(l)
		if p == "" {
			continue
		}
		if !seen[p] {
			// Check if file is excluded
			if !isPathExcluded(p, exclusions) {
				seen[p] = true
				paths = append(paths, p)
			}
		}
	}
	return paths, nil
}

// isPathExcluded checks if a path should be excluded based on the exclusions list.
// Exclusions match prefixes: "vendor" excludes "vendor/lib.go" and "vendor/dep/lib.go"
func isPathExcluded(path string, exclusions []string) bool {
	for _, excl := range exclusions {
		// Normalize exclusion path (remove trailing slashes)
		excl = strings.TrimSuffix(excl, "/")

		// Check exact match or prefix match with directory separator
		if path == excl || strings.HasPrefix(path, excl+"/") {
			return true
		}
	}
	return false
}

func gitRepoRoot(startDir string) (string, error) {
	out, err := gitOutput(startDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("failed to determine git repo root: %w", err)
	}
	root := strings.TrimSpace(out)
	if root == "" {
		return "", fmt.Errorf("failed to determine git repo root: empty output")
	}
	return root, nil
}

func verifyHasParentCommit(repoRoot string) error {
	_, err := gitOutput(repoRoot, "rev-parse", "--verify", "--quiet", "HEAD~1")
	if err != nil {
		return fmt.Errorf("cannot determine changed files for last commit: HEAD~1 not available. Ensure actions/checkout uses fetch-depth >= 2 (or fetch-depth: 0). Original error: %w", err)
	}
	return nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), errMsg)
		}
		return "", fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
	}
	return stdout.String(), nil
}
