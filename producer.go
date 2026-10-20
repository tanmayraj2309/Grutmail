package main

import (
	"database/sql"
	"fmt"
)

// loadRecipientFromDB fetches contacts with 'PENDING' status from PostgreSQL
// and streams them into the recipient channel for workers to process.
func loadRecipientFromDB(db *sql.DB, ch chan Recipient) error {
	defer close(ch)

	recipients, err := fetchPendingContacts(db)
	if err != nil {
		return fmt.Errorf("failed to load pending contacts from db: %w", err)
	}

	if len(recipients) == 0 {
		fmt.Println("ℹ️ No pending contacts found in PostgreSQL database to dispatch.")
		return nil
	}

	fmt.Printf("📬 [Producer] Queued %d pending recipients for worker processing.\n", len(recipients))

	for _, recipient := range recipients {
		ch <- recipient
	}

	return nil
}
