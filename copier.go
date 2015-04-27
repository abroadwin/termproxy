package termproxy

import (
	"io"
	"sync"

	"github.com/erikh/termproxy/framing"
)

type Copier struct {
	Handler func(buf []byte, w io.Writer, r io.Reader) ([]byte, error)

	ioLock *sync.Mutex
}

func NewCopier(lock *sync.Mutex) *Copier {
	copier := &Copier{ioLock: lock}

	if copier.ioLock == nil {
		copier.ioLock = new(sync.Mutex)
	}

	return copier
}

func (c *Copier) Copy(w io.Writer, r io.Reader) error {
	for {
		c.ioLock.Lock()

		buf := make([]byte, 256)
		n, err := r.Read(buf)
		if err != nil {
			c.ioLock.Unlock()
			return err
		}

		buf = buf[:n]

		if c.Handler != nil {
			var err error
			if buf, err = c.Handler(buf, w, r); err != nil {
				c.ioLock.Unlock()
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
