-- name: GetUserByEmail :one
SELECT id, email, password_hash, display_name, status, plan_type, created_at, updated_at
FROM users
WHERE email = sqlc.arg(email);

-- name: GetUserByID :one
SELECT id, email, password_hash, display_name, status, plan_type, created_at, updated_at
FROM users
WHERE id = sqlc.arg(id);

-- name: CreateUser :one
INSERT INTO users (email, password_hash, display_name)
VALUES (sqlc.arg(email), sqlc.arg(password_hash), sqlc.arg(display_name))
RETURNING id, email, password_hash, display_name, status, plan_type, created_at, updated_at;

-- name: ExistsByEmail :one
SELECT EXISTS(SELECT 1 FROM users WHERE email = sqlc.arg(email));
