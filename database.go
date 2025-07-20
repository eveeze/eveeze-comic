// database.go
package main

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3" // Ganti driver ke SQLite
)

// InitDB sekarang menggunakan file lokal dan tidak memerlukan parameter
func InitDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "./eveeze.db")
	if err != nil {
		return nil, err
	}

	// Buat tabel watchlist
	queryWatchlist := `
    CREATE TABLE IF NOT EXISTS watchlist (
        manga_id TEXT NOT NULL,
        user_id TEXT NOT NULL,
        manga_title TEXT NOT NULL,
        user_progress_chapter_id TEXT,
        user_progress_chapter_number REAL,
        PRIMARY KEY (manga_id, user_id)
    );`
	if _, err = db.Exec(queryWatchlist); err != nil {
		return nil, err
	}

	// Buat tabel manga_updates
	queryMangaUpdates := `
	CREATE TABLE IF NOT EXISTS manga_updates (
		manga_id TEXT PRIMARY KEY,
		latest_known_chapter_id TEXT
	);`
	_, err = db.Exec(queryMangaUpdates)
	return db, err
}

// GANTI SEMUA PLACEHOLDER DARI $1, $2, DST. KEMBALI MENJADI ?
func AddToWatchlist(db *sql.DB, item WatchlistItem) error {
	query := `INSERT OR IGNORE INTO watchlist (manga_id, user_id, manga_title, user_progress_chapter_id, user_progress_chapter_number) 
              VALUES (?, ?, ?, ?, ?)`
	_, err := db.Exec(query, item.MangaID, item.UserID, item.MangaTitle, item.UserProgressChapterID, item.UserProgressChapterNumber)
	if err != nil {
		return err
	}

	updateQuery := `INSERT OR REPLACE INTO manga_updates (manga_id, latest_known_chapter_id) VALUES (?, ?)`
	_, err = db.Exec(updateQuery, item.MangaID, item.UserProgressChapterID)
	return err
}

func getUniqueMangaForUpdateCheck(db *sql.DB) (map[string]string, error) {
	query := `SELECT manga_id, latest_known_chapter_id FROM manga_updates`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mangaMap := make(map[string]string)
	for rows.Next() {
		var mangaID, chapterID string
		if err := rows.Scan(&mangaID, &chapterID); err != nil {
			return nil, err
		}
		mangaMap[mangaID] = chapterID
	}
	return mangaMap, nil
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

func updateLatestKnownChapter(db *sql.DB, mangaID, newChapterID string) error {
	query := `UPDATE manga_updates SET latest_known_chapter_id = ? WHERE manga_id = ?`
	_, err := db.Exec(query, newChapterID, mangaID)
	return err
}

func UpdateUserProgress(db *sql.DB, userID, mangaID, chapterID string, chapterNumber float64) error {
	query := `UPDATE watchlist SET user_progress_chapter_id = ?, user_progress_chapter_number = ? WHERE user_id = ? AND manga_id = ?`
	_, err := db.Exec(query, chapterID, chapterNumber, userID, mangaID)
	return err
}

func GetWatchlistForUserPaginated(db *sql.DB, userID string, page int, pageSize int) ([]WatchlistItem, int, error) {
	var totalItems int
	countQuery := `SELECT COUNT(*) FROM watchlist WHERE user_id = ?`
	err := db.QueryRow(countQuery, userID).Scan(&totalItems)
	if err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	query := `SELECT manga_id, user_id, manga_title, user_progress_chapter_id, user_progress_chapter_number FROM watchlist WHERE user_id = ? ORDER BY manga_title ASC LIMIT ? OFFSET ?`
	rows, err := db.Query(query, userID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []WatchlistItem
	for rows.Next() {
		var item WatchlistItem
		if err := rows.Scan(&item.MangaID, &item.UserID, &item.MangaTitle, &item.UserProgressChapterID, &item.UserProgressChapterNumber); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, totalItems, nil
}

func DeleteFromWatchlist(db *sql.DB, mangaID string, userID string) error {
	query := `DELETE FROM watchlist WHERE manga_id = ? AND user_id = ?`
	_, err := db.Exec(query, mangaID, userID)
	return err
}

func GetWatchlistItem(db *sql.DB, userID, mangaID string) (*WatchlistItem, error) {
	var item WatchlistItem
	query := `SELECT manga_id, user_id, manga_title, user_progress_chapter_id, user_progress_chapter_number FROM watchlist WHERE user_id = ? AND manga_id = ?`
	err := db.QueryRow(query, userID, mangaID).Scan(&item.MangaID, &item.UserID, &item.MangaTitle, &item.UserProgressChapterID, &item.UserProgressChapterNumber)
	if err != nil {
		return nil, err
	}
	return &item, nil
}