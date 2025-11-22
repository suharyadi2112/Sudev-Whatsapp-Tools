package model

import (
	"database/sql"
	"errors"
	"fmt"
	"gowa-yourself/database"
	"log"
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
	SessionData     []byte // tambahkan ini
}

type InstanceResp struct {
	ID                int64     `json:"id"`
	InstanceID        string    `json:"instanceId"`
	PhoneNumber       string    `json:"phoneNumber"`
	JID               string    `json:"jid"`
	Status            string    `json:"status"`
	IsConnected       bool      `json:"isConnected"`
	Name              string    `json:"name"`
	ProfilePicture    string    `json:"profilePicture"`
	About             string    `json:"about"`
	Platform          string    `json:"platform"`
	BatteryLevel      int64     `json:"batteryLevel"`
	BatteryCharging   bool      `json:"batteryCharging"`
	QRCode            string    `json:"qrCode"`
	QRExpiresAt       time.Time `json:"qrExpiresAt"`
	CreatedAt         time.Time `json:"createdAt"`
	ConnectedAt       time.Time `json:"connectedAt"`
	DisconnectedAt    time.Time `json:"disconnectedAt"`
	LastSeen          time.Time `json:"lastSeen"`
	ExistsInWhatsmeow bool      `json:"existsInWhatsmeow"`
}

var ErrNoActiveInstance = errors.New("no active instance for this phone number")

// GetActiveInstanceByPhoneNumber mengembalikan instance aktif (terbaru) untuk nomor tertentu.
func GetActiveInstanceByPhoneNumber(phoneNumber string) (*Instance, error) {
	query := `
        SELECT
            id,
            instance_id,
            phone_number,
            jid,
            status,
            is_connected,
            name,
            profile_picture,
            about,
            platform,
            battery_level,
            battery_charging,
            qr_code,
            qr_expires_at,
            created_at,
            connected_at,
            disconnected_at,
            last_seen,
            session_data
        FROM instances
        WHERE phone_number = $1
          AND status = 'online'
          AND is_connected = true
        ORDER BY connected_at DESC, created_at DESC
        LIMIT 1
    `

	inst := &Instance{}
	err := database.AppDB.QueryRow(query, phoneNumber).Scan(
		&inst.ID,
		&inst.InstanceID,
		&inst.PhoneNumber,
		&inst.JID,
		&inst.Status,
		&inst.IsConnected,
		&inst.Name,
		&inst.ProfilePicture,
		&inst.About,
		&inst.Platform,
		&inst.BatteryLevel,
		&inst.BatteryCharging,
		&inst.QRCode,
		&inst.QRExpiresAt,
		&inst.CreatedAt,
		&inst.ConnectedAt,
		&inst.DisconnectedAt,
		&inst.LastSeen,
		&inst.SessionData,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNoActiveInstance
		}
		return nil, err
	}

	return inst, nil
}

// insert informasi instance ke table database custom
func InsertInstance(in *Instance) error {
	query := `
    INSERT INTO instances (
        instance_id, status, is_connected, created_at, session_data
    ) VALUES ($1, $2, $3, $4, $5)`
	_, err := database.AppDB.Exec(
		query,
		in.InstanceID,
		in.Status,
		in.IsConnected,
		in.CreatedAt,
		in.SessionData, // <- pastikan mengisi ini (bisa nil untuk awal)
	)
	return err
}

// update status qr seperti expired dll
func UpdateInstanceQR(instanceID, qr string, expiresAt time.Time) error {
	query := `
        UPDATE instances
        SET qr_code = $1, qr_expires_at = $2, status = $3
        WHERE instance_id = $4
    `
	_, err := database.AppDB.Exec(query, qr, expiresAt, "qr_required", instanceID)
	return err
}

