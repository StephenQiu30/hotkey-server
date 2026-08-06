// Command runner 将集中测试源码和包内 testdata 临时物化到目标包。
// 权威测试资产始终保存在 test/_suite，runner 退出时清理所有链接。
package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(run(root, os.Args[1:], os.Getenv("GO")))
}

func run(root string, args []string, goCommand string) (status int) {
	suiteRoot := filepath.Join(root, "test", "_suite")
	entries, err := suiteEntries(suiteRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, repositoryError(root, err))
		return 1
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "centralized test suite is empty: test/_suite")
		return 1
	}

	if len(args) == 0 {
		args = []string{"test", "./...", "-count=1"}
	}
	if goCommand == "" {
		goCommand = "go"
	}

	links, err := materialize(root, suiteRoot, entries)
	if err != nil {
		fmt.Fprintln(os.Stderr, repositoryError(root, err))
		return 1
	}
	defer func() {
		if err := cleanup(root, links); err != nil {
			fmt.Fprintln(os.Stderr, repositoryError(root, err))
			if status == 0 {
				status = 1
			}
		}
	}()

	command := exec.Command(goCommand, args...)
	command.Dir = root
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = append(os.Environ(), "HOTKEY_TEST_SUITE_ACTIVE=1")
	if err := command.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return exitError.ExitCode()
		} else {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	return 0
}

func suiteEntries(root string) ([]string, error) {
	var entries []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == "testdata" {
			entries = append(entries, path)
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_test.go") {
			entries = append(entries, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan centralized test suite: %w", err)
	}
	sort.Strings(entries)
	return entries, nil
}

func materialize(root, suiteRoot string, entries []string) (links []string, err error) {
	links = make([]string, 0, len(entries))
	defer func() {
		if err == nil {
			return
		}
		if cleanupErr := cleanup(root, links); cleanupErr != nil {
			err = fmt.Errorf("%w; %v", err, cleanupErr)
		}
	}()
	for _, source := range entries {
		relative, err := filepath.Rel(suiteRoot, source)
		if err != nil {
			return links, fmt.Errorf("resolve test suite entry: %w", err)
		}
		if relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return links, fmt.Errorf("test suite entry escapes suite root: %s", relative)
		}
		target := filepath.Join(root, relative)
		if _, err := os.Lstat(target); err == nil {
			return links, fmt.Errorf("test materialization conflict: %s", relative)
		} else if !os.IsNotExist(err) {
			return links, fmt.Errorf("inspect test target %s: %s", relative, repositoryError(root, err))
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return links, fmt.Errorf("create test target directory: %w", err)
		}
		if err := os.Symlink(source, target); err != nil {
			return links, fmt.Errorf("materialize test suite entry %s: %s", relative, repositoryError(root, err))
		}
		links = append(links, target)
	}
	return links, nil
}

func cleanup(root string, links []string) error {
	var failures []string
	for _, link := range links {
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			relative, relativeErr := filepath.Rel(root, link)
			if relativeErr != nil {
				relative = "<unknown>"
			}
			failures = append(failures, fmt.Sprintf("remove test link %s: %s", relative, repositoryError(root, err)))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("test cleanup failed:\n%s", strings.Join(failures, "\n"))
	}
	return nil
}

func repositoryError(root string, err error) string {
	if err == nil {
		return ""
	}
	cleanRoot := filepath.Clean(root)
	message := err.Error()
	message = strings.ReplaceAll(message, cleanRoot+string(filepath.Separator), "")
	return strings.ReplaceAll(message, cleanRoot, ".")
}
