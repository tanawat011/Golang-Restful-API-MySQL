package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

// a talks to a real MySQL database, named by TEST_DB_NAME (with DB_HOST,
// DB_PORT, DB_USER, DB_PASSWORD). Without it the database tests are skipped
// and only the ones that never reach the database run. The tests EMPTY the
// users table of that database.
var a *App

func TestMain(m *testing.M) {
	if name := os.Getenv("TEST_DB_NAME"); name != "" {
		if name == os.Getenv("DB_NAME") {
			log.Fatal("TEST_DB_NAME must not be DB_NAME: the tests empty the users table")
		}
		cfg, err := dbConfig(os.Getenv, name)
		if err != nil {
			log.Fatal(err)
		}
		db, err := openDB(cfg)
		if err != nil {
			log.Fatalf("test database: %v", err)
		}
		a = NewApp(db)
	}
	code := m.Run()
	if a != nil {
		clearTable()
		a.DB.Close()
	}
	os.Exit(code)
}

func needDB(t *testing.T) {
	t.Helper()
	if a == nil {
		t.Skip("set TEST_DB_NAME (and DB_*) to run the database tests; see README")
	}
	clearTable()
}

func clearTable() {
	if _, err := a.DB.Exec("TRUNCATE TABLE users"); err != nil { // also restarts ids at 1
		log.Fatal(err)
	}
}

func addUsers(t *testing.T, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		if _, err := a.DB.Exec("INSERT INTO users (name, age) VALUES (?, ?)", "User "+strconv.Itoa(i+1), (i+1)*10); err != nil {
			t.Fatal(err)
		}
	}
}

func executeRequest(app *App, method, path, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	}
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	return rr
}

func checkResponseCode(t *testing.T, expected int, response *httptest.ResponseRecorder) {
	t.Helper()
	if expected != response.Code {
		t.Errorf("Expected response code %d. Got %d: %s", expected, response.Code, response.Body)
	}
}

func decode(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &m); err != nil {
		t.Fatalf("response is not a JSON object: %s", response.Body)
	}
	return m
}

// --- Tests that never reach the database -------------------------------

func TestInvalidUserID(t *testing.T) {
	app := NewApp(nil)
	for _, c := range []struct{ method, path string }{
		{"GET", "/user/abc"}, {"GET", "/user/0"}, {"PUT", "/user/-1"}, {"DELETE", "/user/1x"},
	} {
		response := executeRequest(app, c.method, c.path, `{"name":"a","age":1}`)
		checkResponseCode(t, http.StatusBadRequest, response)
		if m := decode(t, response); m["error"] != "Invalid user ID" {
			t.Errorf("%s %s: error = %v", c.method, c.path, m["error"])
		}
	}
}

func TestInvalidPayload(t *testing.T) {
	app := NewApp(nil)
	for _, c := range []struct{ body, want string }{
		{`{`, "Invalid request payload"},
		{`{"name":"","age":3}`, "name is required"},
		{`{"name":"` + strings.Repeat("ก", 51) + `","age":3}`, "name is longer than 50 characters"},
		{`{"name":"a","age":-1}`, "age must be between 0 and 150"},
		{`{"name":"a","age":"30"}`, "Invalid request payload"},
	} {
		for _, req := range []struct{ method, path string }{{"POST", "/user"}, {"PUT", "/user/1"}} {
			response := executeRequest(app, req.method, req.path, c.body)
			checkResponseCode(t, http.StatusBadRequest, response)
			if m := decode(t, response); m["error"] != c.want {
				t.Errorf("%s %s %s: error = %v, want %s", req.method, req.path, c.body, m["error"], c.want)
			}
		}
	}
}

func TestRoutes(t *testing.T) {
	app := NewApp(nil)
	checkResponseCode(t, http.StatusNotFound, executeRequest(app, "GET", "/nothing", ""))
	checkResponseCode(t, http.StatusMethodNotAllowed, executeRequest(app, "PATCH", "/user/1", ""))
}

func TestDBConfig(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	if _, err := dbConfig(env(map[string]string{}), "x"); err == nil {
		t.Error("expected an error without DB_USER")
	}
	cfg, err := dbConfig(env(map[string]string{"DB_USER": "u", "DB_PASSWORD": "p@ss:/word"}), "x")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:3306" || !cfg.ClientFoundRows || cfg.Passwd != "p@ss:/word" {
		t.Errorf("unexpected config %+v", cfg)
	}
}

// --- Tests against MySQL -------------------------------------------------

func TestEmptyTable(t *testing.T) {
	needDB(t)
	response := executeRequest(a, "GET", "/users", "")
	checkResponseCode(t, http.StatusOK, response)
	if body := response.Body.String(); body != "[]" {
		t.Errorf("Expected an empty array. Got %s", body)
	}
}

func TestGetNonExistentUser(t *testing.T) {
	needDB(t)
	response := executeRequest(a, "GET", "/user/45", "")
	checkResponseCode(t, http.StatusNotFound, response)
	if m := decode(t, response); m["error"] != "User not found" {
		t.Errorf("Expected the 'error' key of the response to be set to 'User not found'. Got '%v'", m["error"])
	}
}

