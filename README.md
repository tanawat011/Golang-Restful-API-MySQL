# Golang RESTful API with MySQL

A small CRUD API for `users` (id, name, age) in Go with MySQL. It follows
the structure of Semaphore's tutorial "Building and Testing a REST API in Go
with Gorilla Mux and PostgreSQL", moved to MySQL: an `App` with a router and
a database, handlers, a model, and tests that call the handlers through
`httptest`.

Originally written in 2018 while learning Go and MySQL; refreshed in
Nov 2026: Go modules and current Go, the standard library's router
(instead of gorilla/mux), parameterized SQL, settings from environment
variables, input checks, a MySQL in Docker, and CI.

## Endpoints

| Method | Path | Does |
|---|---|---|
| `GET` | `/users?count=10&start=0` | list users (count 1–10, default 10) |
| `POST` | `/user` | create, body `{"name": "...", "age": 30}` → 201 |
| `GET` | `/user/{id}` | one user, 404 if none |
| `PUT` | `/user/{id}` | replace name and age, 404 if none |
| `DELETE` | `/user/{id}` | delete, 404 if none |

A name is 1–50 characters, an age 0–150; anything else answers 400.

## Quick start

Needs Go 1.26.11+ and Docker (for MySQL).

```sh
git clone https://github.com/tanawat011/Golang-Restful-API-MySQL.git
cd Golang-Restful-API-MySQL
cp .env.example .env          # dev-only placeholders; change them if you like
docker compose up -d --wait   # MySQL 8.4 on 127.0.0.1:13306
set -a; . ./.env; set +a      # export the settings to this shell
go run .                      # API on 127.0.0.1:8080

curl -X POST localhost:8080/user -d '{"name":"Somchai","age":30}'
curl localhost:8080/users
```

`docker compose down -v` stops MySQL and deletes its data.

## Tests

```sh
go test ./...                 # without a database: the database tests are skipped
set -a; . ./.env; set +a
go test -race -v ./...        # with the compose MySQL: every test runs
```

The database tests use `TEST_DB_NAME` (compose creates
`golang_rest_api_example_test`) and empty its `users` table; they refuse to
run against `DB_NAME`. CI runs gofmt, vet, all tests against a MySQL 8.4
service on the two supported Go releases (and fails if any was skipped),
and `govulncheck`.
