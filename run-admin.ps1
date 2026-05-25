param(
  [string]$Dashboard = "8000"
)

$exe = Join-Path $PSScriptRoot "meshnet.exe"
if (!(Test-Path $exe)) {
  go build -o $exe ./cmd/meshnet
}

& $exe --dashboard $Dashboard admin
