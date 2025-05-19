package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type ModuleInfo struct {
	Path    string
	Version string
	Update  *struct {
		Path    string
		Version string
	}
}

func main() {
	repoURL := flag.String("repo", "", "Git repository URL to analyze")
	flag.Parse()

	if *repoURL == "" {
		fmt.Println("Usage: dep-analyzer -repo=<git-url>")
		os.Exit(1)
	}

	tmpDir, err := os.MkdirTemp("", "repo-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("git", "clone", *repoURL, tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "Git clone error: %v, %s\n", err, output)
		os.Exit(1)
	}

	repoPath := tmpDir
	modFile := filepath.Join(repoPath, "go.mod")
	modBytes, err := os.ReadFile(modFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading go.mod: %v\n", err)
		os.Exit(1)
	}

	var moduleName, goVersion string
	for _, line := range bytes.Split(modBytes, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("module ")) {
			moduleName = string(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("module "))))
		}
		if bytes.HasPrefix(line, []byte("go ")) {
			goVersion = string(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("go "))))
		}
	}

	fmt.Printf("Module: %s\nGo version: %s\n", moduleName, goVersion)

	listCmd := exec.Command("go", "list", "-m", "-u", "-json", "all")
	listCmd.Dir = repoPath
	out, err := listCmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running go list: %v\n", err)
		os.Exit(1)
	}

	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var curr ModuleInfo
		if err := dec.Decode(&curr); err != nil {
			break
		}
		if curr.Update != nil {
			fmt.Printf("%s: %s -> %s\n", curr.Path, curr.Version, curr.Update.Version)
		}
	}
}
