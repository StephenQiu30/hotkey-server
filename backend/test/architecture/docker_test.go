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
	for _, relative := range []string{"Dockerfile", ".dockerignore"} {
		if _, err := os.Stat(filepath.Join(backendRoot, relative)); err != nil {
			t.Errorf("required backend deployment file %s is missing: %v", relative, err)
		}
	}
	for _, relative := range []string{
		".env.prod.example",
		"docker-compose.yml",
		"docker-compose-env.yml",
		"docker-compose-prod.yml",
	} {
		if _, err := os.Stat(filepath.Join(root, relative)); err != nil {
			t.Errorf("required deployment file %s is missing: %v", relative, err)
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
		"name: hotkey-env",
		"image: hotkey-server:env",
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
		"hotkey-web:",
		"HOTKEY_ENV: development",
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
	assertDockerUsesLatestUpstreamImages(t, dockerfile, baseCompose)

	envOverride := readDockerContractFile(t, root, "docker-compose-env.yml")
	assertDockerContains(t, "docker-compose-env.yml", envOverride,
		"${HOTKEY_POSTGRES_PORT:-5432}:5432",
		"${HOTKEY_REDIS_PORT:-6379}:6379",
		"${HOTKEY_MINIO_PORT:-9000}:9000",
		"${HOTKEY_MINIO_CONSOLE_PORT:-9001}:9001",
		"- ./backend/.env",
	)
	if strings.Contains(envOverride, ".env.prod") {
		t.Error("docker-compose-env.yml must not reference .env.prod")
	}
	assertComposeOverrideContainsOnlyDifferences(t, "docker-compose-env.yml", envOverride)

	readme := readDockerContractFile(t, root, "README.md")
	readmeEN := readDockerContractFile(t, root, "README_EN.md")
	for name, source := range map[string]string{"README.md": readme, "README_EN.md": readmeEN} {
		assertDockerContains(t, name, source,
			"docker compose -f docker-compose.yml -f docker-compose-env.yml up --build -d",
			"-f docker-compose.yml -f docker-compose-prod.yml up --build -d",
		)
	}
}

func TestDockerProductionIsolation(t *testing.T) {
	root := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	baseCompose := readDockerContractFile(t, root, "docker-compose.yml")
	prodOverride := readDockerContractFile(t, root, "docker-compose-prod.yml")
	assertDockerContains(t, "docker-compose-prod.yml", prodOverride,
		"name: hotkey-prod",
		"image: hotkey-server:prod",
		"image: hotkey-web:prod",
		"- .env.prod",
		"${POSTGRES_PASSWORD:?",
		"${MINIO_ROOT_USER:?",
		"${MINIO_ROOT_PASSWORD:?",
		"postgres://hotkey:${POSTGRES_PASSWORD:?",
		"HOTKEY_ENV: production",
		`HOTKEY_REFRESH_COOKIE_SECURE: "true"`,
		"HOTKEY_DEPLOY_ENV: prod",
	)
	assertComposeOverrideContainsOnlyDifferences(t, "docker-compose-prod.yml", prodOverride)
	if strings.Contains(prodOverride, "\n    ports:") {
		t.Error("production override must not publish additional host ports")
	}
	for _, service := range []string{"postgres", "redis", "minio", "minio-init", "db-init"} {
		block := dockerComposeServiceBlock(t, baseCompose, service)
		if strings.Contains(block, "\n    ports:") {
			t.Errorf("production support service %s must not publish host ports in the shared baseline", service)
		}
	}
	for _, service := range []string{"hotkey-server", "hotkey-web"} {
		block := dockerComposeServiceBlock(t, baseCompose, service)
		if !strings.Contains(block, "\n    ports:") {
			t.Errorf("production application service %s must publish its HTTP port", service)
		}
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

func assertComposeOverrideContainsOnlyDifferences(t *testing.T, name, source string) {
	t.Helper()
	for _, duplicatedCommonBlock := range []string{
		"healthcheck:",
		"depends_on:",
		"entrypoint:",
		"stop_grace_period:",
		"\nvolumes:",
	} {
		if strings.Contains(source, duplicatedCommonBlock) {
			t.Errorf("%s duplicates shared Compose block %q", name, duplicatedCommonBlock)
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
			if image == "" || strings.HasPrefix(image, "hotkey-server:") || strings.HasPrefix(image, "hotkey-web:") || image == "pgvector/pgvector:pg16" {
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
