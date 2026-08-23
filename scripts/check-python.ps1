# 在受限沙箱中，系统临时目录下动态创建的子目录内部不可访问（sqlite/listdir 均被拒绝），
# 因此将 Python 的临时目录重定向到工作区内，测试代码通过 tempfile 标准机制自动生效
$scratchTmp = Join-Path $PSScriptRoot '..\.scratch\tmp'
New-Item -ItemType Directory -Force $scratchTmp | Out-Null
$env:TMPDIR = (Resolve-Path $scratchTmp).Path

# 受限沙箱中 Python 以 0o700 权限（tempfile.mkdtemp 的固定行为）创建的目录同样不可访问，
# 注入 sitecustomize 兼容层将临时目录创建放宽为默认权限
$env:PYTHONPATH = Join-Path $PSScriptRoot 'python-sitecustomize'

# 统一使用项目根 .venv 开发环境（依赖对齐 example_hooks/runner.py 的 PEP 723 声明 + pyright/black）；
# hook 部署运行仍由各脚本头部的 uv run 提供，二者互不影响。
# 直接启动解释器进程（stdio 继承），不经过 uv/管道，受限沙箱内同样可用
$pythonExe = Join-Path $PSScriptRoot '..\.venv\Scripts\python.exe'

& $pythonExe -m unittest discover -s .\example_hooks -p *_test.py
$unittestExitCode = $LASTEXITCODE

Remove-Item Env:\PYTHONPATH -ErrorAction SilentlyContinue

if ($unittestExitCode) {
    exit $unittestExitCode
}

& $pythonExe -m pyright .\example_hooks
if ($LASTEXITCODE) {
    exit $LASTEXITCODE
}

& $pythonExe -m black .\example_hooks
if ($LASTEXITCODE) {
    exit $LASTEXITCODE
}
