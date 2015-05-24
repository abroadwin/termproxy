package server

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"sync"

	"github.com/erikh/termproxy/termproxy"
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

func NewSSHServer(listenSpec, username, password, authorizedKeys, privateKey string) (*SSHServer, error) {
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

	if err := srv.initSSH(username, password, authorizedKeys, privateKey); err != nil {
		return nil, err
	}

	return srv, nil
}

func (s *SSHServer) initSSH(username, password, authorizedKeys, privateKey string) error {
	pubKeys := []ssh.PublicKey{}

	if authorizedKeys != "" {
		keysFile, err := ioutil.ReadFile(authorizedKeys)
		if err != nil {
			return fmt.Errorf("Could not parse authorized keys file %s: %v", authorizedKeys, err)
		}

		for _, key := range bytes.Split(keysFile, []byte{'\n'}) {
			parsed, _, _, _, err := ssh.ParseAuthorizedKey(key)
			if err != nil {
				return fmt.Errorf("could not parse public key: %v", err)
			}

			pubKeys = append(pubKeys, parsed)
		}
	}

	s.sshConfig = &ssh.ServerConfig{
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			for _, pubKey := range pubKeys {
				if bytes.Equal(key.Marshal(), pubKey.Marshal()) {
					return nil, nil
				}
			}
			return nil, fmt.Errorf("Public key authentication rejected")
		},
	}

	if password != "" {
		s.sshConfig.PasswordCallback = func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == username && string(pass) == password {
				return nil, nil
			}

			return nil, fmt.Errorf("password rejected for %q", c.User())
		}
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

		serverConn, chans, reqs, err := ssh.NewServerConn(c, s.sshConfig)
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
						winch, err := readWinchPayload(req.Payload)
						if err != nil {
							req.Reply(false, nil)
							continue
						}

						winch.Conn = conn
						s.InWinch <- winch
						req.Reply(true, nil)
					case "pty-req":
						split := bytes.SplitN(req.Payload[4:], []byte{0}, 2)
						// the nul termination split up here chops a byte off the head of
						// our filtered payload. We don't actually care about this byte as
						// terminals are still not large enough to cap 32bits. :)
						// just append a zero to make uint32 happy and move on.
						payload := append([]byte{0}, split[1]...)
						winch, err := readWinchPayload(payload)
						if err != nil {
							req.Reply(false, nil)
							continue
						}

						winch.Conn = conn
						s.InWinch <- winch
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
			go func() {
				serverConn.Wait()
				serverConn.Close()
				conn.Close()
				s.CloseHandler(conn)
			}()

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

func readWinchPayload(payload []byte) (termproxy.Winch, error) {
	buf := bytes.NewBuffer(payload)
	if buf.Len() < 8 {
		return termproxy.Winch{}, fmt.Errorf("Could not read payload for winch")
	}

	tmp := make([]byte, 4)
	c, err := buf.Read(tmp)
	if c != 4 || err != nil {
		return termproxy.Winch{}, fmt.Errorf("Could not read payload for winch")
	}

	width := binary.BigEndian.Uint32(tmp)
	c, err = buf.Read(tmp)
	if c != 4 || err != nil {
		return termproxy.Winch{}, fmt.Errorf("Could not read payload for winch")
	}
	height := binary.BigEndian.Uint32(tmp)

	return termproxy.Winch{Width: uint(width), Height: uint(height)}, nil
}
