//go:build functional

package functional_test

import (
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// skipFunctionalIfNoDocker skips early with a clear message when no Docker socket
// is present, avoiding testcontainers' panic+recover path (noisy "Recovered from panic").
func skipFunctionalIfNoDocker(t *testing.T) {
	t.Helper()
	if dockerSocketLikelyPresent() {
		return
	}
	t.Skipf("functional tests need Docker: no local Docker socket found and DOCKER_HOST is unset or not unix/tcp. Start Docker Desktop (macOS) or the Docker daemon (Linux), then re-run.")
}

func dockerSocketLikelyPresent() bool {
	if host := strings.TrimSpace(os.Getenv("DOCKER_HOST")); host != "" {
		u, err := url.Parse(host)
		if err == nil && (u.Scheme == "unix" || u.Scheme == "npipe" || u.Scheme == "tcp") {
			return true
		}
	}
	if p := os.Getenv("TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	uid := os.Getuid()
	candidates := []string{
		"/var/run/docker.sock",
		filepath.Join(home, ".docker", "run", "docker.sock"),
		filepath.Join(home, ".docker", "desktop", "docker.sock"),
		filepath.Join("/run/user", strconv.Itoa(uid), "docker.sock"),
	}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		candidates = append(candidates, filepath.Join(xdg, "docker.sock"))
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}
