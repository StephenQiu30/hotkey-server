package enum

// UserStatus defines account lifecycle states.
type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusInactive UserStatus = "inactive"
	UserStatusBanned   UserStatus = "banned"
)

// UserPlanType defines the subscription plan for a user.
type UserPlanType string

const (
	UserPlanFree UserPlanType = "free"
	UserPlanPro  UserPlanType = "pro"
)

// TopicStatus defines topic lifecycle states.
type TopicStatus string

const (
	TopicStatusActive TopicStatus = "active"
	TopicStatusMerged TopicStatus = "merged"
	TopicStatusClosed TopicStatus = "closed"
)

// EventStatus defines hot event lifecycle states.
type EventStatus string

const (
	EventStatusActive  EventStatus = "active"
	EventStatusResolved EventStatus = "resolved"
	EventStatusArchived EventStatus = "archived"
)

// MaterialStatus defines annotation review states for topic annotations.
type MaterialStatus string

const (
	MaterialStatusUnreviewed MaterialStatus = "unreviewed"
	MaterialStatusReviewed   MaterialStatus = "reviewed"
	MaterialStatusApproved   MaterialStatus = "approved"
)

// RelationshipType defines how topics relate to events.
type RelationshipType string

const (
	RelationshipTypeMember RelationshipType = "member"
	RelationshipTypeParent RelationshipType = "parent"
	RelationshipTypeRelated RelationshipType = "related"
)

// ObjectType defines the type of object in knowledge_writeback_logs.
type ObjectType string

const (
	ObjectTypeTopic   ObjectType = "topic"
	ObjectTypeEvent   ObjectType = "event"
	ObjectTypeReport  ObjectType = "report"
)

// SourceKind defines the provenance of a theme membership.
type SourceKind string

const (
	SourceKindEvent SourceKind = "event"
	SourceKindTopic SourceKind = "topic"
)

// BundleKind defines the kind of an export bundle.
type BundleKind string

const (
	BundleKindDailyDigest  BundleKind = "daily-digest"
	BundleKindHourlyDigest BundleKind = "hourly-digest"
)