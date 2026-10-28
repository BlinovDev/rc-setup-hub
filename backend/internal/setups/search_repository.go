package setups

import "context"

// searchPublic filters visibility in SQL and returns one joined summary query.
// It never selects or decodes data/notes, and activation flags do not affect history.
func (r *Repository) searchPublic(ctx context.Context, params searchParameters) ([]SearchItem, error) {
	return r.search(ctx, params, "", false)
}
func (r *Repository) searchVisible(ctx context.Context, viewerID string, params searchParameters) ([]SearchItem, error) {
	return r.search(ctx, params, viewerID, true)
}
func (r *Repository) search(ctx context.Context, params searchParameters, viewerID string, visible bool) ([]SearchItem, error) {
	predicate := "s.visibility='public' AND s.moderation_status='approved'"
	args := []any{params.query, params.brandID, params.modelID, nil, nil, params.limit + 1, params.ownerID}
	if visible {
		predicate = visibleSetupPredicate("$8")
		args = append(args, viewerID)
	}
	var cursorTime, cursorID any
	if params.cursor != nil {
		cursorTime = params.cursor.CreatedAt
		cursorID = params.cursor.ID
	}
	args[3], args[4] = cursorTime, cursorID
	rows, err := r.pool.Query(ctx, `SELECT s.id::text,s.title,s.visibility,s.chassis_model_id::text,s.schema_version,s.created_at,s.updated_at,
 u.id::text,u.nickname,u.avatar_url,m.name,b.id::text,b.name, `+coverPhotoJSON+`
 FROM setups s JOIN users u ON u.id=s.owner_id
 LEFT JOIN chassis_models m ON m.id=s.chassis_model_id
 LEFT JOIN chassis_brands b ON b.id=m.brand_id LEFT JOIN setup_photos cp ON cp.setup_id=s.id AND cp.is_cover
 WHERE `+predicate+` AND ($7::uuid IS NULL OR s.owner_id=$7)
 AND ($1::text='' OR strpos(lower(s.title),lower($1))>0 OR strpos(lower(u.nickname),lower($1))>0)
 AND ($2::uuid IS NULL OR b.id=$2)
 AND ($3::uuid IS NULL OR s.chassis_model_id=$3)
 AND ($4::timestamptz IS NULL OR (s.created_at,s.id)<($4::timestamptz,$5::uuid))
 ORDER BY s.created_at DESC,s.id DESC LIMIT $6`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []SearchItem{}
	for rows.Next() {
		var item SearchItem
		var cover []byte
		var modelName, brandID, brandName *string
		if err := rows.Scan(&item.ID, &item.Title, &item.Visibility, &item.ChassisModelID, &item.SchemaVersion, &item.CreatedAt, &item.UpdatedAt,
			&item.Owner.ID, &item.Owner.Nickname, &item.Owner.AvatarURL, &modelName, &brandID, &brandName, &cover); err != nil {
			return nil, err
		}
		if item.ChassisModelID != nil && modelName != nil && brandID != nil && brandName != nil {
			item.Chassis = &SearchChassis{ModelID: *item.ChassisModelID, ModelName: *modelName, BrandID: *brandID, BrandName: *brandName}
		}
		if cover != nil {
			if item.CoverPhoto, err = decodeCoverPhoto(cover); err != nil {
				return nil, err
			}
		}
		item.CreatedAt = item.CreatedAt.UTC()
		item.UpdatedAt = item.UpdatedAt.UTC()
		items = append(items, item)
	}
	return items, rows.Err()
}
