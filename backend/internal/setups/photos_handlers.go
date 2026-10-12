package setups

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
	"github.com/go-chi/chi/v5"
)

// EnablePhotos wires private persistent media; no public static route is registered.
func (s *Service) EnablePhotos(store *photos.Store) {
	s.photos = store
	s.uploadSlots = make(chan struct{}, 2)
}
func (s *Service) ReconcilePhotos(ctx context.Context) error {
	return s.repo.reconcilePhotos(ctx, s.photos)
}
func (s *Service) uploadPhoto(ctx context.Context, ownerID, setupID, requestID string, raw []byte) (Photo, error) {
	digestBytes := sha256.Sum256(raw)
	digest := hex.EncodeToString(digestBytes[:])
	if p, exists, err := s.repo.photoReceipt(ctx, ownerID, setupID, requestID, digest); exists || err != nil {
		return p, err
	}
	img, err := s.photos.Process(ctx, raw)
	if err != nil {
		return Photo{}, err
	}
	p, keep, err := s.repo.addPhoto(ctx, ownerID, setupID, requestID, digest, img)
	if !keep {
		if cleanupErr := s.photos.Remove(img.Key); cleanupErr != nil && err == nil {
			return Photo{}, cleanupErr
		}
	}
	return p, err
}
func photoFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, photos.ErrImage) {
		failure(w, invalid(photos.ErrImage.Error()))
		return
	}
	failure(w, err)
}
func photoIDs(w http.ResponseWriter, r *http.Request) (string, string, string, bool) {
	caller, ok := owner(w, r)
	if !ok {
		return "", "", "", false
	}
	setupID, photoID := chi.URLParam(r, "id"), chi.URLParam(r, "photo_id")
	if err := validID(setupID); err != nil {
		failure(w, err)
		return "", "", "", false
	}
	if photoID != "" {
		if err := validID(photoID); err != nil {
			failure(w, err)
			return "", "", "", false
		}
	}
	return caller, setupID, photoID, true
}
func (h *Handler) gallery(w http.ResponseWriter, r *http.Request) {
	caller, setupID, _, ok := photoIDs(w, r)
	if !ok {
		return
	}
	list, err := h.service.repo.listPhotos(r.Context(), caller, setupID)
	if err != nil {
		photoFailure(w, err)
		return
	}
	respond(w, 200, list)
}
func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	uploadCtx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	r = r.WithContext(uploadCtx)
	// Bound slow multipart clients while holding a scarce upload slot.
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(45 * time.Second))
	defer controller.SetReadDeadline(time.Time{})
	caller, setupID, _, ok := photoIDs(w, r)
	if !ok {
		return
	}
	select {
	case h.service.uploadSlots <- struct{}{}:
		defer func() { <-h.service.uploadSlots }()
	default:
		respond(w, 503, struct {
			Error string `json:"error"`
		}{"photo uploads busy; retry with the same request_id"})
		return
	}
	// Owner authorization precedes multipart parsing and image decoding.
	if _, err := h.service.repo.GetOwned(r.Context(), caller, setupID); err != nil {
		photoFailure(w, err)
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "multipart/form-data" {
		failure(w, invalid("use multipart/form-data"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, photos.MaxBytes+64*1024)
	reader, err := r.MultipartReader()
	if err != nil {
		failure(w, invalid("invalid multipart body"))
		return
	}
	var raw []byte
	var requestID string
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			failure(w, invalid("invalid or oversized multipart body"))
			return
		}
		switch part.FormName() {
		case "file":
			if raw != nil {
				failure(w, invalid("upload exactly one file"))
				return
			}
			raw, err = io.ReadAll(io.LimitReader(part, photos.MaxBytes+1))
			if err != nil || len(raw) == 0 || len(raw) > photos.MaxBytes {
				failure(w, invalid("file exceeds 25 MiB or is empty"))
				return
			}
		case "request_id":
			if requestID != "" {
				failure(w, invalid("duplicate request_id"))
				return
			}
			value, err := io.ReadAll(io.LimitReader(part, 100))
			if err != nil {
				failure(w, invalid("invalid request_id"))
				return
			}
			requestID = string(value)
		default:
			failure(w, invalid("unknown multipart field"))
			return
		}
		_ = part.Close()
	}
	if raw == nil || validID(requestID) != nil {
		failure(w, invalid("file and UUID request_id are required"))
		return
	}
	p, err := h.service.uploadPhoto(r.Context(), caller, setupID, requestID, raw)
	if err != nil {
		photoFailure(w, err)
		return
	}
	respond(w, 201, p)
}
func (h *Handler) coverPhoto(w http.ResponseWriter, r *http.Request)  { h.photoMutation(w, r, false) }
func (h *Handler) deletePhoto(w http.ResponseWriter, r *http.Request) { h.photoMutation(w, r, true) }
func (h *Handler) photoMutation(w http.ResponseWriter, r *http.Request, remove bool) {
	caller, setupID, photoID, ok := photoIDs(w, r)
	if !ok {
		return
	}
	p, err := h.service.repo.mutatePhoto(r.Context(), caller, setupID, photoID, remove)
	if err != nil {
		photoFailure(w, err)
		return
	}
	if remove {
		// Queue remains durable if filesystem cleanup fails; worker retries.
		_ = h.service.ReconcilePhotos(r.Context())
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(204)
		return
	}
	respond(w, 200, p)
}
func (h *Handler) photoImage(w http.ResponseWriter, r *http.Request) {
	caller, setupID, photoID, ok := photoIDs(w, r)
	if !ok {
		return
	}
	p, err := h.service.repo.visiblePhoto(r.Context(), caller, setupID, photoID)
	if err != nil {
		photoFailure(w, err)
		return
	}
	thumbnail := chi.URLParam(r, "variant") == "thumbnail"
	mediaKey := p.key
	if thumbnail {
		mediaKey = p.thumbnailKey
	}
	f, err := h.service.photos.Open(mediaKey, thumbnail)
	if err != nil {
		failure(w, ErrNotFound)
		return
	}
	defer f.Close()
	// Revalidate immediately before delivery, including friendship revocation.
	if err := h.service.repo.photoAccess(r.Context(), caller, setupID); err != nil {
		photoFailure(w, err)
		return
	}
	_, err = f.Stat()
	if err != nil {
		photoFailure(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Disposition", "inline")
	// No ETag/Last-Modified or conditional 304 caching for sensitive content.
	http.ServeContent(w, r, "photo.jpg", time.Time{}, f)
}
