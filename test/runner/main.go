// Command runner executes Go tests with the centralized test suite temporarily
// materialized beside the packages it tests. The source tests remain under test/.
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
		fail(err)
	}
	suiteRoot := filepath.Join(root, "test", "_suite")
	sources, err := testSources(suiteRoot)
	if err != nil {
		fail(err)
	}
	if len(sources) == 0 {
		fail(fmt.Errorf("centralized test suite is empty: %s", suiteRoot))
	}

	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"test", "./...", "-count=1"}
	}
	goCommand := os.Getenv("GO")
	if goCommand == "" {
		goCommand = "go"
	}

	links, err := materialize(root, suiteRoot, sources)
	if err != nil {
		fail(err)
	}
	status := 0
	defer func() {
		if err := cleanup(links); err != nil && status == 0 {
			fmt.Fprintln(os.Stderr, err)
			status = 1
		}
		os.Exit(status)
	}()

	command := exec.Command(goCommand, args...)
	command.Dir = root
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = append(os.Environ(), "HOTKEY_TEST_SUITE_ACTIVE=1")
	if err := command.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			status = exitError.ExitCode()
		} else {
			fmt.Fprintln(os.Stderr, err)
			status = 1
		}
	}
}

func testSources(root string) ([]string, error) {
	var sources []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_test.go") {
			sources = append(sources, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan centralized test suite: %w", err)
	}
	sort.Strings(sources)
	return sources, nil
}

func materialize(root, suiteRoot string, sources []string) (links []string, err error) {
	links = make([]string, 0, len(sources))
	defer func() {
		if err == nil {
			return
		}
		if cleanupErr := cleanup(links); cleanupErr != nil {
			err = fmt.Errorf("%w; %v", err, cleanupErr)
		}
	}()
	for _, source := range sources {
		relative, err := filepath.Rel(suiteRoot, source)
		if err != nil {
			return nil, fmt.Errorf("resolve test source %s: %w", source, err)
		}
		target := filepath.Join(root, relative)
		if _, err := os.Lstat(target); err == nil {
			return nil, fmt.Errorf("test materialization conflict: %s", relative)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("inspect test target %s: %w", target, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, fmt.Errorf("create test target directory: %w", err)
		}
		if err := os.Symlink(source, target); err != nil {
			return nil, fmt.Errorf("materialize test %s: %w", relative, err)
		}
		links = append(links, target)
	}
	return links, nil
}

func cleanup(links []string) error {
	var failures []string
	for _, link := range links {
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			failures = append(failures, fmt.Sprintf("remove test link %s: %v", link, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("test cleanup failed:\n%s", strings.Join(failures, "\n"))
	}
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
