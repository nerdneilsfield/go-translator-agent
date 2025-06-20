package document

import (
	"io"
)

// PathWriter is a special writer that stores a file path
// It's used for processors that work with directories or need to know the output path
type PathWriter struct {
	Path string
	written []byte
}

// Write implements io.Writer
func (w *PathWriter) Write(p []byte) (n int, err error) {
	w.written = append(w.written, p...)
	return len(p), nil
}

// Bytes returns all written bytes
func (w *PathWriter) Bytes() []byte {
	return w.written
}

// PathReader is a special reader that provides a file path
type PathReader struct {
	Path string
	offset int
}

// Read implements io.Reader
func (r *PathReader) Read(p []byte) (n int, err error) {
	if r.offset >= len(r.Path) {
		return 0, io.EOF
	}
	
	n = copy(p, r.Path[r.offset:])
	r.offset += n
	return n, nil
}

// NewPathReader creates a new PathReader
func NewPathReader(path string) *PathReader {
	return &PathReader{Path: path}
}

// NewPathWriter creates a new PathWriter
func NewPathWriter(path string) *PathWriter {
	return &PathWriter{Path: path}
}