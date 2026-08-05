package architecture_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerDeploymentContract(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{
		"Dockerfile",
		".dockerignore",
		".env.prod.example",
		"docker-compose-env.yml",
		"docker-compose-prod.yml",
	} {
		if _, err := os.Stat(filepath.Join(root, relative)); err != nil {
			t.Errorf("required deployment file %s is missing: %v", relative, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "docker-compose.yml")); !errors.Is(err, os.ErrNotExist) {
		t.Error("generic docker-compose.yml must not exist")
	}
	if t.Failed() {
		return
	}

	dockerfile := readDockerContractFile(t, root, "Dockerfile")
	assertDockerContains(t, "Dockerfile", dockerfile,
		"FROM golang:latest AS builder",
		"FROM alpine:latest",
		"CGO_ENABLED=0",
		"ca-certificates",
		"tzdata",
		"wget",
		"/var/lib/hotkey/vault",
		"USER hotkey",
		`ENTRYPOINT ["/usr/local/bin/hotkey"]`,
		`CMD ["serve", "--role", "all"]`,
	)

	dockerignore := readDockerContractFile(t, root, ".dockerignore")
	assertDockerContains(t, ".dockerignore", dockerignore, ".git", ".env*", "var/", "/hotkey")

	envCompose := readDockerContractFile(t, root, "docker-compose-env.yml")
	assertDockerContains(t, "docker-compose-env.yml", envCompose,
		"name: hotkey-server-env",
		"image: hotkey-server:env",
		"pgvector/pgvector:pg16",
		"redis:latest",
		"minio/minio:latest",
		"minio/mc:latest",
		"postgres:",
		"redis:",
		"minio:",
		"minio-init:",
		"db-init:",
		"hotkey-server:",
		"- .env",
		"HOTKEY_ENV: development",
		"db verify ||",
		"db init --empty-only --confirm-empty ||",
		"mc mb --ignore-existing",
		"/readyz",
		"stop_grace_period: 30s",
		"postgres_data:",
		"minio_data:",
		"vault_data:",
	)
	if strings.Contains(envCompose, ".env.prod") {
		t.Error("docker-compose-env.yml must not reference .env.prod")
	}
	assertDockerUsesLatestUpstreamImages(t, dockerfile, envCompose)

	readme := readDockerContractFile(t, root, "README.md")
	readmeEN := readDockerContractFile(t, root, "README_EN.md")
	for name, source := range map[string]string{"README.md": readme, "README_EN.md": readmeEN} {
		assertDockerContains(t, name, source,
			"docker compose -f docker-compose-env.yml up --build -d",
			"docker compose --env-file .env.prod -f docker-compose-prod.yml up --build -d",
		)
	}
}

func TestDockerProductionIsolation(t *testing.T) {
	root := repositoryRoot(t)
	prodCompose := readDockerContractFile(t, root, "docker-compose-prod.yml")
	assertDockerContains(t, "docker-compose-prod.yml", prodCompose,
		"name: hotkey-server-prod",
		"image: hotkey-server:prod",
		"- .env.prod",
		"${POSTGRES_PASSWORD:?",
		"${MINIO_ROOT_USER:?",
		"${MINIO_ROOT_PASSWORD:?",
		"postgres://hotkey:${POSTGRES_PASSWORD}",
		"HOTKEY_ENV: production",
		`HOTKEY_REFRESH_COOKIE_SECURE: "true"`,
		"db verify ||",
		"db init --empty-only --confirm-empty ||",
		"mc mb --ignore-existing",
		"/readyz",
		"stop_grace_period: 30s",
	)
	assertDockerUsesLatestUpstreamImages(t, prodCompose)
	for _, service := range []string{"postgres", "redis", "minio", "minio-init", "db-init"} {
		block := dockerComposeServiceBlock(t, prodCompose, service)
		if strings.Contains(block, "\n    ports:") {
			t.Errorf("production service %s must not publish host ports", service)
		}
		if strings.Contains(block, "\n    env_file:") {
			t.Errorf("production support service %s must not receive the complete .env.prod", service)
		}
	}
	serverBlock := dockerComposeServiceBlock(t, prodCompose, "hotkey-server")
	if !strings.Contains(serverBlock, "\n    ports:") {
		t.Error("production hotkey-server must publish its HTTP port")
	}

	prodExample := readDockerContractFile(t, root, ".env.prod.example")
	assertDockerContains(t, ".env.prod.example", prodExample,
		"POSTGRES_PASSWORD=",
		"MINIO_ROOT_USER=",
		"MINIO_ROOT_PASSWORD=",
		"HOTKEY_JWT_SECRET=",
		"HOTKEY_VERIFICATION_HMAC_SECRET=",
	)
	for _, key := range []string{
		"POSTGRES_PASSWORD",
		"MINIO_ROOT_USER",
		"MINIO_ROOT_PASSWORD",
		"HOTKEY_JWT_SECRET",
		"HOTKEY_VERIFICATION_HMAC_SECRET",
	} {
		if !hasEmptyDockerEnvValue(prodExample, key) {
			t.Errorf(".env.prod.example must leave %s empty", key)
		}
	}

	gitignore := readDockerContractFile(t, root, ".gitignore")
	assertDockerContains(t, ".gitignore", gitignore, "!.env.prod.example")
}

func assertDockerUsesLatestUpstreamImages(t *testing.T, sources ...string) {
	t.Helper()
	for _, source := range sources {
		for _, line := range strings.Split(source, "\n") {
			trimmed := strings.TrimSpace(line)
			var image string
			switch {
			case strings.HasPrefix(trimmed, "FROM "):
				fields := strings.Fields(trimmed)
				if len(fields) >= 2 {
					image = fields[1]
				}
			case strings.HasPrefix(trimmed, "image: "):
				image = strings.TrimSpace(strings.TrimPrefix(trimmed, "image: "))
			}
			if image == "" || strings.HasPrefix(image, "hotkey-server:") || image == "pgvector/pgvector:pg16" {
				continue
			}
			if !strings.HasSuffix(image, ":latest") {
				t.Errorf("upstream Docker image %q must use latest, except pgvector's official floating pg16 tag", image)
			}
		}
	}
}

func readDockerContractFile(t *testing.T, root, relative string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, relative))
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}
	return string(content)
}

func assertDockerContains(t *testing.T, name, source string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(source, fragment) {
			t.Errorf("%s must contain %q", name, fragment)
		}
	}
}

func dockerComposeServiceBlock(t *testing.T, source, service string) string {
	t.Helper()
	marker := "  " + service + ":\n"
	start := strings.Index(source, marker)
	if start < 0 {
		t.Fatalf("compose service %s is missing", service)
	}
	remainder := source[start+len(marker):]
	var block strings.Builder
	block.WriteString(marker)
	for _, line := range strings.Split(remainder, "\n") {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.TrimSpace(line) != "" {
			break
		}
		block.WriteString(line)
		block.WriteByte('\n')
	}
	return block.String()
}

func hasEmptyDockerEnvValue(source, key string) bool {
	for _, line := range strings.Split(source, "\n") {
		if strings.TrimSpace(line) == key+"=" {
			return true
		}
	}
	return false
}
