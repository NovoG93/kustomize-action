package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func writeKustomizationYAML(t *testing.T, dir string) {
	t.Helper()

	content := `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- deployment.yaml
`
	if err := os.WriteFile(filepath.Join(dir, "kustomization.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to write kustomization.yaml: %v", err)
	}
}

func TestBuildKustomizations(t *testing.T) {
	runner := func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
		if name != "kustomize" {
			return errors.New("unexpected command")
		}
		_, _ = io.WriteString(stdout, "apiVersion: v1\nkind: List\nitems: []\n")
		return nil
	}

	// Setup temporary directory
	tmpDir, err := os.MkdirTemp("", "kustomize-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create dummy kustomization files
	dirs := []string{"app1", "app2"}
	for _, d := range dirs {
		p := filepath.Join(tmpDir, d)
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatalf("Failed to create dir %s: %v", p, err)
		}
		writeKustomizationYAML(t, p)
	}

	// Config
	outDir := filepath.Join(tmpDir, "out")
	// Create output directory
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	conf := Config{
		OutputDir:      outDir,
		LoadRestrictor: "LoadRestrictionsNone",
		EnableHelm:     false,
		FailFast:       false,
	}

	// Roots
	roots := []string{
		filepath.Join(tmpDir, "app1"),
		filepath.Join(tmpDir, "app2"),
	}

	// Execute
	summary := buildKustomizations(roots, conf, "kustomize", runner)

	// Verify
	if summary.Success != 2 {
		t.Errorf("Expected 2 successes, got %d", summary.Success)
	}
	if summary.Failed != 0 {
		t.Errorf("Expected 0 failures, got %d", summary.Failed)
	}
	if summary.Roots != 2 {
		t.Errorf("Expected 2 roots, got %d", summary.Roots)
	}

	// Check output files
	if _, err := os.Stat(filepath.Join(outDir)); os.IsNotExist(err) {
		t.Errorf("Output directory not created")
	}

	for _, r := range roots {
		want := filepath.Join(outDir, sanitizeOutName(r)+"_kustomization.yaml")
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("Expected output file %s to exist, got error: %v", want, err)
		}
	}
}

func TestBuildKustomization_ExplicitDirBuilds(t *testing.T) {
	runner := func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
		_, _ = io.WriteString(stdout, "apiVersion: v1\nkind: List\nitems: []\n")
		return nil
	}

	// Setup temporary directory
	tmpDir, err := os.MkdirTemp("", "kustomize-root-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	appDir := filepath.Join(tmpDir, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("Failed to create app dir: %v", err)
	}
	writeKustomizationYAML(t, appDir)

	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	logMsg, err := buildKustomization(context.Background(), appDir, outDir, "LoadRestrictionsNone", false, "kustomize", runner)
	if err != nil {
		t.Fatalf("Expected build to succeed, got error: %v (log=%s)", err, logMsg)
	}

	if _, err := os.Stat(filepath.Join(outDir, sanitizeOutName(appDir)+"_kustomization.yaml")); err != nil {
		t.Fatalf("Expected output file to exist, got error: %v", err)
	}
}

func TestBuildKustomization_FailureWritesErrorFile(t *testing.T) {
	stderrOut := "some error\nsecond line\n"
	runner := func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
		_, _ = io.WriteString(stderr, stderrOut)
		return errors.New("exit status 1")
	}

	tmpDir := t.TempDir()
	appDir := filepath.Join(tmpDir, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("Failed to create app dir: %v", err)
	}
	writeKustomizationYAML(t, appDir)

	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	logMsg, err := buildKustomization(context.Background(), appDir, outDir, "LoadRestrictionsNone", false, "kustomize", runner)
	if err == nil {
		t.Fatalf("Expected error, got nil (log=%s)", logMsg)
	}

	errFile := filepath.Join(outDir, sanitizeOutName(appDir)+"_kustomization-err.yaml")
	got, readErr := os.ReadFile(errFile)
	if readErr != nil {
		t.Fatalf("Expected error file %s to exist, got error: %v", errFile, readErr)
	}
	if string(got) != stderrOut {
		t.Fatalf("Expected error file contents %q, got %q", stderrOut, string(got))
	}

	// Ensure the normal output file was not written
	okFile := filepath.Join(outDir, sanitizeOutName(appDir)+"_kustomization.yaml")
	if _, statErr := os.Stat(okFile); statErr == nil {
		t.Fatalf("Did not expect output file %s on failure", okFile)
	}
}

func TestBuildKustomization_CanceledReturnsContextCanceled(t *testing.T) {
	runner := func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
		<-ctx.Done()
		return ctx.Err()
	}

	tmpDir := t.TempDir()
	appDir := filepath.Join(tmpDir, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("Failed to create app dir: %v", err)
	}
	writeKustomizationYAML(t, appDir)

	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := buildKustomization(ctx, appDir, outDir, "LoadRestrictionsNone", false, "kustomize", runner)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Expected context.Canceled, got %v", err)
	}

	entries, readErr := os.ReadDir(outDir)
	if readErr != nil {
		t.Fatalf("Failed to read output dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("Expected no files written on cancellation, got %d", len(entries))
	}
}

func TestBuildKustomizations_FailFastCancelsOthers_NoOutputsForCanceled(t *testing.T) {
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	app1 := filepath.Join(tmpDir, "app1")
	app2 := filepath.Join(tmpDir, "app2")
	app3 := filepath.Join(tmpDir, "app3")
	for _, d := range []string{app1, app2, app3} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("Failed to create app dir %s: %v", d, err)
		}
		writeKustomizationYAML(t, d)
	}

	roots := []string{app1, app2, app3}
	failDir := app2

	conf := Config{
		OutputDir:      outDir,
		LoadRestrictor: "LoadRestrictionsNone",
		EnableHelm:     false,
		FailFast:       true,
	}

	var mu sync.Mutex
	started := 0
	allStarted := make(chan struct{})
	failed := make(chan struct{})
	var allStartedOnce sync.Once
	var failedOnce sync.Once

	markStarted := func() {
		mu.Lock()
		defer mu.Unlock()
		started++
		if started == len(roots) {
			allStartedOnce.Do(func() { close(allStarted) })
		}
	}

	runner := func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
		if name != "kustomize" {
			return errors.New("unexpected command")
		}
		if len(args) < 2 {
			return errors.New("unexpected args")
		}
		buildDir := args[1]
		markStarted()

		if buildDir == failDir {
			<-allStarted
			_, _ = io.WriteString(stderr, "boom\n")
			failedOnce.Do(func() { close(failed) })
			return errors.New("exit status 1")
		}

		<-failed
		<-ctx.Done()
		return ctx.Err()
	}

	summary := buildKustomizations(roots, conf, "kustomize", runner)
	if summary.Failed != 1 {
		t.Fatalf("Expected 1 failed, got %d", summary.Failed)
	}
	if summary.Success != 0 {
		t.Fatalf("Expected 0 success, got %d", summary.Success)
	}
	if summary.Canceled != 2 {
		t.Fatalf("Expected 2 canceled, got %d", summary.Canceled)
	}

	// Only the failing root should produce an -err file; canceled roots must not write anything.
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("Failed to read output dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Expected exactly 1 file in output dir (the error file), got %d", len(entries))
	}
	wantErr := sanitizeOutName(failDir) + "_kustomization-err.yaml"
	if entries[0].Name() != wantErr {
		t.Fatalf("Expected error file %q, got %q", wantErr, entries[0].Name())
	}
}
