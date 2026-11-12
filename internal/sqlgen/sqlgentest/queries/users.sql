-- name: AddUser :one
INSERT INTO users (id, email, name) VALUES (:id, :email, :name)
RETURNING id;

-- The user with an email address.
-- name: UserByEmail :one
SELECT id, email, name FROM users WHERE email = :email;
