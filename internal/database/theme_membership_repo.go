package database

import (
	"context"

	"gorm.io/gorm"
)

// ThemeMembershipModel is the GORM model for theme_memberships.
type ThemeMembershipModel struct {
	ID         int64  `gorm:"column:id;primaryKey"`
	ObjectType string `gorm:"column:object_type"`
	ObjectID   int64  `gorm:"column:object_id"`
	ThemeRef   string `gorm:"column:theme_ref"`
}

func (ThemeMembershipModel) TableName() string { return "theme_memberships" }

// ThemeMembershipRepo handles writes to the theme_memberships sidecar table.
type ThemeMembershipRepo struct {
	db *gorm.DB
}

// NewThemeMembershipRepo creates a new ThemeMembershipRepo.
func NewThemeMembershipRepo(db *gorm.DB) *ThemeMembershipRepo {
	return &ThemeMembershipRepo{db: db}
}

// SetThemeRef sets the theme_ref field for an object.
func (r *ThemeMembershipRepo) SetThemeRef(ctx context.Context, objectType string, objectID int64, themeRef string) error {
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO theme_memberships (object_type, object_id, theme_ref)
		 VALUES (?, ?, ?)
		 ON CONFLICT (object_type, object_id) DO UPDATE SET theme_ref = EXCLUDED.theme_ref`,
		objectType, objectID, themeRef,
	).Error
}
