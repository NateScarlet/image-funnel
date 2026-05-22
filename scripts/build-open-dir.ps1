param (
    [string]$OutputDir
)

if (-not $OutputDir) {
    throw "OutputDir parameter is required"
}

$SCRIPT_DIR = Split-Path -Parent $MyInvocation.MyCommand.Path
$ROOT_DIR = Split-Path -Parent $SCRIPT_DIR
$OPEN_DIR_SRC = Join-Path $SCRIPT_DIR "open-dir"

Write-Host "自动编译并打包本地路径协议插件到 $OutputDir ..."

# 创建输出目录（如果不存在）
if (-not (Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
}

# 复制源文件到输出目录
Copy-Item -Path (Join-Path $OPEN_DIR_SRC "setup.ps1") -Destination $OutputDir -Force
Copy-Item -Path (Join-Path $OPEN_DIR_SRC "handle-protocol.ps1") -Destination $OutputDir -Force
Copy-Item -Path (Join-Path $OPEN_DIR_SRC "安装.cmd") -Destination $OutputDir -Force

Push-Location $ROOT_DIR
try {
    $setupPs = Join-Path $OutputDir "setup.ps1"
    $handlePs = Join-Path $OutputDir "handle-protocol.ps1"
    $LASTEXITCODE = 0
    & (Join-Path $SCRIPT_DIR "build-ps1-script-as-cmd.ps1") -Files $setupPs, $handlePs
    if ($LASTEXITCODE -ne 0) {
        Write-Host "❌ 协议自包含 CMD 编译失败"
        exit 1
    }
}
finally {
    Pop-Location
}

# 创建警告提示文件，提示不要直接在压缩软件里运行
$commentFile = Join-Path $OutputDir "!解压到单独文件夹后再安装，不支持直接在压缩软件中运行"
New-Item -ItemType File -Path $commentFile -Force | Out-Null

# 打包为 setup.zip
Compress-Archive -Path (Join-Path $OutputDir "安装.cmd"), (Join-Path $OutputDir "setup.ps1.cmd"), (Join-Path $OutputDir "handle-protocol.ps1.cmd"), $commentFile -DestinationPath (Join-Path $OutputDir "setup.zip") -Force

# 清理所有的临时和生成的文件，仅保留 setup.zip
Remove-Item -Path (Join-Path $OutputDir "setup.ps1") -Force
Remove-Item -Path (Join-Path $OutputDir "handle-protocol.ps1") -Force
Remove-Item -Path (Join-Path $OutputDir "安装.cmd") -Force
Remove-Item -Path (Join-Path $OutputDir "setup.ps1.cmd") -Force
Remove-Item -Path (Join-Path $OutputDir "handle-protocol.ps1.cmd") -Force
Remove-Item -Path $commentFile -Force

Write-Host "✅ 本地路径协议插件打包完成"
