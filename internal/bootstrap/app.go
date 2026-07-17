package bootstrap

import (
	"context"
	"flag"
	"fmt"
	"strings"

	eventapplication "github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	eventpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/event/infrastructure/postgres"
	eventtransport "github.com/StephenQiu30/hotkey-server/internal/modules/event/transport/http"
	identityapplication "github.com/StephenQiu30/hotkey-server/internal/modules/identity/application"
	identitypostgres "github.com/StephenQiu30/hotkey-server/internal/modules/identity/infrastructure/postgres"
	identityredis "github.com/StephenQiu30/hotkey-server/internal/modules/identity/infrastructure/redis"
	identitysecurity "github.com/StephenQiu30/hotkey-server/internal/modules/identity/infrastructure/security"
	identitysmtp "github.com/StephenQiu30/hotkey-server/internal/modules/identity/infrastructure/smtp"
	identitytransport "github.com/StephenQiu30/hotkey-server/internal/modules/identity/transport/http"
	ingestionapplication "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/application"
	ingestionminio "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/minio"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/postgres"
	ingestiontransport "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/transport/http"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/application"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/postgres"
	intelligenceprovider "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/provider"
	intelligencetransport "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/transport/http"
	monitorapplication "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/application"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/infrastructure/postgres"
	monitortransport "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/transport/http"
	operationspostgres "github.com/StephenQiu30/hotkey-server/internal/modules/operations/infrastructure/postgres"
	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	sourceinfrastructure "github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/postgres"
	sourcetransport "github.com/StephenQiu30/hotkey-server/internal/modules/source/transport/http"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"github.com/StephenQiu30/hotkey-server/internal/platform/observability"
	sharedclock "github.com/StephenQiu30/hotkey-server/internal/shared/clock"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

func NewApp(cfg config.Config, logger *zap.Logger) (*fx.App, error) {
	return NewAppWithReadiness(cfg, logger, httptransport.ReadinessFunc(func(context.Context) error { return nil }))
}

// NewAppWithReadiness makes the aggregate lifecycle check injectable. Runtime
// packages register their required dependencies here as they are introduced.
func NewAppWithReadiness(cfg config.Config, logger *zap.Logger, readiness httptransport.Readiness, extra ...fx.Option) (*fx.App, error) {
	role, err := ParseRole(cfg.Role)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	options := []fx.Option{
		fx.Supply(cfg, logger),
		fx.WithLogger(func() fxevent.Logger { return &fxevent.ZapLogger{Logger: logger} }),
	}
	usesDatabase := strings.TrimSpace(cfg.DatabaseURL) != ""
	if usesDatabase {
		options = append(options,
			fx.Provide(
				database.NewRuntime,
				intelligencepostgres.NewRepository,
				intelligenceapplication.NewSchemaRegistry,
				newAIProviderRegistry,
				intelligenceapplication.NewModelProfileService,
				newAIRunService,
				intelligenceapplication.NewRelevanceReviewService,
				newAIEmbeddingService,
				intelligenceapplication.NewRunLeaseReclaimer,
				ingestionpostgres.NewContentRepository,
				ingestionpostgres.NewRelevanceRepository,
				ingestionpostgres.NewRelevanceCandidateReader,
				newIngestionRelevanceReviewService,
				newIngestionEvidenceStore,
				sourcepostgres.NewCollectionRepository,
				newIngestionCapturedItemReader,
				newIngestionService,
				eventpostgres.NewRepository,
				newEventReadService,
				newEventLifecycleService,
				newEventGovernanceService,
				newEventHeatService,
				newEventClaimService,
			),
			fx.Invoke(database.RegisterLifecycle),
		)
	}
	if role.StartsAPI() {
		if err := cfg.ValidateAuthenticationRuntime(); err != nil {
			return nil, fmt.Errorf("validate API authentication configuration: %w", err)
		}
		if readiness == nil {
			return nil, fmt.Errorf("api readiness check is required")
		}
		readinessProvider := fx.Provide(func() httptransport.Readiness { return readiness })
		if usesDatabase {
			readinessProvider = fx.Provide(func(runtime *database.Runtime) httptransport.Readiness {
				return httptransport.ReadinessFunc(func(ctx context.Context) error {
					if err := readiness.Check(ctx); err != nil {
						return err
					}
					return runtime.Ping(ctx)
				})
			})
		}
		apiOptions := []fx.Option{
			readinessProvider,
			fx.Provide(observability.NewMetrics, observability.NewTelemetry, httptransport.NewRouter, httptransport.NewServer),
			fx.Invoke(observability.RegisterLifecycle, httptransport.RegisterServer),
		}
		if usesDatabase {
			apiOptions = append(apiOptions,
				fx.Provide(
					newIdentityVerificationStore,
					newIdentityService,
					newIdentityAuthenticator,
					operationspostgres.NewAuditWriter,
					monitorpostgres.NewSourceUsageReader,
					monitorpostgres.NewPublishedReferenceReader,
					sourcepostgres.NewRepository,
					newSourceService,
					sourceinfrastructure.NewConnectorRegistry,
					newCollectionControlService,
					monitorpostgres.NewRepository,
					newMonitorService,
					newIngestionContentQueryService,
					newIngestionRelevanceAPIService,
				),
				fx.Invoke(registerIdentityVerificationStoreLifecycle, registerIdentityRoutes, registerSourceRoutes, registerCollectionRoutes, registerMonitorRoutes, registerIngestionRoutes, registerIntelligenceRoutes, registerEventRoutes),
			)
		} else {
			apiOptions = append(apiOptions, fx.Provide(httptransport.NewUnavailableAuthenticator))
		}
		options = append(options, apiOptions...)
	}
	if role.StartsWorker() {
		options = append(options, fx.Invoke(registerWorkerLifecycle))
		if usesDatabase {
			options = append(options, fx.Invoke(intelligenceapplication.RegisterRunLeaseReclaimerLifecycle))
		}
	}
	options = append(options, extra...)

	return fx.New(options...), nil
}

func newAIProviderRegistry(cfg config.Config) *intelligenceapplication.ProviderRegistry {
	providers := make(map[intelligencedomain.ProviderName]intelligencedomain.Provider, 2)
	if provider, err := intelligenceprovider.NewOpenAIProvider(cfg.AI); err == nil {
		providers[intelligencedomain.ProviderOpenAI] = provider
	}
	if provider, err := intelligenceprovider.NewONNXProvider(cfg.AI); err == nil {
		providers[intelligencedomain.ProviderONNX] = provider
	}
	return intelligenceapplication.NewProviderRegistry(providers)
}

func newAIRunService(runs *intelligencepostgres.Repository, providers *intelligenceapplication.ProviderRegistry, schemas *intelligenceapplication.SchemaRegistry) (*intelligenceapplication.RunService, error) {
	return intelligenceapplication.NewRunService(intelligenceapplication.RunServiceDependencies{Runs: runs, Providers: providers, Schemas: schemas, Clock: sharedclock.System{}})
}

func newAIEmbeddingService(runs *intelligencepostgres.Repository, providers *intelligenceapplication.ProviderRegistry, runService *intelligenceapplication.RunService) (*intelligenceapplication.EmbeddingService, error) {
	return intelligenceapplication.NewEmbeddingService(intelligenceapplication.EmbeddingServiceDependencies{Runs: runs, Providers: providers, RunService: runService})
}

func registerIdentityRoutes(router *gin.Engine, service *identityapplication.Service, authenticator httptransport.Authenticator, cfg config.Config) {
	identitytransport.RegisterRoutes(router, service, authenticator, cfg)
}

func registerSourceRoutes(router *gin.Engine, service *sourceapplication.Service, authenticator httptransport.Authenticator) {
	sourcetransport.RegisterRoutes(router, service, authenticator)
}

func registerCollectionRoutes(router *gin.Engine, service *sourceapplication.CollectionControlService, authenticator httptransport.Authenticator) {
	sourcetransport.RegisterCollectionRoutes(router, service, authenticator)
}

func registerMonitorRoutes(router *gin.Engine, service *monitorapplication.Service, authenticator httptransport.Authenticator) {
	monitortransport.RegisterRoutes(router, service, authenticator)
}

func registerIngestionRoutes(router *gin.Engine, service *ingestionapplication.ContentQueryService, relevance *ingestionapplication.RelevanceAPIService, authenticator httptransport.Authenticator, metrics *observability.Metrics) {
	ingestiontransport.RegisterRoutes(router, service, authenticator, metrics)
	ingestiontransport.RegisterRelevanceRoutes(router, relevance, authenticator)
}

func registerIntelligenceRoutes(router *gin.Engine, service *intelligenceapplication.ModelProfileService, authenticator httptransport.Authenticator) {
	intelligencetransport.RegisterRoutes(router, service, authenticator)
}

func registerEventRoutes(router *gin.Engine, read *eventapplication.ReadService, lifecycle *eventapplication.LifecycleService, governance *eventapplication.GovernanceService, heat *eventapplication.HeatService, claims *eventapplication.ClaimService, authenticator httptransport.Authenticator) {
	eventtransport.RegisterRoutesWithHeatAndClaims(router, read, lifecycle, governance, heat, claims, authenticator)
}

// Fx does not infer interface bindings from a concrete repository. Keep the
// adapters at the composition root so event application services remain
// decoupled from the PostgreSQL implementation.
func newEventReadService(repository *eventpostgres.Repository) *eventapplication.ReadService {
	return eventapplication.NewReadService(repository)
}

func newEventLifecycleService(repository *eventpostgres.Repository) *eventapplication.LifecycleService {
	return eventapplication.NewLifecycleService(repository)
}

func newEventGovernanceService(repository *eventpostgres.Repository) *eventapplication.GovernanceService {
	return eventapplication.NewGovernanceService(repository)
}

func newEventHeatService(repository *eventpostgres.Repository) *eventapplication.HeatService {
	return eventapplication.NewHeatService(repository)
}

func newEventClaimService(repository *eventpostgres.Repository) *eventapplication.ClaimService {
	return eventapplication.NewClaimService(repository)
}

func newSourceService(runtime *database.Runtime, sources *sourcepostgres.Repository, usage *monitorpostgres.SourceUsageReader, references *monitorpostgres.PublishedReferenceReader, audit *operationspostgres.AuditWriter) (*sourceapplication.Service, error) {
	return sourceapplication.NewService(sourceapplication.Dependencies{Runtime: runtime, Sources: sources, MonitorUsage: usage, PublishedReferences: references, Audit: audit})
}

func newCollectionControlService(runtime *database.Runtime, sources *sourcepostgres.Repository, runs *sourcepostgres.CollectionRepository, connectors *sourceinfrastructure.ConnectorRegistry, metrics *observability.Metrics) (*sourceapplication.CollectionControlService, error) {
	return sourceapplication.NewCollectionControlService(sourceapplication.CollectionControlDependencies{Runtime: runtime, Sources: sources, Runs: runs, Connectors: connectors, Metrics: metrics})
}

func newIngestionEvidenceStore(cfg config.Config) (*ingestionminio.Store, error) {
	return ingestionminio.NewStore(cfg.MinIO)
}

func newIngestionCapturedItemReader(runs *sourcepostgres.CollectionRepository) (*sourceapplication.CapturedItemReader, error) {
	return sourceapplication.NewCapturedItemReader(sourceapplication.CapturedItemReaderDependencies{Runs: runs})
}

func newIngestionService(runtime *database.Runtime, captures *sourceapplication.CapturedItemReader, contents *ingestionpostgres.ContentRepository, evidence *ingestionminio.Store) (*ingestionapplication.Service, error) {
	return ingestionapplication.NewService(ingestionapplication.Dependencies{
		Runtime: runtime, Captures: captures, Contents: contents, Evidence: evidence,
	})
}

func newIngestionRelevanceReviewService(snapshots *ingestionpostgres.RelevanceRepository, reviews *intelligenceapplication.RelevanceReviewService) (*ingestionapplication.RelevanceReviewService, error) {
	return ingestionapplication.NewRelevanceReviewService(ingestionapplication.RelevanceReviewServiceDependencies{Snapshots: snapshots, Reviews: reviews})
}

func newIngestionContentQueryService(contents *ingestionpostgres.ContentRepository, sources *sourceapplication.Service) (*ingestionapplication.ContentQueryService, error) {
	return ingestionapplication.NewContentQueryService(ingestionapplication.ContentQueryDependencies{Contents: contents, Sources: sources})
}

func newIngestionRelevanceAPIService(snapshots *ingestionpostgres.RelevanceRepository, contents *ingestionpostgres.ContentRepository, candidates *ingestionpostgres.RelevanceCandidateReader) (*ingestionapplication.RelevanceAPIService, error) {
	return ingestionapplication.NewRelevanceAPIService(ingestionapplication.RelevanceAPIServiceDependencies{Snapshots: snapshots, Contents: contents, Candidates: candidates})
}

func newMonitorService(runtime *database.Runtime, monitors *monitorpostgres.Repository, sources *sourceapplication.Service, audit *operationspostgres.AuditWriter) (*monitorapplication.Service, error) {
	return monitorapplication.NewService(monitorapplication.Dependencies{Runtime: runtime, Monitors: monitors, Sources: sources, Audit: audit})
}

func newIdentityService(runtime *database.Runtime, cfg config.Config, verification *identityredis.VerificationStore) (*identityapplication.Service, error) {
	tokens, err := identitysecurity.NewJWT(identitysecurity.JWTConfig{
		Secret:   cfg.Authentication.JWTSecret,
		Issuer:   cfg.Authentication.JWTIssuer,
		Audience: cfg.Authentication.JWTAudience,
	})
	if err != nil {
		return nil, err
	}
	return identityapplication.NewService(identityapplication.Dependencies{
		Runtime:      runtime,
		Users:        identitypostgres.NewUserRepository(runtime),
		Sessions:     identitypostgres.NewSessionRepository(runtime),
		Audit:        identitypostgres.NewAuditRepository(runtime),
		Passwords:    identitysecurity.NewPasswordHasher(),
		Tokens:       tokens,
		Verification: verification,
		Mailer: identitysmtp.NewMailer(identitysmtp.Config{
			Enabled:   cfg.Authentication.SMTP.Enabled,
			Host:      cfg.Authentication.SMTP.Host,
			Port:      cfg.Authentication.SMTP.Port,
			TLSMode:   cfg.Authentication.SMTP.TLSMode,
			Username:  cfg.Authentication.SMTP.Username,
			Password:  cfg.Authentication.SMTP.Password,
			FromEmail: cfg.Authentication.SMTP.FromEmail,
			FromName:  cfg.Authentication.SMTP.FromName,
		}),
		Clock: sharedclock.System{},
	})
}

func newIdentityVerificationStore(cfg config.Config) (*identityredis.VerificationStore, error) {
	if strings.TrimSpace(cfg.Authentication.RedisURL) == "" {
		return identityredis.NewVerificationStore(nil, cfg.Authentication.VerificationHMACSecret), nil
	}
	return identityredis.NewVerificationStoreFromURL(cfg.Authentication.RedisURL, cfg.Authentication.VerificationHMACSecret)
}

func registerIdentityVerificationStoreLifecycle(lifecycle fx.Lifecycle, verification *identityredis.VerificationStore) {
	lifecycle.Append(fx.Hook{OnStop: func(context.Context) error { return verification.Close() }})
}

func newIdentityAuthenticator(service *identityapplication.Service) httptransport.Authenticator {
	return identityAuthenticator{authenticator: service.Authenticator()}
}

type identityAuthenticator struct {
	authenticator *identityapplication.Authenticator
}

func (adapter identityAuthenticator) Authenticate(ctx context.Context, token string) (httptransport.Subject, error) {
	subject, err := adapter.authenticator.Authenticate(ctx, token)
	if err != nil {
		return httptransport.Subject{}, err
	}
	return httptransport.Subject{UserID: subject.UserID, SessionID: subject.SessionID, Role: httptransport.Role(subject.Role)}, nil
}

func Run(ctx context.Context, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	if len(args) > 0 && args[0] == "db" {
		return runDatabaseCommand(ctx, cfg, args[1:])
	}
	if len(args) > 0 && args[0] == "user" {
		return runUserCommand(ctx, cfg, args[1:])
	}
	if err := applyCommandLine(&cfg, args); err != nil {
		return err
	}
	if err := cfg.ValidateRuntime(); err != nil {
		return fmt.Errorf("validate configuration: %w", err)
	}

	logger, err := logging.New(cfg.Environment)
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	app, err := NewApp(cfg, logger)
	if err != nil {
		return fmt.Errorf("build application: %w", err)
	}

	startCtx, cancelStart := context.WithTimeout(ctx, cfg.ShutdownTimeout)
	defer cancelStart()
	if err := startApp(startCtx, app); err != nil {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancelCleanup()
		_ = stopApp(cleanupCtx, app)
		return err
	}

	<-ctx.Done()
	stopCtx, cancelStop := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancelStop()
	if err := stopApp(stopCtx, app); err != nil {
		return err
	}
	return nil
}

func applyCommandLine(cfg *config.Config, args []string) error {
	if len(args) > 0 && args[0] != "serve" {
		return fmt.Errorf("unknown command %q: expected serve", args[0])
	}
	if len(args) > 0 {
		args = args[1:]
	}

	flags := flag.NewFlagSet("hotkey serve", flag.ContinueOnError)
	flags.SetOutput(new(discardWriter))
	flags.StringVar(&cfg.Role, "role", cfg.Role, "runtime role: all, api, or worker")
	flags.StringVar(&cfg.HTTPAddr, "http-addr", cfg.HTTPAddr, "HTTP listen address")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse serve flags: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	return nil
}

func registerWorkerLifecycle(lifecycle fx.Lifecycle, logger *zap.Logger) {
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			logger.Info("worker runtime started")
			return nil
		},
		OnStop: func(context.Context) error {
			logger.Info("worker runtime stopped")
			return nil
		},
	})
}

type discardWriter struct{}

func (*discardWriter) Write(data []byte) (int, error) {
	return len(data), nil
}
