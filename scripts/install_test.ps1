# Tests for install.ps1. Each case installs from a release built in a temporary
# directory, with the real sentinel.exe of this checkout inside, and asserts the
# exit code, what was printed and what is on disk. `cosign` is replaced by a small
# program first in PATH: the script under test has no test switches of its own.
# $env:INSTALL_SCRIPT points at another copy of the script, to run these cases
# against it. Runs on Windows PowerShell 5.1 and on PowerShell 7; needs Go.

$ErrorActionPreference = 'Stop'
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$root = Split-Path -Parent $here
$script = $env:INSTALL_SCRIPT
if (-not $script) { $script = Join-Path $here 'install.ps1' }
$shell = (Get-Process -Id $PID).Path
$work = Join-Path ([IO.Path]::GetTempPath()) ('sentinel-install-test-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
$total = 0
$failed = 0
$case = 0

# A real cosign on this machine would answer instead of "not installed": the cases
# decide by themselves whether one is there, so its directory is left out.
$cleanPath = (($env:Path -split ';') | Where-Object {
        $_ -and -not (Test-Path (Join-Path $_ 'cosign.exe')) -and -not (Test-Path (Join-Path $_ 'cosign.cmd'))
    }) -join ';'

$arch = 'amd64'
if ("$env:PROCESSOR_ARCHITEW6432$env:PROCESSOR_ARCHITECTURE" -like '*ARM64*') { $arch = 'arm64' }
$asset = "sentinel_windows_$arch.zip"

$build = Join-Path $work 'build'
New-Item -ItemType Directory -Path $build | Out-Null
Push-Location $root
try {
    & go build -o (Join-Path $build 'sentinel.exe') ./cmd/sentinel
    if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
}
finally { Pop-Location }

# New-Release <name> [-Without <file to leave out of the zip>]: one release directory.
function New-Release($name, $content = (Join-Path $build 'sentinel.exe')) {
    $rel = Join-Path $work $name
    New-Item -ItemType Directory -Path $rel | Out-Null
    Compress-Archive -LiteralPath $content -DestinationPath (Join-Path $rel $asset)
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $rel $asset)).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText((Join-Path $rel 'SHA256SUMS'), "$hash  $asset`n")
    [IO.File]::WriteAllText((Join-Path $rel 'SHA256SUMS.sigstore.json'), "{}`n")
    return $rel
}

