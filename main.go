package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
)

type ModuleInfo struct {
	Path    string
	Version string
	Update  *struct {
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

	re := regexp.MustCompile(`https://github.com/([^/]+)/([^/]+)`)
	matches := re.FindStringSubmatch(*repoURL)
	if len(matches) != 3 {
		fmt.Fprintf(os.Stderr, "Некорректный URL репозитория GitHub\n")
		os.Exit(1)
	}
	user, repo := matches[1], matches[2]

	tmpDir, err := os.MkdirTemp("", "repo-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка создания временной директории: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	var wg sync.WaitGroup
	var goModErr, goSumErr error
	var goModBytes []byte

	wg.Add(2)

	go func() {
		defer wg.Done()
		goModURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/go.mod", user, repo)
		goModBytes, goModErr = downloadFile(goModURL)
		if goModErr != nil {
			fmt.Fprintf(os.Stderr, "Ошибка загрузки go.mod: %v\n", goModErr)
		} else {
			if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), goModBytes, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Ошибка сохранения go.mod: %v\n", err)
			}
		}
	}()

	go func() {
		defer wg.Done()
		goSumURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/go.sum", user, repo)
		goSumBytes, err := downloadFile(goSumURL)
		if err != nil {
			goSumErr = err
			fmt.Fprintf(os.Stderr, "Предупреждение: go.sum не найден или не удалось загрузить: %v\n", goSumErr)
		} else {
			if err := os.WriteFile(filepath.Join(tmpDir, "go.sum"), goSumBytes, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Ошибка сохранения go.sum: %v\n", err)
			}
		}
	}()

	wg.Wait()

	if goModErr != nil {
		os.Exit(1)
	}

	var moduleName, goVersion string
	for _, line := range bytes.Split(goModBytes, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("module ")) {
			moduleName = string(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("module "))))
		}
		if bytes.HasPrefix(line, []byte("go ")) {
			goVersion = string(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("go "))))
		}
	}

	fmt.Printf("Module: %s\nGo version: %s\n", moduleName, goVersion)

	listCmd := exec.Command("go", "list", "-m", "-u", "-json", "all")
	listCmd.Dir = tmpDir
	out, err := listCmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка выполнения go list: %v\n", err)
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

func downloadFile(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("не удалось скачать файл: %s", resp.Status)
	}

	var result struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	fileBytes, err := base64.StdEncoding.DecodeString(result.Content)
	if err != nil {
		return nil, err
	}

	return fileBytes, nil
}
