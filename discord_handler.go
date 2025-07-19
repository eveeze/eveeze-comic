// discord_handler.go (Paginated Watchlist with Single Embed - Complete Version)
package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func searchCommandHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Could not defer interaction for /search: %v", err)
		return
	}
	query := i.ApplicationCommandData().Options[0].StringValue()
	response, err := createSearchResponseMessage(s, query, 1)
	if err != nil {
		log.Printf("Error creating search response: %v", err)
		return
	}
	s.InteractionResponseEdit(i.Interaction, response)
}

func watchlistCommandHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("Could not defer /watchlist: %v", err)
		return
	}

	log.Println("Fetching watchlist for user:", i.Member.User.ID)
	response, err := createWatchlistResponseMessage(s, i.Member.User.ID, 1)
	if err != nil {
		log.Printf("Error creating watchlist response: %v", err)
		content := "Gagal mengambil watchlist."
		response = &discordgo.WebhookEdit{Content: &content}
	}

	log.Println("Sending final watchlist response for user:", i.Member.User.ID)
	_, err = s.InteractionResponseEdit(i.Interaction, response)
	if err != nil {
		log.Printf("Error editing interaction response: %v", err)
	} else {
		log.Println("Successfully sent watchlist response for user:", i.Member.User.ID)
	}
}

func createWatchlistResponseMessage(s *discordgo.Session, userID string, page int) (*discordgo.WebhookEdit, error) {
	pageSize := 10 // Karena menggunakan satu embed, bisa lebih banyak item per halaman
	items, totalItems, err := GetWatchlistForUserPaginated(db, userID, page, pageSize)
	if err != nil {
		log.Printf("Error getting paginated watchlist: %v", err)
		return nil, err
	}

	if totalItems == 0 {
		content := "📚 Watchlist Anda masih kosong.\nGunakan `/search` untuk mencari manga dan menambahkannya ke watchlist!"
		return &discordgo.WebhookEdit{Content: &content}, nil
	}

	var embeds []*discordgo.MessageEmbed
	var components []discordgo.MessageComponent

	// Buat SATU embed dengan multiple fields untuk semua manga
	var fields []*discordgo.MessageEmbedField
	for i, item := range items {
		numberLabel := (page-1)*pageSize + i + 1
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("%d. %s", numberLabel, item.MangaTitle),
			Value:  fmt.Sprintf("📖 **ID:** `%s`\n🗑️ *Gunakan tombol %d di bawah untuk menghapus*", item.MangaID, numberLabel),
			Inline: false,
		})
	}

	totalPages := (totalItems + pageSize - 1) / pageSize
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("📚 Watchlist Anda"),
		Description: fmt.Sprintf("Menampilkan manga yang Anda ikuti. Total: **%d manga**", totalItems),
		Color:       0x00bfff,
		Fields:      fields,
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Halaman %d dari %d | %d manga", page, totalPages, totalItems),
		},
		Timestamp: "", // Bisa ditambahkan timestamp jika diperlukan
	}
	embeds = append(embeds, embed)

	// Buat tombol hapus dengan nomor urut untuk mudah diidentifikasi
	var currentRow []discordgo.MessageComponent
	buttonsPerRow := 0
	maxButtonsPerRow := 5
	maxActionRows := 4 // Sisakan 1 untuk pagination

	for i, item := range items {
		numberLabel := (page-1)*pageSize + i + 1
		deleteButton := discordgo.Button{
			Label:    fmt.Sprintf("🗑️ %d", numberLabel),
			Style:    discordgo.DangerButton,
			CustomID: fmt.Sprintf("delete_watchlist_%s", item.MangaID),
		}

		currentRow = append(currentRow, deleteButton)
		buttonsPerRow++

		// Jika sudah mencapai max buttons per row atau ini item terakhir
		if buttonsPerRow == maxButtonsPerRow || i == len(items)-1 {
			actionRow := discordgo.ActionsRow{
				Components: currentRow,
			}
			components = append(components, actionRow)

			// Reset untuk row berikutnya
			currentRow = []discordgo.MessageComponent{}
			buttonsPerRow = 0

			// Jika sudah mencapai max ActionRows, hentikan
			if len(components) >= maxActionRows {
				break
			}
		}
	}

	// Tambahkan pagination controls di baris terakhir (jika ada lebih dari 1 halaman)
	if totalPages > 1 {
		prevPage := page - 1
		nextPage := page + 1

		paginationRow := discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "◀️ Previous",
					Style:    discordgo.SecondaryButton,
					CustomID: fmt.Sprintf("watchlist_page_%d_%s", prevPage, userID),
					Disabled: page <= 1,
				},
				discordgo.Button{
					Label:    fmt.Sprintf("Page %d / %d", page, totalPages),
					Style:    discordgo.SecondaryButton,
					CustomID: "watchlist_page_indicator",
					Disabled: true,
				},
				discordgo.Button{
					Label:    "Next ▶️",
					Style:    discordgo.SecondaryButton,
					CustomID: fmt.Sprintf("watchlist_page_%d_%s", nextPage, userID),
					Disabled: page >= totalPages,
				},
			},
		}
		components = append(components, paginationRow)
	}

	log.Printf("Created paginated watchlist response with %d embeds and %d ActionRows for page %d/%d",
		len(embeds), len(components), page, totalPages)

	return &discordgo.WebhookEdit{
		Embeds:     &embeds,
		Components: &components,
	}, nil
}

