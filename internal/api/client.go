package api

import (
	"bufio"
	"fmt"
	"net"
	"strings"

	"github.com/pink-tools/pink-orchestrator/internal/config"
)

func Send(command, arg string) (string, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", config.Port())
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("orchestrator not running (port %d)", config.Port())
	}
	defer conn.Close()

	// Send command
	_, err = fmt.Fprintf(conn, "%s:%s\n", command, arg)
	if err != nil {
		return "", fmt.Errorf("send failed: %w", err)
	}

	// Read response
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read failed: %w", err)
	}

	line = strings.TrimSpace(line)
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid response")
	}

	status, msg := parts[0], parts[1]
	if status == "error" {
		return "", fmt.Errorf("%s", msg)
	}

	return msg, nil
}
