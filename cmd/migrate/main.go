package main

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"goguide/internal/api"
)

func main() {
	db, err := api.OpenDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("CREATE TABLE IF NOT EXISTS schema_migrations(version VARCHAR(100) PRIMARY KEY,applied_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)) ENGINE=InnoDB"); err != nil {
		log.Fatal(err)
	}
	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		log.Fatal(err)
	}
	sort.Strings(files)
	for _, file := range files {
		version := filepath.Base(file)
		var exists int
		err = db.QueryRow("SELECT 1 FROM schema_migrations WHERE version=?", version).Scan(&exists)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			log.Fatal(err)
		}
		body, err := os.ReadFile(file)
		if err != nil {
			log.Fatal(err)
		}
		for _, statement := range strings.Split(string(body), ";") {
			statement = strings.TrimSpace(statement)
			if statement == "" {
				continue
			}
			if _, err = db.Exec(statement); err != nil {
				log.Fatalf("migration %s failed: %v", version, err)
			}
		}
		if _, err = db.Exec("INSERT INTO schema_migrations(version) VALUES(?)", version); err != nil {
			log.Fatal(err)
		}
		log.Printf("applied %s", version)
	}
	log.Print("migration complete")
}
