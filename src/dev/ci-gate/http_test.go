package main

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestPowerShellHTTPTransport(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pwsh", "-NoLogo", "-NoProfile", "-NonInteractive", "-File", "http-self-test.ps1", "-Destination", t.TempDir())
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "SystemRoot=" + os.Getenv("SystemRoot"), "TEMP=" + t.TempDir(), "TMP=" + t.TempDir(), "HOME=" + t.TempDir(), "DOTNET_CLI_TELEMETRY_OPTOUT=1", "POWERSHELL_TELEMETRY_OPTOUT=1"}
	if output, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("PowerShell transport controls: %v\n%s", e, output)
	}
}
