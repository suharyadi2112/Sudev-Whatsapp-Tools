package model

import (
	"database/sql"
	"gowa-yourself/database"
	"time"
)

// Struct Instance sesuai field table
type Instance struct {
	ID              int64
	InstanceID      string
	PhoneNumber     sql.NullString
	JID             sql.NullString
	Status          string
	IsConnected     bool
	Name            sql.NullString
	ProfilePicture  sql.NullString
	About           sql.NullString
	Platform        sql.NullString
	BatteryLevel    sql.NullInt64
	BatteryCharging sql.NullBool
	QRCode          sql.NullString
	QRExpiresAt     sql.NullTime
	CreatedAt       time.Time
	ConnectedAt     sql.NullTime
	DisconnectedAt  sql.NullTime
	LastSeen        sql.NullTime
}

func InsertInstance(in *Instance) error {
	query := `
    INSERT INTO instances (
        instance_id, status, is_connected, created_at
    ) VALUES ($1, $2, $3, $4)`
	_, err := database.AppDB.Exec(
		query,
		in.InstanceID,
		in.Status,
		in.IsConnected,
		in.CreatedAt,
	)
	return err
}
