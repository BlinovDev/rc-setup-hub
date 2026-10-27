package setups

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// Both reads and mutation results use the same read-only historical projection.
// No activation filter is used here; is_active only controls new selection.
const displayColumns = `s.id::text,s.owner_id::text,s.chassis_model_id::text,s.title,s.visibility,s.data,s.notes,s.schema_version,s.created_at,s.updated_at,
 m.id::text,m.name,b.id::text,b.name, ` + coverPhotoJSON + `,dm.id::text,dm.name,fm.id::text,fm.name,rm.id::text,rm.name,km.id::text,km.name,fwm.id::text,fwm.name,ftm.id::text,ftm.name,rwm.id::text,rwm.name,rtm.id::text,rtm.name`
const displayJoins = ` LEFT JOIN chassis_models m ON m.id=s.chassis_model_id LEFT JOIN chassis_brands b ON b.id=m.brand_id LEFT JOIN setup_photos cp ON cp.setup_id=s.id AND cp.is_cover
 LEFT JOIN chassis_brands dm ON dm.id::text=(s.data #>> '{rear_differential,manufacturer_id}')
 LEFT JOIN chassis_brands fm ON fm.id::text=(s.data #>> '{suspension,front,lower_arm_mount,manufacturer_id}')
 LEFT JOIN chassis_brands rm ON rm.id::text=(s.data #>> '{suspension,rear,lower_arm_mount,manufacturer_id}')
 LEFT JOIN chassis_brands km ON km.id::text=(s.data #>> '{front_steering_knuckle,manufacturer_id}')
 LEFT JOIN chassis_brands fwm ON fwm.id::text=(s.data #>> '{running_gear,front,wheel,manufacturer_id}')
 LEFT JOIN chassis_brands ftm ON ftm.id::text=(s.data #>> '{running_gear,front,tire,manufacturer_id}')
 LEFT JOIN chassis_brands rwm ON rwm.id::text=(s.data #>> '{running_gear,rear,wheel,manufacturer_id}')
 LEFT JOIN chassis_brands rtm ON rtm.id::text=(s.data #>> '{running_gear,rear,tire,manufacturer_id}')`

const columns = `id,owner_id,chassis_model_id,title,visibility,data,notes,schema_version,created_at,updated_at`

func scan(row pgx.Row) (Setup, error) {
	var s Setup
	var raw, cover []byte
	var gearIDs, gearNames [4]*string
	var modelID, modelName, brandID, brandName, differentialBrandID, differentialBrandName, frontMountID, frontMountName, rearMountID, rearMountName, knuckleBrandID, knuckleBrandName *string
	err := row.Scan(&s.ID, &s.OwnerID, &s.ChassisModelID, &s.Title, &s.Visibility, &raw, &s.Notes, &s.SchemaVersion, &s.CreatedAt, &s.UpdatedAt, &modelID, &modelName, &brandID, &brandName, &cover, &differentialBrandID, &differentialBrandName, &frontMountID, &frontMountName, &rearMountID, &rearMountName, &knuckleBrandID, &knuckleBrandName, &gearIDs[0], &gearNames[0], &gearIDs[1], &gearNames[1], &gearIDs[2], &gearNames[2], &gearIDs[3], &gearNames[3])
	if errors.Is(err, pgx.ErrNoRows) {
		return Setup{}, ErrNotFound
	}
	if err != nil {
		return Setup{}, err
	}
	if s.SchemaVersion != SchemaVersion {
		return Setup{}, ErrUnsupportedSchema
	}
	var data *DataV1
	if err := decodeStrict(bytes.NewReader(raw), &data); err != nil {
		return Setup{}, err
	}
	if data == nil {
		return Setup{}, errors.New("invalid stored setup data")
	}
	s.Data = *data
	var gear RunningGearManufacturers
	for i, slot := range []**AxleRunningGearManufacturers{&gear.Front, &gear.Rear} {
		var axle AxleRunningGearManufacturers
		for j, part := range []**DifferentialManufacturer{&axle.Wheel, &axle.Tire} {
			n := i*2 + j
			if gearIDs[n] != nil && gearNames[n] != nil {
				*part = &DifferentialManufacturer{ID: *gearIDs[n], Name: *gearNames[n]}
			}
		}
		if axle.Wheel != nil || axle.Tire != nil {
			*slot = &axle
		}
	}
	if gear.Front != nil || gear.Rear != nil {
		s.RunningGearManufacturers = &gear
	}
	if knuckleBrandID != nil && knuckleBrandName != nil {
		s.FrontSteeringKnuckleManufacturer = &DifferentialManufacturer{ID: *knuckleBrandID, Name: *knuckleBrandName}
	}
	var mounts LowerArmMountManufacturers
	if frontMountID != nil && frontMountName != nil {
		mounts.Front = &DifferentialManufacturer{ID: *frontMountID, Name: *frontMountName}
	}
	if rearMountID != nil && rearMountName != nil {
		mounts.Rear = &DifferentialManufacturer{ID: *rearMountID, Name: *rearMountName}
	}
	if mounts.Front != nil || mounts.Rear != nil {
		s.LowerArmMountManufacturers = &mounts
	}
	if differentialBrandID != nil && differentialBrandName != nil {
		s.RearDifferentialManufacturer = &DifferentialManufacturer{ID: *differentialBrandID, Name: *differentialBrandName}
	}
	if modelID != nil {
		if modelName == nil || brandID == nil || brandName == nil {
			return Setup{}, errors.New("incomplete historical chassis")
		}
		s.Chassis = &SearchChassis{ModelID: *modelID, ModelName: *modelName, BrandID: *brandID, BrandName: *brandName}
	}
	if cover != nil {
		if s.CoverPhoto, err = decodeCoverPhoto(cover); err != nil {
			return Setup{}, err
		}
	}
	s.CreatedAt = s.CreatedAt.UTC()
	s.UpdatedAt = s.UpdatedAt.UTC()
	return s, nil
}
func writeError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" && pgErr.ConstraintName == "setups_chassis_model_id_fkey" {
		return ErrInvalidChassis
	}
	return err
}
func (r *Repository) Create(ctx context.Context, ownerID string, s Setup) (Setup, error) {
	raw, err := json.Marshal(s.Data)
	if err != nil {
		return Setup{}, err
	}
	result, err := scan(r.pool.QueryRow(ctx, `WITH written AS (INSERT INTO setups(owner_id,chassis_model_id,title,visibility,data,notes,schema_version)
        VALUES($1,$2,$3,$4,$5,$6,1) RETURNING `+columns+`) SELECT `+displayColumns+` FROM written s`+displayJoins, ownerID, s.ChassisModelID, s.Title, s.Visibility, raw, s.Notes))
	return result, writeError(err)
}
func (r *Repository) GetOwned(ctx context.Context, ownerID, id string) (Setup, error) {
	return scan(r.pool.QueryRow(ctx, `SELECT `+displayColumns+` FROM setups s`+displayJoins+` WHERE s.id=$1 AND s.owner_id=$2`, id, ownerID))
}

