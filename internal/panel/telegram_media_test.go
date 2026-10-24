package panel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestReceiptPhotoUsesPrivateMultipartAndTruncatesCaption(t *testing.T) {
	var gotPhoto, gotCaption bool
	s := &Server{TelegramHTTP: &http.Client{Transport: featureTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.telegram.org" || !strings.HasSuffix(r.URL.Path, "/sendPhoto") {
			t.Fatal("unexpected media destination")
		}
		if e := r.ParseMultipartForm(2 << 20); e != nil {
			t.Fatal(e)
		}
		if r.FormValue("chat_id") != "123" || r.FormValue("protect_content") != "true" {
			t.Fatal("private delivery settings missing")
		}
		file, _, e := r.FormFile("photo")
		if e != nil {
			t.Fatal(e)
		}
		defer file.Close()
		raw, _ := io.ReadAll(file)
		gotPhoto = len(raw) > 0
		gotCaption = len([]rune(r.FormValue("caption"))) <= 1020
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Header: http.Header{}}, nil
	})}}
	if e := s.telegramSendReply(context.Background(), "fake:test", Data{"chat_id": 123, "protect_content": true, "text": strings.Repeat("a", 1100), "_photo_data": receiptPNG}); e != nil || !gotPhoto || !gotCaption {
		t.Fatal(e, gotPhoto, gotCaption)
	}
	if n := len([]rune(botCaption(strings.Repeat("😀", 800)))); n > 510 {
		t.Fatal("UTF-16 caption overflow", n)
	}
}
func TestSalesCaptionVariablesAndMenuContainBoundedData(t *testing.T) {
	store := Data{"name": strings.Repeat("s", 96), "bot": Data{"welcome": strings.Repeat("{store}", 100)}}
	value := salesText(store, Data{}, "welcome", "", "", 777)
	if len([]rune(value)) > 900 {
		t.Fatal("expanded template exceeded Telegram bound")
	}
	raw, _ := json.Marshal(publicSalesConfig(store))
	if strings.Contains(string(raw), "card_number") {
		t.Fatal("private payment configuration in general public bot view")
	}
}