// Ambil semua instance dari database custom
func GetAllInstances() ([]Instance, error) {
	query := `
        SELECT 
            id,
            instance_id,
            phone_number,
            jid,
            status,
            is_connected,
            name,
            profile_picture,
            about,
            platform,
            battery_level,
            battery_charging,
            qr_code,
            qr_expires_at,
            created_at,
            connected_at,
            disconnected_at,
            last_seen,
            session_data
        FROM instances
        ORDER BY created_at DESC
    `

	rows, err := database.AppDB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var instances []Instance
	for rows.Next() {
		var inst Instance

		err = rows.Scan(
			&inst.ID,
			&inst.InstanceID,
			&inst.PhoneNumber,
			&inst.JID,
			&inst.Status,
			&inst.IsConnected,
			&inst.Name,
			&inst.ProfilePicture,
			&inst.About,
			&inst.Platform,
			&inst.BatteryLevel,
			&inst.BatteryCharging,
			&inst.QRCode,
			&inst.QRExpiresAt,
			&inst.CreatedAt,
			&inst.ConnectedAt,
			&inst.DisconnectedAt,
			&inst.LastSeen,
			&inst.SessionData,
		)

		if err != nil {
			log.Println("Scan error GetAllInstances():", err)
			continue
		}

		instances = append(instances, inst)
	}

	return instances, nil
}

// update untuk eventHandler whatsapp
func UpdateInstanceOnConnected(instanceID, jid, phoneNumber, platform string) error {
	query := `
        UPDATE instances
        SET
            jid = $1,
            phone_number = $2,
            platform = $3,
            status = 'online',
            is_connected = true,
            connected_at = NOW(),
            last_seen = NOW()
        WHERE instance_id = $4
    `
	_, err := database.AppDB.Exec(query, jid, phoneNumber, platform, instanceID)
	return err
}

func UpdateInstanceOnDisconnected(instanceID string) error {
	query := `
        UPDATE instances
        SET
            status = 'disconnected',
            is_connected = false,
            disconnected_at = NOW()
        WHERE instance_id = $1
    `
	_, err := database.AppDB.Exec(query, instanceID)
	return err
}

func UpdateInstanceOnLoggedOut(instanceID string) error {
	query := `
        UPDATE instances
        SET
            status = 'logged_out',
            is_connected = false,
            disconnected_at = NOW()
        WHERE instance_id = $1
    `
	_, err := database.AppDB.Exec(query, instanceID)
	return err
}

// update status by logout api
func UpdateInstanceStatus(instanceID, status string, isConnected bool, disconnectedAt time.Time) error {
	query := `
        UPDATE instances
        SET status = $1, is_connected = $2, disconnected_at = $3
        WHERE instance_id = $4
    `
	_, err := database.AppDB.Exec(query, status, isConnected, disconnectedAt, instanceID)
	return err
}

// Get instance by JID
func GetInstanceByJID(jid string) (*Instance, error) {

	query := `
        SELECT
            id,
            instance_id,
            phone_number,
            jid,
            status,
            is_connected,
            name,
            profile_picture,
            about,
            platform,
            battery_level,
            battery_charging,
            qr_code,
            qr_expires_at,
            created_at,
            connected_at,
            disconnected_at,
            last_seen,
            session_data
        FROM instances
        WHERE jid = $1
        ORDER BY created_at DESC
        LIMIT 1
    `

	inst := &Instance{}
	err := database.AppDB.QueryRow(query, jid).Scan(
		&inst.ID,
		&inst.InstanceID,
		&inst.PhoneNumber,
		&inst.JID,
		&inst.Status,
		&inst.IsConnected,
		&inst.Name,
		&inst.ProfilePicture,
		&inst.About,
		&inst.Platform,
		&inst.BatteryLevel,
		&inst.BatteryCharging,
		&inst.QRCode,
		&inst.QRExpiresAt,
		&inst.CreatedAt,
		&inst.ConnectedAt,
		&inst.DisconnectedAt,
		&inst.LastSeen,
		&inst.SessionData,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err // biar caller bisa bedakan ErrNoRows
		}
		return nil, fmt.Errorf("GetInstanceByJID scan error: %w", err)
	}

	return inst, nil
}

