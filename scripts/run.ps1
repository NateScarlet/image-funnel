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
        $binaryTime = (Get-Item $BINARY).LastWriteTime
        
        # 显式使用项目根目录执行 Git 命令
        $gitArgs = "-C", "$ROOT_DIR"
        Write-Host "项目根目录: $ROOT_DIR" -ForegroundColor DarkGray
        
        # 1. 检查工作区是否干净 (排除 build 目录和脚本自身)
        $gitStatus = git $gitArgs status --porcelain -- ":(exclude)build" ":(exclude)scripts/run.ps1"
        if ($gitStatus) {
            Write-Host "工作区有未提交的更改，跳过自动构建。" -ForegroundColor Gray
        } else {
            # 2. 工作区干净，检查最新提交是否晚于构建时间
            $commitInfo = git $gitArgs log -1 --format="%ct|%h|%D"
            if ($commitInfo) {
                $parts = $commitInfo.Split("|")
                $lastCommitTimestamp = $parts[0]
                $lastCommitHash = $parts[1]
                $lastCommitRef = $parts[2]
                
                $lastCommitTime = [DateTimeOffset]::FromUnixTimeSeconds([long]$lastCommitTimestamp).LocalDateTime
                
                Write-Host ("最新提交: {0:yyyy-MM-dd HH:mm:ss} ({1}) [{2}]" -f $lastCommitTime, $lastCommitHash, $lastCommitRef) -ForegroundColor Gray
                Write-Host ("当前构建: {0:yyyy-MM-dd HH:mm:ss}" -f $binaryTime) -ForegroundColor Gray

                if ($lastCommitTime.Ticks -gt $binaryTime.Ticks) {
                    Write-Host "检测到新提交，正在重新构建..." -ForegroundColor Yellow
                    $needsBuild = $true
                } else {
                    Write-Host "当前构建已是最新。" -ForegroundColor Gray
                }
            }
        }
    } catch {
        Write-Error "检查更新时发生异常: $_"
    }
}

if ($needsBuild) {
    & (Join-Path $SCRIPT_DIR "build.ps1")
    if ($LASTEXITCODE -ne 0) {
        Write-Error "❌ 构建失败，无法运行。"
        exit $LASTEXITCODE
    }
}
# #endregion

# #region 运行
$RUN_DIR = Join-Path $ROOT_DIR "build/run"

# 确保运行目录干净
if (Test-Path $RUN_DIR) {
    Remove-Item -Path $RUN_DIR -Recurse -Force -ErrorAction SilentlyContinue
}
New-Item -ItemType Directory -Path $RUN_DIR -Force | Out-Null

try {
    Write-Host "正在准备运行环境 (目录: $RUN_DIR)..." -ForegroundColor Cyan
    Copy-Item -Path "$BUILD_DIR\*" -Destination $RUN_DIR -Recurse -Force

    $runBinary = Join-Path $RUN_DIR "image-funnel.exe"
    
    Write-Host "--- 开始运行 ---" -ForegroundColor Green
    # 直接运行二进制文件，不切换目录，使程序默认使用当前 Shell 路径作为 Root (CWD)
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
