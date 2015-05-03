package server

import (
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

type Conn struct {
	conn    net.Conn
	channel ssh.Channel
}

func NewConn(conn net.Conn, channel ssh.Channel) *Conn {
	return &Conn{conn, channel}
}

func (c *Conn) Read(buf []byte) (int, error) {
	return c.channel.Read(buf)
}

func (c *Conn) Write(buf []byte) (int, error) {
	return c.channel.Write(buf)
}

func (c *Conn) Close() error {
	if err := c.channel.Close(); err != nil {
		return err
	}

	return c.conn.Close()
}

func (c *Conn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *Conn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func (c *Conn) SendRequest(name string, wantReply bool, payload []byte) (bool, error) {
	return c.channel.SendRequest(name, wantReply, payload)
}
