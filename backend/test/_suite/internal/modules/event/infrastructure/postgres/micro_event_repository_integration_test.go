//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestMicroEventRepositoryPersistsCreateJoinReviewAndStableReplay(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	repository, err := NewMicroEventRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}

	first := seedMicroEventAssignmentFixture(t, runtime, "first", "accepted")
	create := microEventCommitFixture(first, "create", 0, 0, strings.Repeat("1", 64), "micro-event-create")
	created, err := repository.CommitMicroEventMembership(ctx, create)
	if err != nil {
		t.Fatalf("create membership: %v", err)
	}
	if created.Event.ID <= 0 || created.Event.Version != 1 || created.Decision.Action != "create" {
		t.Fatalf("created result = %#v", created)
	}
	replayed, err := repository.CommitMicroEventMembership(ctx, create)
	if err != nil || replayed.Decision.ID != created.Decision.ID || replayed.Event.Version != 1 {
		t.Fatalf("replay = %#v / %v", replayed, err)
	}
	conflict := create
	conflict.CommandFingerprint = strings.Repeat("2", 64)
	if _, err := repository.CommitMicroEventMembership(ctx, conflict); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("conflicting replay error = %v", err)
	}

	second := seedMicroEventAssignmentFixture(t, runtime, "second", "accepted")
	joined, err := repository.CommitMicroEventMembership(ctx, microEventCommitFixture(
		second, "join", created.Event.ID, created.Event.Version, strings.Repeat("3", 64), "micro-event-join",
	))
	if err != nil {
		t.Fatalf("join membership: %v", err)
	}
	if joined.Event.ID != created.Event.ID || joined.Event.Version != 2 || joined.Decision.Action != "join" {
		t.Fatalf("joined result = %#v", joined)
	}

	third := seedMicroEventAssignmentFixture(t, runtime, "third", "accepted")
	reviewed, err := repository.CommitMicroEventMembership(ctx, microEventCommitFixture(
		third, "review", joined.Event.ID, joined.Event.Version, strings.Repeat("4", 64), "micro-event-review",
	))
	if err != nil {
		t.Fatalf("review membership: %v", err)
	}
	if reviewed.Event.Version != 3 || reviewed.Event.Status != "review_pending" || reviewed.Decision.Action != "review" {
		t.Fatalf("reviewed result = %#v", reviewed)
	}

	var decisions, members, outboxEvents int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM micro_event_membership_decisions`).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM micro_event_members`).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM notification_outbox_events`).Scan(&outboxEvents); err != nil {
		t.Fatal(err)
	}
	if decisions != 3 || members != 2 || outboxEvents != 0 {
		t.Fatalf("decisions/members/outbox = %d/%d/%d, want 3/2/0 before Event Refresh evaluates notifications", decisions, members, outboxEvents)
	}
	if _, err := runtime.SQL.Exec(`UPDATE micro_event_membership_decisions SET reason_codes='["changed"]' WHERE id=$1`, created.Decision.ID); err == nil {
		t.Fatal("append-only membership decision accepted mutation")
	}
}

func TestMicroEventRepositoryReusesOneMemberForSyndicatedNearDuplicateAndRepeatedMatches(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	repository, err := NewMicroEventRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	service, err := eventapplication.NewMicroEventService(repository)
	if err != nil {
		t.Fatal(err)
	}

	root := seedMicroEventAssignmentFixture(t, runtime, "family-root", "accepted")
	syndicated := seedMicroEventAssignmentFixture(t, runtime, "family-syndicated", "accepted")
	nearDuplicate := seedMicroEventAssignmentFixture(t, runtime, "family-near-duplicate", "accepted")
	moveMicroEventFixtureIntoFamily(t, runtime, syndicated, root, "syndicated_from")
	moveMicroEventFixtureIntoFamily(t, runtime, nearDuplicate, root, "near_duplicate")

	created, err := service.Assign(ctx, eventapplication.AssignContentFamilyToMicroEventCommand{
		ContentFamilyID: root.familyID, DocumentMatchDecisionID: root.matchDecisionID,
		ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion,
	})
	if err != nil || created.Decision.Action != "create" || created.Event.Version != 1 {
		t.Fatalf("root assignment=%#v / %v", created, err)
	}
	for _, duplicate := range []microEventAssignmentFixture{syndicated, nearDuplicate, syndicated} {
		replayed, replayErr := service.Assign(ctx, eventapplication.AssignContentFamilyToMicroEventCommand{
			ContentFamilyID: root.familyID, DocumentMatchDecisionID: duplicate.matchDecisionID,
			ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion,
		})
		if replayErr != nil || replayed.Event.ID != created.Event.ID || replayed.Event.Version != created.Event.Version ||
			replayed.Decision.ID != created.Decision.ID || replayed.Decision.ContentFamilyID != root.familyID {
			t.Fatalf("family replay for match %d=%#v / %v", duplicate.matchDecisionID, replayed, replayErr)
		}
	}

	var events, decisions, members, familyDocuments, legacyEvents, legacyTopics int
	if err := runtime.SQL.QueryRow(`SELECT
  (SELECT count(*) FROM micro_events),
  (SELECT count(*) FROM micro_event_membership_decisions),
  (SELECT count(*) FROM micro_event_members WHERE active),
  (SELECT count(*) FROM content_family_members WHERE family_id=$1 AND active),
  (SELECT count(*) FROM events),
  (SELECT count(*) FROM topics)`, root.familyID).
		Scan(&events, &decisions, &members, &familyDocuments, &legacyEvents, &legacyTopics); err != nil {
		t.Fatal(err)
	}
	if events != 1 || decisions != 1 || members != 1 || familyDocuments != 3 || legacyEvents != 0 || legacyTopics != 0 {
		t.Fatalf("events/decisions/members/family-documents/legacy-events/legacy-topics=%d/%d/%d/%d/%d/%d, want 1/1/1/3/0/0",
			events, decisions, members, familyDocuments, legacyEvents, legacyTopics)
	}
}

func TestMicroEventRepositoryRejectsNonAcceptedEffectiveMatchAtCommit(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	repository, err := NewMicroEventRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	fixture := seedMicroEventAssignmentFixture(t, runtime, "rejected", "rejected")
	_, err = repository.CommitMicroEventMembership(ctx, microEventCommitFixture(
		fixture, "create", 0, 0, strings.Repeat("5", 64), "micro-event-rejected",
	))
	if err == nil {
		t.Fatal("membership accepted a rejected effective document match")
	}
	var events, decisions int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM micro_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM micro_event_membership_decisions`).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if events != 0 || decisions != 0 {
		t.Fatalf("events/decisions after rollback = %d/%d", events, decisions)
	}
}

