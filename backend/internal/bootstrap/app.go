package bootstrap

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	deliverysmtp "github.com/StephenQiu30/hotkey-server/backend/internal/modules/delivery/infrastructure/smtp"
	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	eventjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/infrastructure/jobs"
	eventpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/infrastructure/postgres"
	eventtransport "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/transport/http"
	identityapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/application"
	identitypostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/infrastructure/postgres"
	identityredis "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/infrastructure/redis"
	identitysecurity "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/infrastructure/security"
	identitysmtp "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/infrastructure/smtp"
	identitytransport "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/transport/http"
	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	ingestionjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/jobs"
	ingestionmarkdown "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/markdown"
	ingestionminio "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/minio"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	ingestiontransport "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/transport/http"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/infrastructure/postgres"
	intelligenceprovider "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/infrastructure/provider"
	intelligencetransport "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/transport/http"
	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	knowledgevault "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/vault"
	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
	monitortransport "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/transport/http"
	notificationapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	notificationjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/infrastructure/jobs"
	notificationpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/infrastructure/postgres"
	notificationtransport "github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/transport/http"
	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	operationstransport "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/transport/http"
	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourceinfrastructure "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure"
	sourcecredentialstore "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/credentialstore"
	sourcejobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/jobs"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	sourcenet "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/sourcenet"
	sourcetransport "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/transport/http"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/logging"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/observability"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedclock "github.com/StephenQiu30/hotkey-server/backend/internal/shared/clock"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
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
				newPaginationCodec,
				operationspostgres.NewAuditWriter,
				operationspostgres.NewJobRepository,
				operationspostgres.NewGovernanceRepository,
				operationspostgres.NewRetentionRepository,
				operationspostgres.NewDecisionQualityRepository,
				identitypostgres.NewUserRepository,
				newSourceRepository,
				newRightsManagementRepository,
				sourcepostgres.NewRightsManagementTransactionAdapter,
				newRightsActorAuthorizer,
				newRightsManagementAuditWriter,
				newRightsManagementService,
				newSourceCredentialStore,
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
				newContentRepository,
				ingestionpostgres.NewCitationRepository,
				ingestionpostgres.NewTextQuoteSelectorRepository,
				ingestionpostgres.NewContentFamilyRepository,
				ingestionpostgres.NewRelevanceRepository,
				ingestionpostgres.NewRelevanceCandidateReader,
				ingestionmarkdown.NewConverter,
				newIngestionRelevanceReviewService,
				newIngestionEvidenceStore,
				newCollectionRepository,
				newIngestionCapturedItemReader,
				newIngestionService,
				newEventContentMetricRefreshService,
				monitorpostgres.NewPublishedCollectionTargetReader,
				monitorpostgres.NewIntentRepository,
				newIntentService,
				newCompiledIntentEmbeddingProjectionService,
				newCompiledIntentEmbeddingProducerAdapter,
				newMonitorIntentCompiler,
				ingestionjobs.NewPublishedMonitorMatchBackfillScheduler,
				newIntentPublicationBackfillAdapter,
				newIntentPublicationService,
				monitorpostgres.NewCompiledRecallProfileReader,
				ingestionpostgres.NewHybridDocumentRecallReader,
				newHybridRecallService,
				ingestionjobs.NewAcceptedDocumentMatchProjectionScheduler,
				newDocumentMatchRepository,
				ingestionapplication.NewRankSignalDocumentMatchReranker,
				newPublishedDocumentMatchService,
				newRelevanceCalibrationService,
				ingestionpostgres.NewPublishedMatchTargetReader,
				eventpostgres.NewMicroEventRepository,
				exposeAcceptedMatchFamilyReader,
				newMicroEventService,
				eventpostgres.NewEventHeatRepository,
				newEventHeatV2Service,
				eventpostgres.NewMicroEventGovernancePostgresRepository,
				newMicroEventGovernanceService,
				eventpostgres.NewClaimEvidencePostgresRepository,
				newClaimEvidenceService,
				eventpostgres.NewEvidenceSummaryPostgresRepository,
				newEvidenceSummaryService,
				newMicroEventQueryRepository,
				newMicroEventQueryService,
				eventapplication.NewAcceptedMatchEventProjectionService,
				exposeAcceptedDocumentMatchEventProjector,
				eventjobs.NewAutomaticClaimEvidenceScheduler,
				exposeAutomaticClaimEvidenceScheduler,
				newAcceptedDocumentMatchEventProjectionAdapter,
				newAcceptedDocumentMatchProjectionHandler,
				newPublishedMatchEvaluationService,
				newPublishedMonitorMatchBackfillService,
				ingestionjobs.NewPublishedMonitorMatchBackfillHandler,
				newDocumentMatchReviewService,
				newDocumentMatchQueryService,
				newMonitorIntentPreviewEvaluator,
				newMonitorIntentAnalysisProcessor,
				exposeMonitorIntentAnalysisAvailability,
				newSourceConnectorRegistry,
				newKnowledgeVaultWriter,
				newKnowledgeProjectionService,
				newTextQuoteSelectorService,
				newAutomaticClaimEvidenceService,
				eventjobs.NewAutomaticClaimEvidenceHandler,
				newContentLineageFeedbackService,
				notificationpostgres.NewRepository,
				newNotificationService,
				newQueueStore,
				exposeCollectionTargetReader,
				sourcejobs.NewCollectionRetryActivator,
				sourcejobs.NewManualCollectionActivator,
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
					operationspostgres.NewRuntimeMetricsCollector,
					newIdentityVerificationStore,
					newIdentityService,
					newIdentityAuthenticator,
					monitorpostgres.NewSourceUsageReader,
					monitorpostgres.NewPublishedReferenceReader,
					monitorpostgres.NewMonitorScanReader,
					newSourceService,
					newCollectionControlService,
					newInstantSearchService,
					newMonitorRepository,
					newMonitorService,
					newIntentControlService,
					newIngestionContentQueryService,
					newIngestionCitationService,
					newIngestionRelevanceAPIService,
					newOperationsOverviewService,
					newGovernanceService,
					newJobService,
					newNotificationHandler,
				),
				fx.Invoke(registerRuntimeMetricsCollector, registerIdentityVerificationStoreLifecycle, registerIdentityRoutes, registerSourceRoutes, registerRightsManagementRoutes, registerBilibiliWebhookRoutes, registerMetricCapabilityRoutes, registerCollectionRoutes, registerInstantSearchRoutes, registerMonitorRoutes, registerMonitorIntentRoutes, registerIngestionRoutes, registerIntelligenceRoutes, registerMicroEventRoutes, registerJobRoutes, registerOverviewRoutes, registerGovernanceRoutes, registerNotificationRoutes),
			)
		} else {
			apiOptions = append(apiOptions, fx.Provide(httptransport.NewUnavailableAuthenticator))
		}
		options = append(options, apiOptions...)
	}
	if role.StartsWorker() {
		if usesDatabase {
			if cfg.MinIO.ValidateRuntime() == nil {
				options = append(options, fx.Provide(
					sourcejobs.NewSourceDocumentGenerationScheduler,
					ingestionjobs.NewPublishedDocumentMatchEvaluationScheduler,
					newEvidenceSnapshotRepository,
					sourcepostgres.NewRightsDecisionReader,
					sourcepostgres.NewEvidenceSelectionManifestReader,
					newSourceEvidenceSelectorVerifier,
					newSourceRawEvidenceStore,
					newSourceRawEvidenceObjectReader,
					newRawEvidenceArchiveService,
					newRawEvidenceCollectionService,
					newEvidenceSelectionService,
					provideSourceEvidenceReaderAdapter,
					ingestionpostgres.NewDocumentVersionRepository,
					newDocumentObservationPersistenceService,
					ingestionpostgres.NewDerivedArtifactRepository,
					ingestionpostgres.NewDocumentProjectionAuthorizationReader,
					ingestionpostgres.NewDocumentRecallProjectionWriter,
					newFeedBodyExtractor,
					newPlatformBodyExtractor,
					newSelectedSourceBodyExtractor,
					newDocumentStructureExtractor,
					newDerivedDocumentProjectionService,
					newDocumentRecallProjectionService,
					newContentFamilyService,
					newDocumentEmbeddingProducerAdapter,
					newSourceDocumentGenerationService,
					ingestionjobs.NewSourceDocumentGenerationHandler,
					newXMetricRefreshService,
					sourcejobs.NewXMetricRefreshHandler,
					newXMetricRefreshScheduler,
					exposeXMetricRefreshSchedulerRunner,
				))
				options = append(options, fx.Invoke(registerXMetricRefreshSchedulerLifecycle))
			}
			options = append(options,
				fx.Provide(
					newCollectionService,
					sourcejobs.NewCollectHandler,
					ingestionjobs.NewNormalizeHandler,
					newNotificationEmailDeliveryService,
					notificationjobs.NewEmailDispatcher,
					newMonitorIntentAnalysisHandler,
					ingestionjobs.NewPublishedDocumentMatchEvaluationHandler,
					newP0Handlers,
					newQueueWorker, exposeWorkerRunner, exposeCollectionDueReader, newCollectionScheduler, exposeCollectionSchedulerRunner,
				),
				fx.Invoke(registerPersistentWorkerLifecycle, registerCollectionSchedulerLifecycle, registerNotificationEmailDispatcherLifecycle),
			)
			options = append(options, fx.Invoke(intelligenceapplication.RegisterRunLeaseReclaimerLifecycle))
		} else {
			options = append(options, fx.Invoke(registerWorkerLifecycle))
		}
	}
	options = append(options, extra...)

	return fx.New(options...), nil
}

