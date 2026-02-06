package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// KustomizeBuilder defines the function signature for building kustomizations
type KustomizeBuilder func(roots []string, conf Config, kustomizePath string) Summary

func main() {
	// Log to stdout in a friendly way for Actions
	log.SetFlags(0)

	config := LoadConfig()
	installer := NewKustomizeInstaller()

	if err := Run(config, installer, BuildKustomizations); err != nil {
		fail("%v", err)
	}
}

func Run(config Config, installer *KustomizeInstaller, builder KustomizeBuilder) error {
	// Ensure kustomize present (download per version)
	kustomizePath, err := installer.Install(config.KustomizeVersion, config.KustomizeSHA256)
	if err != nil {
		return fmt.Errorf("failed to install kustomize: %v", err)
	}

	// Log tool versions
	if out, err := installer.Cmd.Run(kustomizePath, "version"); err == nil {
		log.Printf("ℹ️ Using kustomize version: %s", strings.TrimSpace(string(out)))
	} else {
		log.Printf("⚠️ Failed to get kustomize version: %v", err)
	}

	if out, err := installer.Cmd.Run("helm", "version", "--short"); err == nil {
		log.Printf("ℹ️ Using helm version: %s", strings.TrimSpace(string(out)))
	} else {
		log.Printf("ℹ️ Helm version check failed (helm might not be installed): %v", err)
	}

	var roots []string

	excludedScanDirs := []string{".git", config.OutputDir}
	excludedScanDirs = append(excludedScanDirs, config.IgnoreDirs...)

	// Collect kustomization.yaml files
	if config.BuildAll {
		log.Println("🔍 Scanning for all kustomization files in the working directory...")
		files, err := findKustomizationFilesWithExclusions(config.WorkingDir, excludedScanDirs)
		if err != nil {
			return fmt.Errorf("scan error: %v", err)
		}
		roots = kustomizationDirsFromFiles(files, config.WorkingDir)
	} else {
		log.Println("🔍 Scanning for root kustomization files in the working directory...")
		files, err := findKustomizationFilesWithExclusions(config.WorkingDir, excludedScanDirs)
		if err != nil {
			return fmt.Errorf("scan error: %v", err)
		}
		roots = kustomizationDirsFromFiles(files, config.WorkingDir)
		log.Printf("📂 Found %d candidate kustomizations (before dedupe).", len(roots))

		roots = dedupeTopLevelDirs(roots)
	}

	log.Printf("📦 Keeping %d kustomization files.", len(roots))

	// Create output dir
	if err := os.MkdirAll(config.OutputDir, 0o755); err != nil {
		return fmt.Errorf("cannot create output dir: %v", err)
	}

	// Build all roots in parallel
	repoRoots := mapRootsToRepoRootRelative(config.WorkingDir, roots)
	if config.ChangedOnly {
		log.Println("🧮 changed-only=true: determining changed files for last commit...")
		changed, err := getChangedFilesLastCommit(config.WorkingDir)
		if err != nil {
			return fmt.Errorf("changed-only mode failed: %v", err)
		}
		filtered := selectRootsForChangedFiles(repoRoots, changed)
		log.Printf("🧮 changed-only: %d roots selected from %d discovered.", len(filtered), len(repoRoots))
		repoRoots = filtered
	}
	summary := builder(repoRoots, config, kustomizePath)

	// Write summary
	sumBytes, _ := json.MarshalIndent(summary, "", "  ")
	if err := os.WriteFile(filepath.Join(config.OutputDir, "_summary.json"), sumBytes, 0o644); err != nil {
		log.Printf("⚠️ Could not write summary: %v", err)
	}
	fmt.Println(string(sumBytes))

	// Count final *.yaml files (rendered only)
	manifestCount, _ := countYAMLFiles(config.OutputDir)

	// Emit outputs for the workflow
	setOutput("artifact-name", "kustomize-manifests")
	setOutput("manifest-count", fmt.Sprintf("%d", manifestCount))
	setOutput("success-count", fmt.Sprintf("%d", summary.Success))
	setOutput("fail-count", fmt.Sprintf("%d", summary.Failed))

	rootsJSON, _ := json.Marshal(repoRoots)
	setOutput("roots-json", string(rootsJSON))

	if summary.Failed > 0 && config.FailOnError {
		return fmt.Errorf("kustomize build failed for %d roots", summary.Failed)
	}
	// Exit code: if any failed builds, still exit 0 (let the consumer decide),
	return nil
}

func setOutput(name, value string) {
	// GitHub Actions output
	if path := os.Getenv("GITHUB_OUTPUT"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			defer f.Close()
			writeGitHubOutput(f, name, value)
			return
		}
	}
	// Fallback: print
	fmt.Printf("%s=%s\n", name, value)
}

func writeGitHubOutput(f *os.File, name, value string) {
	// Use heredoc format to safely support multiline and JSON outputs.
	// https://docs.github.com/actions/using-workflows/workflow-commands-for-github-actions#setting-an-output-parameter
	delim := "GH_OUTPUT_" + randomHex(12)
	value = strings.TrimSuffix(value, "\n")
	fmt.Fprintf(f, "%s<<%s\n%s\n%s\n", name, delim, value, delim)
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "fallback"
	}
	const hex = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, v := range b {
		out = append(out, hex[v>>4], hex[v&0x0f])
	}
	return string(out)
}

func countYAMLFiles(dir string) (int, error) {
	n := 0
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		base := strings.ToLower(filepath.Base(p))
		if !strings.HasSuffix(base, ".yaml") && !strings.HasSuffix(base, ".yml") {
			return nil
		}
		// Exclude error output files written on build failures.
		if strings.Contains(base, "_kustomization-err.") {
			return nil
		}
		n++
		return nil
	})
	return n, err
}
