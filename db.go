package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
)

// initPostgres establishes connection to PostgreSQL and creates contacts table if not exists.
func initPostgres() (*sql.DB, error) {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://postgres:password123@localhost:5432/email_db?sslmode=disable"
	}
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	createTableQuery := `
	CREATE TABLE IF NOT EXISTS contacts (
		id SERIAL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		email VARCHAR(255) UNIQUE NOT NULL,
		status VARCHAR(50) DEFAULT 'PENDING',
		error_message TEXT,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err = db.Exec(createTableQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to create contacts table: %w", err)
	}

	return db, nil
}

// seedContactsFromCSV imports records from CSV into PostgreSQL if they do not already exist.
func seedContactsFromCSV(db *sql.DB, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open csv file: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read csv records: %w", err)
	}

	insertQuery := `
	INSERT INTO contacts (name, email, status)
	VALUES ($1, $2, 'PENDING')
	ON CONFLICT (email) DO NOTHING;
	`

	stmt, err := db.Prepare(insertQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer stmt.Close()

	insertedCount := 0
	for _, record := range records[1:] {
		res, err := stmt.Exec(record[0], record[1])
		if err != nil {
			log.Printf("Error inserting contact %s: %v\n", record[1], err)
			continue
		}
		rowsAffected, _ := res.RowsAffected()
		if rowsAffected > 0 {
			insertedCount++
		}
	}

	fmt.Printf("[PostgreSQL] Seeded %d new contacts from CSV into database.\n", insertedCount)
	return nil
}

// fetchPendingContacts returns all contacts that have status = 'PENDING'.
func fetchPendingContacts(db *sql.DB) ([]Recipient, error) {
	query := `SELECT id, name, email FROM contacts WHERE status = 'PENDING' ORDER BY id ASC;`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recipients []Recipient
	for rows.Next() {
		var r Recipient
		if err := rows.Scan(&r.ID, &r.Name, &r.Email); err != nil {
			return nil, err
		}
		recipients = append(recipients, r)
	}

	return recipients, nil
}

// updateContactStatus updates the delivery status and error message of an email in PostgreSQL.
func updateContactStatus(db *sql.DB, email string, status string, errorMessage string) error {
	query := `
	UPDATE contacts
	SET status = $1, error_message = $2, updated_at = CURRENT_TIMESTAMP
	WHERE email = $3;
	`
	_, err := db.Exec(query, status, errorMessage, email)
	return err
}

// Contact represents the full model for web UI display.
type Contact struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message"`
}

// fetchAllContacts returns all contacts stored in the PostgreSQL database.
func fetchAllContacts(db *sql.DB) ([]Contact, error) {
	query := `SELECT id, name, email, status, COALESCE(error_message, '') FROM contacts ORDER BY id ASC;`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Status, &c.ErrorMessage); err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, nil
}

// resetContactsStatus resets all contacts back to 'PENDING' for repeated test campaigns.
func resetContactsStatus(db *sql.DB) error {
	_, err := db.Exec(`UPDATE contacts SET status = 'PENDING', error_message = NULL, updated_at = CURRENT_TIMESTAMP;`)
	return err
}

// insertContact adds a single contact into the PostgreSQL database.
func insertContact(db *sql.DB, name, email string) error {
	query := `INSERT INTO contacts (name, email, status) VALUES ($1, $2, 'PENDING');`
	_, err := db.Exec(query, name, email)
	return err
}