func registerRuntimeMetricsCollector(metrics *observability.Metrics, collector *operationspostgres.RuntimeMetricsCollector) error {
	return metrics.RegisterCollector(collector)
}

func newSourceCredentialStore(runtime *database.Runtime, cfg config.Config) (*sourcecredentialstore.Store, error) {
	return sourcecredentialstore.NewStore(runtime, cfg.SourceCredentialMasterKey)
}

func newSourceConnectorRegistry(cfg config.Config, runtime *database.Runtime, credentials *sourcecredentialstore.Store) (*sourceinfrastructure.ConnectorRegistry, error) {
	resolver, err := sourcenet.NewResolver(cfg.SourceDNSOverHTTPSURL)
	if err != nil {
		return nil, fmt.Errorf("configure source DNS resolver: %w", err)
	}
	return sourceinfrastructure.NewConnectorRegistry(resolver, credentials, sourcepostgres.NewExternalRequestBudget(runtime)), nil
}

func newAIProviderRegistry(cfg config.Config, logger *zap.Logger) *intelligenceapplication.ProviderRegistry {
	providers := make(map[intelligencedomain.ProviderName]intelligencedomain.Provider, 4)
	if provider, err := intelligenceprovider.NewOpenAIProvider(cfg.AI); err == nil {
		providers[intelligencedomain.ProviderOpenAI] = provider
	}
	if provider, err := intelligenceprovider.NewONNXProvider(cfg.AI); err == nil {
		providers[intelligencedomain.ProviderONNX] = provider
	}
	if provider, err := intelligenceprovider.NewDeepSeekProvider(cfg.AI); err == nil {
		providers[intelligencedomain.ProviderDeepSeek] = provider
	} else if strings.TrimSpace(cfg.AI.DeepSeekAPIKey) != "" {
		logger.Warn("AI provider configuration rejected", zap.String("provider", string(intelligencedomain.ProviderDeepSeek)))
	}
	if provider, err := intelligenceprovider.NewOllamaProvider(cfg.AI); err == nil {
		providers[intelligencedomain.ProviderOllama] = provider
	} else if cfg.AI.OllamaEnabled {
		logger.Warn("AI provider configuration rejected", zap.String("provider", string(intelligencedomain.ProviderOllama)))
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

func registerRightsManagementRoutes(router *gin.Engine, service *sourceapplication.RightsManagementService, authenticator httptransport.Authenticator) {
	sourcetransport.RegisterRightsManagementRoutes(router, service, authenticator)
}

func registerBilibiliWebhookRoutes(router *gin.Engine, service *sourceapplication.Service, cfg config.Config) {
	sourcetransport.RegisterBilibiliWebhookRoutes(router, service, cfg.BilibiliWebhookSecret)
}

func registerMetricCapabilityRoutes(router *gin.Engine, service *sourceapplication.MetricCapabilityService, authenticator httptransport.Authenticator) {
	sourcetransport.RegisterMetricCapabilityRoutes(router, service, authenticator)
}

func registerCollectionRoutes(router *gin.Engine, service *sourceapplication.CollectionControlService, authenticator httptransport.Authenticator) {
	sourcetransport.RegisterCollectionRoutes(router, service, authenticator)
}

func registerInstantSearchRoutes(router *gin.Engine, service *sourceapplication.InstantSearchService, authenticator httptransport.Authenticator) {
	sourcetransport.RegisterInstantSearchRoutes(router, service, authenticator)
}

func registerMonitorRoutes(router *gin.Engine, service *monitorapplication.Service, authenticator httptransport.Authenticator) {
	monitortransport.RegisterRoutes(router, service, authenticator)
}

func registerMonitorIntentRoutes(router *gin.Engine, service *monitorapplication.IntentControlService, authenticator httptransport.Authenticator) {
	monitortransport.RegisterIntentRoutes(router, service, authenticator)
}

func registerIngestionRoutes(
	router *gin.Engine,
	service *ingestionapplication.ContentQueryService,
	citations *ingestionapplication.CitationService,
	relevance *ingestionapplication.RelevanceAPIService,
	documentMatches *ingestionapplication.DocumentMatchQueryService,
	documentMatchReviews *ingestionapplication.DocumentMatchReviewService,
	textQuoteSelectors *ingestionapplication.TextQuoteSelectorService,
	contentLineageFeedback *ingestionapplication.ContentLineageFeedbackService,
	authenticator httptransport.Authenticator,
	metrics *observability.Metrics,
	cursorCodec *pagination.Codec,
) {
	ingestiontransport.RegisterRoutes(router, service, authenticator, metrics)
	ingestiontransport.RegisterCitationRoutes(router, citations, authenticator)
	ingestiontransport.RegisterRelevanceRoutesWithCursorCodec(router, relevance, authenticator, cursorCodec)
	ingestiontransport.RegisterDocumentMatchRoutes(router, documentMatches, documentMatchReviews, authenticator)
	ingestiontransport.RegisterTextQuoteSelectorRoutes(router, textQuoteSelectors, authenticator)
	ingestiontransport.RegisterContentLineageFeedbackRoutes(router, contentLineageFeedback, authenticator)
}

func registerIntelligenceRoutes(router *gin.Engine, service *intelligenceapplication.ModelProfileService, authenticator httptransport.Authenticator) {
	intelligencetransport.RegisterRoutes(router, service, authenticator)
}

func registerMicroEventRoutes(router *gin.Engine, queries *eventapplication.MicroEventQueryService, governance *eventapplication.MicroEventGovernanceService, evidence *eventapplication.ClaimEvidenceService, authenticator httptransport.Authenticator) {
	eventtransport.RegisterMicroEventRoutes(router, queries, governance, evidence, authenticator)
}

func registerJobRoutes(router *gin.Engine, service *operationsapplication.JobService, authenticator httptransport.Authenticator) {
	operationstransport.RegisterJobRoutes(router, service, authenticator)
}

func registerOverviewRoutes(router *gin.Engine, service *operationsapplication.OverviewService, authenticator httptransport.Authenticator) {
	operationstransport.RegisterOverviewRoutes(router, service, authenticator)
}

func registerGovernanceRoutes(router *gin.Engine, service *operationsapplication.GovernanceService, authenticator httptransport.Authenticator) {
	operationstransport.RegisterGovernanceRoutes(router, service, authenticator)
}

func newNotificationHandler(service *notificationapplication.Service, cfg config.Config) (*notificationtransport.Handler, error) {
	return notificationtransport.NewHandler(service, notificationtransport.StreamConfig{
		PollInterval: cfg.Notification.PollInterval, HeartbeatInterval: cfg.Notification.HeartbeatInterval,
		MaxConnections: cfg.Notification.MaxConnections, AllowedOrigins: cfg.Authentication.AllowedOrigins,
	})
}

func newNotificationService(repository *notificationpostgres.Repository) (*notificationapplication.Service, error) {
	return notificationapplication.NewService(repository)
}

type notificationEmailSender struct{ mailer *deliverysmtp.Mailer }

func (sender notificationEmailSender) SendNotificationEmail(ctx context.Context, message notificationapplication.NotificationEmailMessageDTO) (string, error) {
	err := sender.mailer.Send(ctx, deliverysmtp.Message{
		To: message.Recipient, Subject: message.Subject, Text: message.Text, HTML: message.HTML,
	})
	return "", err
}

func newNotificationEmailDeliveryService(repository *notificationpostgres.Repository, cfg config.Config) (*notificationapplication.EmailDeliveryService, error) {
	return notificationapplication.NewEmailDeliveryService(notificationapplication.EmailDeliveryServiceDependencies{
		Repository: repository, Sender: notificationEmailSender{mailer: deliverysmtp.NewMailer(cfg.Authentication.SMTP)},
		WebOrigin: cfg.Notification.WebOrigin,
	})
}

func registerNotificationRoutes(router *gin.Engine, handler *notificationtransport.Handler, authenticator httptransport.Authenticator) {
	notificationtransport.RegisterRoutes(router, handler, authenticator)
}

func newEventContentMetricRefreshService(repository *eventpostgres.EventHeatRepository, heat *eventapplication.EventHeatService) (*eventapplication.ContentMetricRefreshService, error) {
	return eventapplication.NewContentMetricRefreshService(repository, heat)
}

func newSourceService(runtime *database.Runtime, sources *sourcepostgres.Repository, usage *monitorpostgres.SourceUsageReader, references *monitorpostgres.PublishedReferenceReader, credentials *sourcecredentialstore.Store, audit *operationspostgres.AuditWriter) (*sourceapplication.Service, error) {
	return sourceapplication.NewService(sourceapplication.Dependencies{Runtime: runtime, Sources: sources, MonitorUsage: usage, PublishedReferences: references, Credentials: credentials, Audit: audit})
}

func newMetricCapabilityService(runtime *database.Runtime, profiles *sourcepostgres.MetricCapabilityRepository, sources *sourcepostgres.Repository, audit *operationspostgres.AuditWriter) (*sourceapplication.MetricCapabilityService, error) {
	return sourceapplication.NewMetricCapabilityService(sourceapplication.MetricCapabilityDependencies{Runtime: runtime, Profiles: profiles, SourceContexts: sources, Audit: audit})
}

func newCollectionControlService(runtime *database.Runtime, sources *sourcepostgres.Repository, runs *sourcepostgres.CollectionRepository, connectors *sourceinfrastructure.ConnectorRegistry, retries *sourcejobs.CollectionRetryActivator, manuals *sourcejobs.ManualCollectionActivator, targets *monitorpostgres.PublishedCollectionTargetReader, scans *monitorpostgres.MonitorScanReader, quota *operationspostgres.GovernanceRepository, metrics *observability.Metrics) (*sourceapplication.CollectionControlService, error) {
	return sourceapplication.NewCollectionControlService(sourceapplication.CollectionControlDependencies{
		Runtime: runtime, Sources: sources, Runs: runs, Connectors: connectors, Retries: retries,
		Manuals: manuals, Targets: targets, Scans: scans, Metrics: metrics, Quota: quota,
	})
}

func newInstantSearchService(sources *sourcepostgres.Repository, connectors *sourceinfrastructure.ConnectorRegistry) (*sourceapplication.InstantSearchService, error) {
	return sourceapplication.NewInstantSearchService(sourceapplication.InstantSearchDependencies{Sources: sources, Connectors: connectors})
}

type collectionServiceParams struct {
	fx.In

	Runtime    *database.Runtime
	Sources    *sourcepostgres.Repository
	Runs       *sourcepostgres.CollectionRepository
	Connectors *sourceinfrastructure.ConnectorRegistry
	Evidence   *sourceapplication.RawEvidenceCollectionService `optional:"true"`
	Logger     *zap.Logger
}

func newCollectionService(params collectionServiceParams) (*sourceapplication.CollectionService, error) {
	return sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
		Runtime: params.Runtime, Sources: params.Sources, Runs: params.Runs, Connectors: params.Connectors,
		Evidence: params.Evidence, Logger: params.Logger,
	})
}

