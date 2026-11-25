package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"
)

// dbConfig builds the MySQL settings from DB_HOST, DB_PORT, DB_USER,
// DB_PASSWORD and the given database name. See .env.example.
func dbConfig(getenv func(string) string, dbName string) (*mysql.Config, error) {
	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	host, port := getenv("DB_HOST"), getenv("DB_PORT")
	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "3306"
	}
	cfg.Addr = net.JoinHostPort(host, port)
	cfg.User = getenv("DB_USER")
	cfg.Passwd = getenv("DB_PASSWORD")
	cfg.DBName = dbName
	cfg.ClientFoundRows = true // UPDATE reports matched rows, not changed ones
	cfg.Timeout = 5 * time.Second
	if cfg.User == "" || dbName == "" {
		return nil, fmt.Errorf("DB_USER and the database name must be set")
	}
	return cfg, nil
}

func openDB(cfg *mysql.Config) (*sql.DB, error) {
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func main() {
	cfg, err := dbConfig(os.Getenv, os.Getenv("DB_NAME"))
	if err != nil {
		log.Fatal(err)
	}
	db, err := openDB(cfg)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           NewApp(db).Router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}