func TestMicroEventServiceReadsAuthorizedStructuredCandidatesAndDegradesDenseColdStartToReview(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	repository, err := NewMicroEventRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	service, err := eventapplication.NewMicroEventService(repository)
	if err != nil {
		t.Fatal(err)
	}
	first := seedMicroEventAssignmentFixture(t, runtime, "service-first", "accepted")
	created, err := service.Assign(ctx, eventapplication.AssignContentFamilyToMicroEventCommand{
		ContentFamilyID: first.familyID, DocumentMatchDecisionID: first.matchDecisionID,
		ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion,
	})
	if err != nil || created.Decision.Action != "create" {
		t.Fatalf("service create = %#v / %v", created, err)
	}
	second := seedMicroEventAssignmentFixture(t, runtime, "service-second", "accepted")
	reviewed, err := service.Assign(ctx, eventapplication.AssignContentFamilyToMicroEventCommand{
		ContentFamilyID: second.familyID, DocumentMatchDecisionID: second.matchDecisionID,
		ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion,
	})
	if err != nil {
		t.Fatalf("service review: %v", err)
	}
	if reviewed.Event.ID != created.Event.ID || reviewed.Decision.Action != "review" ||
		reviewed.Decision.Features.DenseSimilarity != 0 || reviewed.Decision.SameEventScore < .60 {
		t.Fatalf("cold-start review = %#v", reviewed)
	}
}

