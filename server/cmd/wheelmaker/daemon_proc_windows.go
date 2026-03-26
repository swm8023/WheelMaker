//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func listWorkerProcesses(exeName, markerFlag string) ([]daemonProcess, error) {
	exeName = strings.TrimSpace(exeName)
	if exeName == "" {
		return nil, nil
	}
	markerFlag = strings.TrimSpace(markerFlag)
	if markerFlag == "" {
		markerFlag = daemonWorkerArg
	}
	script := fmt.Sprintf(`$p = Get-CimInstance Win32_Process -Filter "Name='%s'" | Where-Object { $_.CommandLine -match '%s' } | Select-Object ProcessId; if ($null -eq $p) { '[]' } else { $p | ConvertTo-Json -Compress }`, exeName, markerFlag)
	out, err := exec.Command("powershell", "-NoProfile", "-Command", script).Output()
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" || raw == "null" || raw == "[]" {
		return nil, nil
	}

	type row struct {
		ProcessID int `json:"ProcessId"`
	}
	var one row
	if json.Unmarshal([]byte(raw), &one) == nil && one.ProcessID > 0 {
		return []daemonProcess{{PID: one.ProcessID}}, nil
	}
	var many []row
	if err := json.Unmarshal([]byte(raw), &many); err != nil {
		return nil, fmt.Errorf("decode workers: %w", err)
	}
	procs := make([]daemonProcess, 0, len(many))
	for _, r := range many {
		if r.ProcessID <= 0 {
			continue
		}
		procs = append(procs, daemonProcess{PID: r.ProcessID})
	}
	return procs, nil
}
