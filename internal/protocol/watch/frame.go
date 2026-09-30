package watch

import (
	"bytes"
	"fmt"
)

// SplitFunc is a bufio.SplitFunc that extracts individual WATCH frames from a
// TCP stream, mirroring Traccar's WatchFrameDecoder.
//
// Devices do not terminate frames with newlines and often send several frames
// in a single TCP segment ([...LK...][...UD...][...TKQ...]). A frame starts at
// '[' and ends when the bracket nesting level returns to zero; literal
// brackets inside a frame (e.g. in Wi-Fi SSIDs) are balanced. Bytes between
// frames (stray CR/LF, garbage) are discarded. The returned token is the raw,
// still-escaped frame; use Unescape before parsing.
func SplitFunc(data []byte, atEOF bool) (advance int, token []byte, err error) {
	start := bytes.IndexByte(data, '[')
	if start < 0 {
		// No frame start: everything buffered is garbage.
		return len(data), nil, nil
	}

	brackets := 0
	for i := start; i < len(data); i++ {
		switch data[i] {
		case '[':
			brackets++
		case ']':
			brackets--
		}
		if brackets == 0 {
			return i + 1, data[start : i+1], nil
		}
	}

	if atEOF {
		// Incomplete frame at EOF, discard.
		return len(data), nil, nil
	}
	// Drop leading garbage and wait for the rest of the frame.
	return start, nil, nil
}

// Unescape reverses the WATCH byte escaping applied to binary payloads:
// }01 -> '}', }02 -> '[', }03 -> ']', }04 -> ',', }05 -> '*'.
func Unescape(frame []byte) ([]byte, error) {
	if bytes.IndexByte(frame, '}') < 0 {
		return frame, nil
	}
	out := make([]byte, 0, len(frame))
	for i := 0; i < len(frame); i++ {
		b := frame[i]
		if b != '}' {
			out = append(out, b)
			continue
		}
		if i+1 >= len(frame) {
			return nil, fmt.Errorf("truncated escape sequence at %d", i)
		}
		i++
		switch frame[i] {
		case 0x01:
			out = append(out, '}')
		case 0x02:
			out = append(out, '[')
		case 0x03:
			out = append(out, ']')
		case 0x04:
			out = append(out, ',')
		case 0x05:
			out = append(out, '*')
		default:
			return nil, fmt.Errorf("unexpected escape byte at %d: 0x%02x", i, frame[i])
		}
	}
	return out, nil
}
