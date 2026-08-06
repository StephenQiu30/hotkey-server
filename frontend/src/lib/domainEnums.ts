export enum UserRole {
  Admin = "admin",
  Editor = "editor",
  Viewer = "viewer",
}

export enum AuthStatus {
  Initializing = "initializing",
  Authenticated = "authenticated",
  Unauthenticated = "unauthenticated",
}

export enum SourceType {
  RSS = "rss",
  HackerNews = "hacker_news",
}

export enum SourceHealthStatus {
  Healthy = "healthy",
  Degraded = "degraded",
  Unavailable = "unavailable",
  Unknown = "unknown",
}

export enum SourceAction {
  Health = "health",
  Toggle = "toggle",
}

export enum MonitorStatus {
  Draft = "draft",
  Active = "active",
  Paused = "paused",
  Archived = "archived",
}

export enum CollectionRunStatus {
  Queued = "queued",
  Running = "running",
  Succeeded = "succeeded",
  Failed = "failed",
  Cancelled = "cancelled",
}

export enum MonitorAction {
  Publish = "publish",
  Pause = "pause",
  Resume = "resume",
  Archive = "archive",
  Restore = "restore",
}

export enum MonitorRegion {
  China = "CN",
  UnitedStates = "US",
  Global = "global",
}

export enum ReportStatus {
  Draft = "draft",
  Published = "published",
  Failed = "failed",
  Archived = "archived",
}

export enum ReportAction {
  Select = "select",
  Build = "build",
  Preview = "preview",
  Publish = "publish",
}

export enum ReportType {
  Daily = "daily",
  Weekly = "weekly",
}

export enum DeliveryChannel {
  Email = "email",
  RSS = "rss",
}

export enum VerificationFlow {
  Registration = "register",
  PasswordReset = "reset_password",
}

export enum VerificationPurpose {
  Registration = "registration",
  PasswordReset = "password_reset",
}

export enum VerificationStep {
  Send = "send",
  Confirm = "confirm",
}

export enum RegistrationStep {
  Email = "email",
  Profile = "profile",
}

export enum WorkspaceTab {
  Signal = "signal",
  Evidence = "evidence",
  Report = "report",
}

export enum EventAction {
  Summary = "summary",
  Extract = "extract",
}

export enum SourceHealthDiagnostic {
  InvalidSourceConnection = "invalid_source_connection",
  RequestFailed = "request_failed",
  UpstreamStatus = "upstream_status",
  ConnectorUnavailable = "connector_unavailable",
  DestinationNotPermitted = "destination_not_permitted",
}

export enum APIErrorCode {
  InvalidRequest = 10000,
  ValidationFailed = 10001,
  VersionConflict = 10002,
  NotFound = 10003,
  RateLimited = 10004,
  Unauthorized = 20000,
  Forbidden = 20001,
  InvalidCredentials = 20002,
  SessionExpired = 20003,
  InvalidVerification = 20004,
  LastAdminRequired = 20005,
  InvalidMonitorState = 30000,
  MonitorVersionConflict = 30001,
  InvalidMonitorConfig = 30002,
  MonitorDraftMissing = 30003,
  MonitorNameConflict = 30004,
  InvalidSourceConfig = 40000,
  SourceConnectionMissing = 40001,
  UnsupportedSourceType = 40002,
  SourceUnavailable = 40003,
  CollectionJobNotFound = 40004,
  CollectionJobConflict = 40005,
  InvalidCollectionRequest = 40006,
  InvalidAIModelConfig = 70000,
  AIModelUnavailable = 70001,
  AIQuotaExhausted = 70002,
  AIRateLimited = 70003,
  AIServiceError = 70004,
  AIServiceTimeout = 70005,
  AIInvalidResponse = 70006,
  AIJobRunning = 70007,
  AIInvalidVector = 70008,
  AILeaseExpired = 70009,
  InternalServerError = 90000,
  ServiceUnavailable = 90001,
  UpstreamServiceError = 90002,
  RequestTimeout = 90003,
}
