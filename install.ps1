# php-call-graph-viz installation script for Windows
# Usage: irm https://raw.githubusercontent.com/hightemp/php-call-graph-viz/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

$REPO = "hightemp/php-call-graph-viz"
$BINARY_NAME = "php-call-graph-viz"
$INSTALL_DIR = "$env:LOCALAPPDATA\php-call-graph-viz"

function Write-ColorOutput($message, $color = "White") {
    Write-Host $message -ForegroundColor $color
}

function Get-LatestRelease {
    try {
        $response = Invoke-RestMethod -Uri "https://api.github.com/repos/$REPO/releases/latest"
        return $response.tag_name
    } catch {
        Write-ColorOutput "[ERROR] Failed to get latest release: $_" "Red"
        exit 1
    }
}

function Get-Architecture {
    $arch = [System.Environment]::Is64BitOperatingSystem
    if ($arch) {
        return "x86_64"
    } else {
        Write-ColorOutput "[ERROR] 32-bit Windows is not supported" "Red"
        exit 1
    }
}

function Install-Binary {
    Write-ColorOutput "[INFO] Installing $BINARY_NAME..." "Green"
    
    $arch = Get-Architecture
    $version = Get-LatestRelease
    
    if (-not $version) {
        Write-ColorOutput "[ERROR] Failed to get latest release version" "Red"
        exit 1
    }
    
    Write-ColorOutput "[INFO] Latest version: $version" "Green"
    
    # Construct download URL
    $versionNum = $version -replace '^v', ''
    $archiveName = "${BINARY_NAME}_${versionNum}_Windows_${arch}.zip"
    $downloadUrl = "https://github.com/$REPO/releases/download/$version/$archiveName"
    
    Write-ColorOutput "[INFO] Downloading $archiveName..." "Green"
    
    # Create temporary directory
    $tempDir = New-Item -ItemType Directory -Path "$env:TEMP\php-call-graph-viz-$(Get-Random)" -Force
    $archivePath = Join-Path $tempDir $archiveName
    
    try {
        # Download archive
        Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath -UseBasicParsing
        
        Write-ColorOutput "[INFO] Extracting archive..." "Green"
        
        # Extract archive
        Expand-Archive -Path $archivePath -DestinationPath $tempDir -Force
        
        # Create install directory
        if (-not (Test-Path $INSTALL_DIR)) {
            New-Item -ItemType Directory -Path $INSTALL_DIR -Force | Out-Null
        }
        
        # Copy binary
        $binaryPath = Join-Path $tempDir "$BINARY_NAME.exe"
        if (-not (Test-Path $binaryPath)) {
            Write-ColorOutput "[ERROR] Binary not found in archive" "Red"
            exit 1
        }
        
        Copy-Item -Path $binaryPath -Destination (Join-Path $INSTALL_DIR "$BINARY_NAME.exe") -Force
        
        Write-ColorOutput "[INFO] Successfully installed $BINARY_NAME $version" "Green"
        
        # Add to PATH if not already there
        $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
        if ($userPath -notlike "*$INSTALL_DIR*") {
            Write-ColorOutput "[INFO] Adding $INSTALL_DIR to PATH..." "Yellow"
            [Environment]::SetEnvironmentVariable("Path", "$userPath;$INSTALL_DIR", "User")
            Write-ColorOutput "[WARN] Please restart your terminal to use $BINARY_NAME" "Yellow"
        } else {
            Write-ColorOutput "[INFO] Run '$BINARY_NAME --help' to get started" "Green"
            Write-ColorOutput "[INFO] You may need to restart your terminal" "Yellow"
        }
        
    } catch {
        Write-ColorOutput "[ERROR] Installation failed: $_" "Red"
        exit 1
    } finally {
        # Cleanup
        if (Test-Path $tempDir) {
            Remove-Item -Path $tempDir -Recurse -Force
        }
    }
}

# Main
Install-Binary
