// database.go
package main

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

func InitDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "./watchlist.db")
	if err != nil {
		return nil, err
	}
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
	query := `INSERT OR IGNORE INTO watchlist (manga_id, user_id, manga_title, last_notified_chapter_id) VALUES (?, ?, ?, ?)`
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
	query := `SELECT user_id FROM watchlist WHERE manga_id = ?`
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
	query := `UPDATE watchlist SET last_notified_chapter_id = ? WHERE manga_id = ?`
	_, err := db.Exec(query, newChapterID, mangaID)
	return err
}

func GetWatchlistForUser(db *sql.DB, userID string) ([]WatchlistItem, error) {
	query := `SELECT manga_id, user_id, manga_title, last_notified_chapter_id FROM watchlist WHERE user_id = ?`
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
