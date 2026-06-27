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