// Get instance by INSTANCE ID
func GetInstanceByInstanceID(instanceID string) (*Instance, error) {

	query := `
        SELECT
            id,
            instance_id,
            phone_number,
            jid,
            status,
            is_connected,
            name,
            profile_picture,
            about,
            platform,
            battery_level,
            battery_charging,
            qr_code,
            qr_expires_at,
            created_at,
            connected_at,
            disconnected_at,
            last_seen,
            session_data
        FROM instances
        WHERE instance_id = $1
        LIMIT 1
    `

	inst := &Instance{}

	var (
		jidNS            sql.NullString
		phoneNS          sql.NullString
		nameNS           sql.NullString
		profileNS        sql.NullString
		aboutNS          sql.NullString
		platformNS       sql.NullString
		qrCodeNS         sql.NullString
		qrExpiresAtNT    sql.NullTime
		connectedAtNT    sql.NullTime
		disconnectedAtNT sql.NullTime
		lastSeenNT       sql.NullTime
	)

	err := database.AppDB.QueryRow(query, instanceID).Scan(
		&inst.ID,
		&inst.InstanceID,
		&phoneNS,
		&jidNS,
		&inst.Status,
		&inst.IsConnected,
		&nameNS,
		&profileNS,
		&aboutNS,
		&platformNS,
		&inst.BatteryLevel,
		&inst.BatteryCharging,
		&qrCodeNS,
		&qrExpiresAtNT,
		&inst.CreatedAt,
		&connectedAtNT,
		&disconnectedAtNT,
		&lastSeenNT,
		&inst.SessionData,
	)
	if err != nil {
		return nil, err
	}

	// Assign dari variabel Null* ke field struct
	inst.QRCode = qrCodeNS           // ← tambahkan baris ini
	inst.QRExpiresAt = qrExpiresAtNT // ← dan ini

	return inst, nil
}

// Hapus instance table custom
func DeleteInstanceByInstanceID(instanceID string) error {
	_, err := database.AppDB.Exec(`DELETE FROM instances WHERE instance_id = $1`, instanceID)
	return err
}

func ToResponse(inst Instance) InstanceResp {
	resp := InstanceResp{
		ID:              inst.ID,
		InstanceID:      inst.InstanceID,
		JID:             inst.JID.String,
		Status:          inst.Status,
		IsConnected:     inst.IsConnected,
		BatteryLevel:    0,
		BatteryCharging: false,
	}

	if inst.PhoneNumber.Valid {
		resp.PhoneNumber = inst.PhoneNumber.String
	}
	if inst.Name.Valid {
		resp.Name = inst.Name.String
	}
	if inst.ProfilePicture.Valid {
		resp.ProfilePicture = inst.ProfilePicture.String
	}
	if inst.About.Valid {
		resp.About = inst.About.String
	}
	if inst.Platform.Valid {
		resp.Platform = inst.Platform.String
	}
	if inst.BatteryLevel.Valid {
		resp.BatteryLevel = inst.BatteryLevel.Int64
	}
	if inst.BatteryCharging.Valid {
		resp.BatteryCharging = inst.BatteryCharging.Bool
	}
	if inst.QRCode.Valid {
		resp.QRCode = inst.QRCode.String
	}
	if inst.QRExpiresAt.Valid {
		resp.QRExpiresAt = inst.QRExpiresAt.Time
	}
	resp.CreatedAt = inst.CreatedAt
	if inst.ConnectedAt.Valid {
		resp.ConnectedAt = inst.ConnectedAt.Time
	}
	if inst.DisconnectedAt.Valid {
		resp.DisconnectedAt = inst.DisconnectedAt.Time
	}
	if inst.LastSeen.Valid {
		resp.LastSeen = inst.LastSeen.Time
	}

	return resp
}
