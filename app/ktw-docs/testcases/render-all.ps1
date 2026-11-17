# Render all diagram and document test cases using ktw-docs.
# Usage: .\app\ktw-docs\testcases\render-all.ps1

$ErrorActionPreference = "Continue"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$binDir = Join-Path (Split-Path -Parent (Split-Path -Parent (Split-Path -Parent $scriptDir))) "bin"
$ktwDocs = Join-Path $binDir "ktw-docs.exe"
$outDir = Join-Path $scriptDir ".tmp"

if (-not (Test-Path $ktwDocs)) {
    Write-Host "ktw-docs.exe not found at $ktwDocs" -ForegroundColor Red
    Write-Host "Build it first: go build -o bin/ktw-docs.exe ./app/ktw-docs/"
    exit 1
}

# Create output directory
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
Write-Host "Output directory: $outDir" -ForegroundColor Cyan
Write-Host ""

$diagrams = @(
    @{ File = "mermaid-flow.mmd";       Type = "mermaid";   Name = "mermaid-flow" }
    @{ File = "plantuml-classes.puml";  Type = "plantuml";  Name = "plantuml-classes" }
    @{ File = "graphviz-dag.dot";       Type = "graphviz";  Name = "graphviz-dag" }
    @{ File = "d2-arch.d2";            Type = "d2";        Name = "d2-arch" }
    @{ File = "erd-schema.erd";        Type = "erd";       Name = "erd-schema" }
    @{ File = "figures/test_simple_diagram.drawio";      Type = "drawio"; Name = "test-simple-diagram" }
)

$passed = 0
$failed = 0

foreach ($d in $diagrams) {
    $src = Join-Path $scriptDir $d.File
    $outSvg = Join-Path $outDir "$($d.Name).svg"
    $outPng = Join-Path $outDir "$($d.Name).png"

    # SVG
    Write-Host "[$($d.Type)] $($d.File) -> SVG ... " -NoNewline
    & $ktwDocs diagram $d.Type $src -o $outSvg -f svg 2>&1 | Out-Null
    if ($LASTEXITCODE -eq 0 -and (Test-Path $outSvg)) {
        $size = (Get-Item $outSvg).Length
        Write-Host "OK $($size) bytes" -ForegroundColor Green
        $passed++
    } else {
        Write-Host "FAILED" -ForegroundColor Red
        & $ktwDocs diagram $d.Type $src -o $outSvg -f svg 2>&1 | Write-Host -ForegroundColor DarkRed
        $failed++
    }

    # PNG
    Write-Host "[$($d.Type)] $($d.File) -> PNG ... " -NoNewline
    & $ktwDocs diagram $d.Type $src -o $outPng -f png 2>&1 | Out-Null
    if ($LASTEXITCODE -eq 0 -and (Test-Path $outPng)) {
        $size = (Get-Item $outPng).Length
        Write-Host "OK $($size) bytes" -ForegroundColor Green
        $passed++
    } else {
        Write-Host "FAILED" -ForegroundColor Red
        $failed++
    }
}

# Auto-detect test (no explicit type)
Write-Host ""
Write-Host "--- Auto-detection tests ---" -ForegroundColor Cyan
$autoFiles = @("mermaid-flow.mmd", "plantuml-classes.puml", "graphviz-dag.dot", "d2-arch.d2", "erd-schema.erd")
foreach ($f in $autoFiles) {
    $src = Join-Path $scriptDir $f
    $name = [System.IO.Path]::GetFileNameWithoutExtension($f)
    $out = Join-Path $outDir "auto-$name.svg"

    Write-Host "[auto] $f ... " -NoNewline
    & $ktwDocs diagram $src -o $out -f svg 2>&1 | Out-Null
    if ($LASTEXITCODE -eq 0 -and (Test-Path $out)) {
        Write-Host "OK" -ForegroundColor Green
        $passed++
    } else {
        Write-Host "FAILED" -ForegroundColor Red
        $failed++
    }
}

# Document rendering tests (requires pandoc + typst)
Write-Host ""
Write-Host "--- Document rendering tests ---" -ForegroundColor Cyan

$hasPandoc = $null -ne (Get-Command pandoc -ErrorAction SilentlyContinue)
if (-not $hasPandoc) {
    Write-Host "pandoc not found -- skipping document tests" -ForegroundColor Yellow
} else {
    $hasTypst = $null -ne (Get-Command typst -ErrorAction SilentlyContinue)

    $docs = @(
        @{ File = "sample-doc.md";        Name = "sample-doc" }
        @{ File = "doc-with-diagrams.md"; Name = "doc-with-diagrams" }
        @{ File = "..\embed\docs\markdown-reference.md"; Name = "markdown-reference" }
        @{ File = "test-diagrams.md"; Name = "test-diagrams" }
    )

    $formats = @("html")
    if ($hasTypst) { $formats += "pdf" }
    $formats += "docx"

    foreach ($d in $docs) {
        $src = Join-Path $scriptDir $d.File
        foreach ($fmt in $formats) {
            $out = Join-Path $outDir "$($d.Name).$fmt"

            Write-Host "[render] $($d.File) -> $fmt ... " -NoNewline
            & $ktwDocs render $src -o $out -t $fmt 2>&1 | Out-Null
            if ($LASTEXITCODE -eq 0 -and (Test-Path $out)) {
                $size = (Get-Item $out).Length
                Write-Host "OK $($size) bytes" -ForegroundColor Green
                $passed++
            } else {
                Write-Host "FAILED" -ForegroundColor Red
                & $ktwDocs render $src -o $out -t $fmt 2>&1 | Write-Host -ForegroundColor DarkRed
                $failed++
            }
        }
    }

    if (-not $hasTypst) {
        Write-Host "typst not found -- PDF tests skipped" -ForegroundColor Yellow
    }
}

Write-Host ""
Write-Host "Results: $passed passed, $failed failed" -ForegroundColor $(if ($failed -eq 0) { "Green" } else { "Yellow" })
Write-Host "Output: $outDir"
