package attachments

import (
	"context"
	"io"
	"strings"
	"time"
)

type Service struct {
	store   Store
	storage Storage
	now     func() time.Time
}

func NewService(store Store, storage Storage) Service {
	return Service{
		store:   store,
		storage: storage,
		now:     time.Now,
	}
}

// Upload saves an uploaded file's bytes via Storage, then records its
// metadata via Store. It does not check whether workItemID is a real work
// item or whether uploadedByUserID is allowed to upload to it — this
// package deliberately doesn't import workitems or auth, the same
// boundary workitems itself keeps with auth. The HTTP handler resolves
// that check by calling the existing workitems.Service.GetByID with the
// caller's identity before ever reaching this method: if that lookup
// fails, the handler never calls Upload at all.
func (s Service) Upload(ctx context.Context, workItemID string, uploadedByUserID string, input UploadInput, content io.Reader, filename string) (Attachment, error) {
	if strings.TrimSpace(workItemID) == "" || strings.TrimSpace(uploadedByUserID) == "" {
		return Attachment{}, ErrInvalidInput
	}

	if !input.Kind.IsValid() {
		return Attachment{}, ErrInvalidKind
	}

	storageURL, size, err := s.storage.Save(ctx, filename, content)
	if err != nil {
		return Attachment{}, err
	}

	if size == 0 {
		return Attachment{}, ErrEmptyFile
	}

	return s.store.Create(ctx, Attachment{
		WorkItemID:       workItemID,
		UploadedByUserID: uploadedByUserID,
		StorageURL:       storageURL,
		MimeType:         input.MimeType,
		FileSize:         size,
		Kind:             input.Kind,
		CreatedAt:        s.now(),
	})
}

// List returns every attachment recorded for a work item, oldest first.
// Same visibility-scoping story as Upload: the caller (the HTTP handler)
// must already have confirmed access to the work item via
// workitems.Service before calling this.
func (s Service) List(ctx context.Context, workItemID string) ([]Attachment, error) {
	if strings.TrimSpace(workItemID) == "" {
		return nil, ErrInvalidInput
	}

	return s.store.ListByWorkItemID(ctx, workItemID)
}

func (s Service) WithClock(now func() time.Time) Service {
	s.now = now
	return s
}
