package tail

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

type Source struct{ Label, Path string }

func LastLines(path string, count int) ([]byte, error) {
	if count <= 0 {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	const chunkSize int64 = 32 * 1024
	position := info.Size()
	var data []byte
	lines := 0
	for position > 0 && lines <= count {
		readSize := chunkSize
		if position < readSize {
			readSize = position
		}
		position -= readSize
		chunk := make([]byte, readSize)
		if _, err := file.ReadAt(chunk, position); err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		data = append(chunk, data...)
		lines = bytes.Count(data, []byte{'\n'})
	}
	parts := bytes.Split(data, []byte{'\n'})
	if len(parts) > count+1 {
		parts = parts[len(parts)-(count+1):]
	}
	return bytes.Join(parts, []byte{'\n'}), nil
}

func PrintLast(w io.Writer, source Source, count int) error {
	data, err := LastLines(source.Path, count)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if _, err := fmt.Fprintf(w, "==> %s (%s) <==\n", source.Label, source.Path); err != nil {
		return err
	}
	if len(data) > 0 {
		if _, err := w.Write(data); err != nil {
			return err
		}
		if data[len(data)-1] != '\n' {
			_, err = io.WriteString(w, "\n")
		}
	}
	return err
}

func Follow(ctx context.Context, w io.Writer, sources []Source) error {
	tails := make([]*follower, 0, len(sources))
	for _, source := range sources {
		f := &follower{source: source}
		if err := f.openAtEnd(); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		tails = append(tails, f)
	}
	defer func() {
		for _, f := range tails {
			f.close()
		}
	}()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			for _, f := range tails {
				data, rotated, err := f.readAvailable()
				if err != nil {
					return err
				}
				if len(data) > 0 {
					if rotated {
						_, _ = fmt.Fprintf(w, "==> %s (rotated) <==\n", f.source.Label)
					}
					if _, err := w.Write(data); err != nil {
						return err
					}
				}
			}
		}
	}
}

type follower struct {
	source Source
	file   *os.File
	info   os.FileInfo
	offset int64
}

func (f *follower) openAtEnd() error {
	file, err := os.Open(f.source.Path)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	f.file, f.info, f.offset = file, info, info.Size()
	return nil
}
func (f *follower) close() {
	if f.file != nil {
		_ = f.file.Close()
		f.file = nil
	}
}
func (f *follower) readAvailable() ([]byte, bool, error) {
	pathInfo, err := os.Stat(f.source.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	rotated := f.file == nil || f.info == nil || !os.SameFile(f.info, pathInfo) || pathInfo.Size() < f.offset
	if rotated {
		f.close()
		file, err := os.Open(f.source.Path)
		if err != nil {
			return nil, false, err
		}
		f.file, f.info, f.offset = file, pathInfo, 0
	}
	if pathInfo.Size() <= f.offset {
		return nil, rotated, nil
	}
	if _, err := f.file.Seek(f.offset, io.SeekStart); err != nil {
		return nil, rotated, err
	}
	var buf bytes.Buffer
	reader := bufio.NewReader(f.file)
	if _, err := io.Copy(&buf, reader); err != nil {
		return nil, rotated, err
	}
	f.offset += int64(buf.Len())
	return buf.Bytes(), rotated, nil
}
