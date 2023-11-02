package main

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/klauspost/compress/zstd"
)

type CompressionAlgorithm = string

const (
	Zstd = "zstd"
	Gzip = "gzip"
)

type compression struct {
	compressionReader io.ReadCloser
	err               error
	wg                *sync.WaitGroup
	finished          chan struct{}
}

func (c *compression) Read(b []byte) (int, error) {
	n, err := c.compressionReader.Read(b)
	if c.err != nil {
		return n, c.err
	}

	return n, err
}

func (c *compression) Close() error {
	err := c.compressionReader.Close()
	c.wg.Wait()
	if c.err != nil {
		return c.err
	}

	return err
}

func newCompressionReader(ctx context.Context, reader io.ReadCloser, level int, algo CompressionAlgorithm) (io.ReadCloser, <-chan struct{}, error) {
	c := &compression{}
	readerPipe, writePipe := io.Pipe()

	w, err := func() (io.WriteCloser, error) {
		switch algo {
		case Zstd:
			w, err := zstd.NewWriter(writePipe, zstd.WithEncoderLevel(zstd.EncoderLevel(level)))
			if err != nil {
				return nil, fmt.Errorf("zstd init write level: %w", err)
			}

			return w, nil
		case Gzip:
			w, err := gzip.NewWriterLevel(writePipe, level)
			if err != nil {
				return nil, fmt.Errorf("gzip init write level: %w", err)
			}

			return w, nil
		default:
			return nil, fmt.Errorf("not implemented %q", algo)
		}
	}()
	if err != nil {
		return nil, nil, fmt.Errorf("get compression writer: %w", err)
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	var readerGzip = reader
	c.finished = make(chan struct{}, 1)
	go func() {
		defer wg.Done()
		_, err := io.Copy(w, readerGzip)
		if err != nil {
			c.err = err
		}

		type flusher interface{ Flush() error }
		if f, isFlusher := w.(flusher); isFlusher {
			f.Flush()
		}

		readerGzip.Close()
		writePipe.Close()
		close(c.finished)
	}()

	c.compressionReader = readerPipe
	c.wg = wg
	return c, c.finished, nil
}
