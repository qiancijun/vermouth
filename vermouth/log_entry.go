package vermouth

import (
	"encoding/binary"
	"errors"
)

const (
	HeaderSize = 4
	MagicNumber uint32 = 0x20010206
)

var (
	ErrLogEntryCrushed = errors.New("log entry is crushed")
)

type LogEntry struct {
	Opt uint16
	Data []byte
}

func (entry *LogEntry) Encode() ([]byte, error) {
	buf := make([]byte, HeaderSize)
	binary.BigEndian.PutUint16(buf, entry.Opt)
	buf = append(buf, entry.Data...)
	return buf, nil
}

func (entry *LogEntry) Decode(data []byte) error {
	entry.Opt = binary.BigEndian.Uint16(data)
	entry.Data = data[4:]
	return nil
}