package stemcell

import (
	"bytes"
	"io/fs"
	"time"
)

// UploadableFile wraps an in-memory buffer to implement director.UploadFile interface
type UploadableFile struct {
	*bytes.Reader
	info fileInfo
}

// fileInfo implements os.FileInfo for in-memory files
type fileInfo struct {
	name string
	size int64
}

func (f fileInfo) Name() string       { return f.name }
func (f fileInfo) Size() int64        { return f.size }
func (f fileInfo) Mode() fs.FileMode  { return 0644 }
func (f fileInfo) ModTime() time.Time { return time.Now() }
func (f fileInfo) IsDir() bool        { return false }
func (f fileInfo) Sys() interface{}   { return nil }

// Stat returns file information
func (u *UploadableFile) Stat() (fs.FileInfo, error) {
	return u.info, nil
}

// Close is a no-op for in-memory files
func (u *UploadableFile) Close() error {
	return nil
}

// NewUploadableFile creates an UploadableFile from byte data
func NewUploadableFile(data []byte, name string) *UploadableFile {
	return &UploadableFile{
		Reader: bytes.NewReader(data),
		info: fileInfo{
			name: name,
			size: int64(len(data)),
		},
	}
}
