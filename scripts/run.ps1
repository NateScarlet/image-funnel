# #region 常量定义
$SCRIPT_DIR = Split-Path -Parent $MyInvocation.MyCommand.Path
$ROOT_DIR = Split-Path -Parent $SCRIPT_DIR
$BUILD_DIR = Join-Path $ROOT_DIR "build/latest"
$BINARY = Join-Path $BUILD_DIR "image-funnel.exe"
# #endregion

# #region 自动构建检查
$needsBuild = $false
if (-not (Test-Path $BINARY)) {
    Write-Host "未找到构建好的二进制文件，正在初始构建..." -ForegroundColor Yellow
    $needsBuild = $true
} else {
    try {
        $lastCommitTimestamp = git log -1 --format=%ct 2>$null
        if ($LASTEXITCODE -eq 0 -and $lastCommitTimestamp) {
            $lastCommitTime = [DateTimeOffset]::FromUnixTimeSeconds([long]$lastCommitTimestamp).LocalDateTime
            $binaryTime = (Get-Item $BINARY).LastWriteTime
            
            if ($lastCommitTime -gt $binaryTime) {
                Write-Host ("代码有更新 (提交: {0:yyyy-MM-dd HH:mm:ss}, 当前构建: {1:yyyy-MM-dd HH:mm:ss})，正在重新构建..." -f $lastCommitTime, $binaryTime) -ForegroundColor Yellow
                $needsBuild = $true
            }
        }
    } catch {}
}

if ($needsBuild) {
    & (Join-Path $SCRIPT_DIR "build.ps1")
    if ($LASTEXITCODE -ne 0) {
        Write-Host "❌ 构建失败，无法运行。" -ForegroundColor Red
        exit $LASTEXITCODE
    }
}
# #endregion

# #region 运行
$RUN_DIR = Join-Path $ROOT_DIR "build/run"

# 确保运行目录干净
if (Test-Path $RUN_DIR) {
    Remove-Item -Path $RUN_DIR -Recurse -Force
}
New-Item -ItemType Directory -Path $RUN_DIR -Force | Out-Null

try {
    Write-Host "正在准备运行环境 (目录: $RUN_DIR)..." -ForegroundColor Cyan
    Copy-Item -Path "$BUILD_DIR\*" -Destination $RUN_DIR -Recurse -Force

    $runBinary = Join-Path $RUN_DIR "image-funnel.exe"
    
    Write-Host "--- 开始运行 ---" -ForegroundColor Green
    & $runBinary $args
}
finally {
    if (Test-Path $RUN_DIR) {
        # 尝试清理，如果进程还在锁定则忽略错误
        Remove-Item -Path $RUN_DIR -Recurse -Force -ErrorAction SilentlyContinue
        Write-Host "`n--- 运行结束，清理完成 ---" -ForegroundColor Gray
    }
}
# #endregion
