package users

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/httpjson"
	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func avatarFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidAvatar), errors.Is(err, photos.ErrImage):
		httpjson.Error(w, 400, ErrInvalidAvatar.Error())
	case errors.Is(err, ErrNotFound):
		httpjson.Error(w, 404, "avatar unavailable")
	case errors.Is(err, photos.ErrBusy):
		httpjson.Error(w, 503, "avatar uploads busy; retry later")
	default:
		httpjson.Error(w, 500, "avatar unavailable")
	}
}
func (h *Handler) uploadAvatar(w http.ResponseWriter, r *http.Request, ownerID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(45 * time.Second))
	defer controller.SetReadDeadline(time.Time{})
	if h.service.avatars == nil {
		avatarFailure(w, errors.New("avatar storage unavailable"))
		return
	}
	select {
	case h.service.avatarSlots <- struct{}{}:
		defer func() { <-h.service.avatarSlots }()
	default:
		avatarFailure(w, photos.ErrBusy)
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "multipart/form-data" {
		httpjson.Error(w, 400, "use multipart/form-data with exactly one file")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxAvatarBytes+64*1024)
	reader, err := r.MultipartReader()
	if err != nil {
		httpjson.Error(w, 400, "invalid multipart body")
		return
	}
	var raw []byte
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			httpjson.Error(w, 400, "invalid or oversized multipart body")
			return
		}
		if part.FormName() != "file" || part.FileName() == "" || raw != nil {
			httpjson.Error(w, 400, "upload exactly one file")
			return
		}
		raw, err = io.ReadAll(io.LimitReader(part, MaxAvatarBytes+1))
		_ = part.Close()
		if err != nil || len(raw) == 0 || len(raw) > MaxAvatarBytes {
			avatarFailure(w, ErrInvalidAvatar)
			return
		}
	}
	if raw == nil {
		avatarFailure(w, ErrInvalidAvatar)
		return
	}
	u, err := h.service.UploadAvatar(ctx, ownerID, raw)
	if err != nil {
		avatarFailure(w, err)
		return
	}
	httpjson.Write(w, 200, u)
}
func (h *Handler) avatar(w http.ResponseWriter, r *http.Request) {
	id, avatarID := chi.URLParam(r, "user_id"), chi.URLParam(r, "avatar_id")
	for _, value := range []string{id, avatarID} {
		var uuid pgtype.UUID
		if err := uuid.Scan(value); err != nil || !uuid.Valid {
			httpjson.Error(w, 400, "invalid user or avatar UUID")
			return
		}
	}
	f, err := h.service.OpenAvatar(r.Context(), id, avatarID)
	if err != nil {
		avatarFailure(w, err)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		avatarFailure(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.WriteHeader(200)
	_, _ = io.Copy(w, f)
}
