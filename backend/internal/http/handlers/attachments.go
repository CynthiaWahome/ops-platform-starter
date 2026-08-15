package handlers

import (
	"errors"
	"net/http"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/attachments"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/auth"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/middleware"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/workitems"
)

// maxUploadMemory is how many bytes of a multipart upload Go buffers in
// memory before spilling the rest to a temp file on disk. It does not cap
// how large an upload can be — ParseMultipartForm handles arbitrarily
// large files by writing the overflow to disk itself.
const maxUploadMemory = 10 << 20 // 10 MB

type AttachmentHandler struct {
	workItemService   workitems.Service
	attachmentService attachments.Service
}

func NewAttachmentHandler(workItemService workitems.Service, attachmentService attachments.Service) AttachmentHandler {
	return AttachmentHandler{
		workItemService:   workItemService,
		attachmentService: attachmentService,
	}
}

// Upload accepts a multipart/form-data upload with a "file" field and a
// "kind" field (one of attachments.Kind). Before touching attachmentService
// at all, it reuses workitems.Service.GetByID as the access gate: that
// call already knows how to check "can this caller see this work item"
// (admin: always, assignee: only their own), so this handler does not
// duplicate that ownership logic — if GetByID fails, Upload never runs.
func (h AttachmentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	workItemID := r.PathValue("id")

	if _, err := h.workItemService.GetByID(r.Context(), workItemID, principal.UserID, principal.HasRole(auth.RoleAdmin), principal.HasRole(auth.RoleSupervisor), false); err != nil {
		writeWorkItemAccessError(w, err)
		return
	}

	if err := r.ParseMultipartForm(maxUploadMemory); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "invalid multipart upload"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "missing file field"})
		return
	}
	defer file.Close()

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	input := attachments.UploadInput{
		Kind:     attachments.Kind(r.FormValue("kind")),
		MimeType: mimeType,
	}

	attachment, err := h.attachmentService.Upload(r.Context(), workItemID, principal.UserID, input, file, header.Filename)
	if err != nil {
		switch {
		case errors.Is(err, attachments.ErrInvalidInput), errors.Is(err, attachments.ErrInvalidKind), errors.Is(err, attachments.ErrEmptyFile):
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to upload attachment"})
		}

		return
	}

	writeJSON(w, http.StatusCreated, attachment)
}

// List returns every attachment recorded for a work item. Same access-gate
// pattern as Upload: workitems.Service.GetByID decides visibility before
// attachmentService.List is ever called.
func (h AttachmentHandler) List(w http.ResponseWriter, r *http.Request) {
	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	workItemID := r.PathValue("id")

	if _, err := h.workItemService.GetByID(r.Context(), workItemID, principal.UserID, principal.HasRole(auth.RoleAdmin), principal.HasRole(auth.RoleSupervisor), false); err != nil {
		writeWorkItemAccessError(w, err)
		return
	}

	list, err := h.attachmentService.List(r.Context(), workItemID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to list attachments"})
		return
	}

	writeJSON(w, http.StatusOK, list)
}

func writeWorkItemAccessError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, workitems.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
	case errors.Is(err, workitems.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Message: "work item not found"})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to fetch work item"})
	}
}
