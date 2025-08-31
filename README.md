# goServer

## Overview
goServer is a secure RESTful API server written in Go, featuring user authentication, JWT access tokens, refresh tokens, and chirp posting. It uses PostgreSQL for persistent storage and SQLC for type-safe database queries. This is a guided lesson from Boot.dev. 

## Features
- User registration and login with hashed passwords
- JWT-based authentication (access tokens expire in 1 hour)
- Refresh token support (expires in 60 days, can be revoked)
- Chirp posting and retrieval (requires authentication)
- User profile update (email and password)
- Token revocation endpoint
- Metrics and admin endpoints

## API Endpoints

### Authentication & Users
- `POST /api/users` — Register a new user
- `POST /api/login` — Login and receive access/refresh tokens
- `PUT /api/users` — Update your email and password (requires access token)

### Chirps
- `POST /api/chirps` — Create a new chirp (requires access token)
- `GET /api/chirps` — List all chirps
  - Supports optional query parameters:
    - `author_id`: Filter chirps by author
    - `sort`: Sort chirps by creation time. Possible values:
      - `asc` (default): Ascending order
      - `desc`: Descending order
- `GET /api/chirps/{chirpID}` — Get a chirp by ID

### Tokens
- `POST /api/refresh` — Get a new access token using a refresh token (in Authorization header)
- `POST /api/revoke` — Revoke a refresh token (in Authorization header)

### Admin
- `GET /admin/metrics` — View server metrics
- `POST /admin/reset` — Reset all users and metrics (dev only)

## Security
- Passwords are hashed using bcrypt
- JWTs are signed with a secret from `.env`
- All sensitive endpoints require proper authentication

## Setup
1. Clone the repo
2. Set up PostgreSQL and update `.env` with your DB credentials and JWT secret
3. Run migrations in `sql/schema`
4. Generate SQLC code: `sqlc generate`
5. Build and run the server: `go run main.go`

## License
MIT
