package database

import (
	"database/sql"
	"log"

	_ "github.com/lib/pq"
)

var AppDB *sql.DB

// Inisialisasi koneksi ke database custom (bukan whatsmeow)
func InitAppDB(appDbURL string) {
	db, err := sql.Open("postgres", appDbURL)
	if err != nil {
		log.Fatal("Failed to connect app DB:", err)
	}
	AppDB = db
	err = AppDB.Ping()
	if err != nil {
		log.Fatal("Failed to ping app DB:", err)
	}
	log.Println("App DB (custom) connected successfully")
}
