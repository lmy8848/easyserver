package api

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// PortInfo represents a single listening port entry.
type PortInfo struct {
	Protocol    string `json:"protocol"`     // tcp/tcp6/udp/udp6
	Port        int    `json:"port"`         // local port
	LocalAddr   string `json:"local_addr"`   // full local address
	State       string `json:"state"`        // LISTEN, ESTABLISHED, etc.
	PID         int    `json:"pid"`          // process ID (0 if unknown)
	ProcessName string `json:"process_name"` // process name (if available)
	User        string `json:"user"`         // owning user
}

// PortMonitorHandler serves port usage data.
type PortMonitorHandler struct{}

// GetListeningPorts returns all TCP/UDP listening ports.
func (h *PortMonitorHandler) GetListeningPorts(c *gin.Context) {
	ports := getListeningPorts()
	Success(c, gin.H{"ports": ports, "total": len(ports)})
}

// parseProcNet parses /proc/net/tcp, /proc/net/tcp6, /proc/net/udp, /proc/net/udp6.
func parseProcNet(path, proto string) []PortInfo {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var ports []PortInfo
	scanner := bufio.NewScanner(file)
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			first = false // skip header
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		// local_address is in format "local_address:port" (hex)
		localAddr := fields[1]
		state := fields[3]

		// Only show LISTEN state (0A for TCP)
		if proto == "tcp" || proto == "tcp6" {
			if state != "0A" {
				continue
			}
		}

		hostPort := strings.Split(localAddr, ":")
		if len(hostPort) != 2 {
			continue
		}
		portHex := hostPort[1]
		portVal, err := strconv.ParseInt(portHex, 16, 32)
		if err != nil {
			continue
		}

		// Get inode (field index 9)
		inode := ""
		if len(fields) > 9 {
			inode = fields[9]
		}

		pi := PortInfo{
			Protocol:  proto,
			Port:      int(portVal),
			LocalAddr: formatHostAddr(hostPort[0], int(portVal)),
			State:     "LISTEN",
		}

		if inode != "" && inode != "0" {
			pi.PID = findPIDByInode(inode)
			if pi.PID > 0 {
				pi.ProcessName = getProcessName(pi.PID)
				pi.User = getProcessUser(pi.PID)
			}
		}

		ports = append(ports, pi)
	}
	return ports
}

// formatHostAddr converts hex IP:port to human-readable format.
func formatHostAddr(hexIP string, port int) string {
	if len(hexIP) == 8 {
		// IPv4
		a, _ := strconv.ParseUint(hexIP[6:8], 16, 8)
		b, _ := strconv.ParseUint(hexIP[4:6], 16, 8)
		c, _ := strconv.ParseUint(hexIP[2:4], 16, 8)
		d, _ := strconv.ParseUint(hexIP[0:2], 16, 8)
		return fmt.Sprintf("%d.%d.%d.%d:%d", a, b, c, d, port)
	}
	return fmt.Sprintf("[%s]:%d", hexIP, port)
}

// getListeningPorts aggregates all proc net files.
func getListeningPorts() []PortInfo {
	if runtime.GOOS != "linux" {
		return nil
	}
	var all []PortInfo
	protos := []struct {
		path  string
		proto string
	}{
		{"/proc/net/tcp", "tcp"},
		{"/proc/net/tcp6", "tcp6"},
		{"/proc/net/udp", "udp"},
		{"/proc/net/udp6", "udp6"},
	}
	for _, p := range protos {
		ports := parseProcNet(p.path, p.proto)
		all = append(all, ports...)
	}
	return all
}

// findPIDByInode searches /proc/*/fd/ for a socket matching the inode.
func findPIDByInode(inode string) int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		fdDir := fmt.Sprintf("/proc/%d/fd", pid)
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink(fmt.Sprintf("%s/%s", fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if strings.Contains(link, "socket:") && strings.Contains(link, inode) {
				return pid
			}
		}
		// Limit search for performance
		if pid > 500 {
			break
		}
	}
	return 0
}

func getProcessName(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func getProcessUser(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Uid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return uidToName(fields[1])
			}
		}
	}
	return ""
}

func uidToName(uid string) string {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return uid
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ":")
		if len(parts) >= 3 && parts[2] == uid {
			return parts[0]
		}
	}
	return uid
}
