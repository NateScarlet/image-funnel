$ErrorActionPreference = "Stop"

function Check-Command {
    param($Name)
    Get-Command $Name -ErrorAction SilentlyContinue | Out-Null
    return $?
}

function Add-To-Path {
    param($Path)
    if (Test-Path $Path) {
        $env:PATH = "$Path;$env:PATH"
        Write-Host "👉 Added to PATH: $Path" -ForegroundColor DarkGray
    }
}

Write-Host "🔍 Checking environment..." -ForegroundColor Cyan

# 1. Check ImageMagick
if (Check-Command "magick") {
    Write-Host "✅ ImageMagick found." -ForegroundColor Green
} else {
    Write-Host "⚠️ ImageMagick not found in PATH." -ForegroundColor Yellow
    
    # Try Winget
    if (Check-Command "winget") {
        Write-Host "Trying to install via Winget..." -ForegroundColor Cyan
        try {
            winget install ImageMagick.ImageMagick -e --silent --accept-package-agreements --accept-source-agreements
            
            # Refresh PATH from Registry
            $machinePath = [Environment]::GetEnvironmentVariable("Path", "Machine")
            $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
            $env:PATH = "$machinePath;$userPath;$env:PATH"
            
            if (Check-Command "magick") {
                Write-Host "✅ ImageMagick installed successfully." -ForegroundColor Green
            } else {
                throw "Installation verified failed"
            }
        } catch {
            Write-Host "❌ Winget install failed or cancelled." -ForegroundColor Red
        }
    }

    # If still not found, try portable
    if (-not (Check-Command "magick")) {
        Write-Host "Trying to download portable version..." -ForegroundColor Cyan
        $magickDir = Join-Path $PSScriptRoot "imagemagick_portable"
        $binPath = $magickDir
        
        # Check if already downloaded
        if (Test-Path $magickDir) {
           # Check for subfolder if it extracted with a root folder
           $subFolders = Get-ChildItem $magickDir -Directory
           if ($subFolders.Count -eq 1) {
               $binPath = $subFolders[0].FullName
           }
        } else {
            $url = "https://download.imagemagick.org/ImageMagick/download/binaries/ImageMagick-portable-Q16-x64.zip" 
            $zipPath = Join-Path $PSScriptRoot "magick.zip"
            
            try {
                Write-Host "⬇️ Downloading ImageMagick Portable from $url..."
                Invoke-WebRequest -Uri $url -OutFile $zipPath -UseBasicParsing
                
                Write-Host "📦 Extracting..."
                Expand-Archive -Path $zipPath -DestinationPath $magickDir -Force
                Remove-Item $zipPath -ErrorAction SilentlyContinue
                
                # Check for subfolder again after extraction
                $subFolders = Get-ChildItem $magickDir -Directory
                if ($subFolders.Count -eq 1) {
                    $binPath = $subFolders[0].FullName
                }
                
                Write-Host "✅ Portable version prepared." -ForegroundColor Green
            } catch {
                Write-Host "❌ Failed to download portable version." -ForegroundColor Red
                Write-Host "Original images will be served instead of compressed ones." -ForegroundColor Yellow
            }
        }
        
        Add-To-Path $binPath
    }
}

# 2. Secret Key Setup
$secretFile = Join-Path $PSScriptRoot ".secret_key"
if (-not $env:IMAGE_FUNNEL_SECRET_KEY) {
   if (Test-Path $secretFile) {
       $key = Get-Content $secretFile -Raw
       $env:IMAGE_FUNNEL_SECRET_KEY = $key.Trim()
       Write-Host "🔑 Loaded secret key locally." -ForegroundColor DarkGray
   } else {
       $bytes = New-Object byte[] 32
       [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
       $key = [Convert]::ToBase64String($bytes)
       $key | Set-Content $secretFile -NoNewline
       $env:IMAGE_FUNNEL_SECRET_KEY = $key
       Write-Host "🔑 Generated new local secret key." -ForegroundColor DarkGray
   }
}

# 3. Start Application
$exePath = Join-Path $PSScriptRoot "image-funnel.exe"
if (-not (Test-Path $exePath)) {
    Write-Error "image-funnel.exe not found in $PSScriptRoot"
    exit 1
}

Write-Host "🚀 Starting ImageFunnel..." -ForegroundColor Green
Write-Host "🌐 Opening Browser..." -ForegroundColor Cyan

# Open Browser (Async)
Start-Process "http://localhost:34898"

# Start Server (Blocking)
& $exePath
