package container

import (
	"encoding/binary"
	"errors"
	"io"
)

// ErrNotMultiplexed is returned when the stream does not follow Docker's 8-byte frame header protocol.
var ErrNotMultiplexed = errors.New("stream is not docker multiplexed")

// DemuxLogs reads a Docker multiplexed log stream (8-byte header per frame)
// and copies stdout to stdoutWriter and stderr to stderrWriter.
// If the stream is not framed (e.g. TTY enabled or raw text), it copies the raw stream to stdoutWriter.
func DemuxLogs(src io.Reader, stdoutWriter, stderrWriter io.Writer) error {
	header := make([]byte, 8)
	firstFrame := true

	for {
		n, err := io.ReadFull(src, header)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			if firstFrame && n > 0 {
				// Less than 8 bytes total, write as raw to stdout
				if stdoutWriter != nil {
					_, _ = stdoutWriter.Write(header[:n])
				}
				return nil
			}
			if err == io.ErrUnexpectedEOF {
				return nil
			}
			return err
		}

		// Validate Docker multiplexed frame header:
		// byte 0 in [0, 1, 2], bytes 1-3 all 0.
		if header[0] > 2 || header[1] != 0 || header[2] != 0 || header[3] != 0 {
			if firstFrame {
				// Stream is not multiplexed (e.g. TTY raw stream), fallback to writing entire stream to stdout
				if stdoutWriter != nil {
					if _, wErr := stdoutWriter.Write(header); wErr != nil {
						return wErr
					}
					_, cErr := io.Copy(stdoutWriter, src)
					return cErr
				}
				return nil
			}
			return ErrNotMultiplexed
		}

		firstFrame = false
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
