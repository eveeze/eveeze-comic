package main

import (
	"fmt"
	"log"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

// -- Cache Sederhana untuk Hasil Pencarian --
var (
	searchResultCache = make(map[string]Manga)
	cacheMutex        = &sync.Mutex{}
)

// -- Handler Utama untuk Semua Interaksi --
func interactionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		// Rute untuk perintah slash seperti /search dan /watchlist
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	case discordgo.InteractionMessageComponent:
		// Handler untuk komponen seperti tombol
		componentHandler(s, i)
	}
}

// -- Handler untuk Perintah Slash --
func searchCommandHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Could not defer interaction for /search: %v", err)
		return
	}

	query := i.ApplicationCommandData().Options[0].StringValue()
	response, err := createSearchResponseMessage(query, 1)
	if err != nil {
		log.Printf("Error creating search response: %v", err)
		content := "❌ Gagal mencari manga atau tidak ada hasil untuk: **" + query + "**"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return
	}
	s.InteractionResponseEdit(i.Interaction, response)
}

func watchlistCommandHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})
	if err != nil {
		log.Printf("Could not defer /watchlist: %v", err)
		return
	}

	response, err := createWatchlistResponseMessage(i.Member.User.ID, 1)
	if err != nil {
		log.Printf("Error creating watchlist response: %v", err)
		content := "Gagal mengambil watchlist."
		response = &discordgo.WebhookEdit{Content: &content}
	}
	s.InteractionResponseEdit(i.Interaction, response)
}

func componentHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	// ... (handler untuk "add_watchlist_" tetap sama) ...
	if strings.HasPrefix(customID, "add_watchlist_") {
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
		})
		if err != nil { return }
		mangaID := strings.TrimPrefix(customID, "add_watchlist_")
		cacheMutex.Lock()
		manga, ok := searchResultCache[mangaID]
		cacheMutex.Unlock()
		if !ok {
			msg := "❌ Gagal menambahkan. Hasil pencarian mungkin sudah kedaluwarsa. Silakan cari ulang."
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
			return
		}
		latestChapter, err := GetLatestChapter(manga.ID)
		if err != nil {
			latestChapter = &Chapter{ID: "0", Number: 0}
		}
		item := WatchlistItem{
			MangaID: manga.ID, UserID: i.Member.User.ID, MangaTitle: manga.Title,
			UserProgressChapterID: latestChapter.ID, UserProgressChapterNumber: latestChapter.Number,
		}
		if err := AddToWatchlist(db, item); err != nil {
			msg := "Gagal menambahkan ke watchlist."
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
			return
		}
		msg := fmt.Sprintf("✅ **%s** berhasil ditambahkan ke watchlist!", manga.Title)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
		return
	}

	// ... (handler untuk "show_unread_" tetap sama) ...
	if strings.HasPrefix(customID, "show_unread_") {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
		})
		mangaID := strings.TrimPrefix(customID, "show_unread_")
		userID := i.Member.User.ID
		watchlistItem, err := GetWatchlistItem(db, userID, mangaID)
		if err != nil {
			msg := "Gagal mendapatkan data watchlist."
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
			return
		}
		chapterList, err := GetChapterList(mangaID, 1, 25)
		if err != nil {
			msg := "Gagal mengambil daftar chapter."
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
			return
		}
		var unreadChapters []Chapter
		for _, chapter := range chapterList.Data {
			if chapter.Number > watchlistItem.UserProgressChapterNumber {
				unreadChapters = append(unreadChapters, chapter)
			}
		}
		if len(unreadChapters) == 0 {
			msg := "Anda sudah membaca chapter terbaru!"
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
			return
		}
		sort.Slice(unreadChapters, func(i, j int) bool { return unreadChapters[i].Number < unreadChapters[j].Number })
		var embeds []*discordgo.MessageEmbed
		for _, chapter := range unreadChapters {
			embeds = append(embeds, &discordgo.MessageEmbed{
				Title: fmt.Sprintf("%s - Chapter %.1f", watchlistItem.MangaTitle, chapter.Number),
				URL:   fmt.Sprintf("%s/chapter/%s", cfg.ReaderBaseURL, chapter.ID),
				Color: 0x00bfff,
			})
			if len(embeds) >= 10 { break }
		}
		newestChapter := unreadChapters[len(unreadChapters)-1]
		components := []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label: "✅ Tandai Semua Sudah Dibaca", Style: discordgo.SuccessButton,
						CustomID: fmt.Sprintf("mark_read_%s_%s_%.1f", mangaID, newestChapter.ID, newestChapter.Number),
					},
				},
			},
		}
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &embeds, Components: &components})
		return
	}

	// ... (handler untuk "mark_read_" tetap sama) ...
	if strings.HasPrefix(customID, "mark_read_") {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseUpdateMessage})
		parts := strings.Split(customID, "_")
		if len(parts) < 5 { return } // Perbarui jumlah parts karena format ID
		mangaID := parts[2]
		newChapterID := parts[3]
		newChapterNumber, _ := strconv.ParseFloat(strings.Join(parts[4:], "."), 64)
		userID := i.Member.User.ID
		err := UpdateUserProgress(db, userID, mangaID, newChapterID, newChapterNumber)
		if err != nil {
			log.Printf("Failed to update user progress: %v", err)
			return
		}
		msg := "✅ Progres Anda telah diperbarui! Jalankan `/watchlist` lagi untuk melihat."
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &msg, Components: &[]discordgo.MessageComponent{}, Embeds: &[]*discordgo.MessageEmbed{},
		})
		return
	}

	// BARU: Handler untuk tombol "Tandai Chapter Terbaru"
	if strings.HasPrefix(customID, "mark_latest_") {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})
		mangaID := strings.TrimPrefix(customID, "mark_latest_")
		userID := i.Member.User.ID

		// Dapatkan chapter terbaru langsung dari API
		latestChapter, err := GetLatestChapter(mangaID)
		if err != nil {
			log.Printf("Failed to get latest chapter for mark_latest: %v", err)
			return
		}

		// Update progres di database ke chapter terbaru
		err = UpdateUserProgress(db, userID, mangaID, latestChapter.ID, latestChapter.Number)
		if err != nil {
			log.Printf("Failed to update user progress for mark_latest: %v", err)
			return
		}
		
		// Refresh halaman watchlist
		response, err := createWatchlistResponseMessage(userID, 1) // Kembali ke halaman 1
		if err != nil {
			return
		}
		s.InteractionResponseEdit(i.Interaction, response)
		return
	}

	// ... (handler untuk delete_watchlist_, watchlist_page_, page_ tetap sama) ...
	if strings.HasPrefix(customID, "delete_watchlist_") {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})
		mangaID := strings.TrimPrefix(customID, "delete_watchlist_")
		userID := i.Member.User.ID
		if err := DeleteFromWatchlist(db, mangaID, userID); err != nil {
			log.Printf("Failed to delete from watchlist: %v", err)
			return
		}
		response, err := createWatchlistResponseMessage(userID, 1)
		if err != nil {
			return
		}
		s.InteractionResponseEdit(i.Interaction, response)
		return
	}
	if strings.HasPrefix(customID, "watchlist_page_") {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})
		parts := strings.SplitN(customID, "_", 4)
		page, _ := strconv.Atoi(parts[2])
		userID := parts[3]
		response, err := createWatchlistResponseMessage(userID, page)
		if err != nil {
			return
		}
		s.InteractionResponseEdit(i.Interaction, response)
		return
	}
	if strings.HasPrefix(customID, "page_") {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})
		parts := strings.SplitN(customID, "_", 3)
		page, _ := strconv.Atoi(parts[1])
		query, _ := url.QueryUnescape(parts[2])
		response, err := createSearchResponseMessage(query, page)
		if err != nil {
			return
		}
		s.InteractionResponseEdit(i.Interaction, response)
		return
	}
}

// -- Fungsi Pembuat Pesan --
func createSearchResponseMessage(query string, page int) (*discordgo.WebhookEdit, error) {
	results, err := SearchManga(query, page)
	if err != nil {
		return nil, err
	}
	if len(results.Data) == 0 {
		return nil, fmt.Errorf("no results found for query: %s", query)
	}

	cacheMutex.Lock()
	searchResultCache = make(map[string]Manga)
	for _, manga := range results.Data {
		searchResultCache[manga.ID] = manga
	}
	cacheMutex.Unlock()

	var embeds []*discordgo.MessageEmbed
	var components []discordgo.MessageComponent
	var buttonRow []discordgo.MessageComponent

	for _, manga := range results.Data {
		embeds = append(embeds, &discordgo.MessageEmbed{
			Title:     manga.Title,
			Color:     0x00ff00,
			Thumbnail: &discordgo.MessageEmbedThumbnail{URL: manga.CoverURL},
		})
		buttonRow = append(buttonRow, discordgo.Button{
			Label:    fmt.Sprintf("➕ %s", truncateTitle(manga.Title, 20)),
			Style:    discordgo.SuccessButton,
			CustomID: fmt.Sprintf("add_watchlist_%s", manga.ID),
		})
	}
	components = append(components, discordgo.ActionsRow{Components: buttonRow})

	prevPage := page - 1
	nextPage := page + 1
	paginationRow := discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "◀️", Style: discordgo.SecondaryButton, CustomID: fmt.Sprintf("page_%d_%s", prevPage, url.QueryEscape(query)), Disabled: page <= 1},
			discordgo.Button{Label: fmt.Sprintf("%d / %d", page, results.Meta.TotalPage), Style: discordgo.SecondaryButton, CustomID: "page_indicator", Disabled: true},
			discordgo.Button{Label: "▶️", Style: discordgo.SecondaryButton, CustomID: fmt.Sprintf("page_%d_%s", nextPage, url.QueryEscape(query)), Disabled: page >= results.Meta.TotalPage},
		},
	}
	components = append(components, paginationRow)

	return &discordgo.WebhookEdit{Embeds: &embeds, Components: &components}, nil
}

