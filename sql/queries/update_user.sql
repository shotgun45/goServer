-- name: UpdateUser :one
UPDATE users SET email = $2, hashed_password = $3, updated_at = $4 WHERE id = $1 RETURNING id, created_at, updated_at, email, hashed_password;
