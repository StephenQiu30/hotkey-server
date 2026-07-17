package domain

import (
	"fmt"
	"strings"
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
	Origin                           string
	Confirmed                        bool
}