// Helper function untuk memotong judul yang terlalu panjang
func truncateTitle(title string, maxLength int) string {
	if len(title) <= maxLength {
		return title
	}
	return title[:maxLength-3] + "..."
}

func interactionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type == discordgo.InteractionApplicationCommand {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
		return
	}
	if i.Type == discordgo.InteractionMessageComponent {
		customID := i.MessageComponentData().CustomID

		if strings.HasPrefix(customID, "add_watchlist_") {
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
			})
			if err != nil {
				log.Printf("Error responding to add_watchlist interaction: %v", err)
				return
			}

			parts := strings.SplitN(customID, "_", 4)
			if len(parts) < 4 {
				msg := "Invalid button data."
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
				return
			}

			mangaID, mangaTitle := parts[2], parts[3]
			latestChapter, err := GetLatestChapter(mangaID)
			if err != nil {
				log.Printf("Error getting latest chapter for %s: %v", mangaID, err)
				msg := "Gagal mendapatkan detail chapter."
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
				return
			}

			item := WatchlistItem{
				MangaID:               mangaID,
				UserID:                i.Member.User.ID,
				MangaTitle:            mangaTitle,
				LastNotifiedChapterID: latestChapter.ID,
			}

			err = AddToWatchlist(db, item)
			if err != nil {
				log.Printf("Error adding to watchlist: %v", err)
				msg := "Gagal menambahkan ke watchlist."
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
				return
			}

			msg := fmt.Sprintf("✅ **%s** berhasil ditambahkan ke watchlist!", mangaTitle)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
			return
		}

		if strings.HasPrefix(customID, "delete_watchlist_") {
			mangaID := strings.TrimPrefix(customID, "delete_watchlist_")
			userID := i.Member.User.ID

			log.Printf("Deleting manga %s from watchlist for user %s", mangaID, userID)

			err := DeleteFromWatchlist(db, mangaID, userID)
			if err != nil {
				log.Printf("Failed to delete from watchlist: %v", err)
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "❌ Gagal menghapus item dari watchlist.",
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
				return
			}

			// Berhasil dihapus - berikan feedback dan refresh halaman
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "✅ Manga berhasil dihapus dari watchlist!\nJalankan `/watchlist` lagi untuk melihat daftar terbaru.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		// Handle pagination untuk watchlist
		if strings.HasPrefix(customID, "watchlist_page_") {
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseUpdateMessage,
			})
			if err != nil {
				log.Printf("Could not respond to watchlist page interaction: %v", err)
				return
			}

			parts := strings.SplitN(customID, "_", 4)
			if len(parts) < 4 {
				log.Printf("Invalid watchlist page button data: %s", customID)
				return
			}

			page, err := strconv.Atoi(parts[2])
			if err != nil {
				log.Printf("Invalid page number: %s", parts[2])
				return
			}

			userID := parts[3]
			response, err := createWatchlistResponseMessage(s, userID, page)
			if err != nil {
				log.Printf("Error creating watchlist response for page %d: %v", page, err)
				return
			}

			s.InteractionResponseEdit(i.Interaction, response)
			return
		}

		// Handle pagination untuk search (tetap sama)
		if strings.HasPrefix(customID, "page_") {
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseUpdateMessage,
			})
			if err != nil {
				log.Printf("Could not respond to page interaction: %v", err)
				return
			}

			parts := strings.SplitN(customID, "_", 3)
			if len(parts) < 3 {
				log.Printf("Invalid page button data: %s", customID)
				return
			}

			page, err := strconv.Atoi(parts[1])
			if err != nil {
				log.Printf("Invalid page number: %s", parts[1])
				return
			}

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
}

