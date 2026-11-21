package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLauncherRunsFromRepositoryRoot(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("find package directory: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(packageDir, "../.."))
	launcher := filepath.Join(repoRoot, "scripts", "generate-analytics-demo-data.sh")
	binDir := t.TempDir()
	fakeGo := filepath.Join(binDir, "go")
	if err := os.WriteFile(fakeGo, []byte("#!/bin/sh\nprintf '%s\\n' \"$PWD\" \"$@\"\n"), 0o700); err != nil {
		t.Fatalf("create fake go command: %v", err)
	}

	command := exec.Command("sh", launcher, "--run-id", "demo run", "--sales", "50")
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run generator launcher: %v: %s", err, output)
	}
	want := []string{repoRoot, "run", "./cmd/analytics-demo-data", "--run-id", "demo run", "--sales", "50"}
	got := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("launcher working directory or arguments = %q, want %q", got, want)
	}
}
