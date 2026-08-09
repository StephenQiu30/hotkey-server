package application

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"
	"time"
)

func TestRawEvidenceApplicationContractsUseOnlyPOJOFields(t *testing.T) {
	t.Parallel()

	contracts := []reflect.Type{
		reflect.TypeOf(RawResponseHeadersDTO{}), reflect.TypeOf(RawEvidenceSnapshotDTO{}), reflect.TypeOf(RawEvidenceReferenceDTO{}),
		reflect.TypeOf(RawEvidenceAttachmentDTO{}), reflect.TypeOf(RawEvidenceMetricsDTO{}), reflect.TypeOf(RawEvidenceItemDTO{}),
		reflect.TypeOf(RawEvidenceFetchDTO{}), reflect.TypeOf(EvidenceSelectorInputDTO{}), reflect.TypeOf(ReserveEvidenceSnapshotCommand{}),
		reflect.TypeOf(PersistedEvidenceSnapshotDTO{}), reflect.TypeOf(RawEvidenceRightsDecisionDTO{}), reflect.TypeOf(EvidenceSelectionManifestDTO{}),
		reflect.TypeOf((*EvidenceSelectorVerifier)(nil)).Elem(), reflect.TypeOf((*RawEvidenceStore)(nil)).Elem(),
		reflect.TypeOf((*EvidenceSnapshotRepository)(nil)).Elem(), reflect.TypeOf((*RawEvidenceArchiveUseCase)(nil)).Elem(),
		reflect.TypeOf((*CollectionEvidenceArchiver)(nil)).Elem(), reflect.TypeOf((*CurrentRawEvidenceRightsReader)(nil)).Elem(),
		reflect.TypeOf((*EvidenceSelectionManifestReader)(nil)).Elem(), reflect.TypeOf((*RawEvidenceObjectReader)(nil)).Elem(),
		reflect.TypeOf((*EvidenceByteSelector)(nil)).Elem(), reflect.TypeOf((*SourceDocumentGenerationScheduler)(nil)).Elem(),
	}
	for _, contract := range contracts {
		assertNoSourceDomainField(t, contract, map[reflect.Type]bool{})
	}

	for _, repositoryContract := range []any{
		ReserveEvidenceSnapshotCommand{}, PersistedEvidenceSnapshotDTO{}, SourceObservationDTO{},
		CommitEvidenceSnapshotCommand{}, CommittedEvidenceReferenceDTO{}, ScheduleSourceDocumentGenerationCommand{},
	} {
		assertNoFieldNamed(t, reflect.TypeOf(repositoryContract), "Body", map[reflect.Type]bool{})
	}
}

func assertNoFieldNamed(t *testing.T, contract reflect.Type, forbidden string, visiting map[reflect.Type]bool) {
	t.Helper()
	for contract.Kind() == reflect.Pointer || contract.Kind() == reflect.Slice || contract.Kind() == reflect.Array || contract.Kind() == reflect.Map {
		contract = contract.Elem()
	}
	if visiting[contract] {
		return
	}
	visiting[contract] = true
	defer delete(visiting, contract)
	if contract.Kind() == reflect.Interface {
		for index := 0; index < contract.NumMethod(); index++ {
			assertNoSourceDomainField(t, contract.Method(index).Type, visiting)
		}
		return
	}
	if contract.Kind() == reflect.Func {
		for index := 0; index < contract.NumIn(); index++ {
			assertNoSourceDomainField(t, contract.In(index), visiting)
		}
		for index := 0; index < contract.NumOut(); index++ {
			assertNoSourceDomainField(t, contract.Out(index), visiting)
		}
		return
	}
	if contract.Kind() != reflect.Struct {
		return
	}
	for index := 0; index < contract.NumField(); index++ {
		field := contract.Field(index)
		if field.Name == forbidden {
			t.Fatalf("repository/scheduler contract %s exposes synchronous raw item %s", contract, forbidden)
		}
		assertNoFieldNamed(t, field.Type, forbidden, visiting)
	}
}

func assertNoSourceDomainField(t *testing.T, contract reflect.Type, visiting map[reflect.Type]bool) {
	t.Helper()
	for contract.Kind() == reflect.Pointer || contract.Kind() == reflect.Slice || contract.Kind() == reflect.Array || contract.Kind() == reflect.Map {
		contract = contract.Elem()
	}
	if contract.PkgPath() == "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain" {
		t.Fatalf("Application POJO reaches Source Domain type %s", contract)
	}
	if contract.Kind() != reflect.Struct || visiting[contract] {
		return
	}
	visiting[contract] = true
	defer delete(visiting, contract)
	for index := 0; index < contract.NumField(); index++ {
		assertNoSourceDomainField(t, contract.Field(index).Type, visiting)
	}
}

