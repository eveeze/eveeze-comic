// main.go
package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http" // Diperlukan untuk server keep-alive
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	// Hapus import pq jika tidak ada file lain yang butuh
)

// -- Variabel dan Struct Global --
var (
	db  *sql.DB
	cfg *Config

	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "search",
			Description: "Mencari manhwa berdasarkan judul",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "judul",
					Description: "Judul manhwa yang ingin dicari",
					Required:    true,
				},
			},
		},
		{
			Name:        "watchlist",
			Description: "Melihat daftar watchlist pribadimu",
		},
	}
	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"search":    searchCommandHandler,
		"watchlist": watchlistCommandHandler,
	}
)

type WatchlistItem struct {
	MangaID                   string
	UserID                    string
	MangaTitle                string
	UserProgressChapterID     string
	UserProgressChapterNumber float64
}

type Manga struct {
	ID          string `json:"manga_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	CoverURL    string `json:"cover_image_url"`
}

type Chapter struct {
	ID          string  `json:"chapter_id"`
	Number      float64 `json:"chapter_number"`
	ReleaseDate string  `json:"release_date"`
}

type APIMeta struct {
	TotalPage int `json:"total_page"`
	Page      int `json:"page"`
}

type APIResponseManga struct {
	Data []Manga `json:"data"`
	Meta APIMeta `json:"meta"`
}

type APIResponseMangaDetail struct {
	Data Manga `json:"data"`
}

type APIResponseChapter struct {
	Data []Chapter `json:"data"`
}

func checkForUpdates(s *discordgo.Session) {
	mangaToCheck, err := getUniqueMangaForUpdateCheck(db)
	if err != nil {
		log.Printf("Error getting unique manga for update check: %v", err)
		return
	}

	for mangaID, knownChapterID := range mangaToCheck {
		latestChapter, err := GetLatestChapter(mangaID)
		if err != nil {
			log.Printf("Failed to get latest chapter for mangaID %s: %v", mangaID, err)
			continue
		}

		if latestChapter.ID != knownChapterID {
			mangaDetails, err := GetMangaDetails(mangaID)
			if err != nil {
				log.Printf("Failed to get details for mangaID %s: %v", mangaID, err)
				continue
			}

			log.Printf("New chapter found for %s: %s", mangaDetails.Title, latestChapter.ID)

			users, err := getUsersForManga(db, mangaID)
			if err != nil || len(users) == 0 {
				continue
			}

			var mentions []string
			for _, userID := range users {
				mentions = append(mentions, fmt.Sprintf("<@%s>", userID))
			}
			messageContent := strings.Join(mentions, " ")
			chapterURL := fmt.Sprintf("%s/chapter/%s", cfg.ReaderBaseURL, latestChapter.ID)

			releaseTime, err := time.Parse(time.RFC3339, latestChapter.ReleaseDate)
			var timestamp string
			if err == nil {
				timestamp = releaseTime.Format(time.RFC3339)
			}
			notificationEmbed := &discordgo.MessageEmbed{
				Author: &discordgo.MessageEmbedAuthor{Name: "🔔 Chapter Baru Telah Rilis!"},
				Title:  mangaDetails.Title,
				URL:    chapterURL,
				Color:  0xffa500,
				Fields: []*discordgo.MessageEmbedField{
					{Name: "Chapter Terbaru", Value: fmt.Sprintf("%.1f", latestChapter.Number), Inline: true},
					{Name: "Tanggal Rilis", Value: releaseTime.Format("02 Jan 2006, 15:04 WIB"), Inline: true},
				},
				Footer:    &discordgo.MessageEmbedFooter{Text: "Eveeze Comic Bot", IconURL: "https://i.imgur.com/R4Ifj2p.png"},
				Timestamp: timestamp,
				Thumbnail: &discordgo.MessageEmbedThumbnail{URL: mangaDetails.CoverURL},
			}

			_, err = s.ChannelMessageSendComplex(cfg.UpdateChannelID, &discordgo.MessageSend{
				Content: messageContent,
				Embed:   notificationEmbed,
			})
			if err != nil {
				log.Printf("Failed to send notification for %s: %v", mangaDetails.Title, err)
				continue
			}
			
			err = updateLatestKnownChapter(db, mangaID, latestChapter.ID)
			if err != nil {
				log.Printf("Failed to update latest known chapter for manga %s: %v", mangaID, err)
			}
		}
		time.Sleep(3 * time.Second)
	}
}

// -- Fungsi Main (Dengan Perubahan) --
func main() {
	var err error

	cfg = LoadConfig()

	// Panggil InitDB tanpa parameter
	db, err = InitDB()
	if err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}
	defer db.Close()

	s, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		log.Fatalf("Invalid bot parameters: %v", err)
	}

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "Bot is alive and running!")
		})
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		log.Printf("Starting keep-alive server on port %s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Printf("Keep-alive server failed to start: %v", err)
		}
	}()

	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
	})
	s.AddHandler(interactionHandler)

	err = s.Open()
	if err != nil {
		log.Fatalf("Cannot open the session: %v", err)
	}
	defer s.Close()

	log.Println("Adding commands...")
	for _, v := range commands {
		_, err := s.ApplicationCommandCreate(s.State.User.ID, "", v)
		if err != nil {
			log.Fatalf("Cannot create '%v' command: %v", v.Name, err)
		}
	}
	log.Println("Commands added.")

	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	go func() {
		log.Println("Performing initial update check...")
		checkForUpdates(s)

		for range ticker.C {
			log.Println("Checking for updates...")
			checkForUpdates(s)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	log.Println("Bot is running. Press Ctrl+C to exit.")
	<-stop

	log.Println("Gracefully shutting down.")
}