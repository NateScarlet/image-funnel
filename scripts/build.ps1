# ImageFunnel 构建脚本
# 执行后将编译结果和前端文件输出到 build/latest/
# 执行前清理已有文件

# 设置变量
$SCRIPT_DIR = Split-Path -Parent $MyInvocation.MyCommand.Path
$ROOT_DIR = Split-Path -Parent $SCRIPT_DIR
$BUILD_DIR = Join-Path $ROOT_DIR "build/latest"
$FRONTEND_DIR = Join-Path $ROOT_DIR "frontend"
$FRONTEND_BUILD_DIR = Join-Path $BUILD_DIR "dist"

# 清理现有构建目录
Write-Host "清理现有构建目录..."
if (Test-Path $BUILD_DIR) {
    Remove-Item -Path $BUILD_DIR -Recurse -Force
}

# 创建构建目录
New-Item -ItemType Directory -Path $BUILD_DIR -Force | Out-Null
New-Item -ItemType Directory -Path $FRONTEND_BUILD_DIR -Force | Out-Null
Write-Host "创建构建目录: $BUILD_DIR"
Write-Host "创建前端目录: $FRONTEND_BUILD_DIR"

# 构建前端
Write-Host "构建前端项目..."
Push-Location $FRONTEND_DIR
pnpm install
pnpm run build
Pop-Location

# 复制前端构建文件
Write-Host "复制前端构建文件..."
$FRONTEND_DIST = Join-Path $FRONTEND_DIR "dist"
if (Test-Path $FRONTEND_DIST) {
    Copy-Item -Path "$FRONTEND_DIST\*" -Destination $FRONTEND_BUILD_DIR -Recurse -Force
} else {
    Write-Host "❌ 前端构建目录不存在: $FRONTEND_DIST"
    exit 1
}

# 构建后端
Write-Host "构建后端项目..."
Push-Location $ROOT_DIR
$gitVersion = git describe --tags --always --dirty 2>$null
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrEmpty($gitVersion)) {
    $gitVersion = "dev"
    Write-Host "无法获取 git 版本号，使用默认值: dev"
} else {
    Write-Host "获取到 git 版本号: $gitVersion"
}
$ldflags = "-X main.version=$gitVersion"
# 直接使用重定向，不捕获到变量
go build -ldflags "$ldflags" -o "$BUILD_DIR/image-funnel.exe" ./cmd/server 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Go编译失败"
    Pop-Location
    exit 1
}
Pop-Location

# 检查构建结果
Write-Host "构建完成，检查结果..."
if (Test-Path "$BUILD_DIR/image-funnel.exe") {
    Write-Host "✅ 后端构建成功: $BUILD_DIR/image-funnel.exe"
    Write-Host "⚠️ 注意: 后端构建未包含测试代码，可能 'go test ./...' 会失败"
} else {
    Write-Host "❌ 后端构建失败"
    exit 1
}

if (Test-Path "$FRONTEND_BUILD_DIR/index.html") {
    Write-Host "✅ 前端构建成功: $FRONTEND_BUILD_DIR/index.html"
} else {
    Write-Host "❌ 前端构建失败"
    exit 1
}

Write-Host "🎉 构建完成！"
Write-Host "构建结果位于: $BUILD_DIR"
