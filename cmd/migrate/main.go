package main

import (
	"flag"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	dsn := flag.String("dsn", os.Getenv("MIGRATE_DSN"), "postgres DSN")
	down := flag.Bool("down", false, "rollback one migration")
	flag.Parse()

	if *dsn == "" {
		log.Fatal("DSN is required: set MIGRATE_DSN or pass -dsn flag")
	}

	m, err := migrate.New("file://migrations", *dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer m.Close()

	if *down {
		if err := m.Steps(-1); err != nil && err != migrate.ErrNoChange {
			log.Fatal(err)
		}
		log.Println("rolled back one migration")
		return
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatal(err)
	}
	log.Println("migrations applied")
}
