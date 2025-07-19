// main.go (Dengan pembaruan struct untuk pagination)
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
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

// Struct untuk menampung 'meta' dari API
type APIMeta struct {
	TotalPage int `json:"total_page"`
	Page      int `json:"page"`
}

// Struct untuk response pencarian manga
type APIResponseManga struct {
	Data []Manga `json:"data"`
	Meta APIMeta `json:"meta"`
}

type APIResponseMangaDetail struct {
	Data Manga `json:"data"`
}

// Struct untuk response chapter
type APIResponseChapter struct {
	Data []Chapter `json:"data"`
}

// -- Fungsi Helper dan Inti --
func slugify(title string) string {
	lower := strings.ToLower(title)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug := re.ReplaceAllString(lower, "-")
	return strings.Trim(slug, "-")
}

// func checkForUpdates(s *discordgo.Session) {
// 	uniqueManga, err := getUniqueMangaFromWatchlist(db)
// 	if err != nil {
// 		log.Printf("Error getting unique manga for update check: %v", err)
// 		return
// 	}
// 	for _, item := range uniqueManga {
// 		latestChapter, err := GetLatestChapter(item.MangaID)
// 		if err != nil {
// 			log.Printf("Failed to get latest chapter for %s: %v", item.MangaTitle, err)
// 			continue
// 		}
// 		if latestChapter.ID != item.LastNotifiedChapterID {
// 			log.Printf("New chapter found for %s: %s", item.MangaTitle, latestChapter.ID)
// 			users, err := getUsersForManga(db, item.MangaID)
// 			if err != nil || len(users) == 0 {
// 				continue
// 			}
//
// 			var mentions []string
// 			for _, userID := range users {
// 				mentions = append(mentions, fmt.Sprintf("<@%s>", userID))
// 			}
// 			messageContent := strings.Join(mentions, " ")
// 			mangaURL := fmt.Sprintf("https://shinigami.id/series/%s", slugify(item.MangaTitle))
//
// 			notificationEmbed := &discordgo.MessageEmbed{
// 				Title:       item.MangaTitle,
// 				URL:         mangaURL,
// 				Description: fmt.Sprintf("## Chapter %.1f Telah Rilis!", latestChapter.Number),
// 				Color:       0xffa500,
// 			}
// 			_, err = s.ChannelMessageSendComplex(cfg.UpdateChannelID, &discordgo.MessageSend{
// 				Content: messageContent,
// 				Embed:   notificationEmbed,
// 			})
// 			if err != nil {
// 				log.Printf("Failed to send notification for %s: %v", item.MangaTitle, err)
// 				continue
// 			}
// 			err = updateAllUsersForManga(db, item.MangaID, latestChapter.ID)
// 			if err != nil {
// 				log.Printf("Failed to update last notified chapter for manga %s: %v", item.MangaID, err)
// 			}
// 		}
// 		time.Sleep(3 * time.Second)
// 	}
// }

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

			// Ambil detail manga untuk mendapatkan URL sampul
			mangaDetails, err := GetMangaDetails(item.MangaID)
			if err != nil {
				log.Printf("Failed to get manga details for %s: %v", item.MangaTitle, err)
				// Tetap lanjutkan tanpa gambar jika gagal
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

			// Parsing waktu rilis
			releaseTime, err := time.Parse(time.RFC3339, latestChapter.ReleaseDate)
			var timestamp string
			if err == nil {
				timestamp = releaseTime.Format(time.RFC3339)
			}

			// -- PEMBUATAN EMBED BARU YANG LEBIH INFORMATIF --
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
					IconURL: "https://i.imgur.com/R4Ifj2p.png", // Contoh URL ikon
				},
				Timestamp: timestamp,
			}

			// Tambahkan thumbnail jika berhasil didapatkan
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

func main() {
	var err error
	cfg, err = LoadConfig()
	if err != nil {
		log.Fatalf("Error loading config.json: %v", err)
	}

	db, err = InitDB()
	if err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}
	defer db.Close()

	s, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		log.Fatalf("Invalid bot parameters: %v", err)
	}

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
	go func() {
		for {
			log.Println("Checking for updates...")
			checkForUpdates(s)
			<-ticker.C
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	log.Println("Bot is running. Press Ctrl+C to exit.")
	<-stop

	log.Println("Gracefully shutting down.")
}
