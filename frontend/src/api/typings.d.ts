declare namespace HotKeyAPI {
  type cancelCollectionJobParams = {
    job_id: string;
  };

  type CollectionJobInput = {
    /** Operation Id */
    operation_id: string;
    kind: CollectionJobKind;
    observation: CollectionJobObservationInput;
    /** Scheduled For At */
    scheduled_for_at?: string | null;
    /** Scope */
    scope: Record<string, any>;
  };

  type CollectionJobKind = "monitor.collect";

  type CollectionJobObservationInput = {
    /** Configuration Ref */
    configuration_ref: string;
    /** Configuration Version */
    configuration_version: number;
    /** Source Key */
    source_key?: string | null;
    source_capability?: SourceCapability | null;
  };

  type ErrorView = {
    /** Code */
    code: string;
    /** Message */
    message: string;
    /** Request Id */
    request_id: string;
    /** Details */
    details?: ValidationErrorItem[] | null;
  };

  type getCollectionJobParams = {
    job_id: string;
  };

  type HealthView = {
    /** Status */
    status: "ok" | "ready";
  };

  type IdentityCredentialsInput = {
    /** Username */
    username: string;
    /** Password */
    password: string;
  };

  type IdentitySessionView = {
    user: IdentityUserView;
    /** Expires At */
    expires_at: string;
  };

  type IdentityUserView = {
    /** Id */
    id: string;
    /** Username */
    username: string;
  };

  type IdentityWorkspaceView = {
    owner: IdentityUserView;
  };

  type JobAcceptanceStatus = "queued";

  type JobAcceptedView = {
    /** Job Id */
    job_id: string;
    status: JobAcceptanceStatus;
  };

  type JobCancellationView = {
    /** Requested At */
    requested_at: string;
    /** Deadline At */
    deadline_at: string | null;
    /** Timed Out */
    timed_out: boolean;
  };

  type JobControlStatus =
    | "queued"
    | "running"
    | "cancelling"
    | "succeeded"
    | "partially_succeeded"
    | "failed"
    | "cancelled";

  type JobObservationContext = {
    /** Configuration Ref */
    configuration_ref: string;
    /** Configuration Version */
    configuration_version: number;
    /** Source Key */
    source_key?: string | null;
    source_capability?: SourceCapability | null;
  };

  type JobProgressView = {
    stage: JobStage | null;
    /** Requests Sent */
    requests_sent: number;
    /** Items Saved */
    items_saved: number;
    /** Updated At */
    updated_at: string | null;
  };

  type JobScopeValue = Record<string, any>;

  type JobStage = "request" | "parse" | "save" | "analysis";

  type JobStatusView = {
    /** Id */
    id: string;
    /** Operation Id */
    operation_id: string;
    /** Kind */
    kind: string;
    observation: JobObservationContext;
    status: JobControlStatus;
    progress: JobProgressView;
    cancellation: JobCancellationView | null;
    /** Scheduled For At */
    scheduled_for_at: string | null;
    /** Started At */
    started_at: string | null;
    /** Completed At */
    completed_at: string | null;
    /** Created At */
    created_at: string;
  };

  type SourceCapability = "search" | "author_posts" | "comments" | "replies";

  type ValidationErrorItem = {
    /** Location */
    location: (string | number)[];
    /** Message */
    message: string;
    /** Type */
    type: string;
  };
}
