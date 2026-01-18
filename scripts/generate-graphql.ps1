# ImageFunnel GraphQL 生成脚本
# 同时更新后端和前端的 GraphQL 相关代码
# 可以在任意工作目录下执行

Write-Host "=== ImageFunnel GraphQL 生成脚本 ===" -ForegroundColor Green

# 获取脚本所在目录的绝对路径
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
# 获取项目根目录（脚本目录的父目录）
$ProjectRoot = Split-Path -Parent $ScriptDir

Write-Host "脚本目录: $ScriptDir" -ForegroundColor Gray
Write-Host "项目根目录: $ProjectRoot" -ForegroundColor Gray


# 更新后端 GraphQL 代码
Write-Host "1. 更新后端 GraphQL 代码..." -ForegroundColor Cyan
Push-Location $ProjectRoot
try {
    go generate ./graph

    if ($LASTEXITCODE -ne 0) {
        Write-Host "后端 GraphQL 代码更新失败！" -ForegroundColor Red
        exit 1
    }

    # 搜索未实现的 resolver
    Write-Host "检查未实现的 resolver..." -ForegroundColor Cyan
    $notImplementedFiles = Get-ChildItem -Path "graph" -Filter "*.resolvers.go" -Recurse | Where-Object {
        $content = Get-Content $_.FullName -Raw
        $content -match "not implemented:"
    }

    if ($notImplementedFiles.Count -gt 0) {
        Write-Host "发现 $($notImplementedFiles.Count) 个文件包含未实现的 resolver:" -ForegroundColor Yellow
        foreach ($file in $notImplementedFiles) {
            Write-Host "  - $($file.FullName)" -ForegroundColor Yellow
        }
        Write-Host "后端 GraphQL 代码更新成功，但还需要实现 resolver！" -ForegroundColor Yellow
    }
    else {
        Write-Host "后端 GraphQL 代码更新成功！" -ForegroundColor Green
    }
}
finally {
    Pop-Location
}

# 更新前端 GraphQL 代码
Write-Host "2. 更新前端 GraphQL 代码..." -ForegroundColor Cyan
Push-Location (Join-Path $ProjectRoot "frontend")
try {
    pnpm generate:graphql

    if ($LASTEXITCODE -ne 0) {
        Write-Host "前端 GraphQL 代码更新失败！" -ForegroundColor Red
        exit 1
    }
    Write-Host "前端 GraphQL 代码更新成功！" -ForegroundColor Green
}
finally {
    Pop-Location
}

Write-Host "=== GraphQL 更新完成 ===" -ForegroundColor Green
Write-Host ""
Write-Host "提示：如果修改了后端代码，请运行 .\scripts\build.ps1 来重新编译整个项目" -ForegroundColor Yellow
