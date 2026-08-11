package model

import (
	"context"
	"database/sql"
	"gowa-yourself/database"
	"strconv"
	"strings"
	"time"
)

// Outbox represents a record in the outbox table
type Outbox struct {
	IDOutbox        int            `json:"id_outbox"`
	Type            int            `json:"type"`
	FromNumber      sql.NullString `json:"from_number"`
	ClientID        sql.NullInt64  `json:"client_id"`
	Destination     string         `json:"destination"`
	Messages        string         `json:"messages"`
	Status          int            `json:"status"`
	Priority        int            `json:"priority"`
	Application     sql.NullString `json:"application"`
	SendingDateTime sql.NullTime   `json:"sending_datetime"`
	InsertDateTime  time.Time      `json:"insert_datetime"`
	TableID         sql.NullString `json:"table_id"`
	File            sql.NullString `json:"file"`
	ErrorCount      int            `json:"error_count"`
	MsgError        sql.NullString `json:"msg_error"`
}

// CreateOutboxBatch inserts multiple outbox records in a single transaction
func CreateOutboxBatch(ctx context.Context, records []Outbox) error {
	if len(records) == 0 {
		return nil
	}

	tx, err := database.OutboxDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO outbox (
			type, from_number, client_id, destination, messages, 
			status, priority, application, sendingDateTime, table_id, file
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	if database.OutboxDriver == "mysql" {
		query = `
			INSERT INTO outbox (
				type, from_number, client_id, destination, messages, 
				status, priority, application, sendingDateTime, table_id, file
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
	}

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range records {
		sendingTime := r.SendingDateTime
		if !sendingTime.Valid {
			sendingTime = sql.NullTime{Time: time.Now(), Valid: true}
		}
		_, err := stmt.ExecContext(
			ctx,
			r.Type,
			r.FromNumber,
			r.ClientID,
			r.Destination,
			r.Messages,
			r.Status,
			r.Priority,
			r.Application,
			sendingTime,
			r.TableID,
			r.File,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// CreateOutboxSingle inserts a single outbox record and returns the inserted ID
func CreateOutboxSingle(ctx context.Context, r *Outbox) error {
	if !r.SendingDateTime.Valid {
		r.SendingDateTime = sql.NullTime{Time: time.Now(), Valid: true}
	}
	if database.OutboxDriver == "mysql" {
		query := `
			INSERT INTO outbox (
				type, from_number, client_id, destination, messages, 
				status, priority, application, sendingDateTime, table_id, file
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		res, err := database.OutboxDB.ExecContext(
			ctx,
			query,
			r.Type,
			r.FromNumber,
			r.ClientID,
			r.Destination,
			r.Messages,
			r.Status,
			r.Priority,
			r.Application,
			r.SendingDateTime,
			r.TableID,
			r.File,
		)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		r.IDOutbox = int(id)
		r.InsertDateTime = time.Now()
		return nil
	}

	query := `
		INSERT INTO outbox (
			type, from_number, client_id, destination, messages, 
			status, priority, application, sendingDateTime, table_id, file
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id_outbox, insertDateTime
	`

	err := database.OutboxDB.QueryRowContext(
		ctx,
		query,
		r.Type,
		r.FromNumber,
		r.ClientID,
		r.Destination,
		r.Messages,
		r.Status,
		r.Priority,
		r.Application,
		r.SendingDateTime,
		r.TableID,
		r.File,
	).Scan(&r.IDOutbox, &r.InsertDateTime)

	return err
}

// GetOutboxQueue retrieves outbox records with optional filters and search
func GetOutboxQueue(ctx context.Context, status *int, application string, search string, limit, offset int) ([]Outbox, error) {
	var query string
	var args []interface{}
	isMySQL := database.OutboxDriver == "mysql"
	argCount := 1

	query = `
		SELECT id_outbox, type, from_number, client_id, destination, messages,
		       status, priority, application, sendingDateTime, insertDateTime, table_id, file, error_count, msg_error
		FROM outbox
		WHERE 1=1
	`

	placeholder := func(idx int) string {
		if isMySQL {
			return "?"
		}
		return "$" + strconv.Itoa(idx)
	}

	if status != nil {
		query += ` AND status = ` + placeholder(argCount)
		args = append(args, *status)
		argCount++
	}

	if application != "" {
		query += ` AND application = ` + placeholder(argCount)
		args = append(args, application)
		argCount++
	}

	if search != "" {
		if isMySQL {
			query += ` AND (LOWER(from_number) LIKE ` + placeholder(argCount) + ` OR LOWER(destination) LIKE ` + placeholder(argCount) + ` OR LOWER(messages) LIKE ` + placeholder(argCount) + ` OR LOWER(application) LIKE ` + placeholder(argCount) + `)`
		} else {
			query += ` AND (from_number ILIKE ` + placeholder(argCount) + ` OR destination ILIKE ` + placeholder(argCount) + ` OR messages ILIKE ` + placeholder(argCount) + ` OR application ILIKE ` + placeholder(argCount) + `)`
		}
		args = append(args, "%"+strings.ToLower(search)+"%")
		argCount++
	}

	query += ` ORDER BY insertDateTime DESC LIMIT ` + placeholder(argCount)
	args = append(args, limit)
	argCount++

	query += ` OFFSET ` + placeholder(argCount)
	args = append(args, offset)

	rows, err := database.OutboxDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []Outbox
	for rows.Next() {
		var r Outbox
		err := rows.Scan(
			&r.IDOutbox,
			&r.Type,
			&r.FromNumber,
			&r.ClientID,
			&r.Destination,
			&r.Messages,
			&r.Status,
			&r.Priority,
			&r.Application,
			&r.SendingDateTime,
			&r.InsertDateTime,
			&r.TableID,
			&r.File,
			&r.ErrorCount,
			&r.MsgError,
		)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}

	return records, rows.Err()
}

// GetOutboxQueueCount retrieves total records count for outbox queue matching filters and search
func GetOutboxQueueCount(ctx context.Context, status *int, application string, search string) (int, error) {
	var query string
	var args []interface{}
	isMySQL := database.OutboxDriver == "mysql"
	argCount := 1

	query = `
		SELECT COUNT(*)
		FROM outbox
		WHERE 1=1
	`

	placeholder := func(idx int) string {
		if isMySQL {
			return "?"
		}
		return "$" + strconv.Itoa(idx)
	}

	if status != nil {
		query += ` AND status = ` + placeholder(argCount)
		args = append(args, *status)
		argCount++
	}

	if application != "" {
		query += ` AND application = ` + placeholder(argCount)
		args = append(args, application)
		argCount++
	}

	if search != "" {
		if isMySQL {
			query += ` AND (LOWER(from_number) LIKE ` + placeholder(argCount) + ` OR LOWER(destination) LIKE ` + placeholder(argCount) + ` OR LOWER(messages) LIKE ` + placeholder(argCount) + ` OR LOWER(application) LIKE ` + placeholder(argCount) + `)`
		} else {
			query += ` AND (from_number ILIKE ` + placeholder(argCount) + ` OR destination ILIKE ` + placeholder(argCount) + ` OR messages ILIKE ` + placeholder(argCount) + ` OR application ILIKE ` + placeholder(argCount) + `)`
		}
		args = append(args, "%"+strings.ToLower(search)+"%")
		argCount++
	}

	var count int
	err := database.OutboxDB.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

// GetOutboxByID retrieves a single outbox record by ID
func GetOutboxByID(ctx context.Context, id int) (*Outbox, error) {
	query := `
		SELECT id_outbox, type, from_number, client_id, destination, messages,
		       status, priority, application, sendingDateTime, insertDateTime, table_id, file, error_count, msg_error
		FROM outbox
		WHERE id_outbox = $1
	`
	if database.OutboxDriver == "mysql" {
		query = `
			SELECT id_outbox, type, from_number, client_id, destination, messages,
			       status, priority, application, sendingDateTime, insertDateTime, table_id, file, error_count, msg_error
			FROM outbox
			WHERE id_outbox = ?
		`
	}

	var r Outbox
	err := database.OutboxDB.QueryRowContext(ctx, query, id).Scan(
		&r.IDOutbox,
		&r.Type,
		&r.FromNumber,
		&r.ClientID,
		&r.Destination,
		&r.Messages,
		&r.Status,
		&r.Priority,
		&r.Application,
		&r.SendingDateTime,
		&r.InsertDateTime,
		&r.TableID,
		&r.File,
		&r.ErrorCount,
		&r.MsgError,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &r, nil
}

// BulkUpdateOutboxStatus updates status of specific outbox record IDs
func BulkUpdateOutboxStatus(ctx context.Context, toStatus int, ids []int) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	isMySQL := database.OutboxDriver == "mysql"
	var query string
	var args []interface{}
	argCount := 1

	placeholder := func(idx int) string {
		if isMySQL {
			return "?"
		}
		return "$" + strconv.Itoa(idx)
	}

	query = "UPDATE outbox SET status = " + placeholder(argCount)
	args = append(args, toStatus)
	argCount++

	if toStatus == 0 {
		query += ", msg_error = NULL"
	} else if toStatus == 2 {
		query += ", msg_error = 'Canceled by user'"
	}

	var idPlaceholders []string
	for _, id := range ids {
		idPlaceholders = append(idPlaceholders, placeholder(argCount))
		args = append(args, id)
		argCount++
	}

	query += " WHERE id_outbox IN (" + strings.Join(idPlaceholders, ", ") + ")"

	res, err := database.OutboxDB.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

// CancelPendingOutboxForApp cancels existing pending (status 0) outbox records for a given destination and application
func CancelPendingOutboxForApp(ctx context.Context, destination string, application string) (int64, error) {
	if destination == "" {
		return 0, nil
	}

	isMySQL := database.OutboxDriver == "mysql"
	var query string
	var args []interface{}
	argCount := 1

	placeholder := func(idx int) string {
		if isMySQL {
			return "?"
		}
		return "$" + strconv.Itoa(idx)
	}

	// Alternate phone format support (08xx vs 628xx)
	var altDestination string
	if strings.HasPrefix(destination, "0") {
		altDestination = "62" + destination[1:]
	} else if strings.HasPrefix(destination, "62") {
		altDestination = "0" + destination[2:]
	}

	query = "UPDATE outbox SET status = 2, msg_error = 'Superseded by new request (replace_pending)' WHERE status = 0"

	if altDestination != "" {
		query += " AND (destination = " + placeholder(argCount) + " OR destination = " + placeholder(argCount+1) + ")"
		args = append(args, destination, altDestination)
		argCount += 2
	} else {
		query += " AND destination = " + placeholder(argCount)
		args = append(args, destination)
		argCount++
	}

	if application != "" {
		query += " AND LOWER(application) = LOWER(" + placeholder(argCount) + ")"
		args = append(args, application)
		argCount++
	}

	res, err := database.OutboxDB.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}
