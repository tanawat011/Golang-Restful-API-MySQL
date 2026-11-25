package main

import (
	"database/sql"
)

type user struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

// Every value goes to MySQL as a parameter (?), never inside the SQL text,
// so a name such as  x', age=0 WHERE 1=1 --  is stored as typed.

func (u *user) getUser(db *sql.DB) error {
	return db.QueryRow("SELECT name, age FROM users WHERE id = ?", u.ID).Scan(&u.Name, &u.Age)
}

// updateUser reports how many rows matched (the connection is opened with
// clientFoundRows, so an update that changes nothing still counts its row).
func (u *user) updateUser(db *sql.DB) (int64, error) {
	res, err := db.Exec("UPDATE users SET name = ?, age = ? WHERE id = ?", u.Name, u.Age, u.ID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (u *user) deleteUser(db *sql.DB) (int64, error) {
	res, err := db.Exec("DELETE FROM users WHERE id = ?", u.ID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (u *user) createUser(db *sql.DB) error {
	res, err := db.Exec("INSERT INTO users (name, age) VALUES (?, ?)", u.Name, u.Age)
	if err != nil {
		return err
	}
	// The id of this INSERT. A separate SELECT LAST_INSERT_ID() may run on
	// another pooled connection and read 0.
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	u.ID = int(id)
	return nil
}

func getUsers(db *sql.DB, start, count int) ([]user, error) {
	rows, err := db.Query("SELECT id, name, age FROM users ORDER BY id LIMIT ? OFFSET ?", count, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []user{}
	for rows.Next() {
		var u user
		if err := rows.Scan(&u.ID, &u.Name, &u.Age); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
