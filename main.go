package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http" // Diperlukan untuk server keep-alive
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	_ "github.com/lib/pq" // Driver untuk PostgreSQL
)

// -- Variabel dan Struct Global (Tidak ada perubahan di sini) --
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
	MangaID               string
	UserID                string
	MangaTitle            string
	LastNotifiedChapterID string
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

// -- Fungsi Helper dan Inti (Tidak ada perubahan di sini) --
func slugify(title string) string {
	lower := strings.ToLower(title)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug := re.ReplaceAllString(lower, "-")
	return strings.Trim(slug, "-")
}

func checkForUpdates(s *discordgo.Session) {
	uniqueManga, err := getUniqueMangaFromWatchlist(db)
	if err != nil {
		log.Printf("Error getting unique manga for update check: %v", err)
		return
	}
	for _, item := range uniqueManga {
		latestChapter, err := GetLatestChapter(item.MangaID)
		if err != nil {
			log.Printf("Failed to get latest chapter for %s: %v", item.MangaTitle, err)
			continue
		}
		if latestChapter.ID != item.LastNotifiedChapterID {
			log.Printf("New chapter found for %s: %s", item.MangaTitle, latestChapter.ID)

			mangaDetails, err := GetMangaDetails(item.MangaID)
			if err != nil {
				log.Printf("Failed to get manga details for %s: %v", item.MangaTitle, err)
			}

			users, err := getUsersForManga(db, item.MangaID)
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
				Author: &discordgo.MessageEmbedAuthor{
					Name: "🔔 Chapter Baru Telah Rilis!",
				},
				Title: item.MangaTitle,
				URL:   chapterURL,
				Color: 0xffa500, // Oranye
				Fields: []*discordgo.MessageEmbedField{
					{
						Name:   "Chapter Terbaru",
						Value:  fmt.Sprintf("%.1f", latestChapter.Number),
						Inline: true,
					},
					{
						Name:   "Tanggal Rilis",
						Value:  releaseTime.Format("02 Jan 2006, 15:04 WIB"),
						Inline: true,
					},
				},
				Footer: &discordgo.MessageEmbedFooter{
					Text:    "Eveeze Comic Bot",
					IconURL: "https://i.imgur.com/R4Ifj2p.png",
				},
				Timestamp: timestamp,
			}

			if mangaDetails != nil {
				notificationEmbed.Thumbnail = &discordgo.MessageEmbedThumbnail{
					URL: mangaDetails.CoverURL,
				}
			}

			_, err = s.ChannelMessageSendComplex(cfg.UpdateChannelID, &discordgo.MessageSend{
				Content: messageContent,
				Embed:   notificationEmbed,
			})
			if err != nil {
				log.Printf("Failed to send notification for %s: %v", item.MangaTitle, err)
				continue
			}
			err = updateAllUsersForManga(db, item.MangaID, latestChapter.ID)
			if err != nil {
				log.Printf("Failed to update last notified chapter for manga %s: %v", item.MangaID, err)
			}
		}
		time.Sleep(3 * time.Second)
	}
}

// -- Fungsi Main (Dengan Perubahan) --
func main() {
	var err error

	// 1. Memuat konfigurasi dari Environment Variables
	cfg = LoadConfig()

	// 2. Menginisialisasi koneksi ke database PostgreSQL
	db, err = InitDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}
	defer db.Close()

	// 3. Membuat sesi Discord
	s, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		log.Fatalf("Invalid bot parameters: %v", err)
	}

	// 4. Menambahkan server keep-alive untuk Render
	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "Bot is alive and running!")
		})
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080" // Port default untuk testing lokal
		}
		log.Printf("Starting keep-alive server on port %s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			// Gunakan log.Printf agar tidak menghentikan bot utama jika server ini gagal
			log.Printf("Keep-alive server failed to start: %v", err)
		}
	}()

	// 5. Menambahkan handler dan membuka koneksi
	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
	})
	s.AddHandler(interactionHandler)

	err = s.Open()
	if err != nil {
		log.Fatalf("Cannot open the session: %v", err)
	}
	defer s.Close()

	// 6. Mendaftarkan slash commands
	log.Println("Adding commands...")
	for _, v := range commands {
		_, err := s.ApplicationCommandCreate(s.State.User.ID, "", v)
		if err != nil {
			log.Fatalf("Cannot create '%v' command: %v", v.Name, err)
		}
	}
	log.Println("Commands added.")

	// 7. Menjalankan ticker untuk pengecekan update
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	go func() {
		// Jalankan pengecekan pertama kali saat bot start
		log.Println("Performing initial update check...")
		checkForUpdates(s)

		// Loop untuk pengecekan berikutnya sesuai ticker
		for range ticker.C {
			log.Println("Checking for updates...")
			checkForUpdates(s)
		}
	}()

	// 8. Menunggu sinyal untuk shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	log.Println("Bot is running. Press Ctrl+C to exit.")
	<-stop

	log.Println("Gracefully shutting down.")
}
