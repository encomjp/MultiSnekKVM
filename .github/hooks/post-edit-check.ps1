# Post-edit validation hook for MultiSnekKVM
# Checks: file size limits, no wailsjs imports in App.svelte, _windows.go files have _stub.go pairs

$input = $Input | Out-String
$event = $input | ConvertFrom-Json -ErrorAction SilentlyContinue

$toolName = $event.toolName
$toolInput = $event.toolInput

# Only run after file edit operations
if ($toolName -notmatch 'edit|create|replace|write') {
    exit 0
}

$filePath = ""
if ($toolInput.filePath) { $filePath = $toolInput.filePath }
elseif ($toolInput.path) { $filePath = $toolInput.path }

if (-not $filePath) { exit 0 }

$messages = @()

# Check 1: File size limits — warn when a file exceeds 300 lines, block at 500
if (Test-Path $filePath -PathType Leaf) {
    $ext = [System.IO.Path]::GetExtension($filePath)
    if ($ext -match '\.(go|svelte|ts|js|css)$') {
        $lineCount = (Get-Content $filePath -ErrorAction SilentlyContinue | Measure-Object -Line).Lines
        if ($lineCount -gt 500) {
            $fileName = Split-Path $filePath -Leaf
            $messages += "STOP: $fileName is $lineCount lines (limit: 500). Split into smaller, focused modules before continuing. Go files: extract a new file in package main. Svelte: extract a component to frontend/src/lib/. TS/JS: extract helpers to a separate module."
        } elseif ($lineCount -gt 300) {
            $fileName = Split-Path $filePath -Leaf
            $messages += "WARNING: $fileName is $lineCount lines (soft limit: 300). Consider splitting soon to keep files focused and maintainable."
        }
    }
}

# Check 2: No wailsjs/ imports in App.svelte
if ($filePath -match 'App\.svelte$') {
    $content = Get-Content $filePath -Raw -ErrorAction SilentlyContinue
    if ($content -and $content -match 'from\s+[''"].*wailsjs/') {
        $messages += "App.svelte imports from wailsjs/ - these bindings are stale. Use window.go.main.App.* directly instead."
    }
}

# Check 3: _windows.go files must have matching _stub.go
if ($filePath -match '_windows\.go$') {
    $stubPath = $filePath -replace '_windows\.go$', '_stub.go'
    if (-not (Test-Path $stubPath)) {
        $stubName = Split-Path $stubPath -Leaf
        $messages += "Created a _windows.go file without a matching $stubName. Add a stub with //go:build !windows and matching no-op signatures."
    }
}

if ($messages.Count -gt 0) {
    $reason = $messages -join "`n"
    $output = @{
        systemMessage = $reason
    } | ConvertTo-Json -Compress
    Write-Output $output
    exit 0
}

exit 0
