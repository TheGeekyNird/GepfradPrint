# Run as a normal user. No elevation is requested.
$ErrorActionPreference = 'Stop'
Start-Process -FilePath "$PSScriptRoot\GepfradPrint.exe" -WorkingDirectory $PSScriptRoot
Start-Sleep -Milliseconds 500
Start-Process 'http://127.0.0.1:17842/'
