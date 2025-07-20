// api.go (Dengan dukungan pagination)
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

func makeAPIRequest(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned non-200 status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	log.Printf("Raw API Response for %s: %s", url, string(body))
	return body, nil
}

func GetChapterList(mangaID string, page int, pageSize int) (*APIResponseChapter, error) {
	apiURL := fmt.Sprintf("%s/v1/chapter/%s/list?page=%d&page_size=%d&sort_by=chapter_number&sort_order=desc", cfg.APIBaseURL, mangaID, page, pageSize)

	body, err := makeAPIRequest(apiURL)
	if err != nil {
		return nil, err
	}

	var apiResp APIResponseChapter
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}
	if len(apiResp.Data) == 0 {
		return nil, fmt.Errorf("no chapters found for manga %s on page %d", mangaID, page)
	}
	return &apiResp, nil
}
// Fungsi SearchManga sekarang menerima 'page'
func SearchManga(query string, page int) (*APIResponseManga, error) {
	encodedQuery := url.QueryEscape(query)
	apiURL := fmt.Sprintf("%s/v1/manga/list?page=%d&page_size=3&sort=latest&sort_order=desc&q=%s", cfg.APIBaseURL, page, encodedQuery)

	body, err := makeAPIRequest(apiURL)
	if err != nil {
		return nil, err
	}

	var apiResp APIResponseManga
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}
	return &apiResp, nil
}

func GetLatestChapter(mangaID string) (*Chapter, error) {
	apiURL := fmt.Sprintf("%s/v1/chapter/%s/list?page=1&page_size=1&sort_by=chapter_number&sort_order=desc", cfg.APIBaseURL, mangaID)

	body, err := makeAPIRequest(apiURL)
	if err != nil {
		return nil, err
	}

	var apiResp APIResponseChapter
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}
	if len(apiResp.Data) == 0 {
		return nil, fmt.Errorf("no chapters found for manga %s", mangaID)
	}
	return &apiResp.Data[0], nil
}

func GetMangaDetails(mangaID string) (*Manga, error) {
	// Menggunakan endpoint detail yang baru dan lebih tepat
	apiURL := fmt.Sprintf("%s/v1/manga/detail/%s", cfg.APIBaseURL, mangaID)

	body, err := makeAPIRequest(apiURL)
	if err != nil {
		return nil, err
	}

	// Menggunakan struct response yang baru
	var apiResp APIResponseMangaDetail
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	// Langsung return objek data-nya
	return &apiResp.Data, nil
}
