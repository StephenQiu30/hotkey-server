package domain

import (
	"fmt"
	"strings"
	"time"
)

type EntityType string

const (
	EntityPerson       EntityType = "person"
	EntityOrganization EntityType = "organization"
	EntityProduct      EntityType = "product"
	EntityLocation     EntityType = "location"
	EntityPolicy       EntityType = "policy"
	EntityEvent        EntityType = "event"
	EntityOther        EntityType = "other"
)

type Entity struct {
	ID, Version            int64
	Key, Name, Description string
	Type                   EntityType
	ManualLocked           bool
}

func (entity Entity) Validate() error {
	if entity.ID <= 0 || entity.Version <= 0 || strings.TrimSpace(entity.Key) == "" || len(entity.Key) > 128 || strings.TrimSpace(entity.Name) == "" || len(entity.Name) > 255 {
		return fmt.Errorf("invalid entity")
	}
	switch entity.Type {
	case EntityPerson, EntityOrganization, EntityProduct, EntityLocation, EntityPolicy, EntityEvent, EntityOther:
	default:
		return fmt.Errorf("invalid entity type")
	}
	return nil
}

type EntityAlias struct {
	ID, EntityID, Version            int64
	Alias, NormalizedAlias, Language string
	Origin                           FactOrigin
	Confirmed                        bool
}

func (alias EntityAlias) Validate() error {
	if alias.EntityID <= 0 || strings.TrimSpace(alias.Alias) == "" || len(alias.Alias) > 255 || strings.TrimSpace(alias.NormalizedAlias) == "" || len(alias.NormalizedAlias) > 255 || strings.TrimSpace(alias.Language) == "" || len(alias.Language) > 16 || !alias.Origin.Valid() {
		return fmt.Errorf("invalid entity alias")
	}
	return nil
}

type FactOrigin string

const (
	FactOriginSource FactOrigin = "source"
	FactOriginModel  FactOrigin = "model"
	FactOriginUser   FactOrigin = "user"
)

func (origin FactOrigin) Valid() bool {
	return origin == FactOriginSource || origin == FactOriginModel || origin == FactOriginUser
}

type EventEntity struct {
	ID, Version, EventID, EntityID int64
	Role                           string
	Confidence                     float64
	Origin                         FactOrigin
	Confirmed                      bool
}

func (entity EventEntity) Validate() error {
	if entity.EventID <= 0 || entity.EntityID <= 0 || strings.TrimSpace(entity.Role) == "" || len(entity.Role) > 64 || entity.Confidence < 0 || entity.Confidence > 100 || !entity.Origin.Valid() {
		return fmt.Errorf("invalid event entity")
	}
	return nil
}

type EntityRelationType string

const (
	EntityRelationRelatedTo      EntityRelationType = "related_to"
	EntityRelationAffiliatedWith EntityRelationType = "affiliated_with"
	EntityRelationLocatedIn      EntityRelationType = "located_in"
	EntityRelationOwns           EntityRelationType = "owns"
	EntityRelationOperates       EntityRelationType = "operates"
	EntityRelationSupports       EntityRelationType = "supports"
	EntityRelationOpposes        EntityRelationType = "opposes"
)

func (relationType EntityRelationType) Valid() bool {
	switch relationType {
	case EntityRelationRelatedTo, EntityRelationAffiliatedWith, EntityRelationLocatedIn, EntityRelationOwns, EntityRelationOperates, EntityRelationSupports, EntityRelationOpposes:
		return true
	default:
		return false
	}
}

type EntityRelation struct {
	ID, Version, FromEntityID, ToEntityID int64
	Type                                  EntityRelationType
	Confidence                            float64
	ValidFrom, ValidTo                    *time.Time
	Origin                                FactOrigin
	Confirmed                             bool
}

func (relation EntityRelation) Validate() error {
	if relation.FromEntityID <= 0 || relation.ToEntityID <= 0 || relation.FromEntityID == relation.ToEntityID || !relation.Type.Valid() || relation.Confidence < 0 || relation.Confidence > 100 || !relation.Origin.Valid() {
		return fmt.Errorf("invalid entity relation")
	}
	if relation.ValidFrom != nil && relation.ValidTo != nil && relation.ValidTo.Before(*relation.ValidFrom) {
		return fmt.Errorf("invalid entity relation validity window")
	}
	return nil
}
