package domain

import "testing"

func TestEntityFactValuesRejectUncontrolledInput(t *testing.T) {
	if err := (EntityAlias{EntityID: 1, Alias: "Acme", NormalizedAlias: "acme", Language: "en", Origin: FactOriginModel}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (EntityAlias{EntityID: 1, Alias: "Acme", NormalizedAlias: "acme", Language: "en", Origin: FactOrigin("unknown")}).Validate(); err == nil {
		t.Fatal("unknown alias origin accepted")
	}
	if err := (EntityRelation{FromEntityID: 1, ToEntityID: 2, Type: EntityRelationRelatedTo, Confidence: 50, Origin: FactOriginModel}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (EntityRelation{FromEntityID: 1, ToEntityID: 2, Type: EntityRelationType("invented"), Confidence: 50, Origin: FactOriginModel}).Validate(); err == nil {
		t.Fatal("uncontrolled relation type accepted")
	}
}
