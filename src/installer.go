package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func ensureKustomize(version string, expectedSHA256 string) (string, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return "", fmt.Errorf("kustomize version is empty")
	}

	// If kustomize is already present and matches, keep it.
	if path, err := exec.LookPath("kustomize"); err == nil {
		cmd := exec.Command(path, "version", "--short")
		out, err := cmd.CombinedOutput()
		if err == nil && strings.Contains(string(out), version) {
			return path, nil
		}
	}

	// Download the specified version
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	url := fmt.Sprintf("https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%%2F%s/kustomize_%s_%s_%s.tar.gz", version, version, goos, goarch)

	tmp, err := os.CreateTemp("", "kustomize-*.tar.gz")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	client := &http.Client{Timeout: 90 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "kustomize-action")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}

	out, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}

	if err := verifySHA256(tmpPath, expectedSHA256); err != nil {
		return "", err
	}

	// Extract the tarball into /usr/local/bin using tar (simpler than re-implementing tar parsing).
	// The archive includes a single 'kustomize' binary.
	installDir := "/usr/local/bin"
	cmd := exec.Command("tar", "-xzf", tmpPath, "-C", installDir)
	if _, err := cmd.CombinedOutput(); err != nil {
		// If extraction to /usr/local/bin failed, try a temporary directory.
		// This is common when running locally without root.
		log.Printf("⚠️ Could not install kustomize to %s (likely permission denied). Falling back to temp dir.", installDir)

		tmpBin, err := os.MkdirTemp("", "kustomize-bin-*")
		if err != nil {
			return "", fmt.Errorf("failed to create temp dir for kustomize: %w", err)
		}
		installDir = tmpBin

		cmd = exec.Command("tar", "-xzf", tmpPath, "-C", installDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("extract failed: %w: %s", err, strings.TrimSpace(string(output)))
		}

		// Update PATH for the current process so exec.Command("kustomize") works
		path := os.Getenv("PATH")
		newPath := installDir + string(os.PathListSeparator) + path
		if err := os.Setenv("PATH", newPath); err != nil {
			return "", fmt.Errorf("failed to update PATH: %w", err)
		}
		log.Printf("ℹ️ Added %s to PATH", installDir)
	}

	bin := filepath.Join(installDir, "kustomize")
	if err := os.Chmod(bin, 0o755); err != nil {
		return "", err
	}
	return bin, nil
}

func verifySHA256(path string, expected string) error {
	expected = strings.TrimSpace(strings.ToLower(expected))
	if expected == "" {
		return nil
	}
	expected = strings.TrimPrefix(expected, "sha256:")
	expected = strings.ReplaceAll(expected, " ", "")
	if len(expected) != 64 {
		return fmt.Errorf("invalid kustomize-sha256: expected 64 hex chars, got %d", len(expected))
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("kustomize tarball sha256 mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}
