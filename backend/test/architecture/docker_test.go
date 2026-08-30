package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerDeploymentContract(t *testing.T) {
	backendRoot := repositoryRoot(t)
	root := filepath.Clean(filepath.Join(backendRoot, ".."))
	agentRoot := filepath.Join(root, "agent")
	for _, relative := range []string{"Dockerfile", ".dockerignore"} {
		if _, err := os.Stat(filepath.Join(backendRoot, relative)); err != nil {
			t.Errorf("required backend deployment file %s is missing: %v", relative, err)
		}
	}
	for _, relative := range []string{
		".env.prod.example",
		"docker-compose.yml",
		"docker-compose-prod.yml",
	} {
		if _, err := os.Stat(filepath.Join(root, relative)); err != nil {
			t.Errorf("required deployment file %s is missing: %v", relative, err)
		}
	}
	for _, relative := range []string{"Dockerfile", ".dockerignore"} {
		if _, err := os.Stat(filepath.Join(agentRoot, relative)); err != nil {
			t.Errorf("required Agent deployment file %s is missing: %v", relative, err)
		}
	}
	if t.Failed() {
		return
	}

	dockerfile := readDockerContractFile(t, backendRoot, "Dockerfile")
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

	dockerignore := readDockerContractFile(t, backendRoot, ".dockerignore")
	assertDockerContains(t, ".dockerignore", dockerignore, ".git", ".env*", "var/", "/hotkey")

	baseCompose := readDockerContractFile(t, root, "docker-compose.yml")
	assertDockerContains(t, "docker-compose.yml", baseCompose,
		"image: hotkey-server:env",
		"image: hotkey-agent:env",
		"image: hotkey-web:env",
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
		"hotkey-agent:",
		"hotkey-web:",
		"HOTKEY_ENV: development",
		`HOTKEY_JWT_SECRET: "${HOTKEY_JWT_SECRET:-development-jwt-secret-change-me-000000}"`,
		`HOTKEY_VERIFICATION_HMAC_SECRET: "${HOTKEY_VERIFICATION_HMAC_SECRET:-development-hmac-secret-change-me-0000}"`,
		`HOTKEY_SOURCE_CREDENTIAL_MASTER_KEY: "${HOTKEY_SOURCE_CREDENTIAL_MASTER_KEY:-ZGV2ZWxvcG1lbnQtc291cmNlLW1hc3Rlci1rZXktMDA=}"`,
		`HOTKEY_AGENT_SHADOW_ENABLED: "${HOTKEY_AGENT_SHADOW_ENABLED:-true}"`,
		"HOTKEY_DEPLOY_ENV: env",
		"db verify ||",
		"db init --empty-only --confirm-empty ||",
		"mc mb --ignore-existing",
		"/readyz",
		"stop_grace_period: 30s",
		"postgres_data:",
		"minio_data:",
		"vault_data:",
	)
	if strings.Contains(baseCompose, "env_file:") || strings.Contains(baseCompose, "./backend/.env") {
		t.Error("default Compose must start without a local environment file")
	}
	assertDockerComposeNamingContract(t, "docker-compose.yml", baseCompose, "hotkey", "hotkey")
	assertDockerUsesLatestUpstreamImages(t, dockerfile, baseCompose)

	readme := readDockerContractFile(t, root, "README.md")
	readmeEN := readDockerContractFile(t, root, "README_EN.md")
	for name, source := range map[string]string{"README.md": readme, "README_EN.md": readmeEN} {
		assertDockerContains(t, name, source,
			"docker compose -f docker-compose.yml up --build --detach --wait --wait-timeout 240",
			"docker compose --env-file .env.prod -f docker-compose-prod.yml up --build --detach --wait --wait-timeout 240",
		)
	}
}

