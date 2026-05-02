package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := os.Getenv("USERPROFILE") + `\\.wheelmaker\\db\\client.sqlite3`
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	fmt.Println("DB:", dbPath)
	fmt.Println("-- projects in sessions --")
	rows, err := db.Query(`SELECT project_name, COUNT(1) FROM sessions GROUP BY project_name ORDER BY COUNT(1) DESC`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var p string
		var c int
		if err := rows.Scan(&p, &c); err != nil {
			panic(err)
		}
		fmt.Printf("project=%q sessions=%d\n", p, c)
	}
	_ = rows.Close()

	fmt.Println("-- latest sessions --")
	rows, err = db.Query(`SELECT id, project_name, agent_type, title, last_active_at FROM sessions ORDER BY last_active_at DESC LIMIT 20`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var id, p, a, t, ts string
		if err := rows.Scan(&id, &p, &a, &t, &ts); err != nil {
			panic(err)
		}
		fmt.Printf("sid=%s project=%q agent=%q title=%q last=%s\n", id, p, a, t, ts)
	}
	_ = rows.Close()

	fmt.Println("-- agent preferences --")
	rows, err = db.Query(`SELECT project_name, agent_type, preference_json FROM agent_preferences ORDER BY project_name, agent_type`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var p, a, j string
		if err := rows.Scan(&p, &a, &j); err != nil {
			panic(err)
		}
		if len(j) > 180 {
			j = j[:180] + "..."
		}
		fmt.Printf("project=%q agent=%q pref=%s\n", p, a, j)
	}
	_ = rows.Close()
}
