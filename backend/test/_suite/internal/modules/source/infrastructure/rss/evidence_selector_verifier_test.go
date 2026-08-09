package rss

import (
	"fmt"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestEvidenceSelectorVerifierReselectsFrozenFeedFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload []byte
	}{
		{name: "RSS", payload: []byte(`<?xml version="1.0"?><rss><channel><item><guid>rss-one</guid><title>One</title><description>Body</description></item></channel></rss>`)},
		{name: "Atom", payload: []byte(`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><entry><id>atom-one</id><title>One</title><content>Body</content></entry></feed>`)},
		{name: "RDF", payload: []byte(`<?xml version="1.0"?><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><item rdf:about="rdf-one"><title>One</title><description>Body</description></item></rdf:RDF>`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capturedAt := time.Date(2026, time.August, 9, 6, 0, 0, 0, time.UTC)
			feed, err := parseFeed(test.payload, capturedAt)
			if err != nil {
				t.Fatal(err)
			}
			if len(feed.Items) != 1 || len(feed.Items[0].EvidenceReferences) != 1 {
				t.Fatalf("parsed feed evidence = %#v", feed)
			}
			snapshot := selectorSnapshot(t, test.payload, capturedAt)
			reference := feed.Items[0].EvidenceReferences[0]
			reference.SnapshotKey = snapshot.Key
			if err := NewEvidenceSelectorVerifier().Verify(evidenceSelectorInput(t, snapshot, reference)); err != nil {
				t.Fatalf("Verify(): %v", err)
			}
		})
	}
}

func TestEvidenceSelectorVerifierRejectsHashBoundsAndUnknownVersions(t *testing.T) {
	t.Parallel()

	payload := []byte(`<?xml version="1.0"?><feed><entry><id>one</id><title>One</title></entry></feed>`)
	capturedAt := time.Date(2026, time.August, 9, 6, 0, 0, 0, time.UTC)
	snapshot := selectorSnapshot(t, payload, capturedAt)
	feed, err := parseFeed(payload, capturedAt)
	if err != nil {
		t.Fatal(err)
	}
	xmlReference := feed.Items[0].EvidenceReferences[0]
	xmlReference.SnapshotKey = snapshot.Key

	tests := []struct {
		name      string
		reference domain.EvidenceReference
	}{
		{name: "incorrect selected hash", reference: func() domain.EvidenceReference {
			value := xmlReference
			value.SelectedPayloadSHA256 = fmt.Sprintf("%064d", 0)
			return value
		}()},
		{name: "unknown XML selector", reference: func() domain.EvidenceReference {
			value := xmlReference
			value.SelectorVersion = "atom-unknown-v1"
			return value
		}()},
		{name: "byte range out of bounds", reference: domain.EvidenceReference{
			SnapshotKey: snapshot.Key, LocatorType: domain.EvidenceLocatorByteRange,
			LocatorValue: fmt.Sprintf("bytes[0:%d]", len(payload)+1), ByteStart: int64Pointer(0), ByteEnd: int64Pointer(int64(len(payload) + 1)),
			SelectedPayloadSHA256: snapshot.PayloadSHA256, SelectorVersion: domain.ByteRangeSelectorVersion,
		}},
		{name: "unknown locator", reference: domain.EvidenceReference{
			SnapshotKey: snapshot.Key, LocatorType: domain.EvidenceLocatorJSONPointer, LocatorValue: "/feed/entry/0",
			SelectedPayloadSHA256: snapshot.PayloadSHA256, SelectorVersion: "json-pointer-sha256-v1",
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := NewEvidenceSelectorVerifier().Verify(evidenceSelectorInput(t, snapshot, test.reference)); err == nil {
				t.Fatal("Verify() accepted unverifiable evidence selection")
			}
		})
	}

	tampered := snapshot
	tampered.PayloadSHA256 = fmt.Sprintf("%064d", 0)
	if err := NewEvidenceSelectorVerifier().Verify(evidenceSelectorInput(t, tampered, xmlReference)); err == nil {
		t.Fatal("Verify() accepted a snapshot with a tampered payload digest")
	}
}

func TestEvidenceSelectorVerifierRecomputesWholePayload(t *testing.T) {
	t.Parallel()

	payload := []byte("immutable raw response")
	snapshot := selectorSnapshot(t, payload, time.Date(2026, time.August, 9, 6, 0, 0, 0, time.UTC))
	reference := domain.EvidenceReference{
		SnapshotKey: snapshot.Key, LocatorType: domain.EvidenceLocatorWholePayload, LocatorValue: "/",
		SelectedPayloadSHA256: snapshot.PayloadSHA256, SelectorVersion: domain.WholePayloadSelectorVersion,
	}
	if err := NewEvidenceSelectorVerifier().Verify(evidenceSelectorInput(t, snapshot, reference)); err != nil {
		t.Fatalf("Verify(): %v", err)
	}
	reference.SelectedPayloadSHA256 = fmt.Sprintf("%064d", 0)
	if err := NewEvidenceSelectorVerifier().Verify(evidenceSelectorInput(t, snapshot, reference)); err == nil {
		t.Fatal("Verify() accepted an incorrect whole-payload hash")
	}
}

func selectorSnapshot(t *testing.T, payload []byte, capturedAt time.Time) domain.EvidenceSnapshot {
	t.Helper()
	profile, err := domain.NewCollectorProfileVersion(CollectorProfileVersion)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := domain.NewEvidenceSnapshot(domain.EvidenceSnapshot{
		Payload: payload, CollectorProfileVersion: profile, MIMEType: "application/xml", StatusCode: 200,
		RequestedURL: "https://feed.example.test/source.xml", FinalURL: "https://feed.example.test/source.xml", CapturedAt: capturedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func evidenceSelectorInput(t *testing.T, snapshot domain.EvidenceSnapshot, reference domain.EvidenceReference) sourceapplication.EvidenceSelectorInputDTO {
	t.Helper()
	headers, err := sourceapplication.NewRawResponseHeadersDTO(snapshot.ResponseHeaders.Values())
	if err != nil {
		t.Fatal(err)
	}
	return sourceapplication.EvidenceSelectorInputDTO{
		Snapshot: sourceapplication.RawEvidenceSnapshotDTO{
			EvidenceKey: snapshot.Key, Payload: append([]byte(nil), snapshot.Payload...),
			CollectorProfileVersion: snapshot.CollectorProfileVersion.String(), MIMEType: snapshot.MIMEType,
			ResponseStatus: snapshot.StatusCode, RequestedURL: snapshot.RequestedURL, FinalURL: snapshot.FinalURL,
			RedirectChain: append([]string(nil), snapshot.RedirectChain...), ResponseHeaders: headers,
			CapturedAt: snapshot.CapturedAt, PayloadSHA256: snapshot.PayloadSHA256,
		},
		Reference: sourceapplication.RawEvidenceReferenceDTO{
			EvidenceKey: reference.SnapshotKey, LocatorType: string(reference.LocatorType), LocatorValue: reference.LocatorValue,
			ByteStart: reference.ByteStart, ByteEnd: reference.ByteEnd,
			SelectedPayloadSHA256: reference.SelectedPayloadSHA256, SelectorVersion: reference.SelectorVersion,
		},
	}
}
