import { existsSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { describe, expect, it } from "vitest";

const frontendRoot = process.cwd();
const repositoryRoot = resolve(frontendRoot, "..");
const readRepositoryFile = (path: string) => readFileSync(join(repositoryRoot, path), "utf8");
const readFrontendFile = (path: string) => readFileSync(join(frontendRoot, path), "utf8");

describe("Docker Compose environment configuration", () => {
  it("keeps the production Compose file structurally consistent with the default", () => {
    const dockerfile = readFrontendFile("Dockerfile");
    const baseCompose = readRepositoryFile("docker-compose.yml");
    const prodCompose = readRepositoryFile("docker-compose-prod.yml");

    expect(existsSync(join(repositoryRoot, "docker-compose.yml"))).toBe(true);
    expect(baseCompose).toContain("name: hotkey-server");
    expect(baseCompose).toContain("image: hotkey-web:env");
    expect(baseCompose).toContain("HOTKEY_DEPLOY_ENV: env");
    expect(baseCompose).toContain("context: ./frontend");
    expect(prodCompose).toContain("name: hotkey-prod");
    expect(prodCompose).toContain("postgres:");
    expect(prodCompose).toContain("redis:");
    expect(prodCompose).toContain("minio:");
    expect(prodCompose).toContain("minio-init:");
    expect(prodCompose).toContain("db-init:");
    expect(prodCompose).toContain("image: hotkey-web:prod");
    expect(prodCompose).toContain("HOTKEY_DEPLOY_ENV: prod");
    expect(prodCompose).toContain("healthcheck:");
    expect(prodCompose).toContain("depends_on:");
    expect(prodCompose).toContain("stop_grace_period: 30s");
    expect(dockerfile.match(/^FROM node:latest AS /gm)).toHaveLength(3);
    expect(dockerfile).toContain("USER node");
  });

  it("documents optional environment settings without requiring defaults", () => {
    const frontendEnvExample = readFrontendFile(".env.example");
    const envExample = readRepositoryFile(".env.example");
    const prodExample = readRepositoryFile(".env.prod.example");

    expect(frontendEnvExample).toContain("# HOTKEY_API_ORIGIN=http://127.0.0.1:8080");
    expect(envExample).toContain("# WEB_PORT=3000");
    expect(prodExample).toContain("# WEB_PORT=3000");
    expect(prodExample).toContain("HOTKEY_JWT_SECRET=");
  });

  it("documents the direct Docker Compose production command", () => {
    const readme = readRepositoryFile("README.md");
    const packageJson = JSON.parse(readFrontendFile("package.json")) as {
      scripts: Record<string, string>;
    };

    expect(readme).toContain("docker compose -f docker-compose.yml up --build");
    expect(readme).toContain(
      "docker compose --env-file .env.prod -f docker-compose-prod.yml up --build"
    );
    expect(Object.keys(packageJson.scripts)).not.toContain("docker:up");
    expect(Object.keys(packageJson.scripts)).not.toContain("docker:config");
  });
});
