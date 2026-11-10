# One-command Windows build. Every cache, temp dir, and output stays in the repo.
# Does not use the C: drive for Go, pnpm, or build temp files.
$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $Root

$Cache = Join-Path $Root ".cache"
$Tmp = Join-Path $Cache "tmp"
$Dirs = @(
  $Tmp,
  (Join-Path $Cache "go-build"),
  (Join-Path $Cache "gomod"),
  (Join-Path $Cache "gopath"),
  (Join-Path $Cache "npm"),
  (Join-Path $Cache "pnpm-home"),
  (Join-Path $Cache "localappdata"),
  (Join-Path $Cache "appdata"),
  (Join-Path $Root ".pnpm-store"),
  (Join-Path $Root "dist")
)
foreach ($dir in $Dirs) {
  New-Item -ItemType Directory -Force -Path $dir | Out-Null
}
$GoEnv = Join-Path $Cache "goenv"
if (-not (Test-Path $GoEnv)) { New-Item -ItemType File -Path $GoEnv | Out-Null }

$env:GOPATH = Join-Path $Cache "gopath"
$env:GOCACHE = Join-Path $Cache "go-build"
$env:GOMODCACHE = Join-Path $Cache "gomod"
$env:GOTMPDIR = $Tmp
$env:GOENV = $GoEnv
$env:GOTELEMETRY = "off"
$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:TMP = $Tmp
$env:TEMP = $Tmp
$env:TMPDIR = $Tmp
$env:LOCALAPPDATA = Join-Path $Cache "localappdata"
$env:APPDATA = Join-Path $Cache "appdata"
$env:XDG_CACHE_HOME = $Cache
$env:npm_config_cache = Join-Path $Cache "npm"
$env:PNPM_HOME = Join-Path $Cache "pnpm-home"
$env:Path = "$(Join-Path $env:GOPATH 'bin');$env:Path"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  throw "需要安装 Go 1.26.10 或更新版本。若只是使用，直接双击 dist\墨册.exe，不必安装 Go。"
}
if (-not (Get-Command pnpm -ErrorAction SilentlyContinue)) {
  throw "需要 pnpm。可执行：corepack enable; corepack prepare pnpm@latest --activate"
}

Write-Host "运行测试"
go test ./pkg/... ./internal/...
if ($LASTEXITCODE -ne 0) { throw "测试未通过" }

if (-not (Get-Command wails -ErrorAction SilentlyContinue)) {
  Write-Host "安装 Wails CLI 到 .cache\gopath"
  go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1
  if ($LASTEXITCODE -ne 0) { throw "安装 Wails 失败" }
}

Write-Host "构建界面"
Push-Location (Join-Path $Root "frontend")
pnpm install --store-dir (Join-Path $Root ".pnpm-store")
if ($LASTEXITCODE -ne 0) { throw "pnpm install 失败" }
pnpm run build
if ($LASTEXITCODE -ne 0) { throw "界面构建失败" }
Pop-Location

Write-Host "编译 Windows exe（不签名）"
wails build -clean -m -s -skipbindings -trimpath -nopackage -platform windows/amd64 -webview2 browser -o moce.exe
if ($LASTEXITCODE -ne 0) { throw "wails build 失败" }

Copy-Item -Force (Join-Path $Root "build\bin\moce.exe") (Join-Path $Root "dist\moce-windows-amd64.exe")
Copy-Item -Force (Join-Path $Root "build\bin\moce.exe") (Join-Path $Root "dist\墨册.exe")
python (Join-Path $Root "scripts\pack-portable.py") (Join-Path $Root "build\bin\moce.exe") (Join-Path $Root "dist\墨册-portable.zip")
$item = Get-Item (Join-Path $Root "dist\墨册.exe")
Write-Host ("输出：" + $item.FullName + "  " + $item.Length + " 字节")
if ($item.Length -gt 15MB) {
  Write-Host "体积超过 15MB。若已安装 UPX，可执行：upx --best dist\墨册.exe"
}
