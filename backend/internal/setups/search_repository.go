package setups

import "context"

// searchPublic filters visibility in SQL and returns one joined summary query.
// It never selects or decodes data/notes, and activation flags do not affect history.
func (r *Repository) searchPublic(ctx context.Context, params searchParameters) ([]SearchItem, error) {
	var cursorTime, cursorID any
	if params.cursor != nil {
		cursorTime = params.cursor.CreatedAt
		cursorID = params.cursor.ID
	}
	rows, err := r.pool.Query(ctx, `SELECT s.id::text,s.title,s.visibility,s.chassis_model_id::text,s.schema_version,s.created_at,s.updated_at,
 u.id::text,u.nickname,u.avatar_url,m.name,b.id::text,b.name
 FROM setups s JOIN users u ON u.id=s.owner_id
 LEFT JOIN chassis_models m ON m.id=s.chassis_model_id
 LEFT JOIN chassis_brands b ON b.id=m.brand_id
 WHERE s.visibility='public'
 AND ($1::text='' OR strpos(lower(s.title),lower($1))>0 OR strpos(lower(u.nickname),lower($1))>0)
 AND ($2::uuid IS NULL OR b.id=$2)
 AND ($3::uuid IS NULL OR s.chassis_model_id=$3)
 AND ($4::timestamptz IS NULL OR (s.created_at,s.id)<($4::timestamptz,$5::uuid))
 ORDER BY s.created_at DESC,s.id DESC LIMIT $6`, params.query, params.brandID, params.modelID, cursorTime, cursorID, params.limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []SearchItem{}
	for rows.Next() {
		var item SearchItem
		var modelName, brandID, brandName *string
		if err := rows.Scan(&item.ID, &item.Title, &item.Visibility, &item.ChassisModelID, &item.SchemaVersion, &item.CreatedAt, &item.UpdatedAt,
			&item.Owner.ID, &item.Owner.Nickname, &item.Owner.AvatarURL, &modelName, &brandID, &brandName); err != nil {
			return nil, err
		}
		if item.ChassisModelID != nil && modelName != nil && brandID != nil && brandName != nil {
			item.Chassis = &SearchChassis{ModelID: *item.ChassisModelID, ModelName: *modelName, BrandID: *brandID, BrandName: *brandName}
		}
		item.CreatedAt = item.CreatedAt.UTC()
		item.UpdatedAt = item.UpdatedAt.UTC()
		items = append(items, item)
	}
	return items, rows.Err()
}
