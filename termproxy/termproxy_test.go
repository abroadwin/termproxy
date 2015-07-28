package termproxy

import (
	"bytes"
	"io"
	"testing"
	"time"
)

func TestCommand(t *testing.T) {
	cmd := NewCommand("echo hello; cat")
	if cmd.String() != "echo hello; cat" {
		t.Fatal("Command string did not equal what was passed")
	}

	var closed, ptyd bool

	cmd.PTYSetupHandler = func(c *Command) {
		ptyd = true
	}

	cmd.CloseHandler = func(c *Command) {
		closed = true
	}

	go func() {
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
	}()

	time.Sleep(1 * time.Second)

	if cmd.PTY() == nil {
		t.Fatal("PTY was nil after execution")
	}

	if !ptyd {
		t.Fatal("PTY handler wasn't invoked")
	}

	myPty := cmd.PTY()

	buf := make([]byte, 32)

	n, err := myPty.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	if string(buf[:n]) != "hello\r\n" {
		t.Fatalf("String %q was not 'hello\\r\\n'", string(buf))
	}

	if err := cmd.Quit(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	if !closed {
		t.Fatal("Command was not closed after run")
	}
}

func TestCopier(t *testing.T) {
	c := NewCopier()
	buf1, buf2, buf3 := new(bytes.Buffer), new(bytes.Buffer), new(bytes.Buffer)
	handled := false

	c.Handler = func(buf []byte, w io.Writer, r io.Reader) ([]byte, error) {
		handled = true
		return buf, nil
	}

	if _, err := buf1.Write([]byte("fart")); err != nil {
		t.Fatal(err)
	}

	if _, err := buf2.Write([]byte("poop")); err != nil {
		t.Fatal(err)
	}

	go c.Copy(buf3, buf2)
	go c.Copy(buf3, buf1)

	time.Sleep(10 * time.Millisecond)

	if string(buf3.Bytes()) != "poopfart" {
		t.Fatal("String was malformed after copy: %q", string(buf3.Bytes()))
	}

	if !handled {
		t.Fatal("Handler was not triggered during copy")
	}
}
