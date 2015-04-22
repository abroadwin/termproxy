package termproxy

import (
	"io"
	"net"
	"sync"

	"github.com/erikh/termproxy/framing"
)

type MultiCopier struct {
	ErrorHandler func(writers []net.Conn, index int, err error) ([]net.Conn, error)
	Handler      func(buf []byte, writers []net.Conn, r io.Reader) ([]byte, error)

	ioLock *sync.Mutex
}

type Copier struct {
	Handler func(buf []byte, w io.Writer, r io.Reader) ([]byte, error)

	ioLock *sync.Mutex
}

func NewCopier(m *MultiCopier) *Copier {
	copier := &Copier{ioLock: new(sync.Mutex)}

	if m != nil {
		copier.ioLock = m.ioLock
	}

	return copier
}

func (c *Copier) Copy(w io.Writer, r io.Reader) error {
	for {
		buf := make([]byte, 256)
		n, err := r.Read(buf)
		if err != nil {
			return err
		}

		c.ioLock.Lock()

		buf = buf[:n]

		if c.Handler != nil {
			var err error
			if buf, err = c.Handler(buf, w, r); err != nil {
				return err
			}
		}

		w.Write(buf)

		c.ioLock.Unlock()
	}
	return nil
}

func (c *Copier) CopyFrames(w io.Writer, r io.Reader) error {
	for {
		buf := make([]byte, 256)
		n, err := r.Read(buf)
		if err != nil {
			return err
		}

		c.ioLock.Lock()

		buf = buf[:n]

		if c.Handler != nil {
			var err error
			if buf, err = c.Handler(buf, w, r); err != nil {
				c.ioLock.Unlock()
				return err
			}
		}

		data := &framing.Data{Data: buf}
		if err := data.WriteTo(w); err != nil {
			c.ioLock.Unlock()
			return err
		}

		c.ioLock.Unlock()
	}
	return nil
}

// takes an optional copier to share a lock with
func NewMultiCopier(c *Copier) *MultiCopier {
	multiCopier := &MultiCopier{ioLock: new(sync.Mutex)}

	if c != nil {
		multiCopier.ioLock = c.ioLock
	}

	return multiCopier
}

func (m *MultiCopier) CopyFrame(writers []net.Conn, reader io.Reader, length int) ([]net.Conn, error) {
	newWriters := writers[:]

	m.ioLock.Lock()
	buf := make([]byte, length)
	n, err := reader.Read(buf)
	if err != nil {
		return nil, err
	}

	buf = buf[:n]

	if m.Handler != nil {
		var err error
		if buf, err = m.Handler(buf, writers, reader); err != nil {
			return nil, err
		}
	}

	for i, w := range writers {
		data := &framing.Data{}
		data.Data = buf
		if err := data.WriteTo(w); err != nil {
			if m.ErrorHandler != nil {
				newWriters, err = m.ErrorHandler(writers, i, err)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	m.ioLock.Unlock()

	return newWriters, nil
}