# Install <release dir> [-Cosign <exit status>] [-Vars @{NAME = value}]  ->  sets $out, $code, $dir
function Install($rel, $Cosign = $null, $Vars = @{}) {
    $script:case++
    $script:dir = Join-Path $work "install.$($script:case)"
    $bin = Join-Path $work "bin.$($script:case)"
    New-Item -ItemType Directory -Path $bin | Out-Null
    $script:calls = Join-Path $bin 'cosign.calls'
    if ($null -ne $Cosign) {
        [IO.File]::WriteAllText((Join-Path $bin 'cosign.cmd'), "@echo off`r`necho %* >> `"%~dp0cosign.calls`"`r`nexit /b $Cosign`r`n")
    }
    $saved = @{}
    $set = @{ Path = "$bin;$cleanPath"; SENTINEL_DOWNLOAD_BASE = $rel; SENTINEL_INSTALL_DIR = $script:dir;
        SENTINEL_VERSION = ''; SENTINEL_REQUIRE_SIGNATURE = '' }
    foreach ($key in $Vars.Keys) { $set[$key] = $Vars[$key] }
    foreach ($key in $set.Keys) {
        $saved[$key] = [Environment]::GetEnvironmentVariable($key)
        [Environment]::SetEnvironmentVariable($key, $set[$key])
    }
    try {
        $ErrorActionPreference = 'Continue'
        $script:out = (& $shell -NoProfile -ExecutionPolicy Bypass -File $script 2>&1 | Out-String)
        $script:code = $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = 'Stop'
        foreach ($key in $saved.Keys) { [Environment]::SetEnvironmentVariable($key, $saved[$key]) }
    }
}

function Check($name, [scriptblock]$condition) {
    $script:total++
    if (& $condition) { Write-Output "ok   $name" }
    else {
        $script:failed++
        Write-Output "FAIL $name"
        Write-Output "     exit $code"
        Write-Output ("     " + (($out -split "`n" | Select-Object -First 5) -join "`n     "))
    }
}

function Installed { Test-Path -LiteralPath (Join-Path $dir 'sentinel.exe') -PathType Leaf }
function NothingInstalled { -not (Test-Path -LiteralPath $dir) -or @(Get-ChildItem -LiteralPath $dir -Force).Count -eq 0 }

try {
    $rel = New-Release 'rel'

    Install $rel
    Check 'installs sentinel.exe and says where' { $code -eq 0 -and (Installed) -and $out -match 'Installed sentinel \S+ \(go' -and $out.Contains($dir) }
    Check 'says the signature was not checked when cosign is absent' { $out.Contains('Signature not checked: cosign is not installed.') }
    Check 'says when the directory is not in PATH' { $out.Contains('is not in your PATH') }
    Check 'leaves one file in the directory' { @(Get-ChildItem -LiteralPath $dir -Force).Count -eq 1 }

    Install $rel
    $first = $dir
    Install $rel -Vars @{ SENTINEL_INSTALL_DIR = $first }
    Check 'installing again replaces the binary and leaves one file' { $code -eq 0 -and @(Get-ChildItem -LiteralPath $first -Force).Count -eq 1 }

    $tampered = New-Release 'tampered'
    Add-Content -LiteralPath (Join-Path $tampered $asset) -Value 'x'
    Install $tampered
    Check 'refuses an archive whose checksum does not match, and installs nothing' { $code -ne 0 -and (NothingInstalled) -and $out.Contains('does not match SHA256SUMS. Nothing was installed.') }

    $unlisted = New-Release 'unlisted'
    [IO.File]::WriteAllText((Join-Path $unlisted 'SHA256SUMS'), ('0' * 64) + "  something_else.zip`n")
    Install $unlisted
    Check 'refuses an archive that SHA256SUMS does not list' { $code -ne 0 -and (NothingInstalled) -and $out.Contains("SHA256SUMS does not list $asset") }

    $readme = Join-Path $work 'README.txt'
    [IO.File]::WriteAllText($readme, 'hello')
    $empty = New-Release 'empty' $readme
    Install $empty
    Check 'refuses an archive without the binary' { $code -ne 0 -and (NothingInstalled) -and $out.Contains('The archive has no sentinel.exe.') }

    Install $rel -Cosign 0
    Check 'checks the signature of SHA256SUMS when cosign is installed' {
        $code -eq 0 -and (Installed) -and $out.Contains('Signature checked') -and
        ((Get-Content -LiteralPath $calls -Raw) -match 'verify-blob --bundle \S*SHA256SUMS\.sigstore\.json --certificate-identity-regexp \S*release\S*yml@refs/tags/v\S* --certificate-oidc-issuer https://token\.actions\.githubusercontent\.com \S*SHA256SUMS')
    }

    Install $rel -Cosign 1
    Check 'refuses a release whose signature cosign rejects, and installs nothing' { $code -ne 0 -and (NothingInstalled) -and $out.Contains('The signature of SHA256SUMS is not valid. Nothing was installed.') }

    $unsigned = New-Release 'unsigned'
    Remove-Item -LiteralPath (Join-Path $unsigned 'SHA256SUMS.sigstore.json')
    Install $unsigned -Cosign 0
    Check 'refuses a release with no signature when cosign is installed' { $code -ne 0 -and (NothingInstalled) -and $out.Contains('The release has no signature for SHA256SUMS.') }

    Install $rel -Vars @{ SENTINEL_REQUIRE_SIGNATURE = '1' }
    Check 'with SENTINEL_REQUIRE_SIGNATURE=1, refuses to install without cosign' { $code -ne 0 -and (NothingInstalled) -and $out.Contains('cosign is not installed. Nothing was installed.') }

    Install 'http://example.invalid/releases'
    Check 'refuses a download address that is not https' { $code -ne 0 -and (NothingInstalled) -and $out.Contains('SENTINEL_DOWNLOAD_BASE must use https.') }

    foreach ($bad in 'latest', '1.2.3', 'v1.2.3/../../x', 'v1.2.3 x', 'v1.2.3;x') {
        Install $rel -Vars @{ SENTINEL_VERSION = $bad }
        Check "refuses the version '$bad', which is not a tag" { $code -ne 0 -and (NothingInstalled) -and $out.Contains('SENTINEL_VERSION must be a tag such as v0.1.0.') }
    }
}
finally {
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Output ''
Write-Output "$failed of $total checks failed"
if ($failed -ne 0) { exit 1 }
# Said outright: the last installer run of these cases is one that must fail, and a
# caller that reads $LASTEXITCODE would take its status for the status of the tests.
exit 0
