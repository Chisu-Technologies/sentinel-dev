# Installs the sentinel command for the current user, on Windows.
#
#   irm https://raw.githubusercontent.com/Chisu-Technologies/sentinel-dev/main/scripts/install.ps1 | iex
#
# It asks for no elevation and writes one file: sentinel.exe, in
# SENTINEL_INSTALL_DIR. The archive is checked against the SHA256SUMS of its
# release before anything is installed. It runs on Windows PowerShell 5.1 and on
# PowerShell 7. On Linux and macOS, use install.sh.
#
#   SENTINEL_VERSION            a release tag such as v0.1.0. Empty: the latest release.
#   SENTINEL_INSTALL_DIR        where the binary goes. Empty: %LOCALAPPDATA%\Programs\sentinel.
#   SENTINEL_DOWNLOAD_BASE      where releases are read from: an https address that
#                               mirrors the release pages, or a directory that holds
#                               the files of one release. Empty: GitHub.
#   SENTINEL_REQUIRE_SIGNATURE  1: stop unless the signature of SHA256SUMS was checked.

function Install-Sentinel {
    $ErrorActionPreference = 'Stop'
    $repo = 'Chisu-Technologies/sentinel-dev'
    # Who may have signed a release: the release workflow of this repository, run
    # for a version tag. Nothing else.
    $signerPattern = '^https://github\.com/' + $repo + '/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$'
    $signerIssuer = 'https://token.actions.githubusercontent.com'

    $version = "$env:SENTINEL_VERSION"
    # The version becomes part of an address, so it is a tag and nothing more.
    if ($version -ne '' -and $version -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+[A-Za-z0-9.+-]*$') {
        throw 'SENTINEL_VERSION must be a tag such as v0.1.0.'
    }

    $base = "$env:SENTINEL_DOWNLOAD_BASE"
    if ($base -eq '') { $base = "https://github.com/$repo/releases" }
    $base = $base.TrimEnd('/', '\')
    $fromDirectory = $false
    if ($base -like 'https://*') {
        # Windows PowerShell 5.1 does not offer TLS 1.2 unless asked.
        [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    }
    elseif ($base -like 'http://*') {
        throw 'SENTINEL_DOWNLOAD_BASE must use https.'
    }
    elseif (Test-Path -LiteralPath $base -PathType Container) {
        $fromDirectory = $true
    }
    else {
        throw 'SENTINEL_DOWNLOAD_BASE is not an https address or a directory.'
    }

    $machine = "$env:PROCESSOR_ARCHITEW6432"
    if ($machine -eq '') { $machine = "$env:PROCESSOR_ARCHITECTURE" }
    switch ($machine) {
        'AMD64' { $arch = 'amd64' }
        'ARM64' { $arch = 'arm64' }
        default { throw "There is no release for the $machine architecture." }
    }
    $asset = "sentinel_windows_$arch.zip"

    # Without a version, ask GitHub which release is the latest and download that
    # tag. The `latest/download` address went on pointing at the previous release
    # for minutes after a new one was published, and a checksum list and an
    # archive read through it could even come from two different releases.
    if ($version -eq '' -and -not $fromDirectory -and $base -eq "https://github.com/$repo/releases") {
        try {
            $latest = Invoke-RestMethod -UseBasicParsing -Headers @{ 'Accept' = 'application/vnd.github+json'; 'Cache-Control' = 'no-cache' } `
                -Uri ("https://api.github.com/repos/$repo/releases/latest?nocache=" + [Guid]::NewGuid().ToString('N'))
            if ("$($latest.tag_name)" -match '^v[0-9]+\.[0-9]+\.[0-9]+$') { $version = "$($latest.tag_name)" }
        }
        catch {
            # The API is rate-limited per address; `latest/download` still works.
        }
    }

    $dir = "$env:SENTINEL_INSTALL_DIR"
    if ($dir -eq '') { $dir = Join-Path $env:LOCALAPPDATA 'Programs\sentinel' }

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ('sentinel-install-' + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        # One file of the release. Returns $false when it is not there.
        $fetch = {
            param($name, $destination)
            try {
                if ($fromDirectory) {
                    Copy-Item -LiteralPath (Join-Path $base $name) -Destination $destination
                }
                else {
                    if ($version -ne '') { $url = "$base/download/$version/$name" }
                    else { $url = "$base/latest/download/$name" }
                    Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $destination
                }
                return $true
            }
            catch { return $false }
        }

        $sums = Join-Path $tmp 'SHA256SUMS'
        $archive = Join-Path $tmp $asset
        if (-not (& $fetch 'SHA256SUMS' $sums)) { throw 'Could not read SHA256SUMS of the release.' }
        if (-not (& $fetch $asset $archive)) { throw "Could not download $asset." }

        # The checksum alone only says the download is the file the release lists;
        # the signature says who built the release.
        if (Get-Command cosign -ErrorAction SilentlyContinue) {
            $bundle = Join-Path $tmp 'SHA256SUMS.sigstore.json'
            if (-not (& $fetch 'SHA256SUMS.sigstore.json' $bundle)) {
                throw 'The release has no signature for SHA256SUMS.'
            }
            # What cosign writes to its error stream must not end the script by
            # itself: its exit status is the answer.
            $ErrorActionPreference = 'Continue'
            & cosign verify-blob --bundle $bundle --certificate-identity-regexp $signerPattern --certificate-oidc-issuer $signerIssuer $sums 2>&1 | Out-Null
            $verified = $LASTEXITCODE
            $ErrorActionPreference = 'Stop'
            if ($verified -ne 0) {
                throw 'The signature of SHA256SUMS is not valid. Nothing was installed.'
            }
            Write-Output "Signature checked: built by the release workflow of $repo."
        }
        elseif ("$env:SENTINEL_REQUIRE_SIGNATURE" -eq '1') {
            throw 'SENTINEL_REQUIRE_SIGNATURE is set and cosign is not installed. Nothing was installed.'
        }
        else {
            Write-Output 'Signature not checked: cosign is not installed.'
        }

        $want = ''
        foreach ($line in Get-Content -LiteralPath $sums) {
            $parts = $line -split '\s+', 2
            if ($parts.Count -eq 2 -and $parts[1].TrimStart('*') -eq $asset) { $want = $parts[0].ToLowerInvariant() }
        }
        if ($want -eq '') { throw "SHA256SUMS does not list $asset." }
        $got = (Get-FileHash -Algorithm SHA256 -LiteralPath $archive).Hash.ToLowerInvariant()
        if ($want -ne $got) {
            throw "The checksum of $asset does not match SHA256SUMS. Nothing was installed."
        }

        $unpacked = Join-Path $tmp 'unpacked'
        Expand-Archive -LiteralPath $archive -DestinationPath $unpacked
        $binary = Join-Path $unpacked 'sentinel.exe'
        if (-not (Test-Path -LiteralPath $binary -PathType Leaf)) { throw 'The archive has no sentinel.exe.' }

        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        # Written beside its destination and renamed, so an interrupted install
        # leaves the previous binary whole.
        $target = Join-Path $dir 'sentinel.exe'
        $staged = Join-Path $dir ('.sentinel.new.' + $PID + '.exe')
        Copy-Item -LiteralPath $binary -Destination $staged
        try {
            Move-Item -LiteralPath $staged -Destination $target -Force
        }
        catch {
            Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue
            throw "Could not replace $target. Close any running sentinel and try again."
        }

        $name = 'sentinel'
        try { $name = (& $target version) } catch { }
        Write-Output "Installed $name in $dir"
        # The folder goes on the user's PATH, so `sentinel` works in this window
        # and in every new one. Only the user's own PATH is changed, and only
        # when the folder is not on it yet. SENTINEL_NO_MODIFY_PATH=1 leaves it.
        $sameDir = { param($entry) $entry.TrimEnd('\') -ieq $dir.TrimEnd('\') }
        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        $entries = @("$userPath" -split ';' | Where-Object { $_ -ne '' })
        $onUserPath = @($entries | Where-Object { & $sameDir $_ }).Count -gt 0
        if ("$env:SENTINEL_NO_MODIFY_PATH" -eq '1') {
            if (-not $onUserPath) { Write-Output "$dir is not in your PATH. Add it, or run $target." }
        }
        else {
            if (-not $onUserPath) {
                [Environment]::SetEnvironmentVariable('Path', (($entries + $dir) -join ';'), 'User')
                Write-Output "Added $dir to your PATH."
            }
            if (@($env:Path -split ';' | Where-Object { & $sameDir $_ }).Count -eq 0) {
                $env:Path = "$env:Path;$dir"
            }
        }
    }
    finally {
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# Everything above only defines a function: a download cut short runs nothing.
Install-Sentinel
