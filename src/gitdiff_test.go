package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetChangedFilesLastCommit_ReturnsRepoRootRelativeSlashPaths(t *testing.T) {
	repoDir := t.TempDir()

	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test User")

	// Base commit
	mustWriteFile(t, filepath.Join(repoDir, "apps/a/kustomization.yaml"), "resources: []\n")
	mustWriteFile(t, filepath.Join(repoDir, "apps/a/deploy.yaml"), "apiVersion: v1\nkind: ConfigMap\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "base")

	// Change commit
	mustWriteFile(t, filepath.Join(repoDir, "apps/a/deploy.yaml"), "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: changed\n")
	mustWriteFile(t, filepath.Join(repoDir, "apps/b/other.txt"), "new")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "change")

	changed, err := getChangedFilesLastCommit(repoDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(changed) == 0 {
		t.Fatalf("expected changed files, got none")
	}
	for _, p := range changed {
		if strings.Contains(p, "\\") {
			t.Fatalf("expected slash path, got %q", p)
		}
		if strings.HasPrefix(p, "/") {
			t.Fatalf("expected repo-root relative path, got %q", p)
		}
		if strings.HasPrefix(p, "./") {
			t.Fatalf("expected normalized path without ./ prefix, got %q", p)
		}
	}
	if !contains(changed, "apps/a/deploy.yaml") {
		t.Fatalf("expected apps/a/deploy.yaml in changed set, got %v", changed)
	}
	if !contains(changed, "apps/b/other.txt") {
		t.Fatalf("expected apps/b/other.txt in changed set, got %v", changed)
	}
}

func TestGetChangedFilesLastCommit_WhenHeadMinus1Missing_ReturnsHelpfulError(t *testing.T) {
	repoDir := t.TempDir()

	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test User")

	mustWriteFile(t, filepath.Join(repoDir, "README.md"), "hello")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "initial")

	_, err := getChangedFilesLastCommit(repoDir)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "HEAD~1") {
		t.Fatalf("expected error to mention HEAD~1, got %q", msg)
	}
	if !strings.Contains(msg, "fetch-depth") {
		t.Fatalf("expected error to mention fetch-depth, got %q", msg)
	}
}

