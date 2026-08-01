package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type buildConfig struct {
	Info struct {
		Version string `yaml:"version"`
	} `yaml:"info"`
}

func main() {
	if len(os.Args) < 2 {
		exitf("usage: go run ./scripts/release <version|notes> [flags]")
	}

	switch os.Args[1] {
	case "version":
		runVersion(os.Args[2:])
	case "notes":
		runNotes(os.Args[2:])
	default:
		exitf("unknown subcommand: %s", os.Args[1])
	}
}

func runVersion(args []string) {
	flags := flag.NewFlagSet("version", flag.ExitOnError)
	configPath := flags.String("config", "build/config.yml", "path to build config")
	_ = flags.Parse(args)

	version, err := readVersion(*configPath)
	if err != nil {
		exitErr(err)
	}

	fmt.Print(version)
}

func runNotes(args []string) {
	flags := flag.NewFlagSet("notes", flag.ExitOnError)
	_ = flags.String("config", "build/config.yml", "path to build config")
	outputPath := flags.String("out", "", "output file path")
	sourcePath := flags.String("source", "", "source markdown file")
	_ = flags.Parse(args)

	if strings.TrimSpace(*outputPath) == "" {
		exitf("notes output path is required")
	}
	if strings.TrimSpace(*sourcePath) == "" {
		exitf("notes source path is required")
	}

	notes, err := resolveReleaseNotes(*sourcePath)
	if err != nil {
		exitErr(err)
	}

	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		exitErr(err)
	}
	if err := os.WriteFile(*outputPath, []byte(notes), 0o644); err != nil {
		exitErr(err)
	}
}

func readVersion(configPath string) (string, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	var cfg buildConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return "", err
	}

	version := strings.TrimSpace(strings.TrimPrefix(cfg.Info.Version, "v"))
	if version == "" {
		return "", errors.New("build/config.yml info.version is empty")
	}
	return version, nil
}

func resolveReleaseNotes(sourcePath string) (string, error) {
	candidate := strings.TrimSpace(sourcePath)
	if candidate == "" {
		return "", errors.New("release notes source path is required")
	}

	content, err := os.ReadFile(candidate)
	if err != nil {
		return "", err
	}

	notes := strings.TrimSpace(string(content))
	if notes == "" {
		return "", fmt.Errorf("release notes file %s is empty", candidate)
	}
	return notes, nil
}

func exitErr(err error) {
	exitf("%v", err)
}

func exitf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