func exposeCollectionTargetReader(reader *monitorpostgres.PublishedCollectionTargetReader) sourcejobs.CollectionTargetReader {
	return reader
}

type p0HandlerParams struct {
	fx.In

	Collect                          *sourcejobs.CollectHandler
	Normalize                        *ingestionjobs.NormalizeHandler
	AnalyzeMonitorIntent             *monitorIntentAnalysisHandler
	GenerateSourceDocument           *ingestionjobs.SourceDocumentGenerationHandler         `optional:"true"`
	EvaluatePublishedDocumentMatches *ingestionjobs.PublishedDocumentMatchEvaluationHandler `optional:"true"`
	BackfillPublishedMonitorMatches  *ingestionjobs.PublishedMonitorMatchBackfillHandler    `optional:"true"`
	ProjectAcceptedDocumentMatch     *ingestionjobs.AcceptedDocumentMatchProjectionHandler  `optional:"true"`
	ExtractAutomaticClaimEvidence    *eventjobs.AutomaticClaimEvidenceHandler               `optional:"true"`
	RefreshXMetrics                  *sourcejobs.XMetricRefreshHandler                      `optional:"true"`
}

func newP0Handlers(params p0HandlerParams) map[string]queue.Handler {
	handlers := map[string]queue.Handler{
		queue.KindCollectSource:        params.Collect.Handle,
		queue.KindNormalizeContent:     params.Normalize.Handle,
		queue.KindAnalyzeMonitorIntent: params.AnalyzeMonitorIntent.Handle,
	}
	if params.GenerateSourceDocument != nil {
		handlers[queue.KindGenerateSourceDocument] = params.GenerateSourceDocument.Handle
	}
	if params.EvaluatePublishedDocumentMatches != nil {
		handlers[queue.KindEvaluatePublishedDocumentMatches] = params.EvaluatePublishedDocumentMatches.Handle
	}
	if params.BackfillPublishedMonitorMatches != nil {
		handlers[queue.KindBackfillPublishedMonitorMatches] = params.BackfillPublishedMonitorMatches.Handle
	}
	if params.ProjectAcceptedDocumentMatch != nil {
		handlers[queue.KindProjectAcceptedDocumentMatch] = params.ProjectAcceptedDocumentMatch.Handle
	}
	if params.ExtractAutomaticClaimEvidence != nil {
		handlers[queue.KindExtractAutomaticClaimEvidence] = params.ExtractAutomaticClaimEvidence.Handle
	}
	if params.RefreshXMetrics != nil {
		handlers[queue.KindRefreshXMetrics] = params.RefreshXMetrics.Handle
	}
	return handlers
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

func newIngestionService(runtime *database.Runtime, captures *sourceapplication.CapturedItemReader, contents *ingestionpostgres.ContentRepository, evidence ingestiondomain.EvidenceStore, markdown *ingestionmarkdown.Converter, metrics *eventapplication.ContentMetricRefreshService) (*ingestionapplication.Service, error) {
	return ingestionapplication.NewService(ingestionapplication.Dependencies{
		Runtime: runtime, Captures: captures, Contents: contents, Evidence: evidence, Markdown: markdown, MetricRefresh: metrics,
	})
}

func newXMetricRefreshService(
	sources *sourcepostgres.Repository,
	connectors *sourceinfrastructure.ConnectorRegistry,
	candidates *ingestionpostgres.ContentRepository,
	metrics *ingestionapplication.Service,
	evidence *sourceapplication.RawEvidenceCollectionService,
) (*sourceapplication.XMetricRefreshService, error) {
	return sourceapplication.NewXMetricRefreshService(sourceapplication.XMetricRefreshDependencies{
		Sources: sources, Connectors: connectors, Candidates: candidates, Metrics: metrics, Evidence: evidence,
	})
}

type unavailableEvidenceStore struct{}

func (unavailableEvidenceStore) PutText(context.Context, ingestiondomain.EvidenceObject) (ingestiondomain.EvidenceReceipt, error) {
	return ingestiondomain.EvidenceReceipt{}, fmt.Errorf("evidence object store is unavailable")
}
func (unavailableEvidenceStore) ReadText(context.Context, string, int64) (ingestiondomain.EvidenceText, error) {
	return ingestiondomain.EvidenceText{}, fmt.Errorf("evidence object store is unavailable")
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

func newIngestionContentQueryService(contents *ingestionpostgres.ContentRepository, sources *sourceapplication.Service, evidence ingestiondomain.EvidenceStore, lifecycle *ingestionapplication.Service) (*ingestionapplication.ContentQueryService, error) {
	return ingestionapplication.NewContentQueryService(ingestionapplication.ContentQueryDependencies{Contents: contents, Sources: sources, Evidence: evidence, Lifecycle: lifecycle})
}

func newIngestionCitationService(citations *ingestionpostgres.CitationRepository, projections *knowledgeapplication.ProjectionService) (*ingestionapplication.CitationService, error) {
	return ingestionapplication.NewCitationService(ingestionapplication.CitationDependencies{Citations: citations, Projections: projections})
}

func newIngestionRelevanceAPIService(snapshots *ingestionpostgres.RelevanceRepository, contents *ingestionpostgres.ContentRepository, candidates *ingestionpostgres.RelevanceCandidateReader) (*ingestionapplication.RelevanceAPIService, error) {
	return ingestionapplication.NewRelevanceAPIService(ingestionapplication.RelevanceAPIServiceDependencies{Snapshots: snapshots, Contents: contents, Candidates: candidates})
}

func newMonitorService(runtime *database.Runtime, monitors *monitorpostgres.Repository, sources *sourceapplication.Service, audit *operationspostgres.AuditWriter, quota *operationspostgres.GovernanceRepository, publication *monitorapplication.IntentPublicationService) (*monitorapplication.Service, error) {
	return monitorapplication.NewService(monitorapplication.Dependencies{
		Runtime: runtime, Monitors: monitors, Sources: sources, Audit: audit, Quota: quota,
		IntentPublication: publication,
	})
}

func newIntentService(repository *monitorpostgres.IntentRepository) (*monitorapplication.IntentService, error) {
	return monitorapplication.NewIntentService(monitorapplication.IntentServiceDependencies{
		Drafts: repository, Runs: repository, Clock: sharedclock.System{},
	})
}

func newIntentControlService(intents *monitorapplication.IntentService, repository *monitorpostgres.IntentRepository, analysis monitorapplication.IntentAnalysisAvailability) (*monitorapplication.IntentControlService, error) {
	return monitorapplication.NewIntentControlService(monitorapplication.IntentControlDependencies{
		Intents: intents, CurrentDrafts: repository, RunStatuses: repository, Analysis: analysis, Authorizer: repository,
	})
}

func newKnowledgeVaultWriter(cfg config.Config) *knowledgevault.Writer {
	return knowledgevault.NewWriter(cfg.VaultPath)
}

func newKnowledgeProjectionService(writer *knowledgevault.Writer) (*knowledgeapplication.ProjectionService, error) {
	return knowledgeapplication.NewProjectionService(writer)
}

func newJobService(repository *operationspostgres.JobRepository, audit *operationspostgres.AuditWriter) (*operationsapplication.JobService, error) {
	return operationsapplication.NewJobService(repository, audit)
}

func newOperationsOverviewService(repository *operationspostgres.JobRepository) (*operationsapplication.OverviewService, error) {
	return operationsapplication.NewOverviewService(repository)
}

func newGovernanceService(runtime *database.Runtime, store *operationspostgres.GovernanceRepository, retention *operationspostgres.RetentionRepository, audit *operationspostgres.AuditWriter) (*operationsapplication.GovernanceService, error) {
	return operationsapplication.NewGovernanceService(operationsapplication.GovernanceDependencies{Runtime: runtime, Store: store, Retention: retention, Audit: audit})
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
	if len(args) > 0 && args[0] == "relevance" {
		return runRelevanceCommand(ctx, cfg, args[1:], os.Stdout)
	}
	if len(args) > 0 && args[0] == "quality" {
		return runDecisionQualityCommand(ctx, cfg, args[1:], os.Stdout)
	}
	if len(args) > 0 && args[0] == "maintenance" {
		return runMaintenanceCommand(ctx, cfg, args[1:], os.Stdout)
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
