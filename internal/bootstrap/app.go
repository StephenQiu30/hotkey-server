package bootstrap

import (
	"context"
	"flag"
	"fmt"
	"strings"

	deliveryapplication "github.com/StephenQiu30/hotkey-server/internal/modules/delivery/application"
	deliveryjobs "github.com/StephenQiu30/hotkey-server/internal/modules/delivery/infrastructure/jobs"
	deliverypostgres "github.com/StephenQiu30/hotkey-server/internal/modules/delivery/infrastructure/postgres"
	deliverysmtp "github.com/StephenQiu30/hotkey-server/internal/modules/delivery/infrastructure/smtp"
	deliverytransport "github.com/StephenQiu30/hotkey-server/internal/modules/delivery/transport/http"
	eventapplication "github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	eventjobs "github.com/StephenQiu30/hotkey-server/internal/modules/event/infrastructure/jobs"
	eventpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/event/infrastructure/postgres"
	eventtransport "github.com/StephenQiu30/hotkey-server/internal/modules/event/transport/http"
	identityapplication "github.com/StephenQiu30/hotkey-server/internal/modules/identity/application"
	identitypostgres "github.com/StephenQiu30/hotkey-server/internal/modules/identity/infrastructure/postgres"
	identityredis "github.com/StephenQiu30/hotkey-server/internal/modules/identity/infrastructure/redis"
	identitysecurity "github.com/StephenQiu30/hotkey-server/internal/modules/identity/infrastructure/security"
	identitysmtp "github.com/StephenQiu30/hotkey-server/internal/modules/identity/infrastructure/smtp"
	identitytransport "github.com/StephenQiu30/hotkey-server/internal/modules/identity/transport/http"
	ingestionapplication "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	ingestionjobs "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/jobs"
	ingestionminio "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/minio"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/postgres"
	ingestiontransport "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/transport/http"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/application"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	intelligencejobs "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/jobs"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/postgres"
	intelligenceprovider "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/provider"
	intelligencetransport "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/transport/http"
	knowledgeapplication "github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/application"
	knowledgejobs "github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/infrastructure/jobs"
	knowledgeminio "github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/infrastructure/minio"
	knowledgepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/infrastructure/postgres"
	knowledgevault "github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/infrastructure/vault"
	knowledgetransport "github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/transport/http"
	monitorapplication "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/application"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/infrastructure/postgres"
	monitortransport "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/transport/http"
	operationsapplication "github.com/StephenQiu30/hotkey-server/internal/modules/operations/application"
	operationspostgres "github.com/StephenQiu30/hotkey-server/internal/modules/operations/infrastructure/postgres"
	operationstransport "github.com/StephenQiu30/hotkey-server/internal/modules/operations/transport/http"
	reportapplication "github.com/StephenQiu30/hotkey-server/internal/modules/report/application"
	reportjobs "github.com/StephenQiu30/hotkey-server/internal/modules/report/infrastructure/jobs"
	reportpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/report/infrastructure/postgres"
	reportvault "github.com/StephenQiu30/hotkey-server/internal/modules/report/infrastructure/vault"
	reporttransport "github.com/StephenQiu30/hotkey-server/internal/modules/report/transport/http"
	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	sourceinfrastructure "github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure"
	sourcejobs "github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/jobs"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/postgres"
	sourcetransport "github.com/StephenQiu30/hotkey-server/internal/modules/source/transport/http"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"github.com/StephenQiu30/hotkey-server/internal/platform/observability"
	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
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
				operationspostgres.NewAuditWriter,
				operationspostgres.NewJobRepository,
				sourcepostgres.NewRepository,
				sourcepostgres.NewMetricCapabilityRepository,
				newMetricCapabilityService,
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
				newEventContentMetricRefreshService,
				newEventClaimService,
				newEventIntelligenceRunner,
				newEventIntelligenceReadService,
				newEventSummaryService,
				newEventClaimExtractionService,
				monitorpostgres.NewPublishedCollectionTargetReader,
				knowledgepostgres.NewRepository,
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
					monitorpostgres.NewSourceUsageReader,
					monitorpostgres.NewPublishedReferenceReader,
					newSourceService,
					sourceinfrastructure.NewConnectorRegistry,
					newCollectionControlService,
					monitorpostgres.NewRepository,
					newMonitorService,
					newIngestionContentQueryService,
					newIngestionRelevanceAPIService,
					deliverypostgres.NewRepository,
					newDeliverySubscriptionService,
					reportpostgres.NewRepository,
					newReportService,
					newKnowledgeVaultWriter,
					newKnowledgeProposalService,
					newKnowledgeReconciler,
					newKnowledgeHandler,
					newOperationsOverviewService,
					newJobService,
				),
				fx.Invoke(registerIdentityVerificationStoreLifecycle, registerIdentityRoutes, registerSourceRoutes, registerMetricCapabilityRoutes, registerCollectionRoutes, registerMonitorRoutes, registerIngestionRoutes, registerIntelligenceRoutes, registerEventRoutes, registerDeliveryRoutes, registerDeliverySubscriptionRoutes, registerReportRoutes, registerKnowledgeRoutes, registerJobRoutes, registerOverviewRoutes),
			)
		} else {
			apiOptions = append(apiOptions, fx.Provide(httptransport.NewUnavailableAuthenticator))
		}
		options = append(options, apiOptions...)
	}
	if role.StartsWorker() {
		if usesDatabase {
			options = append(options,
				fx.Provide(
					sourceinfrastructure.NewConnectorRegistry,
					newKnowledgeVaultWriter,
					newKnowledgeProposalService,
					newKnowledgeReconciler,
					deliverypostgres.NewRepository,
					reportpostgres.NewRepository,
					newReportService,
					newCollectionService,
					newCandidateRecallService,
					newClusteringExecutionService,
					exposeCollectionTargetReader,
					exposeContentRepository,
					exposeRelevanceRepository,
					sourcejobs.NewCollectHandler,
					ingestionjobs.NewNormalizeHandler,
					ingestionjobs.NewEvaluateHandler,
					eventjobs.NewClusterHandler,
					eventjobs.NewHeatHandler,
					newSummaryHandler,
					newKnowledgeProjectHandler,
					newKnowledgeReconcileHandler,
					newReportBuildHandler,
					newDeliveryEmailService,
					newDeliverEmailHandler,
					newP0Handlers,
					newQueueWorker, exposeWorkerRunner, newQueueStore, exposeCollectionDueReader, newCollectionScheduler, exposeCollectionSchedulerRunner,
				),
				fx.Invoke(registerPersistentWorkerLifecycle, registerCollectionSchedulerLifecycle),
			)
			options = append(options, fx.Invoke(intelligenceapplication.RegisterRunLeaseReclaimerLifecycle))
		} else {
			options = append(options, fx.Invoke(registerWorkerLifecycle))
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

func registerMetricCapabilityRoutes(router *gin.Engine, service *sourceapplication.MetricCapabilityService, authenticator httptransport.Authenticator) {
	sourcetransport.RegisterMetricCapabilityRoutes(router, service, authenticator)
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

func registerEventRoutes(router *gin.Engine, read *eventapplication.ReadService, lifecycle *eventapplication.LifecycleService, governance *eventapplication.GovernanceService, heat *eventapplication.HeatService, claims *eventapplication.ClaimService, intelligence *eventapplication.EventIntelligenceReadService, summaries *eventapplication.EventSummaryService, extractions *eventapplication.EventClaimExtractionService, authenticator httptransport.Authenticator) {
	eventtransport.RegisterRoutesWithIntelligence(router, read, lifecycle, governance, heat, claims, intelligence, summaries, extractions, authenticator)
}

func registerDeliveryRoutes(router *gin.Engine, repository *deliverypostgres.Repository) {
	deliverytransport.RegisterRoutes(router, repository)
}

func registerDeliverySubscriptionRoutes(router *gin.Engine, service *deliveryapplication.SubscriptionService, authenticator httptransport.Authenticator) {
	deliverytransport.RegisterSubscriptionRoutes(router, service, authenticator)
}

func registerReportRoutes(router *gin.Engine, service *reportapplication.Service, authenticator httptransport.Authenticator) {
	reporttransport.RegisterRoutes(router, service, authenticator)
}

func registerKnowledgeRoutes(router *gin.Engine, handler *knowledgetransport.Handler, authenticator httptransport.Authenticator) {
	knowledgetransport.RegisterRoutes(router, handler, authenticator)
}

func registerJobRoutes(router *gin.Engine, service *operationsapplication.JobService, authenticator httptransport.Authenticator) {
	operationstransport.RegisterJobRoutes(router, service, authenticator)
}

func registerOverviewRoutes(router *gin.Engine, service *operationsapplication.OverviewService, authenticator httptransport.Authenticator) {
	operationstransport.RegisterOverviewRoutes(router, service, authenticator)
}

// Fx does not infer interface bindings from a concrete repository. Keep the
// adapters at the composition root so event application services remain
// decoupled from the PostgreSQL implementation.
func newEventReadService(repository *eventpostgres.Repository) *eventapplication.ReadService {
	return eventapplication.NewReadService(repository)
}

func newEventLifecycleService(repository *eventpostgres.Repository, heat *eventapplication.HeatService) *eventapplication.LifecycleService {
	return eventapplication.NewLifecycleService(repository, heat)
}

func newEventGovernanceService(repository *eventpostgres.Repository, heat *eventapplication.HeatService) *eventapplication.GovernanceService {
	return eventapplication.NewGovernanceService(repository, heat)
}

func newEventHeatService(repository *eventpostgres.Repository, capabilities *sourceapplication.MetricCapabilityService) (*eventapplication.HeatService, error) {
	return eventapplication.NewHeatService(eventapplication.HeatServiceDependencies{Snapshots: repository, Capabilities: capabilities})
}

func newEventContentMetricRefreshService(repository *eventpostgres.Repository, heat *eventapplication.HeatService) (*eventapplication.ContentMetricRefreshService, error) {
	return eventapplication.NewContentMetricRefreshService(repository, heat)
}

func newEventClaimService(repository *eventpostgres.Repository) *eventapplication.ClaimService {
	return eventapplication.NewClaimService(repository)
}

func newEventIntelligenceRunner(runs *intelligenceapplication.RunService) *intelligenceapplication.EventIntelligenceService {
	return intelligenceapplication.NewEventIntelligenceService(runs)
}

func newEventIntelligenceReadService(repository *eventpostgres.Repository) *eventapplication.EventIntelligenceReadService {
	return eventapplication.NewEventIntelligenceReadService(repository)
}

func newEventSummaryService(repository *eventpostgres.Repository, runner *intelligenceapplication.EventIntelligenceService) *eventapplication.EventSummaryService {
	return eventapplication.NewEventSummaryService(repository, runner)
}

func newEventClaimExtractionService(repository *eventpostgres.Repository, runner *intelligenceapplication.EventIntelligenceService) *eventapplication.EventClaimExtractionService {
	return eventapplication.NewEventClaimExtractionService(repository, runner, repository)
}

func newSourceService(runtime *database.Runtime, sources *sourcepostgres.Repository, usage *monitorpostgres.SourceUsageReader, references *monitorpostgres.PublishedReferenceReader, audit *operationspostgres.AuditWriter) (*sourceapplication.Service, error) {
	return sourceapplication.NewService(sourceapplication.Dependencies{Runtime: runtime, Sources: sources, MonitorUsage: usage, PublishedReferences: references, Audit: audit})
}

func newMetricCapabilityService(runtime *database.Runtime, profiles *sourcepostgres.MetricCapabilityRepository, sources *sourcepostgres.Repository, audit *operationspostgres.AuditWriter) (*sourceapplication.MetricCapabilityService, error) {
	return sourceapplication.NewMetricCapabilityService(sourceapplication.MetricCapabilityDependencies{Runtime: runtime, Profiles: profiles, SourceContexts: sources, Audit: audit})
}

func newCollectionControlService(runtime *database.Runtime, sources *sourcepostgres.Repository, runs *sourcepostgres.CollectionRepository, connectors *sourceinfrastructure.ConnectorRegistry, metrics *observability.Metrics) (*sourceapplication.CollectionControlService, error) {
	return sourceapplication.NewCollectionControlService(sourceapplication.CollectionControlDependencies{Runtime: runtime, Sources: sources, Runs: runs, Connectors: connectors, Metrics: metrics})
}

func newCollectionService(runtime *database.Runtime, sources *sourcepostgres.Repository, runs *sourcepostgres.CollectionRepository, connectors *sourceinfrastructure.ConnectorRegistry) (*sourceapplication.CollectionService, error) {
	return sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{Runtime: runtime, Sources: sources, Runs: runs, Connectors: connectors})
}

func newCandidateRecallService(candidates *ingestionpostgres.RelevanceCandidateReader) (*ingestionapplication.CandidateRecallService, error) {
	return ingestionapplication.NewCandidateRecallService(candidates, nil)
}

func newClusteringExecutionService(repository *eventpostgres.Repository) *eventapplication.ClusteringExecutionService {
	return eventapplication.NewClusteringExecutionService(eventapplication.NewRecallService(repository), eventapplication.NewClusteringService(), repository)
}

func exposeCollectionTargetReader(reader *monitorpostgres.PublishedCollectionTargetReader) sourcejobs.CollectionTargetReader {
	return reader
}

func exposeContentRepository(repository *ingestionpostgres.ContentRepository) ingestionjobs.ContentRepository {
	return repository
}

func exposeRelevanceRepository(repository *ingestionpostgres.RelevanceRepository) ingestionjobs.RelevanceRepository {
	return repository
}

func newSummaryHandler(service *eventapplication.EventSummaryService) (*intelligencejobs.SummaryHandler, error) {
	return intelligencejobs.NewSummaryHandler(func(ctx context.Context, eventID int64) error {
		_, err := service.Generate(ctx, eventID)
		return err
	})
}

func newP0Handlers(collect *sourcejobs.CollectHandler, normalize *ingestionjobs.NormalizeHandler, evaluate *ingestionjobs.EvaluateHandler, cluster *eventjobs.ClusterHandler, heat *eventjobs.HeatHandler, summary *intelligencejobs.SummaryHandler, projectKnowledge *projectKnowledgeHandler, reconcileKnowledge *reconcileKnowledgeHandler, buildReport *reportjobs.BuildHandler, deliverEmail *deliveryjobs.DeliverEmailHandler) map[string]queue.Handler {
	return map[string]queue.Handler{
		queue.KindCollectSource:        collect.Handle,
		queue.KindNormalizeContent:     normalize.Handle,
		queue.KindEvaluateRelevance:    evaluate.Handle,
		queue.KindClusterContent:       cluster.Handle,
		queue.KindRecomputeEventHeat:   heat.Handle,
		queue.KindGenerateEventSummary: summary.Handle,
		queue.KindProjectKnowledge:     projectKnowledge.Handle,
		queue.KindReconcileKnowledge:   reconcileKnowledge.Handle,
		queue.KindBuildReport:          buildReport.Handle,
		queue.KindDeliverEmail:         deliverEmail.Handle,
	}
}

func newIngestionEvidenceStore(cfg config.Config) (ingestiondomain.EvidenceStore, error) {
	if err := cfg.MinIO.ValidateRuntime(); err != nil {
		// Worker/API composition remains available when object storage is not
		// configured; the ingestion Job then reports an unavailable dependency
		// and is retried rather than preventing the process from booting.
		return unavailableEvidenceStore{}, nil
	}
	return ingestionminio.NewStore(cfg.MinIO)
}

func newIngestionCapturedItemReader(runs *sourcepostgres.CollectionRepository) (*sourceapplication.CapturedItemReader, error) {
	return sourceapplication.NewCapturedItemReader(sourceapplication.CapturedItemReaderDependencies{Runs: runs})
}

func newIngestionService(runtime *database.Runtime, captures *sourceapplication.CapturedItemReader, contents *ingestionpostgres.ContentRepository, evidence ingestiondomain.EvidenceStore, metrics *eventapplication.ContentMetricRefreshService) (*ingestionapplication.Service, error) {
	return ingestionapplication.NewService(ingestionapplication.Dependencies{
		Runtime: runtime, Captures: captures, Contents: contents, Evidence: evidence, MetricRefresh: metrics,
	})
}

type unavailableEvidenceStore struct{}

func (unavailableEvidenceStore) PutText(context.Context, ingestiondomain.EvidenceObject) (ingestiondomain.EvidenceReceipt, error) {
	return ingestiondomain.EvidenceReceipt{}, fmt.Errorf("evidence object store is unavailable")
}
func (unavailableEvidenceStore) Delete(context.Context, string) error {
	return fmt.Errorf("evidence object store is unavailable")
}
func (unavailableEvidenceStore) ListPrefix(context.Context, string) ([]ingestiondomain.EvidenceReceipt, error) {
	return nil, fmt.Errorf("evidence object store is unavailable")
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

func newReportService(repository *reportpostgres.Repository, events *eventapplication.ReadService, cfg config.Config) (*reportapplication.Service, error) {
	service, err := reportapplication.NewService(repository, events)
	if err != nil {
		return nil, err
	}
	service.SetPublisher(reportvault.NewPublisher(cfg.VaultPath))
	return service, nil
}

func newKnowledgeProposalService(repository *knowledgepostgres.Repository, cfg config.Config) *knowledgeapplication.ProposalService {
	if snapshots, err := knowledgeminio.NewStore(cfg.MinIO); err == nil {
		return knowledgeapplication.NewProposalService(repository, repository, snapshots)
	}
	// MinIO remains optional for local P0 operation; when configured, every
	// successful proposal application receives an immutable object snapshot.
	return knowledgeapplication.NewProposalService(repository, repository)
}

func newKnowledgeReconciler(repository *knowledgepostgres.Repository, writer *knowledgevault.Writer) *knowledgeapplication.Reconciler {
	return knowledgeapplication.NewReconciler(repository, writer)
}

func newKnowledgeVaultWriter(cfg config.Config) *knowledgevault.Writer {
	return knowledgevault.NewWriter(cfg.VaultPath)
}

func newKnowledgeHandler(proposals *knowledgeapplication.ProposalService, repository *knowledgepostgres.Repository, reconcile *knowledgeapplication.Reconciler, writer *knowledgevault.Writer) *knowledgetransport.Handler {
	return knowledgetransport.NewHandler(proposals, repository, reconcile, writer)
}

type projectKnowledgeHandler struct{ handler *knowledgejobs.Handler }

func (handler *projectKnowledgeHandler) Handle(ctx context.Context, job queue.Job) error {
	return handler.handler.Handle(ctx, job)
}

func newKnowledgeProjectHandler(proposals *knowledgeapplication.ProposalService, writer *knowledgevault.Writer) (*projectKnowledgeHandler, error) {
	handler, err := knowledgejobs.NewHandler(queue.KindProjectKnowledge, func(ctx context.Context, id int64) error {
		_, err := proposals.ApplyByID(ctx, id, writer)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &projectKnowledgeHandler{handler: handler}, nil
}

type reconcileKnowledgeHandler struct{ handler *knowledgejobs.Handler }

func (handler *reconcileKnowledgeHandler) Handle(ctx context.Context, job queue.Job) error {
	return handler.handler.Handle(ctx, job)
}

func newKnowledgeReconcileHandler(reconcile *knowledgeapplication.Reconciler) (*reconcileKnowledgeHandler, error) {
	handler, err := knowledgejobs.NewHandler(queue.KindReconcileKnowledge, func(ctx context.Context, _ int64) error {
		_, err := reconcile.Reconcile(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &reconcileKnowledgeHandler{handler: handler}, nil
}

func newReportBuildHandler(service *reportapplication.Service) (*reportjobs.BuildHandler, error) {
	return reportjobs.NewBuildHandler(func(ctx context.Context, id int64) error {
		_, err := service.BuildByID(ctx, id)
		return err
	})
}

type deliveryMailSender struct{ mailer *deliverysmtp.Mailer }

func (sender deliveryMailSender) Send(ctx context.Context, message deliveryapplication.MailMessage) error {
	return sender.mailer.Send(ctx, deliverysmtp.Message{To: message.To, Subject: message.Subject, HTML: message.HTML, Text: message.Text})
}

func newDeliveryEmailService(repository *deliverypostgres.Repository, cfg config.Config) (*deliveryapplication.EmailService, error) {
	return deliveryapplication.NewEmailService(repository, deliveryMailSender{mailer: deliverysmtp.NewMailer(cfg.Authentication.SMTP)}, nil)
}

func newDeliverEmailHandler(service *deliveryapplication.EmailService, repository *deliverypostgres.Repository) (*deliveryjobs.DeliverEmailHandler, error) {
	return deliveryjobs.NewDeliverEmailHandler(service, repository)
}

func newJobService(repository *operationspostgres.JobRepository, audit *operationspostgres.AuditWriter) (*operationsapplication.JobService, error) {
	return operationsapplication.NewJobService(repository, audit)
}

func newOperationsOverviewService(repository *operationspostgres.JobRepository) (*operationsapplication.OverviewService, error) {
	return operationsapplication.NewOverviewService(repository)
}

func newDeliverySubscriptionService(runtime *database.Runtime, repository *deliverypostgres.Repository, audit *operationspostgres.AuditWriter) (*deliveryapplication.SubscriptionService, error) {
	return deliveryapplication.NewSubscriptionService(deliveryapplication.SubscriptionDependencies{Runtime: runtime, Store: repository, Audit: audit})
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
