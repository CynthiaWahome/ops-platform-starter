package attachments

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
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
	baseDir string
}

func NewLocalDiskStorage(baseDir string) (*LocalDiskStorage, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("attachments: create upload directory: %w", err)
	}

	return &LocalDiskStorage{baseDir: baseDir}, nil
}

// Save streams content onto disk with io.Copy rather than reading it into
// memory first, so an upload's size is bounded by disk space, not RAM.
//
// The stored filename used to be a simple in-process counter
// (0001-photo.jpg, 0002-photo.jpg, ...). That was fine while attachment
// metadata was in-memory too — everything reset together on restart. Once
// OPS-048 made metadata durable, it stopped being safe: the counter still
// resets to zero on every restart, so a later upload could reuse an
// earlier one's exact path, and os.Create below would silently truncate
// the old file while its Postgres row kept pointing at that now-clobbered
// path. A timestamp (nanosecond-resolution, so nothing this process
// itself creates back-to-back can collide) plus a short random suffix
// (guards against two uploads landing in the same nanosecond, or a clock
// that doesn't advance between calls on some platforms) needs no
// persisted state at all, so there's nothing left to reset.
func (s *LocalDiskStorage) Save(_ context.Context, filename string, content io.Reader) (string, int64, error) {
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", 0, fmt.Errorf("attachments: generate unique filename: %w", err)
	}

	storedName := fmt.Sprintf("%d-%s-%s", time.Now().UnixNano(), hex.EncodeToString(suffix), filepath.Base(filename))
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