func TestGetChangedFilesLastCommit_IncludesDeletedFiles(t *testing.T) {
	repoDir := t.TempDir()

	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test User")

	// Base commit
	mustWriteFile(t, filepath.Join(repoDir, "apps/c/delete.txt"), "bye")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "base")

	// Delete commit
	runGit(t, repoDir, "rm", "apps/c/delete.txt")
	runGit(t, repoDir, "commit", "-m", "delete")

	changed, err := getChangedFilesLastCommit(repoDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !contains(changed, "apps/c/delete.txt") {
		t.Fatalf("expected deleted file in changed set, got %v", changed)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestGetChangedFilesLastCommit_ExcludesSpecificDirectories(t *testing.T) {
	repoDir := t.TempDir()

	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test User")

	// Base commit
	mustWriteFile(t, filepath.Join(repoDir, "apps/kustomize/kustomization.yaml"), "resources: []\n")
	mustWriteFile(t, filepath.Join(repoDir, "apps/helm/values.yaml"), "key: value\n")
	mustWriteFile(t, filepath.Join(repoDir, "vendor/lib/code.go"), "package lib\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "base")

	// Change commit
	mustWriteFile(t, filepath.Join(repoDir, "apps/kustomize/kustomization.yaml"), "resources: [deployment]\n")
	mustWriteFile(t, filepath.Join(repoDir, "apps/helm/values.yaml"), "key: modified\n")
	mustWriteFile(t, filepath.Join(repoDir, "vendor/lib/code.go"), "package lib\n//modified\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "change")

	changed, err := getChangedFilesLastCommitWithExclusions(repoDir, []string{"vendor"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if contains(changed, "vendor/lib/code.go") {
		t.Fatalf("expected vendor/lib/code.go to be excluded, got %v", changed)
	}
	if !contains(changed, "apps/kustomize/kustomization.yaml") {
		t.Fatalf("expected apps/kustomize/kustomization.yaml in changed set, got %v", changed)
	}
	if !contains(changed, "apps/helm/values.yaml") {
		t.Fatalf("expected apps/helm/values.yaml in changed set, got %v", changed)
	}
}

func TestGetChangedFilesLastCommit_ExcludesMultipleDirectories(t *testing.T) {
	repoDir := t.TempDir()

	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test User")

	// Base commit
	mustWriteFile(t, filepath.Join(repoDir, "src/main.go"), "package main\n")
	mustWriteFile(t, filepath.Join(repoDir, "vendor/dep.go"), "package dep\n")
	mustWriteFile(t, filepath.Join(repoDir, "build/output.o"), "binary\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "base")

	// Change commit
	mustWriteFile(t, filepath.Join(repoDir, "src/main.go"), "package main\n//modified\n")
	mustWriteFile(t, filepath.Join(repoDir, "vendor/dep.go"), "package dep\n//modified\n")
	mustWriteFile(t, filepath.Join(repoDir, "build/output.o"), "binary modified\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "change")

	exclusions := []string{"vendor", "build"}
	changed, err := getChangedFilesLastCommitWithExclusions(repoDir, exclusions)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if contains(changed, "vendor/dep.go") {
		t.Fatalf("expected vendor/dep.go to be excluded, got %v", changed)
	}
	if contains(changed, "build/output.o") {
		t.Fatalf("expected build/output.o to be excluded, got %v", changed)
	}
	if !contains(changed, "src/main.go") {
		t.Fatalf("expected src/main.go in changed set, got %v", changed)
	}
}

func TestGetChangedFilesLastCommit_ExcludesNestedDirectories(t *testing.T) {
	repoDir := t.TempDir()

	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test User")

	// Base commit
	mustWriteFile(t, filepath.Join(repoDir, "apps/prod/kustomization.yaml"), "resources: []\n")
	mustWriteFile(t, filepath.Join(repoDir, "apps/test/kustomization.yaml"), "resources: []\n")
	mustWriteFile(t, filepath.Join(repoDir, "apps/test/integration/suite.go"), "package main\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "base")

	// Change commit
	mustWriteFile(t, filepath.Join(repoDir, "apps/prod/kustomization.yaml"), "resources: [deployment]\n")
	mustWriteFile(t, filepath.Join(repoDir, "apps/test/kustomization.yaml"), "resources: [deployment]\n")
	mustWriteFile(t, filepath.Join(repoDir, "apps/test/integration/suite.go"), "package main\n//modified\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "change")

	changed, err := getChangedFilesLastCommitWithExclusions(repoDir, []string{"apps/test/integration"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if contains(changed, "apps/test/integration/suite.go") {
		t.Fatalf("expected apps/test/integration/suite.go to be excluded, got %v", changed)
	}
	if !contains(changed, "apps/test/kustomization.yaml") {
		t.Fatalf("expected apps/test/kustomization.yaml to be included, got %v", changed)
	}
	if !contains(changed, "apps/prod/kustomization.yaml") {
		t.Fatalf("expected apps/prod/kustomization.yaml to be included, got %v", changed)
	}
}

func TestGetChangedFilesLastCommit_EmptyExclusionsList(t *testing.T) {
	repoDir := t.TempDir()

	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test User")

	// Base commit
	mustWriteFile(t, filepath.Join(repoDir, "file1.txt"), "content")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "base")

	// Change commit
	mustWriteFile(t, filepath.Join(repoDir, "file1.txt"), "modified")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "change")

	changed, err := getChangedFilesLastCommitWithExclusions(repoDir, []string{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !contains(changed, "file1.txt") {
		t.Fatalf("expected file1.txt in changed set, got %v", changed)
	}
}

func TestGetChangedFilesLastCommit_AllFilesExcluded(t *testing.T) {
	repoDir := t.TempDir()

	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test User")

	// Base commit
	mustWriteFile(t, filepath.Join(repoDir, "apps/kustomization.yaml"), "resources: []\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "base")

	// Change commit
	mustWriteFile(t, filepath.Join(repoDir, "apps/kustomization.yaml"), "resources: [deployment]\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "change")

	changed, err := getChangedFilesLastCommitWithExclusions(repoDir, []string{"apps"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(changed) != 0 {
		t.Fatalf("expected empty list when all files excluded, got %v", changed)
	}
}

func TestGetChangedFilesLastCommit_ExcludesDirectoriesPrefixMatch(t *testing.T) {
	repoDir := t.TempDir()

	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test User")

	// Base commit
	mustWriteFile(t, filepath.Join(repoDir, "test/unit/test.go"), "package test\n")
	mustWriteFile(t, filepath.Join(repoDir, "test/integration/test.go"), "package test\n")
	mustWriteFile(t, filepath.Join(repoDir, "testing/helpers.go"), "package testing\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "base")

	// Change commit
	mustWriteFile(t, filepath.Join(repoDir, "test/unit/test.go"), "package test\n//modified\n")
	mustWriteFile(t, filepath.Join(repoDir, "test/integration/test.go"), "package test\n//modified\n")
	mustWriteFile(t, filepath.Join(repoDir, "testing/helpers.go"), "package testing\n//modified\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "change")

	// Exclude "test" directory - should exclude both test/unit and test/integration but not testing
	changed, err := getChangedFilesLastCommitWithExclusions(repoDir, []string{"test"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if contains(changed, "test/unit/test.go") {
		t.Fatalf("expected test/unit/test.go to be excluded, got %v", changed)
	}
	if contains(changed, "test/integration/test.go") {
		t.Fatalf("expected test/integration/test.go to be excluded, got %v", changed)
	}
	if !contains(changed, "testing/helpers.go") {
		t.Fatalf("expected testing/helpers.go to be included (not test/ prefix), got %v", changed)
	}
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