func TestDockerProductionIsolation(t *testing.T) {
	backendRoot := repositoryRoot(t)
	root := filepath.Clean(filepath.Join(backendRoot, ".."))
	dockerfile := readDockerContractFile(t, backendRoot, "Dockerfile")
	baseCompose := readDockerContractFile(t, root, "docker-compose.yml")
	prodOverride := readDockerContractFile(t, root, "docker-compose-prod.yml")
	assertDockerContains(t, "docker-compose-prod.yml", prodOverride,
		"pgvector/pgvector:pg16",
		"redis:latest",
		"minio/minio:latest",
		"minio/mc:latest",
		"image: hotkey-server:prod",
		"image: hotkey-agent:prod",
		"image: hotkey-web:prod",
		"- .env.prod",
		"context: ./backend",
		"context: ./agent",
		"context: ./frontend",
		"healthcheck:",
		"depends_on:",
		"entrypoint:",
		"stop_grace_period: 30s",
		"postgres_data:",
		"minio_data:",
		"vault_data:",
		"${POSTGRES_PASSWORD:?",
		"${MINIO_ROOT_USER:?",
		"${MINIO_ROOT_PASSWORD:?",
		"postgres://hotkey:${POSTGRES_PASSWORD:?",
		"HOTKEY_ENV: production",
		"HOTKEY_ROLE: all",
		"HOTKEY_REDIS_URL: redis://redis:6379/0",
		"HOTKEY_MINIO_ENDPOINT: minio:9000",
		`HOTKEY_REFRESH_COOKIE_SECURE: "true"`,
		`HOTKEY_AGENT_SHADOW_ENABLED: "${HOTKEY_AGENT_SHADOW_ENABLED:-false}"`,
		"HOTKEY_DEPLOY_ENV: prod",
	)
	assertDockerComposeNamingContract(t, "docker-compose-prod.yml", prodOverride, "hotkey-prod", "hotkey-prod")
	assertDockerUsesLatestUpstreamImages(t, dockerfile, prodOverride)
	for name, compose := range map[string]string{"docker-compose.yml": baseCompose, "docker-compose-prod.yml": prodOverride} {
		for _, service := range []string{"postgres", "redis", "minio", "minio-init", "db-init", "hotkey-agent"} {
			block := dockerComposeServiceBlock(t, compose, service)
			if strings.Contains(block, "\n    ports:") {
				t.Errorf("%s support service %s must not publish host ports", name, service)
			}
		}
		for _, service := range []string{"hotkey-server", "hotkey-web"} {
			block := dockerComposeServiceBlock(t, compose, service)
			if !strings.Contains(block, "\n    ports:") {
				t.Errorf("%s application service %s must publish its HTTP port", name, service)
			}
		}
	}

	prodExample := readDockerContractFile(t, root, ".env.prod.example")
	assertDockerContains(t, ".env.prod.example", prodExample,
		"HOTKEY_CONTAINER_PREFIX=hotkey-prod",
		"POSTGRES_PASSWORD=",
		"MINIO_ROOT_USER=",
		"MINIO_ROOT_PASSWORD=",
		"HOTKEY_JWT_SECRET=",
		"HOTKEY_VERIFICATION_HMAC_SECRET=",
		"HOTKEY_AGENT_SHADOW_ENABLED=false",
		"HOTKEY_AGENT_AUTH_TOKEN=",
	)
	for _, key := range []string{
		"POSTGRES_PASSWORD",
		"MINIO_ROOT_USER",
		"MINIO_ROOT_PASSWORD",
		"HOTKEY_JWT_SECRET",
		"HOTKEY_VERIFICATION_HMAC_SECRET",
		"HOTKEY_AGENT_AUTH_TOKEN",
	} {
		if !hasEmptyDockerEnvValue(prodExample, key) {
			t.Errorf(".env.prod.example must leave %s empty", key)
		}
	}

	gitignore := readDockerContractFile(t, root, ".gitignore")
	assertDockerContains(t, ".gitignore", gitignore, "!.env.prod.example")
}

func assertDockerComposeNamingContract(t *testing.T, name, source, projectName, defaultPrefix string) {
	t.Helper()
	projectLine := "name: " + projectName
	if !strings.Contains("\n"+source+"\n", "\n"+projectLine+"\n") {
		t.Errorf("%s must contain the exact project name line %q", name, projectLine)
	}
	for _, service := range []struct {
		name   string
		suffix string
	}{
		{name: "postgres", suffix: "postgres"},
		{name: "redis", suffix: "redis"},
		{name: "minio", suffix: "minio"},
		{name: "minio-init", suffix: "minio-init"},
		{name: "db-init", suffix: "db-init"},
		{name: "hotkey-agent", suffix: "agent"},
		{name: "hotkey-server", suffix: "api"},
		{name: "hotkey-web", suffix: "web"},
	} {
		block := dockerComposeServiceBlock(t, source, service.name)
		expected := `container_name: "${HOTKEY_CONTAINER_PREFIX:-` + defaultPrefix + `}-` + service.suffix + `"`
		if !strings.Contains(block, expected) {
			t.Errorf("%s service %s must contain %q", name, service.name, expected)
		}
	}
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
			if image == "" || strings.HasPrefix(image, "hotkey-server:") || strings.HasPrefix(image, "hotkey-agent:") || strings.HasPrefix(image, "hotkey-web:") || image == "pgvector/pgvector:pg16" || image == "python:3.12-slim" {
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
