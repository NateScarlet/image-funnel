
param(
    # 透传给 Python 解释器的全部参数（脚本路径 + 脚本自己的参数）
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]] $PythonArgs
)

# 用项目 .venv 执行任意 Python 脚本，并预先配好受限沙箱所需的两个环境设置。
# 需要与 check-python.ps1 保持一致的原因见该脚本注释：临时目录重定向到 .scratch，
# 以及通过 sitecustomize 兼容层放宽 mkdtemp 的目录权限。
#
# 用法：
#   pwsh scripts/python.ps1 .scratch/demo/script.py arg1 arg2
#
# 之所以做成子进程包装而不是 dot-source 的环境脚本：本仓库的运行环境是
# PowerShell ConstrainedLanguage 模式，dot-source 会报「在不同语言模式下定义」，
# 因此环境设置必须在同一个进程内完成并直接拉起解释器。

if ($PythonArgs.Count -eq 0) {
    Write-Error "用法: pwsh scripts/python.ps1 <脚本路径> [参数...]"
    exit 1
}

$rootDir = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)

$scratchTmp = Join-Path $rootDir ".scratch/tmp"
New-Item -ItemType Directory -Force $scratchTmp | Out-Null
$env:TEMP = (Resolve-Path $scratchTmp).Path
$env:TMP = $env:TEMP

$env:PYTHONPATH = Join-Path $rootDir "scripts/python-sitecustomize"

# 直接启动解释器进程（stdio 继承），不经过 uv——沙箱内 uv 缓存目录不可写
$pythonExe = Join-Path $rootDir ".venv/Scripts/python.exe"

& $pythonExe @PythonArgs
exit $LASTEXITCODE
