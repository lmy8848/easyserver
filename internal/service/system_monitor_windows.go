//go:build windows
// +build windows

package service

import (
	"context"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// checkDiskSpace checks disk usage on all fixed drives.
// Alerts if any drive exceeds 90% usage.
func (m *SystemEventMonitor) checkDiskSpace() {
	// Get list of logical drives
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	getLogicalDrives := kernel32.NewProc("GetLogicalDrives")

	ret, _, _ := getLogicalDrives.Call()
	if ret == 0 {
		return
	}

	for drive := 'A'; drive <= 'Z'; drive++ {
		if ret&(1<<(drive-'A')) == 0 {
			continue
		}

		drivePath := string(drive) + ":\\"
		drivePtr, _ := windows.UTF16PtrFromString(drivePath)

		var freeBytesAvailable, totalBytes, totalFreeBytes int64
		getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")
		ret, _, _ := getDiskFreeSpaceEx.Call(
			uintptr(unsafe.Pointer(drivePtr)),
			uintptr(unsafe.Pointer(&freeBytesAvailable)),
			uintptr(unsafe.Pointer(&totalBytes)),
			uintptr(unsafe.Pointer(&totalFreeBytes)),
		)
		if ret == 0 || totalBytes == 0 {
			continue
		}

		used := totalBytes - totalFreeBytes
		percent := float64(used) / float64(totalBytes) * 100
		if percent >= 90 {
			m.logEvent(context.Background(), "DISK_HIGH_USAGE",
				fmt.Sprintf("Disk usage on %s is %.1f%%", drivePath, percent))
		}
	}
}

// checkMemoryUsage checks memory usage using Windows API.
// Alerts if memory usage exceeds 90%.
func (m *SystemEventMonitor) checkMemoryUsage() {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	globalMemoryStatusEx := kernel32.NewProc("GlobalMemoryStatusEx")

	type memoryStatusEx struct {
		Length               uint32
		MemoryLoad           uint32
		TotalPhys            uint64
		AvailPhys            uint64
		TotalPageFile        uint64
		AvailPageFile        uint64
		TotalVirtual         uint64
		AvailVirtual         uint64
		AvailExtendedVirtual uint64
	}

	var memStatus memoryStatusEx
	memStatus.Length = uint32(unsafe.Sizeof(memStatus))

	ret, _, _ := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memStatus)))
	if ret == 0 {
		return
	}

	if memStatus.TotalPhys > 0 {
		used := memStatus.TotalPhys - memStatus.AvailPhys
		percent := float64(used) / float64(memStatus.TotalPhys) * 100
		if percent >= 90 {
			m.logEvent(context.Background(), "MEMORY_HIGH_USAGE",
				fmt.Sprintf("Memory usage is %.1f%%", percent))
		}
	}
}

// checkServiceFailures checks for failed Windows services.
func (m *SystemEventMonitor) checkServiceFailures() {
	// Use PowerShell to get stopped services that should be running
	output, _, _, err := m.executor.Run(context.Background(), "powershell", "-NoProfile", "-Command",
		"Get-Service | Where-Object {$_.Status -eq 'Stopped' -and $_.StartType -eq 'Automatic'} | Select-Object -ExpandProperty Name")
	if err != nil {
		// PowerShell may not be available, try sc command
		m.checkServiceFailuresSC()
		return
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		serviceName := strings.TrimSpace(line)
		if serviceName == "" {
			continue
		}
		m.logEvent(context.Background(), "SERVICE_FAILED",
			"Service "+serviceName+" has stopped unexpectedly")
	}
}

// checkServiceFailuresSC is a fallback using the sc command.
func (m *SystemEventMonitor) checkServiceFailuresSC() {
	output, _, _, err := m.executor.Run(context.Background(), "sc", "query", "type=", "service", "state=", "stopped")
	if err != nil {
		return
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SERVICE_NAME:") {
			serviceName := strings.TrimPrefix(line, "SERVICE_NAME:")
			serviceName = strings.TrimSpace(serviceName)
			m.logEvent(context.Background(), "SERVICE_FAILED",
				"Service "+serviceName+" is stopped")
		}
	}
}
