package services

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pink-tools/pink-core"
)

// sendIPCStop sends STOP command via IPC to gracefully shutdown a service
// Returns true if the service acknowledged the stop command
func sendIPCStop(name string) bool {
	portFile := filepath.Join(core.ServiceDir(name), name+".port")

	data, err := os.ReadFile(portFile)
	if err != nil {
		return false
	}

	port, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))
	conn.Write([]byte("STOP\n"))

	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	return strings.TrimSpace(response) == "OK"
}

// isIPCRunning checks if service is running via IPC ping
func isIPCRunning(name string) bool {
	portFile := filepath.Join(core.ServiceDir(name), name+".port")

	data, err := os.ReadFile(portFile)
	if err != nil {
		return false
	}

	port, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(2 * time.Second))
	conn.Write([]byte("PING\n"))

	reader := bufio.NewReader(conn)
	response, _ := reader.ReadString('\n')
	return strings.TrimSpace(response) == "PONG"
}
