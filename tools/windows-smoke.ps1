$ErrorActionPreference = 'Stop'

$root = Join-Path $env:TEMP "everythingx-smoke-$PID"
$db = Join-Path $env:TEMP "everythingx-smoke-$PID.db"
$service = Join-Path $PSScriptRoot '..\bin\everythingxd.exe'
$ev = Join-Path $PSScriptRoot '..\bin\ev.exe'
$proc = $null

function Wait-ForPath([string]$term, [string]$path, [bool]$present) {
    $deadline = (Get-Date).AddSeconds(30)
    while ((Get-Date) -lt $deadline) {
        if ($proc.HasExited) { throw "everythingxd exited with code $($proc.ExitCode)" }
        $results = @(& $ev -d $db $term 2>$null)
        if (($results -contains $path) -eq $present) { return }
        Start-Sleep -Milliseconds 250
    }
    throw "Timed out waiting for presence=$present of $path"
}

try {
    New-Item -ItemType Directory -Path $root | Out-Null
    $seed = Join-Path $root 'seedprobe.txt'
    Set-Content -Path $seed -Value 'initial index'
    $proc = Start-Process -FilePath $service -ArgumentList @("--monitor_path=`"$root`"", "--db_path=`"$db`"", '--nocache') -PassThru
    Wait-ForPath 'seedprobe' $seed $true

    $dir = Join-Path $root 'nested'
    New-Item -ItemType Directory -Path $dir | Out-Null
    $created = Join-Path $dir 'agentprobe.txt'
    Set-Content -Path $created -Value 'created after watcher startup'
    Wait-ForPath 'agentprobe' $created $true

    Rename-Item -Path $dir -NewName 'renamed'
    $moved = Join-Path (Join-Path $root 'renamed') 'agentprobe.txt'
    Wait-ForPath 'agentprobe' $moved $true
    Wait-ForPath 'agentprobe' $created $false

    Remove-Item $moved
    Wait-ForPath 'agentprobe' $moved $false
} finally {
    if ($proc -and !$proc.HasExited) { Stop-Process -Id $proc.Id -Force }
    Remove-Item $root -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item "$db*" -Force -ErrorAction SilentlyContinue
}