type accessMetadata struct {
	OwnerID    string
	Visibility Visibility
}

// getAccess reads only authorization metadata, never protected data or its schema.
func (r *Repository) getAccess(ctx context.Context, id string) (accessMetadata, error) {
	var access accessMetadata
	err := r.pool.QueryRow(ctx, `SELECT owner_id::text,visibility FROM setups WHERE id=$1`, id).Scan(&access.OwnerID, &access.Visibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return accessMetadata{}, ErrNotFound
	}
	return access, err
}

// getAuthorized decodes only a row whose metadata still matches the authorized read.
// Owner/visibility changes fail closed. The shared scope also rechecks friendship
// acceptance in this statement, before any protected document is decoded.
func (r *Repository) getAuthorized(ctx context.Context, id string, access accessMetadata, viewerID string) (Setup, error) {
	return scan(r.pool.QueryRow(ctx, `SELECT `+displayColumns+` FROM setups s`+displayJoins+` WHERE s.id=$1 AND s.owner_id=$2 AND s.visibility=$3 AND `+visibleSetupPredicate("$4"), id, access.OwnerID, access.Visibility, viewerID))
}

func (r *Repository) ListOwned(ctx context.Context, ownerID string) ([]Setup, error) {
	return r.ListVisible(ctx, ownerID, VisibleVisibilities(ownerID, ownerID, false))
}

// ListVisible filters in PostgreSQL, before decoding any setup documents.
func (r *Repository) ListVisible(ctx context.Context, ownerID string, allowed []Visibility) ([]Setup, error) {
	values := make([]string, len(allowed))
	for i, visibility := range allowed {
		values[i] = string(visibility)
	}
	rows, err := r.pool.Query(ctx, `SELECT `+displayColumns+` FROM setups s`+displayJoins+` WHERE s.owner_id=$1 AND s.visibility=ANY($2::text[]) ORDER BY s.created_at DESC,s.id DESC`, ownerID, values)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Setup{}
	for rows.Next() {
		s, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

// UpdateOwned uses the read timestamp to avoid erasing another concurrent partial update.
func (r *Repository) UpdateOwned(ctx context.Context, ownerID, id string, s Setup, expected time.Time) (Setup, error) {
	raw, err := json.Marshal(s.Data)
	if err != nil {
		return Setup{}, err
	}
	result, err := scan(r.pool.QueryRow(ctx, `WITH written AS (UPDATE setups SET chassis_model_id=$3,title=$4,visibility=$5,data=$6,notes=$7,updated_at=now()
        WHERE id=$1 AND owner_id=$2 AND updated_at=$8 AND schema_version=1 RETURNING `+columns+`) SELECT `+displayColumns+` FROM written s`+displayJoins,
		id, ownerID, s.ChassisModelID, s.Title, s.Visibility, raw, s.Notes, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := r.GetOwned(ctx, ownerID, id); lookupErr != nil {
			return Setup{}, lookupErr
		}
		return Setup{}, ErrConflict
	}
	return result, writeError(err)
}
func (r *Repository) DeleteOwned(ctx context.Context, ownerID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM setups WHERE id=$1 AND owner_id=$2`, id, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
