package main

import (
	"os"
	"strings"
)

type Config struct {
	OutputDir        string
	KustomizeVersion string
	KustomizeSHA256  string
	EnableHelm       bool
	LoadRestrictor   string
	WorkingDir       string
	BuildAll         bool
	ChangedOnly      bool
	FailOnError      bool
	FailFast         bool
	IgnoreDirs       []string
}

func LoadConfig() Config {
	return Config{
		OutputDir:        getInput("output-dir", "kustomize-builds"),
		KustomizeVersion: getInput("kustomize-version", "v5.8.0"),
		KustomizeSHA256:  getInput("kustomize-sha256", ""),
		EnableHelm:       strings.ToLower(getInput("enable-helm", "true")) == "true",
		LoadRestrictor:   getInput("load-restrictor", "LoadRestrictionsNone"),
		WorkingDir:       getInput("working-directory", "."),
		BuildAll:         strings.ToLower(getInput("build-all", "false")) == "true",
		ChangedOnly:      strings.ToLower(getInput("changed-only", "true")) == "true",
		FailOnError:      strings.ToLower(getInput("fail-on-error", "false")) == "true",
		FailFast:         strings.ToLower(getInput("fail-fast", "false")) == "true",
		IgnoreDirs:       strings.Split(getInput("ignore-dirs", ""), ","),
	}
}

func getInput(name, defaultVal string) string {
	// 1. Try INPUT_NAME (hyphens preserved, uppercase)
	// e.g. output-dir -> INPUT_OUTPUT-DIR
	keyHyphen := "INPUT_" + strings.ToUpper(name)
	if v := os.Getenv(keyHyphen); v != "" {
		return v
	}

	// 2. Try INPUT_NAME (hyphens to underscores, uppercase)
	// e.g. output-dir -> INPUT_OUTPUT_DIR
	keyUnderscore := "INPUT_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	if v := os.Getenv(keyUnderscore); v != "" {
		return v
	}

	// 3. Try Legacy/Local NAME (hyphens to underscores, uppercase)
	// e.g. output-dir -> OUTPUT_DIR
	keyLegacy := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	if v := os.Getenv(keyLegacy); v != "" {
		return v
	}

	return defaultVal
}
