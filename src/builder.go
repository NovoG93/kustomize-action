package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type runCommandFunc func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error

func defaultRunCommand(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

type Summary struct {
	Success       int      `json:"success"`
	Failed        int      `json:"failed"`
	Canceled      int      `json:"canceled"`
	Roots         int      `json:"roots"`
	FailedRoots   []string `json:"failed_roots"`
	CanceledRoots []string `json:"canceled_roots"`
}

func BuildKustomizations(roots []string, conf Config, kustomizePath string) Summary {
	return buildKustomizations(roots, conf, kustomizePath, defaultRunCommand)
}

func buildKustomizations(roots []string, conf Config, kustomizePath string, runner runCommandFunc) Summary {
	if runner == nil {
		runner = defaultRunCommand
	}

	ctx := context.Background()
	var cancel context.CancelFunc
	if conf.FailFast {
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
	}

	var wg sync.WaitGroup
	// Limit concurrency to 4
	sem := make(chan struct{}, 4)

	var mu sync.Mutex
	summary := Summary{
		Roots: len(roots),
	}

	for _, dir := range roots {
		if conf.FailFast && ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if conf.FailFast && ctx.Err() != nil {
				mu.Lock()
				summary.Canceled++
				summary.CanceledRoots = append(summary.CanceledRoots, d)
				mu.Unlock()
				return
			}

			logMsg, err := buildKustomization(ctx, d, conf.OutputDir, conf.LoadRestrictor, conf.EnableHelm, kustomizePath, runner)

			// Critical section for updating summary and printing logs
			mu.Lock()
			defer mu.Unlock()

			fmt.Println("::group::Building " + d)
			if logMsg != "" {
				fmt.Println(logMsg)
			}
			fmt.Println("::endgroup::")

			if err != nil {
				if errors.Is(err, context.Canceled) {
					summary.Canceled++
					summary.CanceledRoots = append(summary.CanceledRoots, d)
					return
				}
				summary.Failed++
				summary.FailedRoots = append(summary.FailedRoots, d)
				if conf.FailFast && cancel != nil {
					cancel()
				}
			} else {
				summary.Success++
			}
		}(dir)
	}

	// If fail-fast triggered, count any unlaunched roots as canceled.
	if conf.FailFast && ctx.Err() != nil {
		// Best-effort: identify remaining roots not yet launched based on Failed/Success/Canceled counts.
	}

	wg.Wait()
	return summary
}

func BuildKustomization(ctx context.Context, dir, outputDir, loadRestrictor string, enableHelm bool, kustomizePath string) (string, error) {
	return buildKustomization(ctx, dir, outputDir, loadRestrictor, enableHelm, kustomizePath, defaultRunCommand)
}

func buildKustomization(ctx context.Context, dir, outputDir, loadRestrictor string, enableHelm bool, kustomizePath string, runner runCommandFunc) (string, error) {
	if runner == nil {
		runner = defaultRunCommand
	}

	buildDir := dir
	if buildDir == "" {
		buildDir = "."
	}

	fileName := "kustomization.yaml"
	path := filepath.Join(buildDir, fileName)
	if !fileExists(path) {
		fileName = "kustomization.yml"
		path = filepath.Join(buildDir, fileName)
		if !fileExists(path) {
			// Skip if neither variant exists
			return "", nil
		}
	}

	outName := sanitizeOutName(dir) + "_" + fileName
	outPath := filepath.Join(outputDir, outName)

	var args []string
	args = append(args, "build", buildDir, "--load-restrictor="+loadRestrictor)
	if enableHelm {
		args = append(args, "--enable-helm")
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if err := runner(ctx, kustomizePath, args, stdout, stderr); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return fmt.Sprintf("⏭️ Canceled: %s", dir), context.Canceled
		}
		// write error file with -err.yaml/-err.yml suffix
		errOut := strings.TrimSuffix(outName, ".yaml")
		errOut = strings.TrimSuffix(errOut, ".yml")
		if strings.HasSuffix(outName, ".yaml") {
			errOut += "-err.yaml"
		} else {
			errOut += "-err.yml"
		}
		_ = os.WriteFile(filepath.Join(outputDir, errOut), stderr.Bytes(), 0o644)

		return fmt.Sprintf("❌ Failed: %s\n%s\nError: %v", dir, tail(stderr.String(), 20), err), fmt.Errorf("build failed")
	}

	if err := os.WriteFile(outPath, stdout.Bytes(), 0o644); err != nil {
		return fmt.Sprintf("❌ Failed to write output for %s: %v", dir, err), fmt.Errorf("write failed: %v", err)
	}
	return fmt.Sprintf("✅ Built %s", dir), nil
}

func sanitizeOutName(dir string) string {
	dir = strings.Trim(dir, "./")
	dir = strings.TrimPrefix(dir, "/")
	dir = strings.ReplaceAll(dir, string(filepath.Separator), "/")
	dir = strings.ReplaceAll(dir, "/", "_")
	if dir == "" {
		dir = "root"
	}
	return dir
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func tail(s string, lines int) string {
	sc := bufio.NewScanner(strings.NewReader(s))
	var buf []string
	for sc.Scan() {
		buf = append(buf, sc.Text())
		if len(buf) > lines {
			buf = buf[1:]
		}
	}
	return strings.Join(buf, "\n")
}
