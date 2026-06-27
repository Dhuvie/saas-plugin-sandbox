# setup_toolchains.ps1
# Set execution policy for this session and enable TLS 1.2
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$ToolchainDir = Join-Path $PSScriptRoot ".toolchain"
if (-not (Test-Path $ToolchainDir)) {
    New-Item -ItemType Directory -Path $ToolchainDir | Out-Null
}

# Add local bin folder for any wrappers
$BinDir = Join-Path $PSScriptRoot ".bin"
if (-not (Test-Path $BinDir)) {
    New-Item -ItemType Directory -Path $BinDir | Out-Null
}

Write-Host "=========================================="
Write-Host "Setting up local user-space toolchains..."
Write-Host "Destination: $ToolchainDir"
Write-Host "=========================================="

# Helper function to download a file with progress using curl.exe
function Download-File ($url, $outputPath) {
    if (Test-Path $outputPath) {
        $file = Get-Item $outputPath
        if ($file.Length -gt 1024) {
            Write-Host "  File already exists and is valid: $(Split-Path $outputPath -Leaf)"
            return
        }
        Write-Host "  Removing invalid or zero-byte file: $(Split-Path $outputPath -Leaf)"
        Remove-Item $outputPath -Force
    }
    Write-Host "  Downloading $url..."
    curl.exe -L -o $outputPath $url
    Write-Host "  Download complete."
}

# Helper function to unzip a file quickly using .NET
function Unzip-File ($zipPath, $extractPath) {
    Write-Host "  Extracting $(Split-Path $zipPath -Leaf) to $extractPath..."
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    try {
        [System.IO.Compression.ZipFile]::ExtractToDirectory($zipPath, $extractPath)
        Write-Host "  Extraction complete."
    } catch {
        Write-Host "  Extraction failed! Removing corrupted file: $(Split-Path $zipPath -Leaf)"
        Remove-Item $zipPath -Force -ErrorAction SilentlyContinue
        throw $_
    }
}

# 1. Download & Extract WinLibs GCC (UCRT/SEH compatible for Go CGO traps)
$GCCZip = Join-Path $ToolchainDir "winlibs-13.2.0-r5.zip"
$GCCExtract = $ToolchainDir
$GCCExpectedFolder = Join-Path $ToolchainDir "mingw64"
Download-File "https://github.com/brechtsanders/winlibs_mingw/releases/download/13.2.0posix-17.0.6-11.0.1-ucrt-r5/winlibs-x86_64-posix-seh-gcc-13.2.0-llvm-17.0.6-mingw-w64ucrt-11.0.1-r5.zip" $GCCZip
if (-not (Test-Path $GCCExpectedFolder)) {
    Unzip-File $GCCZip $GCCExtract
} else {
    Write-Host "  WinLibs GCC already extracted."
}

# 2. Download & Extract Go 1.22.4
$GoZip = Join-Path $ToolchainDir "go1.22.4.windows-amd64.zip"
$GoExtract = Join-Path $ToolchainDir "go-extracted"
Download-File "https://dl.google.com/go/go1.22.4.windows-amd64.zip" $GoZip

$GoDir = Join-Path $ToolchainDir "go"
if (-not (Test-Path $GoDir)) {
    Unzip-File $GoZip $GoExtract
    $ExtractedGoPath = Join-Path $GoExtract "go"
    if (Test-Path $ExtractedGoPath) {
        Move-Item -Path $ExtractedGoPath -Destination $GoDir
        Remove-Item -Path $GoExtract -Recurse -Force -ErrorAction SilentlyContinue
    }
} else {
    Write-Host "  Go already extracted."
}

# 3. Download & Run Rustup Installer
$RustupInit = Join-Path $ToolchainDir "rustup-init.exe"
Download-File "https://static.rust-lang.org/rustup/dist/x86_64-pc-windows-msvc/rustup-init.exe" $RustupInit

$CargoHome = Join-Path $ToolchainDir "cargo"
$RustupHome = Join-Path $ToolchainDir "rustup"

if (-not (Test-Path (Join-Path $CargoHome "bin\cargo.exe"))) {
    Write-Host "  Running rustup-init.exe silently..."
    # Set environment variables for the installer
    $env:CARGO_HOME = $CargoHome
    $env:RUSTUP_HOME = $RustupHome
    
    # Run the installer silently
    $process = Start-Process -FilePath $RustupInit -ArgumentList "-y", "--no-modify-path", "--default-host", "x86_64-pc-windows-msvc", "--default-toolchain", "stable" -NoNewWindow -Wait -PassThru
    if ($process.ExitCode -eq 0) {
        Write-Host "  Rust installation complete."
    } else {
        Write-Error "  Rust installation failed with exit code $($process.ExitCode)"
    }
} else {
    Write-Host "  Rust already installed."
}

# 4. Add target wasm32-wasip1 to Rust
Write-Host "  Adding wasm32-wasip1 target to rustup..."
$env:CARGO_HOME = $CargoHome
$env:RUSTUP_HOME = $RustupHome
$RustupExe = Join-Path $CargoHome "bin\rustup.exe"
if (Test-Path $RustupExe) {
    & $RustupExe target add wasm32-wasip1
}

# 5. Create path setup script
$ProfileScriptPath = Join-Path $PSScriptRoot "use_toolchain.ps1"
$ProfileScriptContent = @'
$ToolchainDir = Join-Path $PSScriptRoot ".toolchain"
$CargoHome = Join-Path $ToolchainDir "cargo"
$RustupHome = Join-Path $ToolchainDir "rustup"

# Set Rust variables
$env:CARGO_HOME = $CargoHome
$env:RUSTUP_HOME = $RustupHome

# Add toolchains to environment path (using WinLibs mingw64/bin)
$env:PATH = "$CargoHome\bin;$ToolchainDir\go\bin;$ToolchainDir\mingw64\bin;$env:PATH"

Write-Host "Toolchains configured in current shell session!" -ForegroundColor Green
Write-Host "go version: " -NoNewline; go version
Write-Host "rustc version: " -NoNewline; rustc --version
Write-Host "gcc version: " -NoNewline; gcc --version
'@

Set-Content -Path $ProfileScriptPath -Value $ProfileScriptContent

Write-Host "=========================================="
Write-Host "Setup complete!"
Write-Host "To use the toolchains in any PowerShell window, run:"
Write-Host "  . .\use_toolchain.ps1"
Write-Host "=========================================="
