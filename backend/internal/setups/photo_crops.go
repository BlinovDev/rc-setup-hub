package setups

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
)

// CropInput uses explicit revision and crop fields; null crop resets to centered.
type CropInput struct {
	ExpectedRevision *int         `json:"expected_revision"`
	Crop             *photos.Crop `json:"crop"`
}

func (s *Service) applyCrop(ctx context.Context, ownerID, setupID, photoID string, input CropInput) (Photo, error) {
	var generated string
	p, keep, err := s.repo.cropPhoto(ctx, ownerID, setupID, photoID, input, func(p Photo, crop photos.Crop) (photos.Image, error) {
		img, err := s.photos.CropThumbnail(ctx, p.key, crop)
		generated = img.Key
		return img, err
	})
	if !keep && generated != "" {
		_ = s.photos.Remove(generated)
	}
	if err == nil && generated != "" {
		s.cleanupPhotos(ctx)
	}
	return p, err
}

func (h *Handler) cropPhoto(w http.ResponseWriter, r *http.Request) {
	caller, setupID, photoID, ok := photoIDs(w, r)
	if !ok {
		return
	}
	// Authorization precedes parsing, decoding and processing.
	if _, err := h.service.repo.GetOwned(r.Context(), caller, setupID); err != nil {
		photoFailure(w, err)
		return
	}
	var body struct {
		ExpectedRevision *int `json:"expected_revision"`
		Crop             *struct {
			X    *int `json:"x"`
			Y    *int `json:"y"`
			Size *int `json:"size"`
		} `json:"crop"`
	}
	// Require crop even when null; decode strict object shape then typed fields.
	var raw map[string]json.RawMessage
	if !input(w, r, &raw) {
		return
	}
	if len(raw) != 2 || raw["expected_revision"] == nil || raw["crop"] == nil {
		failure(w, invalid("expected_revision and crop (object or null) are required"))
		return
	}
	data, _ := json.Marshal(raw)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		failure(w, invalid("invalid crop request"))
		return
	}
	value := CropInput{ExpectedRevision: body.ExpectedRevision}
	if body.Crop != nil {
		if body.Crop.X == nil || body.Crop.Y == nil || body.Crop.Size == nil {
			failure(w, invalid("x, y and size are required integers"))
			return
		}
		value.Crop = &photos.Crop{X: *body.Crop.X, Y: *body.Crop.Y, Size: *body.Crop.Size}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	p, err := h.service.applyCrop(ctx, caller, setupID, photoID, value)
	if errors.Is(err, photos.ErrBusy) {
		respond(w, 503, struct {
			Error string `json:"error"`
		}{"photo processor busy; retry the same crop and expected_revision"})
		return
	}
	if err != nil {
		photoFailure(w, err)
		return
	}
	respond(w, 200, p)
}
