package attachments

import (
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("attachment not found")
	ErrInvalidInput = errors.New("invalid attachment input")
	ErrInvalidKind  = errors.New("invalid attachment kind")
	ErrEmptyFile    = errors.New("uploaded file is empty")
)

// Kind mirrors the attachment_kind enum from
// notes/OPS_PLATFORM_STARTER_DOMAIN_MODEL.md. Kept as a constrained string
// type, not free text, so a caller can't tag an upload with an arbitrary
// label — same pattern as workitems.Status and workitems.Priority.
type Kind string

const (
	KindSupportingDocument   Kind = "supporting_document"
	KindSitePhoto            Kind = "site_photo"
	KindEvidencePhoto        Kind = "evidence_photo"
	KindQuoteDocument        Kind = "quote_document"
	KindVerificationDocument Kind = "verification_document"
)

func (k Kind) IsValid() bool {
	switch k {
	case KindSupportingDocument, KindSitePhoto, KindEvidencePhoto,
		KindQuoteDocument, KindVerificationDocument:
		return true
	default:
		return false
	}
}

// Attachment is the metadata record for one uploaded file. It never holds
// the file's bytes itself — StorageURL is a pointer to where Storage put
// them, resolved through the Storage interface, not stored inline here.
type Attachment struct {
	ID               string    `json:"id"`
	WorkItemID       string    `json:"workItemId"`
	UploadedByUserID string    `json:"uploadedByUserId"`
	StorageURL       string    `json:"storageUrl"`
	MimeType         string    `json:"mimeType"`
	FileSize         int64     `json:"fileSize"`
	Kind             Kind      `json:"kind"`
	CreatedAt        time.Time `json:"createdAt"`
}

type UploadInput struct {
	Kind     Kind
	MimeType string
	FileSize int64
}
