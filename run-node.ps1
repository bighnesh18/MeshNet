param(
  [string]$Name = "",
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$Connect
)

$exe = Join-Path $PSScriptRoot "meshnet.exe"
if (!(Test-Path $exe)) {
  go build -o $exe ./cmd/meshnet
}

if ($Name -eq "") {
  & $exe @Connect
} else {
  & $exe $Name @Connect
}
