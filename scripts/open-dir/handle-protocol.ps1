#requires -version 2.0
$ErrorActionPreference = "Stop"

$rawURL = $env:ARG1

if ($rawURL -match "`"(.+)`"") {
    $rawURL = $Matches[1]
}

$url = [uri]($rawURL)
if ($url.Scheme -ne "io.github.natescarlet.open-dir" ) {
    throw "invalid url: $url"
}
$path = $url.LocalPath
$path = $path -replace "/", "\"
while (-not (Test-Path $path)) {
    $path = Split-Path -Parent $path
    if (-not $path) {
        Add-Type –AssemblyName PresentationFramework
        [System.Windows.MessageBox]::Show("路径及其父级均不存在:`n$rawURL", "找不到路径")
        return
    }
}

if (Test-Path $path -PathType Container) {
    Start-Process explorer.exe -ArgumentList @(
        $path
    )
}
else {
    Start-Process explorer.exe -ArgumentList @(
        "/select,`"$path`""
    )
}