func createSearchResponseMessage(s *discordgo.Session, query string, page int) (*discordgo.WebhookEdit, error) {
	results, err := SearchManga(query, page)
	if err != nil || len(results.Data) == 0 {
		content := "❌ Tidak ada hasil untuk: **" + query + "**\nCoba gunakan kata kunci yang berbeda."
		return &discordgo.WebhookEdit{Content: &content}, nil
	}

	var embeds []*discordgo.MessageEmbed
	var components []discordgo.MessageComponent

	for _, manga := range results.Data {
		embeds = append(embeds, &discordgo.MessageEmbed{
			Title:     manga.Title,
			Color:     0x00ff00,
			Thumbnail: &discordgo.MessageEmbedThumbnail{URL: manga.CoverURL},
		})
	}

	// Buat buttons untuk add watchlist dengan batasan yang sama
	var currentRow []discordgo.MessageComponent
	buttonsPerRow := 0
	maxButtonsPerRow := 5

	for _, manga := range results.Data {
		button := discordgo.Button{
			Label:    fmt.Sprintf("➕ %s", truncateTitle(manga.Title, 15)),
			Style:    discordgo.SuccessButton,
			CustomID: fmt.Sprintf("add_watchlist_%s_%s", manga.ID, manga.Title),
		}

		currentRow = append(currentRow, button)
		buttonsPerRow++

		// Jika sudah mencapai max buttons per row atau ini item terakhir
		if buttonsPerRow == maxButtonsPerRow || len(currentRow) == len(results.Data) {
			actionRow := discordgo.ActionsRow{
				Components: currentRow,
			}
			components = append(components, actionRow)

			// Reset untuk row berikutnya
			currentRow = []discordgo.MessageComponent{}
			buttonsPerRow = 0
			break // Karena kita hanya butuh satu row untuk search results
		}
	}

	// Pagination controls
	prevPage := page - 1
	nextPage := page + 1
	paginationRow := discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "◀️ Previous",
				Style:    discordgo.SecondaryButton,
				CustomID: fmt.Sprintf("page_%d_%s", prevPage, query),
				Disabled: page <= 1,
			},
			discordgo.Button{
				Label:    fmt.Sprintf("Page %d / %d", page, results.Meta.TotalPage),
				Style:    discordgo.SecondaryButton,
				CustomID: "page_indicator",
				Disabled: true,
			},
			discordgo.Button{
				Label:    "Next ▶️",
				Style:    discordgo.SecondaryButton,
				CustomID: fmt.Sprintf("page_%d_%s", nextPage, query),
				Disabled: page >= results.Meta.TotalPage,
			},
		},
	}
	components = append(components, paginationRow)

	return &discordgo.WebhookEdit{
		Embeds:     &embeds,
		Components: &components,
	}, nil
}
