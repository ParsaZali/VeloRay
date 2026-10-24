package panel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
	"unicode/utf16"
)

func (s *Server) telegramSendReply(ctx context.Context, token string, reply Data) error {
	image := str(reply, "_photo_data")
	delete(reply, "_photo_data")
	if image == "" {
		method := "sendMessage"
		if str(reply, "photo") != "" {
			method = "sendPhoto"
			reply["caption"] = botCaption(str(reply, "text"))
			delete(reply, "text")
		}
		_, e := s.telegramRequest(ctx, token, method, reply)
		return e
	}
	if e := validateProofImage(image); e != nil {
		return e
	}
	parts := strings.SplitN(image, ",", 2)
	raw, e := base64.StdEncoding.DecodeString(parts[1])
	if e != nil {
		return e
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, key := range []string{"chat_id", "protect_content"} {
		if v, ok := reply[key]; ok {
			_ = writer.WriteField(key, fmt.Sprint(v))
		}
	}
	_ = writer.WriteField("caption", botCaption(str(reply, "text")))
	if markup, ok := reply["reply_markup"]; ok {
		b, _ := json.Marshal(markup)
		_ = writer.WriteField("reply_markup", string(b))
	}
	file, e := writer.CreateFormFile("photo", "receipt")
	if e != nil {
		return e
	}
	if _, e = file.Write(raw); e != nil {
		return e
	}
	if e = writer.Close(); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "POST", "https://api.telegram.org/bot"+token+"/sendPhoto", &body)
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := s.TelegramHTTP
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, e := copyClient.Do(req)
	if e != nil {
		return errors.New("Telegram receipt delivery failed")
	}
	defer res.Body.Close()
	b, e := io.ReadAll(io.LimitReader(res.Body, 65537))
	if e != nil || len(b) > 65536 {
		return errors.New("invalid Telegram response")
	}
	d, e := decode(b)
	if e != nil || res.StatusCode != 200 || !flag(d, "ok") {
		return errors.New("Telegram receipt delivery failed")
	}
	return nil
}

func botCaption(text string) string {
	used := 0
	out := []rune{}
	for _, r := range text {
		n := utf16.RuneLen(r)
		if used+n > 1020 {
			break
		}
		out = append(out, r)
		used += n
	}
	return string(out)
}
