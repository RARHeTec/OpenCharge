package protocol

import (
	"bufio"
	"io"
)

// Splitter reassembles frames from a byte stream, handling TCP fragmentation
// and coalescing (multiple frames in one read, or one frame across reads).
type Splitter struct {
	r *bufio.Reader
}

func NewSplitter(r io.Reader) *Splitter {
	return &Splitter{r: bufio.NewReaderSize(r, 4096)}
}

// NextFrame returns the next complete, validated wire frame.
// It discards one stray byte and resyncs when it sees a non-0x68 start byte.
func (s *Splitter) NextFrame() ([]byte, error) {
	for {
		b, err := s.r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b != StartByte {
			continue // resync on next 0x68
		}
		lenByte, err := s.r.ReadByte()
		if err != nil {
			return nil, err
		}
		n := int(lenByte)
		if n < 4 || n > MaxBodyLen {
			continue // invalid length, keep scanning
		}
		total := 1 + 1 + n + 2
		frame := make([]byte, total)
		frame[0], frame[1] = StartByte, lenByte
		if _, err := io.ReadFull(s.r, frame[2:]); err != nil {
			return nil, err
		}
		return frame, nil
	}
}
