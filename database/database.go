package database

import (
	"context"
	"database/sql"
	"log"

	_ "github.com/lib/pq"
	"go.mau.fi/whatsmeow/store/sqlstore"
)

var Container *sqlstore.Container

func InitWhatsmeow(dbURL string) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Failed to connect database:", err)
	}

	// Create whatsmeow container
	Container = sqlstore.NewWithDB(db, "postgres", nil)

	// Upgrade dengan context
	err = Container.Upgrade(context.Background())
	if err != nil {
		log.Fatal("Failed to upgrade database:", err)
	}

	log.Println("Database connected successfully")
}
