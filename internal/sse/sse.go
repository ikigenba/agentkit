package sse

import (
	"bufio"
	"bytes"
	"io"
	"strings"
)

// Event is one parsed Server-Sent Event frame.
type Event struct {
	Type string
	Data []byte
}

// ReadAll parses an SSE byte stream into frames.
func ReadAll(r io.Reader) ([]Event, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var events []Event
	var typ string
	var data bytes.Buffer

	flush := func() {
		if typ == "" && data.Len() == 0 {
			return
		}
		if typ == "" {
			typ = "message"
		}
		b := append([]byte(nil), data.Bytes()...)
		if len(b) > 0 && b[len(b)-1] == '\n' {
			b = b[:len(b)-1]
		}
		events = append(events, Event{Type: typ, Data: b})
		typ = ""
		data.Reset()
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if ok && strings.HasPrefix(value, " ") {
			value = value[1:]
		}
		switch field {
		case "event":
			typ = value
		case "data":
			data.WriteString(value)
			data.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	flush()
	return events, nil
}
