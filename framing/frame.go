package framing

import (
	"encoding/binary"
	"fmt"
	"io"
)

type MessageType int16

const (
	WinchMessage MessageType = iota
	DataMessage
)

type Message interface {
	Type() MessageType
	WriteType(io.Writer) error
	WriteTo(io.Writer) error
	ReadFrom(io.Reader) error
}

type Winch struct {
	Width  uint16
	Height uint16
}

type Data struct {
	Data   []byte
	length int32
}

func (msg *Data) Type() MessageType {
	return DataMessage
}

func (w *Winch) Type() MessageType {
	return WinchMessage
}

func (data *Data) WriteType(w io.Writer) error {
	return binary.Write(w, binary.LittleEndian, DataMessage)
}

func (winch *Winch) WriteType(w io.Writer) error {
	return binary.Write(w, binary.LittleEndian, WinchMessage)
}

func (msg *Data) Len() int {
	return int(msg.length)
}

func (winch *Winch) WriteTo(w io.Writer) error {
	if err := binary.Write(w, binary.LittleEndian, winch.Width); err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, winch.Height); err != nil {
		return err
	}

	return nil
}

func (w *Winch) ReadFrom(r io.Reader) (err error) {
	err = binary.Read(r, binary.LittleEndian, &w.Width)
	if err != nil {
		return
	}
	err = binary.Read(r, binary.LittleEndian, &w.Height)
	return
}

func (msg *Data) WriteTo(w io.Writer) error {
	msg.length = int32(len(msg.Data))
	binary.Write(w, binary.LittleEndian, msg.length)
	_, err := w.Write(msg.Data)
	return err
}

func (msg *Data) ReadFrom(r io.Reader) (err error) {
	var (
		b []byte
		n int
	)
	err = binary.Read(r, binary.LittleEndian, &msg.length)
	if err != nil {
		return
	}

	if msg.Data == nil || msg.length > int32(len(msg.Data)) {
		b = make([]byte, msg.length)
	} else {
		b = msg.Data
	}

	n, err = r.Read(b)
	if err != nil && msg.length < int32(n) && err != io.EOF {
		return
	}
	if msg.length > int32(n) {
		return fmt.Errorf("expected %d bytes, read only %d", msg.length, n)
	}
	msg.Data = b
	return
}

func (msg *Data) Bytes() []byte {
	if len(msg.Data) > int(msg.length) {
		return msg.Data[:msg.length]
	}
	return msg.Data
}

type StreamParser struct {
	DataHandler    func(io.Reader) error
	ErrorHandler   func(error)
	MsgTypeHandler func(MessageType)
	WinchHandler   func(io.Reader) error
	Reader         io.Reader
}

func (s *StreamParser) Loop() {
	var (
		err     error
		msgType MessageType
	)

	for err == nil {
		err = binary.Read(s.Reader, binary.LittleEndian, &msgType)
		if err != nil && s.ErrorHandler != nil {
			s.ErrorHandler(err)
			return
		}

		if s.MsgTypeHandler != nil {
			s.MsgTypeHandler(msgType)
		}

		switch msgType {
		case WinchMessage:
			err = s.WinchHandler(s.Reader)
		case DataMessage:
			err = s.DataHandler(s.Reader)
		}
	}

	if err != io.EOF && s.ErrorHandler != nil {
		s.ErrorHandler(err)
	}
}

func WinchPrinter(r io.Reader) (err error) {
	w := &Winch{}
	err = w.ReadFrom(r)
	fmt.Println(w)
	return
}

func DataPrinter(r io.Reader) (err error) {
	d := &Data{}
	err = d.ReadFrom(r)
	fmt.Printf("msg length: %d message : %q\n", d.length, string(d.Bytes()))
	return
}

func ErrorPrinter(err error) {
	if err == io.EOF {
		fmt.Println("reached end of stream")
		return
	}
	if err != nil {
		fmt.Printf("encountered error: %v", err)
	}
}

func MsgTypePrinter(msgType MessageType) {
	fmt.Printf("found a message of type %d\n", msgType)
}
