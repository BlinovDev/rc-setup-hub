package setups

import (
	"context"
	"errors"
	"net/http"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/httpjson"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type CloneInput struct {
	RequestID string `json:"request_id"`
}

func (s *Service) Clone(ctx context.Context, ownerID, sourceID string, input CloneInput) (Setup, error) {
	source, err := canonicalUUID(&sourceID, "source setup ID")
	if err != nil {
		return Setup{}, err
	}
	request, err := canonicalUUID(&input.RequestID, "request_id")
	if err != nil {
		return Setup{}, err
	}
	if source == nil || request == nil {
		return Setup{}, invalid("source and request_id are required")
	}
	return s.repo.clone(ctx, ownerID, *source, *request)
}

func (r *Repository) clone(ctx context.Context, ownerID, sourceID, requestID string) (Setup, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Setup{}, err
	}
	defer tx.Rollback(context.Background())
	// Serialize this caller's request identity, including reuse for another source.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, ownerID+":"+requestID); err != nil {
		return Setup{}, err
	}
	var previousSource string
	var cloneID *string
	err = tx.QueryRow(ctx, `SELECT source_id::text,clone_id::text FROM setup_clone_requests WHERE owner_id=$1 AND request_id=$2`, ownerID, requestID).Scan(&previousSource, &cloneID)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Setup{}, err
	}
	if exists && (previousSource != sourceID || cloneID == nil) {
		return Setup{}, ErrConflict
	}
	// Parent share lock conflicts with source edits, photo re-moderation, decisions
	// and deletion. Eligibility and the copied snapshot remain stable until commit.
	var id string
	err = tx.QueryRow(ctx, `SELECT id::text FROM setups WHERE id=$1 AND visibility='public' AND moderation_status='approved' FOR SHARE`, sourceID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Setup{}, ErrNotFound
	}
	if err != nil {
		return Setup{}, err
	}
	if !exists {
		// Decode/validate supported stored schema before copying its original JSONB.
		// Preserve legacy nulls/precision and historical inactive catalogue references.
		if _, err := scan(tx.QueryRow(ctx, `SELECT `+displayColumns+` FROM setups s`+displayJoins+` WHERE s.id=$1`, sourceID)); err != nil {
			return Setup{}, err
		}
		var newID string
		err = tx.QueryRow(ctx, `INSERT INTO setups(owner_id,chassis_model_id,title,visibility,data,notes,schema_version,source_setup_id)
   SELECT $2,chassis_model_id,left(title,143)||' (Copy)','private',data,notes,schema_version,id FROM setups WHERE id=$1 RETURNING id::text`, sourceID, ownerID).Scan(&newID)
		if err != nil {
			return Setup{}, err
		}
		cloneID = &newID
		if _, err := tx.Exec(ctx, `INSERT INTO setup_clone_requests(owner_id,request_id,source_id,clone_id) VALUES($1,$2,$3,$4)`, ownerID, requestID, sourceID, *cloneID); err != nil {
			return Setup{}, err
		}
	}
	result, err := scan(tx.QueryRow(ctx, `SELECT `+displayColumns+` FROM setups s`+displayJoins+` WHERE s.id=$1 AND s.owner_id=$2`, *cloneID, ownerID))
	if err != nil {
		return Setup{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Setup{}, err
	}
	return result, nil
}

func (h *Handler) clone(w http.ResponseWriter, r *http.Request) {
	caller, ok := owner(w, r)
	if !ok {
		return
	}
	var body *CloneInput
	if !input(w, r, &body) {
		return
	}
	if body == nil {
		failure(w, invalid("body must be a JSON object"))
		return
	}
	result, err := h.service.Clone(r.Context(), caller, chi.URLParam(r, "id"), *body)
	if err != nil {
		failure(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/setups/"+result.ID)
	httpjson.Write(w, http.StatusCreated, result)
}
