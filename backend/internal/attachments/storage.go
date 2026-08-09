package attachments

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Storage saves raw file bytes somewhere and hands back a locator (a path
// or URL) to find them again later. It knows nothing about work items,
// uploaders, or any other metadata — that is Store's job, kept in a
// separate struct entirely. Splitting the two means a later swap to S3 or
// similar only ever requires a new Storage implementation; nothing else in
// this package, or the rest of the codebase, has to change.
type Storage interface {
	Save(ctx context.Context, filename string, content io.Reader) (storageURL string, size int64, err error)
}

// LocalDiskStorage is the starter's default Storage implementation: it
// writes uploaded files straight to a directory on disk. This is a real,
// working implementation, not a placeholder — the starter's own rule
// (see OPS-030's issue body) is that evidence upload has to actually work
// end to end, even before a cloud-backed Storage exists.
type LocalDiskStorage struct {
	mu      sync.Mutex
	baseDir string
	seq     int
}

func NewLocalDiskStorage(baseDir string) (*LocalDiskStorage, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("attachments: create upload directory: %w", err)
	}

	return &LocalDiskStorage{baseDir: baseDir}, nil
}

// Save streams content onto disk with io.Copy rather than reading it into
// memory first, so an upload's size is bounded by disk space, not RAM.
func (s *LocalDiskStorage) Save(_ context.Context, filename string, content io.Reader) (string, int64, error) {
	s.mu.Lock()
	s.seq++
	seq := s.seq
	s.mu.Unlock()

	storedName := fmt.Sprintf("%04d-%s", seq, filepath.Base(filename))
	fullPath := filepath.Join(s.baseDir, storedName)

	destination, err := os.Create(fullPath)
	if err != nil {
		return "", 0, fmt.Errorf("attachments: create destination file: %w", err)
	}
	defer destination.Close()

	size, err := io.Copy(destination, content)
	if err != nil {
		return "", 0, fmt.Errorf("attachments: write file contents: %w", err)
	}

	return fullPath, size, nil
}
