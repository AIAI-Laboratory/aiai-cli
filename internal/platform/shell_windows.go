package platform

import (
	"context"
	"os/exec"
	"strconv"
	"time"
)

func shellCommand(ctx context.Context, _ bool) (*exec.Cmd, error) {
	// PowerShell handles cd across drives, pipelines and native .cmd launchers
	// such as npm without ending a wrapper batch file before cwd is saved.
	script := `$aiaiStatus = 0
try {
    Invoke-Expression $env:AIAI_COMMAND
    if (-not $?) { $aiaiStatus = 1 }
    if (Test-Path variable:LASTEXITCODE) { $aiaiStatus = $LASTEXITCODE }
} catch {
    Write-Error $_
    $aiaiStatus = 1
} finally {
    [System.IO.File]::WriteAllText($env:AIAI_CWD_FILE, (Get-Location).ProviderPath)
}
exit $aiaiStatus`
	return exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-Command", script), nil
}

func configureCapturedCommand(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}
	cmd.WaitDelay = time.Second
}
