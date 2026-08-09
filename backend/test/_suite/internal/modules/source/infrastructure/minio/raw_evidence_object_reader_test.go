package minio

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	miniosdk "github.com/minio/minio-go/v7"
)

type rawEvidenceReadClientFake struct {
	fixture  rawObjectFixture
	statSize *int64
	statErr  error
	readErr  error
	stats    int
	reads    int
}

func (client *rawEvidenceReadClientFake) StatObject(_ context.Context, _, objectKey string, _ miniosdk.StatObjectOptions) (miniosdk.ObjectInfo, error) {
	client.stats++
	if client.statErr != nil {
		return miniosdk.ObjectInfo{}, client.statErr
	}
	metadata := make(http.Header, len(client.fixture.metadata)+1)
	metadata.Set("Content-Type", client.fixture.mimeType)
	for name, value := range client.fixture.metadata {
		metadata.Set("X-Amz-Meta-"+name, value)
	}
	size := int64(len(client.fixture.payload))
	if client.statSize != nil {
		size = *client.statSize
	}
	return miniosdk.ObjectInfo{Key: objectKey, Size: size, ContentType: client.fixture.mimeType, Metadata: metadata}, nil
}

func (client *rawEvidenceReadClientFake) ReadObject(_ context.Context, _, _ string, _ miniosdk.GetObjectOptions) (io.ReadCloser, error) {
	client.reads++
	if client.readErr != nil {
		return nil, client.readErr
	}
	return io.NopCloser(bytes.NewReader(client.fixture.payload)), nil
}

func TestRawEvidenceObjectReaderVerifiesImmutableManifestAndReturnsDefensivePayload(t *testing.T) {
	t.Parallel()
	command := rawStoreObject(42, "raw response")
	client := &rawEvidenceReadClientFake{fixture: rawReadFixture(command)}
	reader := newRawEvidenceObjectReader(client, "evidence")
	var port application.RawEvidenceObjectReader = reader
	query := rawReadQuery(command)

	first, err := port.Read(context.Background(), query)
	if err != nil {
		t.Fatalf("Read(first): %v", err)
	}
	if !bytes.Equal(first.Payload, command.Payload) || client.stats != 2 || client.reads != 1 {
		t.Fatalf("verified read calls = stats:%d reads:%d", client.stats, client.reads)
	}
	first.Payload[0] ^= 0xff
	second, err := port.Read(context.Background(), query)
	if err != nil {
		t.Fatalf("Read(second): %v", err)
	}
	if !bytes.Equal(second.Payload, command.Payload) || client.stats != 4 || client.reads != 2 {
		t.Fatal("reader did not return a defensive payload copy")
	}
	for _, resultType := range []reflect.Type{
		reflect.TypeOf(application.ReadRawEvidenceObjectResult{}),
		reflect.TypeOf(application.SelectedEvidenceDTO{}),
	} {
		if _, found := resultType.FieldByName("ObjectKey"); found {
			t.Fatalf("cross-module %s exposes MinIO object key", resultType.Name())
		}
	}
}

func TestRawEvidenceObjectReaderFailsClosedOnObjectIntegrityMismatch(t *testing.T) {
	t.Parallel()
	command := rawStoreObject(42, "raw response")
	query := rawReadQuery(command)

	tests := []struct {
		name   string
		mutate func(*rawEvidenceReadClientFake)
	}{
		{name: "payload digest", mutate: func(client *rawEvidenceReadClientFake) {
			client.fixture.payload = []byte("bad response")
		}},
		{name: "manifest size", mutate: func(client *rawEvidenceReadClientFake) {
			wrong := int64(len(command.Payload) + 1)
			client.statSize = &wrong
		}},
		{name: "manifest MIME", mutate: func(client *rawEvidenceReadClientFake) {
			client.fixture.mimeType = "application/xml"
		}},
		{name: "manifest payload identity", mutate: func(client *rawEvidenceReadClientFake) {
			client.fixture.metadata[rawMetadataPayloadSHA256] = digestValueForRawReader("different")
		}},
		{name: "manifest collector profile", mutate: func(client *rawEvidenceReadClientFake) {
			client.fixture.metadata[rawMetadataCollectorProfileVersion] = "different-profile-v1"
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &rawEvidenceReadClientFake{fixture: rawReadFixture(command)}
			test.mutate(client)
			reader := newRawEvidenceObjectReader(client, "evidence")
			if _, err := reader.Read(context.Background(), query); !errors.Is(err, domain.ErrRawEvidenceConflict) {
				t.Fatalf("Read() error = %v", err)
			}
		})
	}
}

func TestRawEvidenceObjectReaderMapsInvalidMissingAndUnavailableWithoutReadingBytes(t *testing.T) {
	t.Parallel()
	command := rawStoreObject(42, "raw response")
	query := rawReadQuery(command)

	invalidClient := &rawEvidenceReadClientFake{fixture: rawReadFixture(command)}
	invalidReader := newRawEvidenceObjectReader(invalidClient, "evidence")
	invalid := query
	invalid.ObjectKey = "untrusted/object/key"
	if _, err := invalidReader.Read(context.Background(), invalid); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("invalid query error = %v", err)
	}
	if invalidClient.stats != 0 || invalidClient.reads != 0 {
		t.Fatal("invalid query reached the object client")
	}

	missingClient := &rawEvidenceReadClientFake{
		fixture: rawReadFixture(command), statErr: miniosdk.ErrorResponse{Code: "NoSuchKey", StatusCode: http.StatusNotFound},
	}
	if _, err := newRawEvidenceObjectReader(missingClient, "evidence").Read(context.Background(), query); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("missing object error = %v", err)
	}

	unavailableClient := &rawEvidenceReadClientFake{fixture: rawReadFixture(command), readErr: errors.New("provider detail must not escape")}
	if _, err := newRawEvidenceObjectReader(unavailableClient, "evidence").Read(context.Background(), query); !errors.Is(err, sharedrepository.ErrUnavailable) {
		t.Fatalf("unavailable object error = %v", err)
	} else if bytes.Contains([]byte(err.Error()), []byte("provider detail")) {
		t.Fatalf("provider error escaped adapter boundary: %v", err)
	}
}

func rawReadFixture(command application.StoreRawEvidenceCommand) rawObjectFixture {
	return rawObjectFixture{
		payload: append([]byte(nil), command.Payload...), mimeType: command.MIMEType,
		metadata: map[string]string{
			rawMetadataPayloadSHA256: command.PayloadSHA256, rawMetadataEvidenceKey: command.EvidenceKey,
			rawMetadataSourceConnectionID: "42", rawMetadataCollectorProfileVersion: command.CollectorProfileVersion,
		},
	}
}

func rawReadQuery(command application.StoreRawEvidenceCommand) application.ReadRawEvidenceObjectQuery {
	return application.ReadRawEvidenceObjectQuery{
		SourceConnectionID: command.SourceConnectionID, EvidenceKey: command.EvidenceKey, ObjectKey: command.ObjectKey,
		PayloadSHA256: command.PayloadSHA256, CollectorProfileVersion: command.CollectorProfileVersion,
		MIMEType: command.MIMEType, SizeBytes: int64(len(command.Payload)), MaximumBytes: application.MaximumRawEvidenceReadBytes,
	}
}

func digestValueForRawReader(value string) string {
	command := rawStoreObject(42, value)
	return command.PayloadSHA256
}
