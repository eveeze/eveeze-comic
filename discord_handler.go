// discord_handler.go (Perbaikan Timeout pada /search)
package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func searchCommandHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// =================================================================
	// ## PERBAIKAN DI SINI ##
	// LANGKAH 1: DEFER RESPON UNTUK MENCEGAH TIMEOUT PADA /SEARCH
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Could not defer interaction for /search: %v", err)
		return
	}
	// =================================================================

	query := i.ApplicationCommandData().Options[0].StringValue()

	// LANGKAH 2: LAKUKAN PEKERJAAN LAMBAT (MEMANGGIL API)
	results, err := SearchManga(query)

	// Siapkan pesan balasan
	var response discordgo.WebhookEdit

	if err != nil || len(results) == 0 {
		content := "Tidak ada hasil untuk: " + query
		response.Content = &content
	} else {
		var embeds []*discordgo.MessageEmbed
		var buttons []discordgo.MessageComponent
		for _, manga := range results {
			embeds = append(embeds, &discordgo.MessageEmbed{
				Title:     manga.Title,
				Color:     0x00ff00,
				Thumbnail: &discordgo.MessageEmbedThumbnail{URL: manga.CoverURL},
			})
			buttons = append(buttons, discordgo.Button{
				Label:    fmt.Sprintf("Tambah '%s'", manga.Title),
				Style:    discordgo.SuccessButton,
				CustomID: fmt.Sprintf("add_watchlist_%s_%s", manga.ID, manga.Title),
			})
		}
		response.Embeds = &embeds
		response.Components = &[]discordgo.MessageComponent{
			discordgo.ActionsRow{Components: buttons},
		}
	}

	// LANGKAH 3: EDIT RESPON AWAL DENGAN HASIL PENCARIAN
	s.InteractionResponseEdit(i.Interaction, &response)
}

// Handler utama untuk semua interaksi (kode ini sudah benar dari sebelumnya)
func interactionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	case discordgo.InteractionMessageComponent:
		customID := i.MessageComponentData().CustomID
		if strings.HasPrefix(customID, "add_watchlist_") {
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Flags: discordgo.MessageFlagsEphemeral,
				},
			})
			if err != nil {
				log.Printf("Could not defer interaction: %v", err)
				return
			}

			parts := strings.SplitN(customID, "_", 4)
			if len(parts) < 4 {
				return
			}
			mangaID, mangaTitle := parts[2], parts[3]

			latestChapter, err := GetLatestChapter(mangaID)
			if err != nil {
				log.Printf("Error getting latest chapter: %v", err)
				errorMsg := "Gagal mendapatkan detail chapter."
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &errorMsg,
				})
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
				log.Printf("Failed to add to watchlist: %v", err)
				errorMsg := "Gagal menambahkan ke watchlist."
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &errorMsg,
				})
				return
			}

			successMessage := fmt.Sprintf("✅ **%s** berhasil ditambahkan ke watchlist pribadi Anda!", mangaTitle)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &successMessage,
			})
		}
	}
}

func watchlistCommandHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Ambil data dari database untuk user yang meminta
	items, err := GetWatchlistForUser(db, i.Member.User.ID)
	if err != nil {
		log.Printf("Error getting watchlist for user %s: %v", i.Member.User.ID, err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Gagal mengambil watchlist.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if len(items) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Watchlist Anda masih kosong. Cari komik dengan `/search` untuk menambahkannya.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Format daftar menjadi teks yang rapi
	var description strings.Builder
	description.WriteString("Berikut adalah daftar pantauan Anda:\n\n")
	for idx, item := range items {
		description.WriteString(fmt.Sprintf("%d. **%s**\n", idx+1, item.MangaTitle))
	}

	// Buat Embed untuk ditampilkan
	embed := &discordgo.MessageEmbed{
		Title:       "Watchlist Pribadi Anda 📖",
		Description: description.String(),
		Color:       0x00bfff, // Biru langit
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Gunakan tombol Hapus (fitur selanjutnya) untuk menghapus item.",
		},
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral, // Pesan ini hanya bisa dilihat oleh Anda
		},
	})
}
