package main

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"unicode/utf8"
)

//go:embed schema.sql
var schema string

type App struct {
	Router *http.ServeMux
	DB     *sql.DB
}

// NewApp wires the routes to db. db may be nil in tests that never reach it.
func NewApp(db *sql.DB) *App {
	a := &App{Router: http.NewServeMux(), DB: db}
	a.initializeRoutes()
	return a
}

// ensureSchema creates the users table when it is not there yet.
func ensureSchema(db *sql.DB) error {
	_, err := db.Exec(schema)
	return err
}

func (a *App) initializeRoutes() {
	a.Router.HandleFunc("GET /users", a.getUsers)
	a.Router.HandleFunc("POST /user", a.createUser)
	a.Router.HandleFunc("GET /user/{id}", a.getUser)
	a.Router.HandleFunc("PUT /user/{id}", a.updateUser)
	a.Router.HandleFunc("DELETE /user/{id}", a.deleteUser)
}

const maxBody = 1 << 20 // 1 MB is plenty for one user

func userID(r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	return id, err == nil && id > 0
}

// validate keeps a user within what the users table holds.
func (u user) validate() string {
	switch {
	case u.Name == "":
		return "name is required"
	case utf8.RuneCountInString(u.Name) > 50:
		return "name is longer than 50 characters"
	case u.Age < 0 || u.Age > 150:
		return "age must be between 0 and 150"
	}
	return ""
}

func decodeUser(w http.ResponseWriter, r *http.Request) (user, string) {
	var u user
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&u); err != nil {
		return u, "Invalid request payload"
	}
	return u, u.validate()
}

func (a *App) getUser(w http.ResponseWriter, r *http.Request) {
	id, ok := userID(r)
	if !ok {
		respondWithError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}
	u := user{ID: id}
	if err := u.getUser(a.DB); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, http.StatusNotFound, "User not found")
			return
		}
		internalError(w, err)
		return
	}
	respondWithJSON(w, http.StatusOK, u)
}

func (a *App) getUsers(w http.ResponseWriter, r *http.Request) {
	count, _ := strconv.Atoi(r.FormValue("count"))
	start, _ := strconv.Atoi(r.FormValue("start"))
	if count > 10 || count < 1 {
		count = 10
	}
	if start < 0 {
		start = 0
	}
	users, err := getUsers(a.DB, start, count)
	if err != nil {
		internalError(w, err)
		return
	}
	respondWithJSON(w, http.StatusOK, users)
}

func (a *App) createUser(w http.ResponseWriter, r *http.Request) {
	u, problem := decodeUser(w, r)
	if problem != "" {
		respondWithError(w, http.StatusBadRequest, problem)
		return
	}
	if err := u.createUser(a.DB); err != nil {
		internalError(w, err)
		return
	}
	respondWithJSON(w, http.StatusCreated, u)
}

func (a *App) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := userID(r)
	if !ok {
		respondWithError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}
	u, problem := decodeUser(w, r)
	if problem != "" {
		respondWithError(w, http.StatusBadRequest, problem)
		return
	}
	u.ID = id
	n, err := u.updateUser(a.DB)
	if err != nil {
		internalError(w, err)
		return
	}
	if n == 0 {
		respondWithError(w, http.StatusNotFound, "User not found")
		return
	}
	respondWithJSON(w, http.StatusOK, u)
}

func (a *App) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := userID(r)
	if !ok {
		respondWithError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}
	u := user{ID: id}
	n, err := u.deleteUser(a.DB)
	if err != nil {
		internalError(w, err)
		return
	}
	if n == 0 {
		respondWithError(w, http.StatusNotFound, "User not found")
		return
	}
	respondWithJSON(w, http.StatusOK, map[string]string{"result": "success"})
}

// internalError logs the cause and tells the client only that it failed:
// database messages can name tables, columns and values.
func internalError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	respondWithError(w, http.StatusInternalServerError, "Internal server error")
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}

func respondWithJSON(w http.ResponseWriter, code int, payload any) {
	response, err := json.Marshal(payload)
	if err != nil {
		log.Printf("encode response: %v", err)
		code, response = http.StatusInternalServerError, []byte(`{"error":"Internal server error"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if _, err := w.Write(response); err != nil {
		log.Printf("write response: %v", err)
	}
}
