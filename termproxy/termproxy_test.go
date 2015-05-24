package termproxy

import (
	"testing"
	"time"
)

func TestCommand(t *testing.T) {
	cmd := NewCommand("echo hello; read")
	if cmd.String() != "echo hello; read" {
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

	time.Sleep(100 * time.Millisecond)

	if cmd.PTY() == nil {
		t.Fatal("PTY was nil after execution")
	}

	if !ptyd {
		t.Fatal("PTY handler wasn't invoked")
	}

	buf := make([]byte, 32)

	n, err := cmd.PTY().Read(buf)
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
