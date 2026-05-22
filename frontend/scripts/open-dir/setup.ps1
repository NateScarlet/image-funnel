#requires -version 2.0
$ErrorActionPreference = "Stop"

$protocol = "io.github.natescarlet.open-dir"
$target = "$env:APPDATA\$protocol.cmd"
Copy-Item handle-protocol.ps1.cmd $target

reg add HKCU\Software\Classes\$protocol /f /v "URL Protocol"
if ($LASTEXITCODE) {
    throw "安装失败：无法编辑注册表"
}

reg add HKCU\Software\Classes\$protocol\shell\open\command /f /d "\`"$target\`" \`"%1\`""
if ($LASTEXITCODE) {
    throw "安装失败：无法编辑注册表"
}


Add-Type –AssemblyName PresentationFramework
[System.Windows.MessageBox]::Show("已将 ${protocol}: 链接关联至`n$target`n当前将在资源管理器中打开对应路径`n可编辑此脚本设置其他行为", "安装成功")
