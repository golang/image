// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tiff

import "io"

// fillChunkSize is the maximum number of bytes to allocate or read at once
// when growing the buffer. This follows the approach of internal/saferio
// in the standard library: read data in chunks to avoid allocating a huge
// buffer for an invalid file while still supporting arbitrarily large
// valid files.
const fillChunkSize = 10 << 20 // 10 MB

// buffer buffers an io.Reader to satisfy io.ReaderAt.
type buffer struct {
	r   io.Reader
	buf []byte
}

// fill reads data from b.r until the buffer contains at least end bytes.
func (b *buffer) fill(end int) error {
	m := len(b.buf)
	if end > m {
		// Grow and read in chunks to avoid allocating a large buffer
		// up front based on an untrusted offset. If the offset is
		// beyond the actual data, ReadFull will return an error after
		// reading only what is available, limiting memory usage to
		// the actual file size rather than the claimed offset.
		for m < end {
			next := end - m
			if next > fillChunkSize {
				next = fillChunkSize
			}
			if m+next > cap(b.buf) {
				newcap := cap(b.buf)
				if newcap < 1024 {
					newcap = 1024
				}
				for newcap < m+next {
					newcap *= 2
				}
				newbuf := make([]byte, m+next, newcap)
				copy(newbuf, b.buf)
				b.buf = newbuf
			} else {
				b.buf = b.buf[:m+next]
			}
			n, err := io.ReadFull(b.r, b.buf[m:m+next])
			m += n
			b.buf = b.buf[:m]
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *buffer) ReadAt(p []byte, off int64) (int, error) {
	o := int(off)
	end := o + len(p)
	if int64(end) != off+int64(len(p)) {
		return 0, io.ErrUnexpectedEOF
	}

	err := b.fill(end)
	if o >= len(b.buf) {
		return 0, err
	}
	if end > len(b.buf) {
		end = len(b.buf)
	}
	return copy(p, b.buf[o:end]), err
}

// Slice returns a slice of the underlying buffer. The slice contains
// n bytes starting at offset off.
func (b *buffer) Slice(off, n int) ([]byte, error) {
	end := off + n
	if err := b.fill(end); err != nil {
		return nil, err
	}
	return b.buf[off:end], nil
}

// newReaderAt converts an io.Reader into an io.ReaderAt.
func newReaderAt(r io.Reader) io.ReaderAt {
	if ra, ok := r.(io.ReaderAt); ok {
		return ra
	}
	return &buffer{
		r:   r,
		buf: make([]byte, 0, 1024),
	}
}
