package attachments

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestServiceUploadSavesFileAndRecordsMetadata(t *testing.T) {
	t.Parallel()

	storage, err := NewLocalDiskStorage(t.TempDir())
	if err != nil {
		t.Fatalf("expected storage to initialize, got error: %v", err)
	}

	service := NewService(NewMemoryStore(), storage).WithClock(func() time.Time {
		return time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	})

	content := strings.NewReader("fake photo bytes")

	attachment, err := service.Upload(context.Background(), "workitem-0001", "user-assignee-001", UploadInput{
		Kind:     KindEvidencePhoto,
		MimeType: "image/jpeg",
	}, content, "leak.jpg")
	if err != nil {
		t.Fatalf("expected upload to succeed, got error: %v", err)
	}

	if attachment.ID == "" {
		t.Fatal("expected generated attachment id")
	}

	if attachment.WorkItemID != "workitem-0001" {
		t.Fatalf("expected work item id workitem-0001, got %q", attachment.WorkItemID)
	}

	if attachment.FileSize != int64(len("fake photo bytes")) {
		t.Fatalf("expected file size %d, got %d", len("fake photo bytes"), attachment.FileSize)
	}

	if attachment.StorageURL == "" {
		t.Fatal("expected a storage url pointing at the saved file")
	}
}

func TestServiceUploadRejectsInvalidKind(t *testing.T) {
	t.Parallel()

	storage, err := NewLocalDiskStorage(t.TempDir())
	if err != nil {
		t.Fatalf("expected storage to initialize, got error: %v", err)
	}

	service := NewService(NewMemoryStore(), storage)

	_, err = service.Upload(context.Background(), "workitem-0001", "user-assignee-001", UploadInput{
		Kind: Kind("not-a-real-kind"),
	}, strings.NewReader("bytes"), "file.txt")
	if !errors.Is(err, ErrInvalidKind) {
		t.Fatalf("expected invalid kind error, got %v", err)
	}
}

func TestServiceUploadRejectsEmptyFile(t *testing.T) {
	t.Parallel()

	storage, err := NewLocalDiskStorage(t.TempDir())
	if err != nil {
		t.Fatalf("expected storage to initialize, got error: %v", err)
	}

	service := NewService(NewMemoryStore(), storage)

	_, err = service.Upload(context.Background(), "workitem-0001", "user-assignee-001", UploadInput{
		Kind: KindEvidencePhoto,
	}, strings.NewReader(""), "empty.txt")
	if !errors.Is(err, ErrEmptyFile) {
		t.Fatalf("expected empty file error, got %v", err)
	}
}

func TestServiceListReturnsAttachmentsForWorkItemOnly(t *testing.T) {
	t.Parallel()

	storage, err := NewLocalDiskStorage(t.TempDir())
	if err != nil {
		t.Fatalf("expected storage to initialize, got error: %v", err)
	}

	service := NewService(NewMemoryStore(), storage)

	if _, err := service.Upload(context.Background(), "workitem-0001", "user-assignee-001", UploadInput{
		Kind: KindEvidencePhoto,
	}, strings.NewReader("photo one"), "one.jpg"); err != nil {
		t.Fatalf("expected upload to succeed, got error: %v", err)
	}

	if _, err := service.Upload(context.Background(), "workitem-0002", "user-assignee-001", UploadInput{
		Kind: KindSitePhoto,
	}, strings.NewReader("photo two"), "two.jpg"); err != nil {
		t.Fatalf("expected upload to succeed, got error: %v", err)
	}

	list, err := service.List(context.Background(), "workitem-0001")
	if err != nil {
		t.Fatalf("expected list to succeed, got error: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("expected 1 attachment for workitem-0001, got %d", len(list))
	}

	if list[0].WorkItemID != "workitem-0001" {
		t.Fatalf("expected attachment for workitem-0001, got %q", list[0].WorkItemID)
	}
}
