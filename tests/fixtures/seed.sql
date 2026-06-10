-- E2E optional seed data (minimal).
-- Schema is applied from db/schema.sql before this file runs.

INSERT INTO users (
    id, email, password_hash, role, status, timezone, daily_send_at,
    email_enabled, weekly_enabled, weekly_send_at, created_at, updated_at
)
VALUES (
    'u_e2e_001',
    'admin@e2e.test',
    '$2a$10$e2eplaceholderhashxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
    'admin',
    'active',
    'Asia/Shanghai',
    '08:30',
    true,
    false,
    '09:00',
    now(),
    now()
)
ON CONFLICT (id) DO NOTHING;