func TestRawEvidencePOJOMappersRoundTripThroughDomainValidation(t *testing.T) {
	t.Parallel()

	capturedAt := time.Date(2026, time.August, 9, 16, 0, 0, 0, time.UTC)
	headers := RawResponseHeadersDTO{
		ContentType: pointerToString("application/atom+xml; charset=utf-8"),
		ETag:        pointerToString(`"feed-v1"`), LastModified: pointerToString("Sun, 09 Aug 2026 15:59:00 GMT"),
		Date: pointerToString("Sun, 09 Aug 2026 16:00:00 GMT"), Links: []string{"<https://feed.example.test/next>; rel=next"},
		RetryAfter: pointerToString("60"),
	}
	snapshot := RawEvidenceSnapshotDTO{
		Payload:                 []byte("<feed><entry>body-marker-stays-in-memory</entry></feed>"),
		CollectorProfileVersion: "rss-http-feed-go-xml-v1", MIMEType: "application/atom+xml; charset=utf-8",
		ResponseStatus: 200, RequestedURL: "https://feed.example.test/source.xml", FinalURL: "https://feed.example.test/source.xml",
		ResponseHeaders: headers, CapturedAt: capturedAt,
	}
	referenceStart, referenceEnd := int64(6), int64(55)
	reference := RawEvidenceReferenceDTO{
		LocatorType: "byte_range", LocatorValue: "bytes[6:55]", ByteStart: &referenceStart, ByteEnd: &referenceEnd,
		SelectedPayloadSHA256: digestValueForPOJOTest(snapshot.Payload[referenceStart:referenceEnd]), SelectorVersion: "byte-range-sha256-v1",
	}
	item := RawEvidenceItemDTO{
		SourceCode: "rss", ExternalID: "item-1", ContentType: "article", Title: "Title",
		Body: "body-marker-stays-in-memory", Language: "en", URL: "https://publisher.example.test/item-1", Author: "Publisher",
		ObservedAt: capturedAt, EvidenceCompleteness: "full_body",
		Attachments: []RawEvidenceAttachmentDTO{{URL: "https://publisher.example.test/asset.jpg", MIMEType: "image/jpeg"}},
		Metrics:     RawEvidenceMetricsDTO{ViewCount: pointerToInt64(0)}, EvidenceReferences: []RawEvidenceReferenceDTO{reference},
	}

	domainSnapshot, err := rawEvidenceSnapshotEntityFromDTO(snapshot)
	if err != nil {
		t.Fatalf("snapshot DTO to Domain: %v", err)
	}
	reference.EvidenceKey = domainSnapshot.Key
	item.SnapshotKey, item.ItemLocator, item.EvidenceReferences[0] = domainSnapshot.Key, reference.LocatorValue, reference
	domainItem, err := rawEvidenceItemEntityFromDTO(item)
	if err != nil {
		t.Fatalf("item DTO to Domain: %v", err)
	}
	roundTripSnapshot := rawEvidenceSnapshotDTOFromEntity(domainSnapshot)
	roundTripItem := rawEvidenceItemDTOFromEntity(domainItem)
	if roundTripSnapshot.EvidenceKey != domainSnapshot.Key || roundTripSnapshot.PayloadSHA256 != domainSnapshot.PayloadSHA256 ||
		!reflect.DeepEqual(roundTripSnapshot.ResponseHeaders, headers) || roundTripItem.Body != item.Body ||
		roundTripItem.EvidenceCompleteness != item.EvidenceCompleteness || roundTripItem.EvidenceReferences[0].LocatorType != reference.LocatorType {
		t.Fatalf("POJO round trip drifted: snapshot=%#v item=%#v", roundTripSnapshot, roundTripItem)
	}
}

func TestRawEvidencePOJOMappersRejectInvalidDomainStrings(t *testing.T) {
	t.Parallel()

	start, end := int64(0), int64(1)
	reference := RawEvidenceReferenceDTO{
		EvidenceKey: digestValueForPOJOTest([]byte("key")), LocatorType: "invented_locator", LocatorValue: "bytes[0:1]",
		ByteStart: &start, ByteEnd: &end, SelectedPayloadSHA256: digestValueForPOJOTest([]byte("x")), SelectorVersion: "byte-range-sha256-v1",
	}
	if _, err := rawEvidenceReferenceEntityFromDTO(reference); err == nil {
		t.Fatal("locator string bypassed Domain EvidenceReference validation")
	}
	item := RawEvidenceItemDTO{
		SourceCode: "rss", ExternalID: "item", ContentType: "article", Language: "en",
		ObservedAt: time.Now().UTC(), EvidenceCompleteness: "invented_completeness",
	}
	if _, err := rawEvidenceItemEntityFromDTO(item); err == nil {
		t.Fatal("completeness string bypassed Domain SourceItem validation")
	}
	decision := RawEvidenceRightsDecisionDTO{Action: "invented_action", Decision: "allow"}
	if _, _, err := rawEvidenceRightsDecisionEntitiesFromDTO(decision); err == nil {
		t.Fatal("rights action string bypassed Domain RightsAction validation")
	}
	decision.Action, decision.Decision = "store_raw", "invented_decision"
	if _, _, err := rawEvidenceRightsDecisionEntitiesFromDTO(decision); err == nil {
		t.Fatal("rights decision string bypassed Domain RightsState validation")
	}
}

func pointerToString(value string) *string { return &value }

func pointerToInt64(value int64) *int64 { return &value }

func digestValueForPOJOTest(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
