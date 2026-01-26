package api

import (
	"bufio"
	"fmt"
	"net"
	"strings"

	"github.com/pink-tools/pink-orchestrator/internal/config"
	"github.com/pink-tools/pink-orchestrator/internal/services"
)

type Server struct {
	listener net.Listener
}

func NewServer() (*Server, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", config.Port())
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &Server{listener: listener}, nil
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
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		conn.Write([]byte("error:invalid command format\n"))
		return
	}

	cmd, arg := parts[0], parts[1]

	switch cmd {
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
}
