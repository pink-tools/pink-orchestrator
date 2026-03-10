package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/pink-tools/pink-core"
	"github.com/pink-tools/pink-orchestrator/internal/config"
	"github.com/pink-tools/pink-orchestrator/internal/dialog"
	"github.com/pink-tools/pink-orchestrator/internal/services"
)

type Server struct {
	listener net.Listener
	portFile string
}

func NewServer() (*Server, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", config.Port())
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	// Write port file for pink-core IPC discovery
	portDir := core.ServiceDir("pink-orchestrator")
	if err := os.MkdirAll(portDir, 0755); err != nil {
		listener.Close()
		return nil, fmt.Errorf("create service dir: %w", err)
	}
	portFile := filepath.Join(portDir, "pink-orchestrator.port")
	if err := os.WriteFile(portFile, []byte(fmt.Sprintf("%d", config.Port())), 0644); err != nil {
		listener.Close()
		return nil, fmt.Errorf("write port file: %w", err)
	}

	return &Server{listener: listener, portFile: portFile}, nil
}

func (s *Server) Start() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			continue
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		conn.Write([]byte("error:read failed\n"))
		return
	}

	line = strings.TrimSpace(line)

	// Handle pink-core IPC commands (no colon)
	if line == "PING" {
		conn.Write([]byte("PONG\n"))
		return
	}

	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		conn.Write([]byte("error:invalid command format\n"))
		return
	}

	cmd, arg := parts[0], parts[1]

	switch cmd {
	case "dialog":
		// Show WebView dialog and return user choice
		var req dialog.Request
		if err := json.Unmarshal([]byte(arg), &req); err != nil {
			conn.Write([]byte("error:invalid dialog json\n"))
			return
		}
		result := dialog.Show(req)
		conn.Write([]byte(result + "\n"))

	case "update":
		var msgs []string
		err := services.Update(arg, func(msg string) {
			msgs = append(msgs, msg)
		})
		if err != nil {
			conn.Write([]byte(fmt.Sprintf("error:%s\n", err.Error())))
			return
		}
		conn.Write([]byte(fmt.Sprintf("ok:%s\n", strings.Join(msgs, "; "))))

	case "restart":
		err := services.Restart(arg)
		if err != nil {
			conn.Write([]byte(fmt.Sprintf("error:%s\n", err.Error())))
			return
		}
		conn.Write([]byte("ok:restarted\n"))

	case "stop":
		err := services.Stop(arg)
		if err != nil {
			conn.Write([]byte(fmt.Sprintf("error:%s\n", err.Error())))
			return
		}
		conn.Write([]byte("ok:stopped\n"))

	case "start":
		err := services.Start(arg)
		if err != nil {
			conn.Write([]byte(fmt.Sprintf("error:%s\n", err.Error())))
			return
		}
		conn.Write([]byte("ok:started\n"))

	default:
		conn.Write([]byte("error:unknown command\n"))
	}
}

func (s *Server) Close() {
	s.listener.Close()
	os.Remove(s.portFile)
}
