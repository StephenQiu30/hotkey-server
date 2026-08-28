\set ON_ERROR_STOP on

BEGIN;

INSERT INTO users (id,version,email,password_hash,display_name,role,status)
VALUES (
    910001,
    1,
    'browser-e2e-sentinel@example.test',
    '$2y$10$TOs7BsHx9.lu745Fu6RWUe5PWt8SL7OOUx3L/MQCd5Md.RgAOGY0G',
    'Browser Acceptance Editor',
    'editor',
    'active'
);

INSERT INTO users (id,version,email,password_hash,display_name,role,status)
VALUES (
    910003,
    1,
    'browser-secret-admin@example.test',
    '$2y$10$TOs7BsHx9.lu745Fu6RWUe5PWt8SL7OOUx3L/MQCd5Md.RgAOGY0G',
    'Browser Secret Admin',
    'admin',
    'active'
);

INSERT INTO users (id,version,email,password_hash,display_name,role,status)
VALUES (
    910002,
    1,
    'browser-empty-state@example.test',
    '$2y$10$TOs7BsHx9.lu745Fu6RWUe5PWt8SL7OOUx3L/MQCd5Md.RgAOGY0G',
    'Browser Acceptance Analyst',
    'analyst',
    'active'
);

INSERT INTO monitors (id,version,name,description,status,created_by,updated_by)
VALUES (910001,1,:'fixture_run_id','Disposable browser acceptance monitor','active',910001,910001);

COMMIT;
