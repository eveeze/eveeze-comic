// discord_handler.go (Dengan tata letak tombol baru dan logika paginasi)
package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// Fungsi untuk membuat pesan pencarian (digunakan oleh search dan paginasi)
func createSearchResponseMessage(s *discordgo.Session, query string, page int) (*discordgo.WebhookEdit, error) {
	results, err := SearchManga(query, page)
	if err != nil || len(results.Data) == 0 {
		content := "Tidak ada hasil untuk: " + query
		return &discordgo.WebhookEdit{Content: &content}, nil
	}

	// Siapkan embeds dan komponen
	var embeds []*discordgo.MessageEmbed
	var components []discordgo.MessageComponent

	for _, manga := range results.Data {
		// 1. Buat embed untuk setiap manga
		embeds = append(embeds, &discordgo.MessageEmbed{
			Title:     manga.Title,
			Color:     0x00ff00,
			Thumbnail: &discordgo.MessageEmbedThumbnail{URL: manga.CoverURL},
		})
		// 2. Buat satu action row dengan tombol "Tambah" HANYA untuk manga ini
		components = append(components, discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    fmt.Sprintf("Tambah '%s'", manga.Title),
					Style:    discordgo.SuccessButton,
					CustomID: fmt.Sprintf("add_watchlist_%s_%s", manga.ID, manga.Title),
				},
			},
		})
	}

	// 3. Tambahkan tombol paginasi di paling bawah
	prevPage := page - 1
	nextPage := page + 1

	paginationRow := discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "◀️ Previous",
				Style:    discordgo.SecondaryButton,
				CustomID: fmt.Sprintf("page_%d_%s", prevPage, query),
				Disabled: page <= 1, // Nonaktifkan jika di halaman pertama
			},
			discordgo.Button{
				Label:    fmt.Sprintf("Page %d / %d", page, results.Meta.TotalPage),
				Style:    discordgo.SecondaryButton,
				CustomID: "page_indicator", // ID dummy, tidak bisa diklik
				Disabled: true,
			},
			discordgo.Button{
				Label:    "Next ▶️",
				Style:    discordgo.SecondaryButton,
				CustomID: fmt.Sprintf("page_%d_%s", nextPage, query),
				Disabled: page >= results.Meta.TotalPage, // Nonaktifkan jika di halaman terakhir
			},
		},
	}
	components = append(components, paginationRow)

	return &discordgo.WebhookEdit{
		Embeds:     &embeds,
		Components: &components,
	}, nil
}

func searchCommandHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Could not defer interaction for /search: %v", err)
		return
	}

	query := i.ApplicationCommandData().Options[0].StringValue()

	response, err := createSearchResponseMessage(s, query, 1) // Mulai dari halaman 1
	if err != nil {
		log.Printf("Error creating search response: %v", err)
		return
	}

	s.InteractionResponseEdit(i.Interaction, response)
}

func interactionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionMessageComponent {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
		return
	}

	customID := i.MessageComponentData().CustomID

	// Logika untuk tombol Add Watchlist
	if strings.HasPrefix(customID, "add_watchlist_") {
		// ... (logika ini sama seperti sebelumnya, tidak perlu diubah) ...
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
		})
		if err != nil {
			return
		}

		parts := strings.SplitN(customID, "_", 4)
		mangaID, mangaTitle := parts[2], parts[3]
		latestChapter, err := GetLatestChapter(mangaID)
		if err != nil {
			msg := "Gagal mendapatkan detail chapter."
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
			return
		}
		item := WatchlistItem{
			MangaID: mangaID, UserID: i.Member.User.ID, MangaTitle: mangaTitle,
			LastNotifiedChapterID: latestChapter.ID,
		}
		err = AddToWatchlist(db, item)
		if err != nil {
			msg := "Gagal menambahkan ke watchlist."
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
			return
		}
		msg := fmt.Sprintf("✅ **%s** berhasil ditambahkan!", mangaTitle)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
		return
	}

	// Logika baru untuk tombol Paginasi
	if strings.HasPrefix(customID, "page_") {
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage, // Update pesan yang ada, jangan buat baru
		})
		if err != nil {
			log.Printf("Could not respond to page interaction: %v", err)
			return
		}

		parts := strings.SplitN(customID, "_", 3)
		page, _ := strconv.Atoi(parts[1])
		query := parts[2]

		response, err := createSearchResponseMessage(s, query, page)
		if err != nil {
			log.Printf("Error creating search response for page %d: %v", page, err)
			return
		}

		s.InteractionResponseEdit(i.Interaction, response)
		return
	}
}

func watchlistCommandHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// ... (fungsi ini tidak perlu diubah)
	items, err := GetWatchlistForUser(db, i.Member.User.ID)
	if err != nil {
		log.Printf("Error getting watchlist for user %s: %v", i.Member.User.ID, err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Gagal mengambil watchlist.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}
	if len(items) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Watchlist Anda masih kosong.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}
	var description strings.Builder
	description.WriteString("Berikut adalah daftar pantauan Anda:\n\n")
	for idx, item := range items {
		description.WriteString(fmt.Sprintf("%d. **%s**\n", idx+1, item.MangaTitle))
	}
	embed := &discordgo.MessageEmbed{
		Title: "Watchlist Pribadi Anda 📖", Description: description.String(), Color: 0x00bfff,
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Flags: discordgo.MessageFlagsEphemeral},
	})
}