func TestCreateUser(t *testing.T) {
	needDB(t)
	response := executeRequest(a, "POST", "/user", `{"name":"test user","age":30}`)
	checkResponseCode(t, http.StatusCreated, response)
	m := decode(t, response)
	if m["name"] != "test user" {
		t.Errorf("Expected user name to be 'test user'. Got '%v'", m["name"])
	}
	if m["age"] != 30.0 {
		t.Errorf("Expected user age to be '30'. Got '%v'", m["age"])
	}
	// the id is compared to 1.0 because JSON unmarshaling converts numbers to
	// floats, when the target is a map[string]any
	if m["id"] != 1.0 {
		t.Errorf("Expected user ID to be '1'. Got '%v'", m["id"])
	}
}

func TestGetUser(t *testing.T) {
	needDB(t)
	addUsers(t, 1)
	response := executeRequest(a, "GET", "/user/1", "")
	checkResponseCode(t, http.StatusOK, response)
	if m := decode(t, response); m["name"] != "User 1" || m["age"] != 10.0 || m["id"] != 1.0 {
		t.Errorf("got %v", m)
	}
}

func TestListUsersPages(t *testing.T) {
	needDB(t)
	addUsers(t, 12)
	var page []user
	response := executeRequest(a, "GET", "/users?count=5&start=10", "")
	checkResponseCode(t, http.StatusOK, response)
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].ID != 11 || page[1].ID != 12 {
		t.Errorf("start=10 count=5 gave %+v, want users 11 and 12", page)
	}
	response = executeRequest(a, "GET", "/users?count=100", "") // capped at 10
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil || len(page) != 10 {
		t.Errorf("count=100 gave %d users, want 10", len(page))
	}
}

func TestUpdateUser(t *testing.T) {
	needDB(t)
	addUsers(t, 1)
	originalUser := decode(t, executeRequest(a, "GET", "/user/1", ""))
	response := executeRequest(a, "PUT", "/user/1", `{"name":"test user - updated name","age":21}`)
	checkResponseCode(t, http.StatusOK, response)
	m := decode(t, response)
	if m["id"] != originalUser["id"] {
		t.Errorf("Expected the id to remain the same (%v). Got %v", originalUser["id"], m["id"])
	}
	stored := decode(t, executeRequest(a, "GET", "/user/1", ""))
	if stored["name"] != "test user - updated name" || stored["age"] != 21.0 {
		t.Errorf("stored user is %v", stored)
	}
}

func TestUpdateWithTheSameValues(t *testing.T) {
	needDB(t)
	addUsers(t, 1)
	// Without clientFoundRows MySQL counts 0 changed rows and this was a 404.
	checkResponseCode(t, http.StatusOK, executeRequest(a, "PUT", "/user/1", `{"name":"User 1","age":10}`))
}

func TestUpdateNonExistentUser(t *testing.T) {
	needDB(t)
	checkResponseCode(t, http.StatusNotFound, executeRequest(a, "PUT", "/user/7", `{"name":"x","age":1}`))
}

func TestDeleteUser(t *testing.T) {
	needDB(t)
	addUsers(t, 1)
	checkResponseCode(t, http.StatusOK, executeRequest(a, "GET", "/user/1", ""))
	checkResponseCode(t, http.StatusOK, executeRequest(a, "DELETE", "/user/1", ""))
	checkResponseCode(t, http.StatusNotFound, executeRequest(a, "GET", "/user/1", ""))
	checkResponseCode(t, http.StatusNotFound, executeRequest(a, "DELETE", "/user/1", ""))
}

// A name that would rewrite the SQL if it were pasted into the statement.
func TestNameIsStoredAsTyped(t *testing.T) {
	needDB(t)
	addUsers(t, 2)
	evil := `x', age=0 WHERE 1=1 -- `
	body, _ := json.Marshal(user{Name: evil, Age: 5})
	checkResponseCode(t, http.StatusOK, executeRequest(a, "PUT", "/user/1", string(body)))

	if m := decode(t, executeRequest(a, "GET", "/user/1", "")); m["name"] != evil || m["age"] != 5.0 {
		t.Errorf("user 1 is %v", m)
	}
	if m := decode(t, executeRequest(a, "GET", "/user/2", "")); m["name"] != "User 2" || m["age"] != 20.0 {
		t.Errorf("user 2 changed: %v", m)
	}
}

func TestFiftyThaiCharactersFit(t *testing.T) {
	needDB(t)
	name := strings.Repeat("ก", 50)
	response := executeRequest(a, "POST", "/user", `{"name":"`+name+`","age":0}`)
	checkResponseCode(t, http.StatusCreated, response)
	if m := decode(t, executeRequest(a, "GET", "/user/1", "")); m["name"] != name {
		t.Errorf("stored name is %v", m["name"])
	}
}
