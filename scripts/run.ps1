# LLM_NOTICE: 这里是生产环境启动脚本，开发环境在 pnpm dev，不要尝试运行此脚本来调试

# #region 配置项
$IDLE_TIMEOUT_MINUTES = 30 # 默认闲置时间（分钟），服务端日志无输出且有构建更新时自动重启
# #endregion

# #region 常量定义
$SCRIPT_DIR = Split-Path -Parent $MyInvocation.MyCommand.Path
$ROOT_DIR = Split-Path -Parent $SCRIPT_DIR
$BUILD_DIR = Join-Path $ROOT_DIR "build/latest"
$BINARY = Join-Path $BUILD_DIR "image-funnel.exe"
# #endregion

# #region 密钥生成
$BASE_BUILD_DIR = Join-Path $ROOT_DIR "build"
$SECRET_FILE = Join-Path $BASE_BUILD_DIR ".secret"

if (-not (Test-Path $BASE_BUILD_DIR)) {
    New-Item -ItemType Directory -Path $BASE_BUILD_DIR -Force | Out-Null
}

if (-not (Test-Path $SECRET_FILE)) {
    Write-Host "生成新的应用密钥..." -ForegroundColor Cyan
    $bytes = New-Object Byte[] 32
    $null = [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
    $secretString = [Convert]::ToBase64String($bytes)
    $secretString | Out-File -FilePath $SECRET_FILE -Encoding utf8 -NoNewline
}

$env:IMAGE_FUNNEL_SECRET_KEY = Get-Content $SECRET_FILE -Raw
# #endregion

# #region Git Hook 设置
function Register-GitHook {
    try {
        $gitArgs = "-C", "$ROOT_DIR"
        $gitPathResult = git $gitArgs rev-parse --git-path hooks/post-commit
        if ($LASTEXITCODE -ne 0 -or -not $gitPathResult) {
            return
        }
        $hookFile = $gitPathResult.Trim()
        $hooksDir = Split-Path -Parent $hookFile
        if (-not (Test-Path $hooksDir)) {
            New-Item -ItemType Directory -Path $hooksDir -Force | Out-Null
        }
        
        $hookCode = @'
# #region rebuild-trigger-e6fe8b49e7bf
REPO_DIR=$(git rev-parse --show-toplevel)
if [ -z "$(git status --porcelain -- ':(exclude)build' ':(exclude)scripts/run.ps1')" ]; then
    rm -f "$REPO_DIR/build/run/.delete_to_rebuild"
fi
# #endregion
'@
        $hookCode = $hookCode.Replace("`r`n", "`n")

        if (Test-Path $hookFile) {
            $content = [System.IO.File]::ReadAllText($hookFile)
            $content = $content.Replace("`r`n", "`n")
            
            $pattern = "(?s)#\s*#region\s+rebuild-trigger-e6fe8b49e7bf.*?#\s*#endregion"
            if ($content -match $pattern) {
                $content = $content -replace $pattern, $hookCode
            } else {
                if ($content -and -not $content.EndsWith("`n")) {
                    $content += "`n"
                }
                $content += $hookCode
            }
            [System.IO.File]::WriteAllText($hookFile, $content)
        } else {
            $content = "#!/bin/sh`n" + $hookCode
            [System.IO.File]::WriteAllText($hookFile, $content)
        }
    } catch {
        Write-Warning "自动设置 Git Hook 失败: $_"
    }
}

Register-GitHook
# #endregion

# #region 辅助函数
function Test-NeedsBuild {
    param(
        [bool]$Verbose = $false
    )
    if (-not (Test-Path $BINARY)) {
        if ($Verbose) {
            Write-Host "未找到构建好的二进制文件，正在初始构建..." -ForegroundColor Yellow
        }
        return $true
    }
    try {
        $binaryTime = (Get-Item $BINARY).LastWriteTime
        $gitArgs = "-C", "$ROOT_DIR"
        if ($Verbose) {
            Write-Host "项目根目录: $ROOT_DIR" -ForegroundColor DarkGray
        }
        
        # 1. 检查工作区是否干净 (排除 build 目录和脚本自身)
        $gitStatus = git $gitArgs status --porcelain -- ":(exclude)build" ":(exclude)scripts/run.ps1"
        if ($gitStatus) {
            if ($Verbose) {
                Write-Host "工作区有未提交的更改，跳过自动构建。" -ForegroundColor Gray
            }
            return $false
        }
        
        # 2. 工作区干净，检查最新提交是否晚于构建时间
        $commitInfo = git $gitArgs log -1 --format="%ct|%h|%D"
        if ($commitInfo) {
            $parts = $commitInfo.Split("|")
            $lastCommitTimestamp = $parts[0]
            $lastCommitHash = $parts[1]
            $lastCommitRef = $parts[2]
            
            $lastCommitTime = [DateTimeOffset]::FromUnixTimeSeconds([long]$lastCommitTimestamp).LocalDateTime
            
            if ($Verbose) {
                Write-Host ("最新提交: {0:yyyy-MM-dd HH:mm:ss} ({1}) [{2}]" -f $lastCommitTime, $lastCommitHash, $lastCommitRef) -ForegroundColor Gray
                Write-Host ("当前构建: {0:yyyy-MM-dd HH:mm:ss}" -f $binaryTime) -ForegroundColor Gray
            }

            if ($lastCommitTime.Ticks -gt $binaryTime.Ticks) {
                if ($Verbose) {
                    Write-Host "检测到新提交，正在重新构建..." -ForegroundColor Yellow
                }
                return $true
            } else {
                if ($Verbose) {
                    Write-Host "当前构建已是最新。" -ForegroundColor Gray
                }
            }
        }
    } catch {
        Write-Error "检查更新时发生异常: $_"
    }
    return $false
}

# 判断路径是否带 Low 完整性标签。Windows 以「令牌级别与文件标签中较低者」作为新进程的
# 完整性级别，Low 标签的二进制会让整个服务降级为 Low 进程，无法写入全局 Medium 缓存
# （例如 uv cache），hook 里的 uv 调用会直接报 Access denied。
# icacls 的输出随系统语言本地化，因此同时匹配 SID 与中英文级别名。
function Test-LowIntegrity {
    param([string]$Path)
    $output = @(icacls $Path 2>&1 | ForEach-Object { [string]$_ })
    if ($LASTEXITCODE -ne 0) {
        Write-Warning "无法读取完整性标签: $Path"
        return $false
    }
    return (($output -join "`n") -match 'S-1-16-4096\b|Low Mandatory Level|低强制级别|低完整性')
}
# #endregion

# 进程清理钩子：确保关闭窗口或强制退出时也能结束子进程
$script:current_process = $null
$exit_event = "ImageFunnel_Process_Exit_Handler"
Get-EventSubscriber -SourceIdentifier $exit_event -ErrorAction SilentlyContinue | Unregister-Event
Register-EngineEvent -SourceIdentifier PowerShell.Exiting -SupportEvent -Action {
    if ($script:current_process -and -not $script:current_process.HasExited) {
        $script:current_process.Kill()
    }
}

$exitCode = 0
$shouldRestart = $false

while ($true) {
    $shouldRestart = $false

    # #region 自动构建检查
    if (Test-NeedsBuild -Verbose $true) {
        & (Join-Path $SCRIPT_DIR "build.ps1")
        Write-Host ""
        if ($LASTEXITCODE -ne 0) {
            Write-Error "❌ 构建失败，无法运行。"
            exit $LASTEXITCODE
        }
    }
    # #endregion

    # #region 运行
    # 运行目录必须不带 Low 标签：Low 的二进制会让服务整体降级为 Low 进程，
    # 无法写入全局 Medium 缓存。build 目录若继承了工作区的 Low 标签，
    # 就改用系统临时目录下的专属运行目录——复制到无标签目录会按目标重设标签。
    $BUILD_PARENT_DIR = Join-Path $ROOT_DIR "build"
    $RUN_DIR = Join-Path $BUILD_PARENT_DIR "run"
    if (Test-LowIntegrity $BUILD_PARENT_DIR) {
        $RUN_DIR = Join-Path $env:TEMP "image-funnel-run"
        Write-Warning "build 目录带 Low 完整性标签，改用运行目录: $RUN_DIR"
    }

    # 确保运行目录干净
    if (Test-Path $RUN_DIR) {
        Remove-Item -Path $RUN_DIR -Recurse -Force -ErrorAction SilentlyContinue
    }
    New-Item -ItemType Directory -Path $RUN_DIR -Force | Out-Null

    $stdoutEvent = $null
    $stderrEvent = $null
    $process = $null

    try {
        Write-Host "正在准备运行环境 (目录: $RUN_DIR)..." -ForegroundColor Cyan
        Copy-Item -Path "$BUILD_DIR\*" -Destination $RUN_DIR -Recurse -Force -ProgressAction SilentlyContinue

        # 重新生成标记文件供下一次通知
        $rebuildFlagFile = Join-Path $RUN_DIR ".delete_to_rebuild"
        New-Item -ItemType File -Path $rebuildFlagFile -Force | Out-Null

        $runBinary = Join-Path $RUN_DIR "image-funnel.exe"

        # 启动前确认二进制不再是 Low：这里失败说明运行目录本身带了 Low 标签，
        # 继续启动只会让 hook 里的外部工具在运行时才报 Access denied。
        if (Test-LowIntegrity $runBinary) {
            throw "运行目录仍带 Low 完整性标签，无法启动: $RUN_DIR"
        }

        Write-Host "--- 开始运行 ---" -ForegroundColor Green
        
        # 共享状态对象，用于事件与主线程通信
        $sharedState = [PSCustomObject]@{
            LastLogTime = [DateTime]::Now
            NeedsRebuild = $false
        }

        # 创建 .NET 进程实例
        $process = New-Object System.Diagnostics.Process
        $script:current_process = $process
        
        $process.StartInfo.FileName = $runBinary
        $process.StartInfo.Arguments = $args -join " "
        # 保持使用当前 Shell 的工作目录，确保调用者传入的相对路径参数能被正确解析
        $process.StartInfo.WorkingDirectory = (Get-Location).Path
        $process.StartInfo.CreateNoWindow = $true
        $process.StartInfo.RedirectStandardOutput = $true
        $process.StartInfo.RedirectStandardError = $true
        $process.StartInfo.UseShellExecute = $false
        
        # 注册标准输出事件
        $stdoutEvent = Register-ObjectEvent -InputObject $process -EventName OutputDataReceived -MessageData $sharedState -Action {
            $data = $Event.SourceEventArgs.Data
            if ($data -ne $null) {
                Write-Host $data
                $Event.MessageData.LastLogTime = [DateTime]::Now
            }
        }
        
        # 注册标准错误事件
        $stderrEvent = Register-ObjectEvent -InputObject $process -EventName ErrorDataReceived -MessageData $sharedState -Action {
            $data = $Event.SourceEventArgs.Data
            if ($data -ne $null) {
                [Console]::Error.WriteLine($data)
                $Event.MessageData.LastLogTime = [DateTime]::Now
            }
        }

        # 启用退出事件并注册到专属事件源
        $process.EnableRaisingEvents = $true
        Get-EventSubscriber -SourceIdentifier "ProcessExited" -ErrorAction SilentlyContinue | Unregister-Event
        $exitEvent = Register-ObjectEvent -InputObject $process -EventName Exited -SourceIdentifier "ProcessExited"

        # 启动进程并开始读取
        $process.Start() | Out-Null
        $process.BeginOutputReadLine()
        $process.BeginErrorReadLine()
        
        # 创建文件系统事件监听器以检测标记文件是否被删除
        $watcher = New-Object System.IO.FileSystemWatcher
        $watcher.Path = $RUN_DIR
        $watcher.Filter = ".delete_to_rebuild"
        $watcher.EnableRaisingEvents = $true
        
        Get-EventSubscriber -SourceIdentifier "RebuildIndicatorDeleted" -ErrorAction SilentlyContinue | Unregister-Event
        $watcherEvent = Register-ObjectEvent -InputObject $watcher -EventName Deleted -SourceIdentifier "RebuildIndicatorDeleted"
        
        # 防漏初始检查：如果在设置监听前标记文件已经不存在，则直接标记需要重建
        if (-not (Test-Path $rebuildFlagFile)) {
            $sharedState.NeedsRebuild = $true
        }

        # 初始清理事件队列
        Get-Event | Remove-Event -ErrorAction SilentlyContinue

        # 主循环：挂起等待事件唤醒
        while (-not $process.HasExited) {
            # 1. 检查事件队列中是否收到了删除标记文件事件
            $rebuildEvents = Get-Event -SourceIdentifier "RebuildIndicatorDeleted" -ErrorAction SilentlyContinue
            if ($rebuildEvents) {
                $sharedState.NeedsRebuild = $true
                $rebuildEvents | Remove-Event
            }

            if (-not $sharedState.NeedsRebuild) {
                # 如果不需要重建，主线程无限期等待事件到达（进程退出、日志输出、或者标记文件被删除）
                $null = Wait-Event
            } else {
                # 如果需要重建，计算距离超时的剩余时间
                $idleDuration = [DateTime]::Now - $sharedState.LastLogTime
                $timeoutSeconds = $IDLE_TIMEOUT_MINUTES * 60
                $remainingSeconds = $timeoutSeconds - $idleDuration.TotalSeconds
                
                if ($remainingSeconds -le 0) {
                    Write-Host ("`n[自动更新] 检测到服务端日志已闲置超过 {0} 分钟，且已收到构建更新通知。正在重启服务..." -f $IDLE_TIMEOUT_MINUTES) -ForegroundColor Yellow
                    try {
                        $process.Kill()
                        $null = $process.WaitForExit(5000)
                    } catch {}
                    $shouldRestart = $true
                    break
                } else {
                    # 还有剩余时间，精准等待剩余秒数
                    # 期间若有日志产生或进程退出，会立刻唤醒主线程以便重新计算防抖时间或退出
                    $null = Wait-Event -Timeout $remainingSeconds
                }
            }
            
            # 仅清理已被消费的 stdout 和 stderr 日志事件，避免内存泄露，同时不影响控制事件
            if ($stdoutEvent) { $null = Get-Event -SourceIdentifier $stdoutEvent.Name -ErrorAction SilentlyContinue | Remove-Event }
            if ($stderrEvent) { $null = Get-Event -SourceIdentifier $stderrEvent.Name -ErrorAction SilentlyContinue | Remove-Event }
        }
        
        # 记录程序退出码（只在非自动更新重启时记录）
        if (-not $shouldRestart) {
            $exitCode = $process.ExitCode
        }
    }
    catch {
        Write-Error "运行过程中发生错误: $_"
        $exitCode = 1
    }
    finally {
        # 1. 注销事件
        if ($stdoutEvent) { Unregister-Event -SourceIdentifier $stdoutEvent.Name -ErrorAction SilentlyContinue }
        if ($stderrEvent) { Unregister-Event -SourceIdentifier $stderrEvent.Name -ErrorAction SilentlyContinue }
        if ($watcherEvent) { Unregister-Event -SourceIdentifier $watcherEvent.Name -ErrorAction SilentlyContinue }
        if ($exitEvent) { Unregister-Event -SourceIdentifier $exitEvent.Name -ErrorAction SilentlyContinue }
        
        # 2. 释放 watcher 资源
        if ($watcher) {
            try {
                $watcher.EnableRaisingEvents = $false
                $watcher.Dispose()
            } catch {}
        }
        
        # 3. 确保释放进程句柄并终止它
        if ($process) {
            if (-not $process.HasExited) {
                try {
                    $process.Kill()
                    $null = $process.WaitForExit(5000)
                } catch {}
            }
            try { $process.Dispose() } catch {}
        }
        
        # 4. 清理并移除事件队列中的所有事件
        $null = Get-Event | Remove-Event -ErrorAction SilentlyContinue

        # 5. 清理临时目录
        if (Test-Path $RUN_DIR) {
            Remove-Item -Path $RUN_DIR -Recurse -Force -ErrorAction SilentlyContinue
            Write-Host "`n--- 运行结束，清理完成 ---" -ForegroundColor Gray
        }
    }
    # #endregion

    # 如果不需要重启，则正常退出循环
    if (-not $shouldRestart) {
        break
    }
    
    Write-Host "检测到版本已更新，正在重新启动脚本逻辑..." -ForegroundColor Cyan
}

exit $exitCode
