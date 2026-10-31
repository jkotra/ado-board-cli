package ado

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractRepoNameFromRemote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "git@ssh.dev.azure.com:v3/my-org/my-project/my-repo",
			expected: "my-repo",
		},
		{
			input:    "my-org@vs-ssh.visualstudio.com:v3/my-org/my-project/my-repo",
			expected: "my-repo",
		},
		{
			input:    "https://dev.azure.com/my-org/my-project/_git/my-repo",
			expected: "my-repo",
		},
		{
			input:    "https://my-org.visualstudio.com/my-project/_git/my-repo/",
			expected: "my-repo",
		},
		{
			input:    "https://user:pat@dev.azure.com/my-org/my-project/_git/my-repo.git",
			expected: "my-repo",
		},
		{
			input:    "git@github.com:my-org/my-repo.git",
			expected: "my-repo",
		},
		{
			input:    "https://github.com/my-org/my-repo",
			expected: "my-repo",
		},
		{
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		actual := ExtractRepoNameFromRemote(tt.input)
		if actual != tt.expected {
			t.Errorf("ExtractRepoNameFromRemote(%q) = %q, want %q", tt.input, actual, tt.expected)
		}
	}
}

func TestDetectLocalGitRepoNameFromConfig(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create temp git dir: %v", err)
	}

	configContent := `[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
[remote "origin"]
	url = git@ssh.dev.azure.com:v3/my-org/my-project/my-repo
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
	merge = refs/heads/main
`
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	subDir := filepath.Join(tmpDir, "pkg", "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create sub dir: %v", err)
	}

	detected := DetectLocalGitRepoName(subDir)
	if detected != "my-repo" {
		t.Errorf("DetectLocalGitRepoName(%q) = %q, want 'my-repo'", subDir, detected)
	}
}
