// database.go
package main

import (
	"database/sql"

	_ "github.com/lib/pq" // <-- Ganti driver dari sqlite3 ke pq
)

func InitDB(databaseURL string) (*sql.DB, error) {
	// Membuka koneksi menggunakan URL dari environment variable
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}

	// Cek koneksi
	if err = db.Ping(); err != nil {
		return nil, err
	}

	// Query CREATE TABLE tetap sama, sintaks ini kompatibel
	query := `
    CREATE TABLE IF NOT EXISTS watchlist (
        manga_id TEXT NOT NULL,
        user_id TEXT NOT NULL,
        manga_title TEXT NOT NULL,
        last_notified_chapter_id TEXT,
        PRIMARY KEY (manga_id, user_id)
    );`
	_, err = db.Exec(query)
	return db, err
}

func AddToWatchlist(db *sql.DB, item WatchlistItem) error {
	// Ganti placeholder dari '?' menjadi '$1, $2, ...'
	query := `INSERT INTO watchlist (manga_id, user_id, manga_title, last_notified_chapter_id) VALUES ($1, $2, $3, $4) ON CONFLICT (manga_id, user_id) DO NOTHING`
	_, err := db.Exec(query, item.MangaID, item.UserID, item.MangaTitle, item.LastNotifiedChapterID)
	return err
}

func getUniqueMangaFromWatchlist(db *sql.DB) ([]WatchlistItem, error) {
	query := `SELECT DISTINCT manga_id, manga_title, last_notified_chapter_id FROM watchlist`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []WatchlistItem
	for rows.Next() {
		var item WatchlistItem
		if err := rows.Scan(&item.MangaID, &item.MangaTitle, &item.LastNotifiedChapterID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func getUsersForManga(db *sql.DB, mangaID string) ([]string, error) {
	query := `SELECT user_id FROM watchlist WHERE manga_id = $1`
	rows, err := db.Query(query, mangaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs, nil
}

func updateAllUsersForManga(db *sql.DB, mangaID, newChapterID string) error {
	query := `UPDATE watchlist SET last_notified_chapter_id = $1 WHERE manga_id = $2`
	_, err := db.Exec(query, newChapterID, mangaID)
	return err
}

func GetWatchlistForUser(db *sql.DB, userID string) ([]WatchlistItem, error) {
	query := `SELECT manga_id, user_id, manga_title, last_notified_chapter_id FROM watchlist WHERE user_id = $1`
	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []WatchlistItem
	for rows.Next() {
		var item WatchlistItem
		if err := rows.Scan(&item.MangaID, &item.UserID, &item.MangaTitle, &item.LastNotifiedChapterID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func DeleteFromWatchlist(db *sql.DB, mangaID string, userID string) error {
	query := `DELETE FROM watchlist WHERE manga_id = $1 AND user_id = $2`
	_, err := db.Exec(query, mangaID, userID)
	return err
}

func GetWatchlistForUserPaginated(db *sql.DB, userID string, page int, pageSize int) ([]WatchlistItem, int, error) {
	var totalItems int
	countQuery := `SELECT COUNT(*) FROM watchlist WHERE user_id = $1`
	err := db.QueryRow(countQuery, userID).Scan(&totalItems)
	if err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	query := `SELECT manga_id, user_id, manga_title, last_notified_chapter_id FROM watchlist WHERE user_id = $1 ORDER BY manga_title ASC LIMIT $2 OFFSET $3`
	rows, err := db.Query(query, userID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []WatchlistItem
	for rows.Next() {
		var item WatchlistItem
		if err := rows.Scan(&item.MangaID, &item.UserID, &item.MangaTitle, &item.LastNotifiedChapterID); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, totalItems, nil
}
