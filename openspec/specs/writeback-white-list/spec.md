## ADDED Requirements

### Requirement: Writeback white-list validation

The system SHALL validate that every incoming writeback field is one of the predefined allowed fields before applying any change.

Allowed fields:
- `manual_tags` (string array)
- `analyst_conclusion` (string)
- `theme_ref` (string)
- `material_status` (enum: draft, review, final)

The system SHALL reject any field not in this list with a typed error `ErrFieldNotAllowed`.

#### Scenario: Whitelisted field passes validation
- **WHEN** a writeback change contains field `manual_tags` with value `["ai监管"]`
- **THEN** the validator SHALL return nil (no error)

#### Scenario: Machine field is rejected
- **WHEN** a writeback change contains field `heat` with value `99.9`
- **THEN** the validator SHALL return `ErrFieldNotAllowed`

#### Scenario: Fields write to sidecar only
- **WHEN** a validated writeback change is applied
- **THEN** the change SHALL only be written to `event_annotations`, `topic_annotations`, or `theme_memberships` sidecar tables
- **THEN** no machine fact tables (`events`, `topics`, `platform_posts`, `monitor_post_hits`) SHALL be modified

### Requirement: Writeback conflict detection

The system SHALL detect writeback conflicts based on revision digest comparison before applying any change.

Each vault object SHALL carry a `revision_digest` field in its YAML frontmatter.
The system SHALL compare the incoming revision digest against the current revision digest stored in the database.

- If both digests are set and match: allow writeback.
- If both digests are set but differ: reject with `ErrWritebackConflict`.
- If either side has no digest set: allow writeback (backward compatibility).

#### Scenario: Matching revisions pass conflict check
- **WHEN** current revision is `rev-2` and incoming revision is `rev-2`
- **THEN** conflict check SHALL pass (return nil)

#### Scenario: Stale revision triggers conflict
- **WHEN** current revision is `rev-2` and incoming revision is `rev-1`
- **THEN** conflict check SHALL return `ErrWritebackConflict`

#### Scenario: Empty revision allows writeback
- **WHEN** current revision is empty and incoming revision is empty
- **THEN** conflict check SHALL pass (return nil)

### Requirement: Writeback audit log

Every writeback attempt SHALL be recorded in `knowledge_writeback_logs` with the following fields:
- `object_type`: type of the object being written back (e.g. "theme", "event", "topic")
- `object_id`: database ID of the object
- `field_name`: the field being written back
- `field_value`: the new value (JSONB)
- `status`: one of `detected`, `validated`, `applied`, `conflicted`, `rejected`
- `conflict_reason`: human-readable reason if status is `conflicted` or `rejected` (default empty)
- `source_path`: vault file path the change was read from (default empty)
- `created_at`: server timestamp of the writeback attempt

#### Scenario: Successful writeback creates audit record
- **WHEN** a validated writeback change is successfully applied
- **THEN** a `knowledge_writeback_logs` row SHALL be created with status `applied`

#### Scenario: Rejected writeback creates audit record
- **WHEN** a writeback change is rejected due to field not in whitelist
- **THEN** a `knowledge_writeback_logs` row SHALL be created with status `rejected`

#### Scenario: Conflicted writeback creates audit record
- **WHEN** a writeback change is rejected due to revision conflict
- **THEN** a `knowledge_writeback_logs` row SHALL be created with status `conflicted`

### Requirement: Writeback roundtrip validation

The system SHALL provide an automated roundtrip validation that demonstrates "export → manual structured edit → writeback → re-export" consistency.

The roundtrip test SHALL:
1. Take a sample set of knowledge objects
2. Export them to a temporary Obsidian vault
3. Simulate a whitelisted manual change (e.g., add `manual_tags`)
4. Parse and apply the change through the writeback pipeline
5. Re-export from the updated database state
6. Verify that the re-exported output includes the manual tag change

#### Scenario: Roundtrip preserves manual tags
- **WHEN** a manual tag is added in the vault and written back
- **THEN** the re-exported vault content SHALL contain the added manual tag

#### Scenario: Writeback does not alter machine fields
- **WHEN** a whitelisted writeback is applied
- **THEN** the re-exported machine-calculated fields (heat, trend, post_count) SHALL remain unchanged from pre-writeback state
