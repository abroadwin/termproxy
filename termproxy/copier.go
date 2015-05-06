package termproxy

import (
	"io"
	"sync"
)

type Copier struct {
	Handler func(buf []byte, w io.Writer, r io.Reader) ([]byte, error)

	ioLock *sync.Mutex
}

func NewCopier() *Copier {
	return &Copier{ioLock: new(sync.Mutex)}
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
				c.ioLock.Unlock()
				return err
			}
		}

		w.Write(buf)

		c.ioLock.Unlock()
	}
	return nil
}
