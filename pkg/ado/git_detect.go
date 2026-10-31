package ado

import (
	"bufio"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExtractRepoNameFromRemote extracts the clean repository name from a Git remote URL.
// Supports Azure DevOps SSH/HTTPS, Visual Studio Team Services, GitHub, GitLab, etc.
// Examples:
// - git@ssh.dev.azure.com:v3/my-org/my-project/my-repo -> my-repo
// - my-org@vs-ssh.visualstudio.com:v3/my-org/my-project/my-repo -> my-repo
// - https://dev.azure.com/my-org/my-project/_git/my-repo -> my-repo
// - https://my-org.visualstudio.com/my-project/_git/my-repo/ -> my-repo
// - https://user:pat@dev.azure.com/org/proj/_git/my-repo.git -> my-repo
// - git@github.com:myorg/my-repo.git -> my-repo
func ExtractRepoNameFromRemote(remoteURL string) string {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return ""
	}

	// Remove query parameters or fragments if present
	if qIdx := strings.IndexAny(remoteURL, "?#"); qIdx != -1 {
		remoteURL = remoteURL[:qIdx]
	}

	// Remove trailing slashes and .git suffix
	remoteURL = strings.TrimRight(remoteURL, "/")
	remoteURL = strings.TrimSuffix(remoteURL, ".git")
	remoteURL = strings.TrimRight(remoteURL, "/")

	// Case 1: ADO HTTP(S) format: /_git/{repoName}
	if idx := strings.LastIndex(remoteURL, "/_git/"); idx != -1 {
		part := remoteURL[idx+6:]
		if slash := strings.Index(part, "/"); slash != -1 {
			part = part[:slash]
		}
		return strings.TrimSpace(part)
	}

	// Case 2: Standard URL parse (for http/https)
	if strings.HasPrefix(remoteURL, "http://") || strings.HasPrefix(remoteURL, "https://") {
		if parsed, err := url.Parse(remoteURL); err == nil {
			path := strings.TrimRight(parsed.Path, "/")
			path = strings.TrimSuffix(path, ".git")
			if lastSlash := strings.LastIndex(path, "/"); lastSlash != -1 {
				return strings.TrimSpace(path[lastSlash+1:])
			}
			return strings.TrimSpace(strings.TrimPrefix(path, "/"))
		}
	}

	// Case 3: SSH format (e.g. git@ssh.dev.azure.com:v3/org/proj/repo or git@github.com:org/repo)
	// Find last slash or colon
	idx := strings.LastIndexAny(remoteURL, "/:")
	if idx != -1 && idx < len(remoteURL)-1 {
		part := remoteURL[idx+1:]
		if slash := strings.Index(part, "/"); slash != -1 {
			part = part[:slash]
		}
		return strings.TrimSpace(part)
	}

	return remoteURL
}

// findGitConfigFile walks upward from dir to locate .git/config
func findGitConfigFile(startDir string) string {
	if startDir == "" {
		startDir, _ = os.Getwd()
	}
	dir, err := filepath.Abs(startDir)
	if err != nil {
		dir = startDir
	}

	for {
		candidate := filepath.Join(dir, ".git", "config")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}

		// Check if .git is a worktree reference file (gitdir: ...)
		gitFilePath := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitFilePath); err == nil && !info.IsDir() {
			data, err := os.ReadFile(gitFilePath)
			if err == nil {
				line := strings.TrimSpace(string(data))
				if strings.HasPrefix(line, "gitdir:") {
					gitDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
					if !filepath.IsAbs(gitDir) {
						gitDir = filepath.Join(dir, gitDir)
					}
					wtConfig := filepath.Join(gitDir, "config")
					if _, err := os.Stat(wtConfig); err == nil {
						return wtConfig
					}
				}
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// readRemoteUrlFromConfigFile reads remote URL from .git/config file
func readRemoteUrlFromConfigFile(configPath string) string {
	f, err := os.Open(configPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inOriginSection := false
	inAnyRemoteSection := false
	fallbackURL := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sec := strings.ToLower(line)
			inOriginSection = strings.Contains(sec, "remote") && strings.Contains(sec, "\"origin\"")
			inAnyRemoteSection = strings.Contains(sec, "remote")
			continue
		}

		if (inOriginSection || inAnyRemoteSection) && strings.HasPrefix(line, "url") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				urlVal := strings.TrimSpace(parts[1])
				if inOriginSection {
					return urlVal
				}
				if fallbackURL == "" {
					fallbackURL = urlVal
				}
			}
		}
	}
	return fallbackURL
}

// DetectLocalGitRepoName attempts to discover the active local repository name
// by querying git remote origin or inspecting .git/config.
func DetectLocalGitRepoName(workingDir string) string {
	if workingDir == "" {
		workingDir, _ = os.Getwd()
	}

	// 1. Try `git remote get-url origin`
	cmd := exec.Command("git", "remote", "get-url", "origin")
	if workingDir != "" {
		cmd.Dir = workingDir
	}
	if out, err := cmd.Output(); err == nil {
		if repo := ExtractRepoNameFromRemote(string(out)); repo != "" {
			return repo
		}
	}

	// 2. Try `git config --get remote.origin.url`
	cmd = exec.Command("git", "config", "--get", "remote.origin.url")
	if workingDir != "" {
		cmd.Dir = workingDir
	}
	if out, err := cmd.Output(); err == nil {
		if repo := ExtractRepoNameFromRemote(string(out)); repo != "" {
			return repo
		}
	}

	// 3. Try `git remote -v` (any remote)
	cmd = exec.Command("git", "remote", "-v")
	if workingDir != "" {
		cmd.Dir = workingDir
	}
	if out, err := cmd.Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if repo := ExtractRepoNameFromRemote(fields[1]); repo != "" {
					return repo
				}
			}
		}
	}

	// 4. Fallback: Parse .git/config directly without depending on git executable
	if configFile := findGitConfigFile(workingDir); configFile != "" {
		if rawURL := readRemoteUrlFromConfigFile(configFile); rawURL != "" {
			if repo := ExtractRepoNameFromRemote(rawURL); repo != "" {
				return repo
			}
		}
	}

	return ""
}
