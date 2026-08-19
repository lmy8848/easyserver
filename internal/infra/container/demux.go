package container

import (
	"encoding/binary"
	"io"
)

// DemuxLogs reads a Docker multiplexed log stream (8-byte header per frame)
// and copies stdout to stdoutWriter and stderr to stderrWriter.
func DemuxLogs(src io.Reader, stdoutWriter, stderrWriter io.Writer) error {
	header := make([]byte, 8)
	for {
		_, err := io.ReadFull(src, header)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return err
		}

		streamType := header[0]
		size := binary.BigEndian.Uint32(header[4:8])

		var dst io.Writer
		switch streamType {
		case 1: // stdout
			dst = stdoutWriter
		case 2: // stderr
			dst = stderrWriter
		default:
			dst = stdoutWriter
		}

		if size > 0 {
			lr := io.LimitReader(src, int64(size))
			if dst != nil {
				if _, err := io.Copy(dst, lr); err != nil {
					return err
				}
			} else {
				if _, err := io.Copy(io.Discard, lr); err != nil {
					return err
				}
			}
		}
	}
}