func TestMicroEventServiceUsesRightsBoundANNAsIndependentDenseSignal(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	repository, _ := NewMicroEventRepository(runtime)
	service, _ := eventapplication.NewMicroEventService(repository)
	first := seedMicroEventAssignmentFixture(t, runtime, "ann-first", "accepted")
	second := seedMicroEventAssignmentFixture(t, runtime, "ann-second", "accepted")
	attachMicroEventEmbeddingFixture(t, runtime, first)
	attachMicroEventEmbeddingFixture(t, runtime, second)
	created, err := service.Assign(ctx, eventapplication.AssignContentFamilyToMicroEventCommand{
		ContentFamilyID: first.familyID, DocumentMatchDecisionID: first.matchDecisionID,
		ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	joined, err := service.Assign(ctx, eventapplication.AssignContentFamilyToMicroEventCommand{
		ContentFamilyID: second.familyID, DocumentMatchDecisionID: second.matchDecisionID,
		ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion,
	})
	if err != nil {
		t.Fatalf("ANN join: %v", err)
	}
	if joined.Event.ID != created.Event.ID || joined.Decision.Action != "join" ||
		joined.Decision.Features.DenseSimilarity < .99 || slices.Contains(joined.Decision.ReasonCodes, "dense_unavailable") {
		t.Fatalf("ANN decision = %#v", joined)
	}
}

type microEventAssignmentFixture struct {
	familyID, matchDecisionID, monitorID, monitorVersionID int64
	sourceID, documentVersionID, retainDecisionID          int64
	displayDecisionID                                      int64
	occurredAt                                             time.Time
}

func seedMicroEventAssignmentFixture(t *testing.T, runtime *database.Runtime, suffix, decision string) microEventAssignmentFixture {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	fixture := microEventAssignmentFixture{occurredAt: now}
	transaction, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatalf("begin fixture transaction: %v", err)
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`SET LOCAL session_replication_role='replica'`); err != nil {
		t.Fatalf("disable fixture triggers: %v", err)
	}
	var sourceID, observationID, documentID, documentVersionID, fingerprintID, lineageDecisionID int64
	if err := transaction.QueryRow(`INSERT INTO source_connections (source_type,name,endpoint)
VALUES ('rss',$1,$2) RETURNING id`, "micro-event-source-"+suffix, "https://"+suffix+".example/feed").Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	fixture.sourceID = sourceID
	if err := transaction.QueryRow(`INSERT INTO source_observations (
source_connection_id,external_id,upstream_identity,source_code,content_type,title,language,canonical_url,
body_origin,completeness,discovered_at,captured_at,published_at)
VALUES ($1,$2,$3,'rss','article',$4,'zh-CN',$5,'feed_content','full',$6,$6,$6) RETURNING id`,
		sourceID, "work-"+suffix, strings.Repeat("a", 64), "同一主体发布同一动作", "https://"+suffix+".example/article", now).Scan(&observationID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`INSERT INTO documents (source_connection_id,document_key,external_work_id)
VALUES ($1,$2,$3) RETURNING id`, sourceID, strings.Repeat("b", 64), "work-"+suffix).Scan(&documentID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`INSERT INTO document_versions (
document_id,source_observation_id,revision_no,version_key,body_origin,completeness,word_count,language,
content_sha256,extractor_version,extractor_profile_version,extractor_profile_sha256,lifecycle_state,captured_at)
VALUES ($1,$2,1,$3,'feed_content','full',12,'zh-CN',$4,'rss-v1','rss-profile-v1',$5,'derived_available',$6)
RETURNING id`, documentID, observationID, strings.Repeat("c", 64), strings.Repeat("d", 64), strings.Repeat("e", 64), now).Scan(&documentVersionID); err != nil {
		t.Fatal(err)
	}
	fixture.documentVersionID = documentVersionID
	if _, err := transaction.Exec(`UPDATE documents SET current_document_version_id=$1 WHERE id=$2`, documentVersionID, documentID); err != nil {
		t.Fatal(err)
	}
	var storeDecisionID, retainDecisionID, displayDecisionID int64
	if err := transaction.QueryRow(`INSERT INTO source_rights_decisions (
decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,priority_rank,
basis_summary,subject_type,subject_key,input_digest,action,decision,reason_codes,evaluator,evaluated_at,effective_from)
VALUES ($1,$2,$3,1,'source_endpoint',$4,200,'fixture authorization','document_version',$5,$6,
'store_derived','allow',ARRAY['fixture'],'fixture',$7,$7) RETURNING id`,
		700000+documentVersionID*2, sourceID, 710000+documentVersionID*2, "https://"+suffix+".example/feed",
		fmt.Sprint(documentVersionID), strings.Repeat("d", 64), now.Add(-time.Hour)).Scan(&storeDecisionID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`INSERT INTO source_rights_decisions (
decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,priority_rank,
basis_summary,subject_type,subject_key,input_digest,action,decision,reason_codes,evaluator,evaluated_at,effective_from,retention_days)
VALUES ($1,$2,$3,1,'source_endpoint',$4,200,'fixture retention','document_version',$5,$6,
'retain','allow',ARRAY['fixture'],'fixture',$7,$7,30) RETURNING id`,
		700001+documentVersionID*2, sourceID, 710001+documentVersionID*2, "https://"+suffix+".example/feed",
		fmt.Sprint(documentVersionID), strings.Repeat("d", 64), now.Add(-time.Hour)).Scan(&retainDecisionID); err != nil {
		t.Fatal(err)
	}
	fixture.retainDecisionID = retainDecisionID
	if err := transaction.QueryRow(`INSERT INTO source_rights_decisions (
decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,priority_rank,
basis_summary,subject_type,subject_key,input_digest,action,decision,reason_codes,evaluator,evaluated_at,effective_from)
VALUES ($1,$2,$3,1,'source_endpoint',$4,200,'fixture display','document_version',$5,$6,
'display_private','allow',ARRAY['fixture'],'fixture',$7,$7) RETURNING id`,
		760000+documentVersionID, sourceID, 770000+documentVersionID, "https://"+suffix+".example/feed",
		fmt.Sprint(documentVersionID), strings.Repeat("d", 64), now.Add(-time.Hour)).Scan(&displayDecisionID); err != nil {
		t.Fatal(err)
	}
	fixture.displayDecisionID = displayDecisionID
	if _, err := transaction.Exec(`UPDATE document_versions
SET lifecycle_state='readable',display_private_rights_decision_id=$1
WHERE id=$2`, displayDecisionID, documentVersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(`INSERT INTO document_version_search_indexes (
document_version_id,source_connection_id,derived_artifact_id,store_derived_rights_decision_id,retain_rights_decision_id,
normalization_profile_version,normalized_text_sha256,title_search_vector,body_search_vector,title_trigrams,body_trigrams,
entity_keys,action_keys,location_keys,region_keys,retention_until,indexed_at)
VALUES ($1,$2,$3,$4,$5,'canonical-nfc-plaintext-v1',$6,to_tsvector('simple','shared subject released'),
to_tsvector('simple','shared subject released body'),ARRAY['sha','har'],ARRAY['sha','har','rel'],
ARRAY['subject:shared'],ARRAY['action:released'],ARRAY['location:shared'],ARRAY['region:shared'],$7,$8)`,
		documentVersionID, sourceID, 720000+documentVersionID, storeDecisionID, retainDecisionID,
		strings.Repeat("d", 64), now.Add(24*time.Hour), now); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`INSERT INTO content_fingerprints (
source_connection_id,document_version_id,derived_artifact_id,store_derived_rights_decision_id,retain_rights_decision_id,
profile_version,normalized_content_sha256,simhash_hex,minhash,retention_until)
VALUES ($1,$2,$3,$4,$5,'content-fingerprint-v1',$6,$7,$8,$9) RETURNING id`,
		sourceID, documentVersionID, 730000+documentVersionID, storeDecisionID, retainDecisionID,
		strings.Repeat("f", 64), strings.Repeat("1", 16), make([]byte, 512), now.Add(24*time.Hour)).Scan(&fingerprintID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`INSERT INTO content_families (root_document_version_id,lineage_profile_version)
VALUES ($1,'content-family-decision-v1') RETURNING id`, documentVersionID).Scan(&fixture.familyID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`INSERT INTO content_lineage_decisions (
document_version_id,fingerprint_id,family_id,result_family_version,action,relation,hamming_distance,minhash_similarity,
decision_profile_version,reason_codes,idempotency_key,command_fingerprint)
VALUES ($1,$2,$3,1,'create','unrelated',64,0,'content-family-decision-v1','["fixture"]',$4,$5) RETURNING id`,
		documentVersionID, fingerprintID, fixture.familyID, "lineage-"+suffix, strings.Repeat("2", 64)).Scan(&lineageDecisionID); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(`INSERT INTO content_family_members (
family_id,document_version_id,fingerprint_id,lineage_decision_id,lineage_profile_version,relation)
VALUES ($1,$2,$3,$4,'content-family-decision-v1','unrelated')`, fixture.familyID, documentVersionID, fingerprintID, lineageDecisionID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`INSERT INTO monitors (name,status) VALUES ($1,'active') RETURNING id`, "micro-event-monitor-"+suffix).Scan(&fixture.monitorID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`INSERT INTO monitor_config_versions (monitor_id,revision,state,config_hash,published_at)
VALUES ($1,1,'published',$2,$3) RETURNING id`, fixture.monitorID, strings.Repeat("3", 64), now).Scan(&fixture.monitorVersionID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`INSERT INTO document_match_decisions (
monitor_id,monitor_version_id,compiled_profile_id,document_version_id,relevance_profile_id,
matching_algorithm_version,reranker_version,calibration_version,rrf_score,relevance_probability,decision,degraded,
reason_codes,input_hash,decided_at)
VALUES ($1,$2,800001,$3,800002,'rrf-k60-v1','reranker-v1','calibration-v1',0.1,0.95,$4,false,
ARRAY['fixture'],$5,$6) RETURNING id`, fixture.monitorID, fixture.monitorVersionID, documentVersionID, decision,
		strings.Repeat("4", 64), now).Scan(&fixture.matchDecisionID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatalf("commit fixture transaction: %v", err)
	}
	return fixture
}

func revokeMicroEventDisplayRights(t *testing.T, runtime *database.Runtime, fixture microEventAssignmentFixture) {
	t.Helper()
	transaction, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`SET LOCAL session_replication_role='replica'`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	if _, err := transaction.Exec(`INSERT INTO source_rights_decisions (
decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,priority_rank,
basis_summary,subject_type,subject_key,input_digest,action,decision,reason_codes,evaluator,evaluated_at,effective_from)
VALUES ($1,$2,$3,2,'source_endpoint',$4,300,'fixture display revocation','document_version',$5,$6,
'display_private','deny',ARRAY['fixture_revoked'],'fixture',$7,$7)`,
		780000+fixture.documentVersionID, fixture.sourceID, 790000+fixture.documentVersionID,
		fmt.Sprintf("source:%d", fixture.sourceID), fmt.Sprint(fixture.documentVersionID), strings.Repeat("d", 64), now); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
}

func attachMicroEventEmbeddingFixture(t *testing.T, runtime *database.Runtime, fixture microEventAssignmentFixture) {
	t.Helper()
	transaction, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`SET LOCAL session_replication_role='replica'`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	var embedDecisionID int64
	if err := transaction.QueryRow(`INSERT INTO source_rights_decisions (
decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,priority_rank,
basis_summary,subject_type,subject_key,input_digest,action,decision,reason_codes,evaluator,evaluated_at,effective_from)
VALUES ($1,$2,$3,1,'source_endpoint',$4,200,'fixture embedding','document_version',$5,$6,
'embed_local','allow',ARRAY['fixture'],'fixture',$7,$7) RETURNING id`,
		740000+fixture.documentVersionID, fixture.sourceID, 750000+fixture.documentVersionID,
		fmt.Sprintf("source:%d", fixture.sourceID), fmt.Sprint(fixture.documentVersionID), strings.Repeat("d", 64),
		now.Add(-time.Hour)).Scan(&embedDecisionID); err != nil {
		t.Fatal(err)
	}
	components := make([]string, 1024)
	components[0] = "1"
	for index := 1; index < len(components); index++ {
		components[index] = "0"
	}
	if _, err := transaction.Exec(`INSERT INTO document_version_embeddings (
document_version_id,source_connection_id,embed_local_rights_decision_id,retain_rights_decision_id,
model_profile_id,model_profile_version,model_version,normalized_text_sha256,embedding,ai_run_id,retention_until)
VALUES ($1,$2,$3,$4,990001,1,'embedding-fixture-v1',$5,$6::halfvec,990002,$7)`,
		fixture.documentVersionID, fixture.sourceID, embedDecisionID, fixture.retainDecisionID,
		strings.Repeat("d", 64), "["+strings.Join(components, ",")+"]", now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
}

func moveMicroEventFixtureIntoFamily(t *testing.T, runtime *database.Runtime, child, root microEventAssignmentFixture, relation string) {
	t.Helper()
	if relation != "syndicated_from" && relation != "near_duplicate" {
		t.Fatalf("unsupported fixture relation %q", relation)
	}
	transaction, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	var fingerprintID, familyVersion, lineageDecisionID int64
	if err := transaction.QueryRow(`SELECT fingerprint_id FROM content_family_members
WHERE family_id=$1 AND document_version_id=$2 AND active`, child.familyID, child.documentVersionID).Scan(&fingerprintID); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(`UPDATE content_family_members
SET active=false,retired_at=now(),version=version+1
WHERE family_id=$1 AND document_version_id=$2 AND active`, child.familyID, child.documentVersionID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`UPDATE content_families SET version=version+1,updated_at=now()
WHERE id=$1 RETURNING version`, root.familyID).Scan(&familyVersion); err != nil {
		t.Fatal(err)
	}
	idempotencyKey := fmt.Sprintf("lineage-family-%d-%s", child.documentVersionID, relation)
	commandFingerprint := fmt.Sprintf("%064x", child.documentVersionID)
	if err := transaction.QueryRow(`INSERT INTO content_lineage_decisions (
document_version_id,fingerprint_id,family_id,result_family_version,candidate_root_document_version_id,
action,relation,hamming_distance,minhash_similarity,decision_profile_version,reason_codes,idempotency_key,command_fingerprint)
VALUES ($1,$2,$3,$4,$5,'join',$6,1,.99,'content-family-decision-v1','["fixture"]',$7,$8)
RETURNING id`, child.documentVersionID, fingerprintID, root.familyID, familyVersion, root.documentVersionID,
		relation, idempotencyKey, commandFingerprint).Scan(&lineageDecisionID); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(`INSERT INTO content_family_members (
family_id,document_version_id,fingerprint_id,lineage_decision_id,lineage_profile_version,relation,parent_document_version_id)
VALUES ($1,$2,$3,$4,'content-family-decision-v1',$5,$6)`, root.familyID, child.documentVersionID,
		fingerprintID, lineageDecisionID, relation, root.documentVersionID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
}

func microEventCommitFixture(fixture microEventAssignmentFixture, action string, candidateID, expectedVersion int64, fingerprint, key string) eventapplication.CommitMicroEventMembershipCommand {
	return eventapplication.CommitMicroEventMembershipCommand{
		ContentFamilyID: fixture.familyID, DocumentMatchDecisionID: fixture.matchDecisionID,
		MonitorID: fixture.monitorID, MonitorVersionID: fixture.monitorVersionID,
		EventKey: strings.Repeat(fmt.Sprintf("%x", len(key)%16), 64), PrimarySubjectKey: "subject:shared",
		PrimaryActionKey: "action:released", OccurredAt: fixture.occurredAt,
		Action: action, CandidateMicroEventID: candidateID, ExpectedEventVersion: expectedVersion,
		SameEventScore: 0.95, LeadingMargin: 0.20,
		Features: eventapplication.MicroEventFeaturesDTO{SparseSimilarity: 0.90, DenseSimilarity: 0.92,
			EntityOverlap: 1, ActionOverlap: 1, LocationConsistency: 1, IdentifierConsistency: 1,
			TimeSimilarity: 0.90, LineageRelation: 0.75},
		ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion,
		ReasonCodes:              []string{"fixture"}, IdempotencyKey: key, CommandFingerprint: fingerprint,
	}
}
