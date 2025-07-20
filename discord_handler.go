// discord_handler.go (Versi Final Lengkap)
package main

import (
	"fmt"
	"log"
	"net/url"
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

// -- Handler untuk Tombol --
func componentHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	if strings.HasPrefix(customID, "add_watchlist_") {
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
		})
		if err != nil {
			log.Printf("Error deferring add_watchlist: %v", err)
			return
		}

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
			log.Printf("Error getting latest chapter for %s: %v", manga.ID, err)
			latestChapter = &Chapter{ID: "0"}
		}

		item := WatchlistItem{
			MangaID: manga.ID, UserID: i.Member.User.ID, MangaTitle: manga.Title, LastNotifiedChapterID: latestChapter.ID,
		}

		if err := AddToWatchlist(db, item); err != nil {
			log.Printf("Error adding to watchlist: %v", err)
			msg := "Gagal menambahkan ke watchlist."
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
			return
		}

		msg := fmt.Sprintf("✅ **%s** berhasil ditambahkan ke watchlist!", manga.Title)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
		return
	}

	if strings.HasPrefix(customID, "delete_watchlist_") {
		mangaID := strings.TrimPrefix(customID, "delete_watchlist_")
		userID := i.Member.User.ID

		if err := DeleteFromWatchlist(db, mangaID, userID); err != nil {
			log.Printf("Failed to delete from watchlist: %v", err)
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseUpdateMessage,
				Data: &discordgo.InteractionResponseData{Content: "❌ Gagal menghapus.", Flags: discordgo.MessageFlagsEphemeral},
			})
			return
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})
		response, err := createWatchlistResponseMessage(userID, 1)
		if err != nil {
			log.Printf("Error refreshing watchlist: %v", err)
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
			log.Printf("Error creating watchlist page: %v", err)
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
			log.Printf("Error creating search page: %v", err)
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
	pageSize := 5
	items, totalItems, err := GetWatchlistForUserPaginated(db, userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	if totalItems == 0 {
		content := "📚 Watchlist Anda masih kosong."
		return &discordgo.WebhookEdit{Content: &content}, nil
	}

	var description strings.Builder
	for i, item := range items {
		numberLabel := (page-1)*pageSize + i + 1
		description.WriteString(fmt.Sprintf("**%d.** %s\n", numberLabel, item.MangaTitle))
	}

	totalPages := (totalItems + pageSize - 1) / pageSize
	embed := &discordgo.MessageEmbed{
		Title:       "📚 Watchlist Anda",
		Description: description.String(),
		Color:       0x00bfff,
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Halaman %d dari %d | Total %d manga", page, totalPages, totalItems)},
	}

	var buttonRow, paginationRow []discordgo.MessageComponent
	for i, item := range items {
		buttonRow = append(buttonRow, discordgo.Button{
			Label: fmt.Sprintf("Hapus %d", (page-1)*pageSize+i+1), Style: discordgo.DangerButton, CustomID: fmt.Sprintf("delete_watchlist_%s", item.MangaID),
		})
	}

	if totalPages > 1 {
		prevPage := page - 1
		nextPage := page + 1
		paginationRow = append(paginationRow,
			discordgo.Button{Label: "◀️", Style: discordgo.SecondaryButton, CustomID: fmt.Sprintf("watchlist_page_%d_%s", prevPage, userID), Disabled: page <= 1},
			discordgo.Button{Label: "▶️", Style: discordgo.SecondaryButton, CustomID: fmt.Sprintf("watchlist_page_%d_%s", nextPage, userID), Disabled: page >= totalPages},
		)
	}

	var components []discordgo.MessageComponent
	components = append(components, discordgo.ActionsRow{Components: buttonRow})
	if len(paginationRow) > 0 {
		components = append(components, discordgo.ActionsRow{Components: paginationRow})
	}

	return &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}, Components: &components}, nil
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