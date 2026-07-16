package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
)

func TestSourceWritesRequireAdministratorBeforeOpeningTransaction(t *testing.T) {
	service, err := NewService(Dependencies{Runtime: &database.Runtime{}, Sources: sourceRepositoryFake{}, MonitorUsage: usageFake{}, Audit: auditFake{}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	_, err = service.Create(context.Background(), CreateInput{Subject: identitydomain.Subject{UserID: 2, Role: identitydomain.RoleEditor}, Connection: validConnection()})
	assertAppCode(t, err, sharederrors.CodeForbidden)
	_, err = service.Create(context.Background(), CreateInput{Subject: identitydomain.Subject{Role: identitydomain.RoleAdmin}, Connection: validConnection()})
	assertAppCode(t, err, sharederrors.CodeUnauthenticated)
}

func TestSourceReadModelsCannotExposeCredentialReferences(t *testing.T) {
	for _, value := range []any{domain.PublicSourceConnection{}, domain.ManagementSourceConnection{}, domain.MonitorSourceConnection{}} {
		if _, found := reflect.TypeOf(value).FieldByName("CredentialRef"); found {
			t.Fatalf("%T unexpectedly exposes CredentialRef", value)
		}
	}

	connection := validConnection()
	connection.ID, connection.Version, connection.CredentialRef = 9, 3, "env:RSS_TOKEN"
	public := publicProjection(connection)
	management := managementProjection(connection)
	monitor := monitorProjection(connection)
	if !public.CredentialConfigured || !management.CredentialConfigured {
		t.Fatal("safe projections must retain only credential_configured")
	}
	if monitor.Endpoint == "" || monitor.Config.MaxPagesPerRun == 0 {
		t.Fatal("Monitor projection must retain safe execution input")
	}
}

func validConnection() domain.SourceConnection {
	return domain.SourceConnection{SourceType: domain.SourceTypeRSS, Name: "Test RSS", Endpoint: "https://feeds.example.test/rss", AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true}
}

func assertAppCode(t *testing.T, err error, want int) {
	t.Helper()
	var appError *sharederrors.AppError
	if !errors.As(err, &appError) || appError.Code != want {
		t.Fatalf("error = %v, want application code %d", err, want)
	}
}

type sourceRepositoryFake struct{}

func (sourceRepositoryFake) Create(context.Context, *domain.SourceConnection) error { return nil }
func (sourceRepositoryFake) FindByID(context.Context, int64) (*domain.SourceConnection, error) {
	return nil, nil
}
func (sourceRepositoryFake) LockByID(context.Context, int64) (*domain.SourceConnection, error) {
	return nil, nil
}
func (sourceRepositoryFake) Update(context.Context, *domain.SourceConnection) error { return nil }
func (sourceRepositoryFake) HasPublishedReference(context.Context, int64) (bool, error) {
	return false, nil
}

type usageFake struct{ usage domain.SourceUsage }

func (fake usageFake) UsageForSource(context.Context, int64) (domain.SourceUsage, error) {
	return fake.usage, nil
}

type auditFake struct{ err error }

func (fake auditFake) Write(context.Context, operationsdomain.AuditEntry) error { return fake.err }
