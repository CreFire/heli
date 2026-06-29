Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$thrift = Join-Path $root "tools\kitex\echo.thrift"
Push-Location $root
try {
    $env:Path += ';C:\Users\user\go\bin'
    kitex -module game -service EchoService -I tools\kitex -gen-path src $thrift
}
finally {
    Pop-Location
}
