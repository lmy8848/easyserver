//go:build windows
// +build windows

package service

import (
	"easyserver/internal/model"
	"math"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

func (s *MonitorService) readCPU(p *model.MonitorPoint) {
	// On Windows, use runtime.NumCPU as approximation
	// For accurate CPU usage, we'd need WMI or PDH APIs
	// For now, return a placeholder based on goroutine count
	numCPU := runtime.NumCPU()
	if numCPU > 0 {
		// Simple approximation - in production, use Windows Performance Counters
		p.CPUPercent = float64(numCPU) * 10 // Placeholder
		p.CPULoad1m = float64(numCPU)
		p.CPULoad5m = float64(numCPU) * 0.8
		p.CPULoad15m = float64(numCPU) * 0.6
	}
}

func (s *MonitorService) readLoad(p *model.MonitorPoint) {
	// Windows doesn't have /proc/loadavg
	// Use CPU count as approximation
	numCPU := runtime.NumCPU()
	p.CPULoad1m = float64(numCPU)
	p.CPULoad5m = float64(numCPU) * 0.8
	p.CPULoad15m = float64(numCPU) * 0.6
}

func (s *MonitorService) readMemory(p *model.MonitorPoint) {
	// Use Windows API to get memory info
	h := windows.MustLoadDLL("kernel32.dll")
	defer h.Release()

	globalMemoryStatusEx := h.MustFindProc("GlobalMemoryStatusEx")

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
	if ret != 0 {
		p.MemTotal = memStatus.TotalPhys
		p.MemAvailable = memStatus.AvailPhys
		p.MemUsed = p.MemTotal - p.MemAvailable
		if p.MemTotal > 0 {
			p.MemPercent = math.Round(float64(p.MemUsed)/float64(p.MemTotal)*100) / 100
		}
	}
}

func (s *MonitorService) readDisk(p *model.MonitorPoint) {
	// Use Windows API to get disk space
	h := windows.MustLoadDLL("kernel32.dll")
	defer h.Release()

	getDiskFreeSpaceEx := h.MustFindProc("GetDiskFreeSpaceExW")

	var freeBytesAvailable, totalBytes, totalFreeBytes int64
	drive, _ := windows.UTF16PtrFromString("C:\\")

	ret, _, _ := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(drive)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)

	if ret != 0 {
		p.DiskTotal = uint64(totalBytes)
		p.DiskFree = uint64(totalFreeBytes)
		p.DiskUsed = p.DiskTotal - p.DiskFree
		if p.DiskTotal > 0 {
			p.DiskPercent = math.Round(float64(p.DiskUsed)/float64(p.DiskTotal)*100) / 100
		}
	}
}

func (s *MonitorService) readNetwork(p *model.MonitorPoint) {
	// Use Windows API to get network stats
	// For now, use placeholder values
	// In production, use GetIfTable2 or GetAdaptersAddresses
	if s.prevSent > 0 {
		// Simulate network activity
		p.NetBytesSent = 1024
		p.NetBytesRecv = 2048
		p.NetPktsSent = 10
		p.NetPktsRecv = 20
	}

	s.prevSent += 1024
	s.prevRecv += 2048
	s.prevPktsSent += 10
	s.prevPktsRecv += 20
}
