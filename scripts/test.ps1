param (
    [switch]$Go,
    [switch]$Python,
    [switch]$Frontend
)

# 默认运行全部三者
if (-not $Go -and -not $Python -and -not $Frontend) {
    $Go = $true
    $Python = $true
    $Frontend = $true
}

$SCRIPT_DIR = Split-Path -Parent $MyInvocation.MyCommand.Path
$ROOT_DIR = Split-Path -Parent $SCRIPT_DIR
$SCRATCH_DIR = Join-Path $ROOT_DIR ".scratch"

# 在受限沙箱中，工作区外的构建缓存与动态创建的临时目录不可访问，
# 统一重定向到 .scratch 下，使测试在沙箱内外均可正常运行（.scratch 已被 git 忽略）
if ($Go) {
    $env:GOCACHE = Join-Path $SCRATCH_DIR "go-build"
}

Push-Location $ROOT_DIR
try {
    if ($Go) {
        Write-Host "运行 Go 测试..."
        go test --timeout 120s ./...
        if ($LASTEXITCODE -ne 0) {
            Write-Host "❌ Go 测试失败"
            exit 1
        }
        Write-Host "✅ Go 测试通过"
        Write-Host ""
    }

    if ($Python) {
        # check-python.ps1 自带 Python 临时目录的沙箱适配
        & (Join-Path $SCRIPT_DIR "check-python.ps1")
        if ($LASTEXITCODE -ne 0) {
            Write-Host "❌ Python 检查失败"
            exit 1
        }
        Write-Host "✅ Python 检查通过"
        Write-Host ""
    }

    if ($Frontend) {
        Write-Host "运行前端测试..."
        Push-Location (Join-Path $ROOT_DIR "frontend")
        try {
            # 经 package.json 的 test script 运行（自带 --configLoader native）：
            # bundle 模式打包 config 时会先于内置兼容层触发 net use 子进程，沙箱内必炸
            pnpm test
            if ($LASTEXITCODE -ne 0) {
                Write-Host "❌ 前端测试失败"
                exit 1
            }
        }
        finally {
            Pop-Location
        }
        Write-Host "✅ 前端测试通过"
    }
}
finally {
    Pop-Location
}
