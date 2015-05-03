package server

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"sync"

	"github.com/erikh/termproxy"
	"golang.org/x/crypto/ssh"
)

type SSHServer struct {
	AcceptHandler func(net.Conn)
	CloseHandler  func(net.Conn)

	InWinch  chan termproxy.Winch
	OutWinch chan termproxy.Winch

	connections []net.Conn
	listener    net.Listener

	copier *termproxy.Copier

	sshConfig *ssh.ServerConfig

	mutex sync.Mutex
}

func defaultCloseHandler(conn net.Conn) {
	conn.Close()
}

func NewSSHServer(listenSpec, username, password, privateKey string) (*SSHServer, error) {
	listener, err := net.Listen("tcp", listenSpec)
	if err != nil {
		return nil, err
	}

	srv := &SSHServer{
		InWinch:      make(chan termproxy.Winch),
		OutWinch:     make(chan termproxy.Winch),
		CloseHandler: defaultCloseHandler,
		listener:     listener,
		copier:       termproxy.NewCopier(),
	}

	if err := srv.initSSH(username, password, privateKey); err != nil {
		return nil, err
	}

	return srv, nil
}

func (s *SSHServer) initSSH(username, password, privateKey string) error {
	s.sshConfig = &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == username && string(pass) == password {
				return nil, nil
			}
			return nil, fmt.Errorf("password rejected for %q", c.User())
		},
	}

	privateBytes, err := ioutil.ReadFile(privateKey)
	if err != nil {
		return err
	}

	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		return err
	}

	s.sshConfig.AddHostKey(private)

	return nil
}

func (s *SSHServer) Listen() {
	for {
		c, err := s.listener.Accept()
		if err != nil {
			continue
		}

		_, chans, reqs, err := ssh.NewServerConn(c, s.sshConfig)
		if err != nil {
			continue
		}
		// The incoming Request channel must be serviced.
		go ssh.DiscardRequests(reqs)

		// Service the incoming Channel channel.
		for newChannel := range chans {
			// Channels have a type, depending on the application level
			// protocol intended. In the case of a shell, the type is
			// "session" and ServerShell may be used to present a simple
			// terminal interface.
			if newChannel.ChannelType() != "session" {
				newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
				continue
			}
			channel, requests, err := newChannel.Accept()
			if err != nil {
				break
			}

			s.mutex.Lock()
			conn := NewConn(c, channel)
			s.connections = append(s.connections, conn)
			s.mutex.Unlock()

			go func(in <-chan *ssh.Request) {
				for req := range in {
					ok := false
					switch req.Type {
					case "window-change":
						var width, height uint

						for i := uint(0); i < 4; i++ {
							width <<= 8
							width |= uint(req.Payload[i])
						}

						for i := uint(4); i < 8; i++ {
							height <<= 8
							height |= uint(req.Payload[i])
						}

						fmt.Println(req.Payload)

						s.InWinch <- termproxy.Winch{conn, width, height}
						req.Reply(true, nil)
					case "pty-req":
						req.Reply(true, nil)
					case "shell":
						ok = true
						if len(req.Payload) > 0 {
							ok = false
						}
						req.Reply(ok, nil)
					default:
						req.Reply(false, nil)
					}
				}
			}(requests)

			if s.AcceptHandler == nil {
				panic("no accept handler provided")
			}

			go s.AcceptHandler(conn)

			break
		}
	}
}

func (s *SSHServer) Prune(i int) {
	if len(s.connections)-1 == i {
		s.connections = s.connections[:i]
	} else {
		s.connections = append(s.connections[:i], s.connections[i+1:]...)
	}
}

func (s *SSHServer) Iterate(iterator func(*SSHServer, net.Conn, int) error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for i, conn := range s.connections {
		if err := iterator(s, conn, i); err != nil {
			s.CloseHandler(conn)
			s.Prune(i)
		}
	}
}

func (s *SSHServer) MultiCopy(buf []byte) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for i, conn := range s.connections {
		if _, err := conn.Write(buf); err != nil && err != io.EOF {
			s.CloseHandler(conn)
			s.Prune(i)
		}
	}
}
