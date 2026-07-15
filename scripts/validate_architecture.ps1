$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$errors = [System.Collections.Generic.List[string]]::new()

$requiredDirectories = @(
    "internal/bootstrap",
    "internal/platform",
    "internal/shared",
    "internal/modules",
    "db/migrations"
)

$forbiddenDirectories = @(
    "internal/controller",
    "internal/service",
    "internal/repository",
    "internal/model",
    "internal/queue",
    "internal/worker",
    "internal/fxapp"
)

foreach ($relativePath in $requiredDirectories) {
    $path = Join-Path $root $relativePath
    if (-not (Test-Path -LiteralPath $path -PathType Container)) {
        $errors.Add("missing required directory: $relativePath")
    }
}

foreach ($relativePath in $forbiddenDirectories) {
    $path = Join-Path $root $relativePath
    if (Test-Path -LiteralPath $path) {
        $errors.Add("legacy directory is forbidden: $relativePath")
    }
}

$goModPath = Join-Path $root "go.mod"
if (Test-Path -LiteralPath $goModPath) {
    $goMod = Get-Content -Raw -Encoding utf8 $goModPath
    foreach ($module in @("segmentio/kafka-go", "tmc/langchaingo", "redis/go-redis")) {
        if ($goMod.Contains($module)) {
            $errors.Add("legacy dependency is forbidden: $module")
        }
    }
}

$modulesRoot = Join-Path $root "internal/modules"
if (Test-Path -LiteralPath $modulesRoot) {
    $domainFiles = Get-ChildItem -LiteralPath $modulesRoot -Recurse -Filter "*.go" |
        Where-Object { $_.FullName -match "[\\/]domain[\\/]" }

    foreach ($file in $domainFiles) {
        $content = Get-Content -Raw -Encoding utf8 $file.FullName
        foreach ($forbiddenImport in @("github.com/gin-gonic/gin", "gorm.io/gorm", "riverqueue/river", "minio/minio-go")) {
            if ($content.Contains($forbiddenImport)) {
                $relative = [System.IO.Path]::GetRelativePath($root, $file.FullName)
                $errors.Add("domain imports infrastructure package: $relative -> $forbiddenImport")
            }
        }
    }
}

if ($errors.Count -gt 0) {
    $errors | ForEach-Object { Write-Error $_ -ErrorAction Continue }
    exit 1
}

Write-Output "Architecture validation passed."
