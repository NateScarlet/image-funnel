param (
    [string[]]$Files,
    [string]$LaunchArgs = "-NoProfile -Sta"
)

$utf8NoBOM = New-Object System.Text.UTF8Encoding($false)

foreach ($filePath in $Files) {
    try {
        if (-not (Test-Path $filePath)) {
            Write-Warning "File not found: $filePath"
            continue
        }
        
        $absolutePath = (Resolve-Path $filePath).Path
        Write-Host "Compiling: $absolutePath"
        
        # 以 UTF-8 无 BOM 格式读取 ps1 文件内容
        $ps1Content = [System.IO.File]::ReadAllText($absolutePath, $utf8NoBOM)
        
        $wrapper = @"
@echo off
SETLOCAL EnableDelayedExpansion
SET "ARG0=%0"
SET /A index=1
FOR %%i in (%*) DO (
  SET "ARG!index!=%%i"
  SET /A index+=1
)
PowerShell ${LaunchArgs} -Command "Get-Content '%~dpnx0' -Encoding UTF8 | Select-Object -Skip 12 | Out-String | Invoke-Expression"
IF ERRORLEVEL 1 PAUSE
GOTO :EOF

"@
        
        $combined = $wrapper + $ps1Content
        $combined = $combined -replace "\r?\n", "`r`n"
        
        $dstPath = "$absolutePath.cmd"
        [System.IO.File]::WriteAllText($dstPath, $combined, $utf8NoBOM)
        Write-Host "Compiled: $dstPath"
    }
    catch {
        Write-Error "编译失败 $filePath : $_"
        exit 1
    }
}