func createWatchlistResponseMessage(userID string, page int) (*discordgo.WebhookEdit, error) {
	pageSize := 2 // Ubah ke 2 item per halaman agar tidak terlalu ramai
	items, totalItems, err := GetWatchlistForUserPaginated(db, userID, page, pageSize)
	if err != nil {
		return nil, err
	}
	if totalItems == 0 {
		content := "📚 Watchlist Anda masih kosong."
		return &discordgo.WebhookEdit{Content: &content}, nil
	}

	var embeds []*discordgo.MessageEmbed
	var components []discordgo.MessageComponent

	for _, item := range items {
		latestChapter, err := GetLatestChapter(item.MangaID)
		if err != nil {
			latestChapter = &Chapter{Number: item.UserProgressChapterNumber}
		}
		mangaDetails, err := GetMangaDetails(item.MangaID)
		if err != nil {
			log.Printf("Could not get manga details for %s: %v", item.MangaID, err)
		}

		chaptersBehind := math.Max(0, latestChapter.Number-item.UserProgressChapterNumber)
		var description string
		if chaptersBehind > 0 {
			description = fmt.Sprintf("Anda telah membaca chapter **%.1f** dari **%.1f**.\n**Tersisa %.0f chapter untuk dibaca!**",
				item.UserProgressChapterNumber, latestChapter.Number, chaptersBehind)
		} else {
			description = fmt.Sprintf("Anda sudah di chapter terbaru! (**%.1f**)", latestChapter.Number)
		}

		embed := &discordgo.MessageEmbed{
			Title:       item.MangaTitle,
			Description: description,
			Color:       0x00bfff,
		}
		if mangaDetails != nil {
			embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: mangaDetails.CoverURL}
		}
		embeds = append(embeds, embed)

		// Baris tombol pertama: Aksi Utama
		actionRow1 := discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    fmt.Sprintf("📖 Lihat Chapter (%.0f)", chaptersBehind),
					Style:    discordgo.PrimaryButton,
					CustomID: fmt.Sprintf("show_unread_%s", item.MangaID),
					Disabled: chaptersBehind == 0,
				},
				// BARU: Tombol untuk "catch up"
				discordgo.Button{
					Label:    "✅ Tandai Terbaru",
					Style:    discordgo.SuccessButton,
					CustomID: fmt.Sprintf("mark_latest_%s", item.MangaID),
					Disabled: chaptersBehind == 0, // Non-aktif jika sudah di chapter terbaru
				},
			},
		}
		// Baris tombol kedua: Aksi Sekunder
		actionRow2 := discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "🗑️ Hapus dari Watchlist",
					Style:    discordgo.DangerButton,
					CustomID: fmt.Sprintf("delete_watchlist_%s", item.MangaID),
				},
			},
		}
		components = append(components, actionRow1, actionRow2)
	}

	totalPages := (totalItems + pageSize - 1) / pageSize
	if totalPages > 1 {
		prevPage := page - 1
		nextPage := page + 1
		paginationRow := discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label: "◀️ Sebelumnya", Style: discordgo.SecondaryButton,
					CustomID: fmt.Sprintf("watchlist_page_%d_%s", prevPage, userID), Disabled: page <= 1,
				},
				discordgo.Button{
					Label: "Berikutnya ▶️", Style: discordgo.SecondaryButton,
					CustomID: fmt.Sprintf("watchlist_page_%d_%s", nextPage, userID), Disabled: page >= totalPages,
				},
			},
		}
		components = append(components, paginationRow)
	}

	return &discordgo.WebhookEdit{Embeds: &embeds, Components: &components}, nil
}


func truncateTitle(title string, maxLength int) string {
	if len(title) <= maxLength {
		return title
	}
	runes := []rune(title)
	if len(runes) <= maxLength {
		return title
	}
	return string(runes[:maxLength-3]) + "..."
}