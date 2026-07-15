$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$errors = [System.Collections.Generic.List[string]]::new()
$crudPath = Join-Path $root "internal/shared/repository/crud.go"

if (-not (Test-Path -LiteralPath $crudPath -PathType Leaf)) {
    $errors.Add("missing shared CRUD contract: internal/shared/repository/crud.go")
} else {
    $crud = Get-Content -Raw -Encoding utf8 $crudPath
    foreach ($method in @("Create(", "GetByID(", "List(", "Update(", "Delete(")) {
        if (-not $crud.Contains($method)) {
            $errors.Add("CRUD contract is missing method: $method")
        }
    }
}

$businessModules = @(
    "identity",
    "monitor",
    "source",
    "ingestion",
    "event",
    "intelligence",
    "knowledge",
    "report",
    "delivery",
    "operations"
)

foreach ($module in $businessModules) {
    $moduleRoot = Join-Path $root "internal/modules/$module"
    foreach ($layer in @("domain", "application", "infrastructure", "transport/http")) {
        $layerPath = Join-Path $moduleRoot $layer
        if (-not (Test-Path -LiteralPath $layerPath -PathType Container)) {
            $errors.Add("module layer is missing: internal/modules/$module/$layer")
        }
    }
}

if ($errors.Count -gt 0) {
    $errors | ForEach-Object { Write-Error $_ -ErrorAction Continue }
    exit 1
}

Write-Output "Repository structure validation passed."
