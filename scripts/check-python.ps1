# 在受限沙箱中，系统临时目录下动态创建的子目录内部不可访问（sqlite/listdir 均被拒绝），
# 因此将 Python 的临时目录重定向到工作区内，测试代码通过 tempfile 标准机制自动生效
$scratchTmp = Join-Path $PSScriptRoot '..\.scratch\tmp'
New-Item -ItemType Directory -Force $scratchTmp | Out-Null
$env:TMPDIR = (Resolve-Path $scratchTmp).Path

# 受限沙箱中 Python 以 0o700 权限（tempfile.mkdtemp 的固定行为）创建的目录同样不可访问，
# 仅对 unittest 步骤注入 sitecustomize 兼容层，将临时目录创建放宽为默认权限
$env:PYTHONPATH = Join-Path $PSScriptRoot 'python-sitecustomize'

# 统一使用项目虚拟环境运行，与 pyright 解析到同一套依赖
$pythonExe = Join-Path $PSScriptRoot '..\example_hooks\.venv\Scripts\python.exe'

& $pythonExe -m unittest discover -s .\example_hooks -p *_test.py
$unittestExitCode = $LASTEXITCODE

# 及时移除注入，避免影响后续的 pyright / black 步骤
Remove-Item Env:\PYTHONPATH -ErrorAction SilentlyContinue

if ($unittestExitCode) {
    exit $unittestExitCode
}

& pyright .\example_hooks
if ($LASTEXITCODE) {
    exit $LASTEXITCODE
}

& black .\example_hooks
if ($LASTEXITCODE) {
    exit $LASTEXITCODE
}

