& pyright .\example_hooks
if ($LASTEXITCODE) {
    exit $LASTEXITCODE
}

& python -m unittest discover -s .\example_hooks -p *_test.py
if ($LASTEXITCODE) {
    exit $LASTEXITCODE
}

& black .\example_hooks
if ($LASTEXITCODE) {
    exit $LASTEXITCODE
}

