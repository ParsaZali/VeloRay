package panel

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

func emailRecipients(raw string) ([]string, error) {
	if len(raw) > 2048 || strings.ContainsAny(raw, "\r\n\x00") {
		return nil, errors.New("invalid email recipient")
	}
	addresses, e := mail.ParseAddressList(raw)
	if e != nil || len(addresses) == 0 || len(addresses) > 10 {
		return nil, errors.New("enter 1–10 email recipients")
	}
	out := []string{}
	for _, a := range addresses {
		if !strings.Contains(a.Address, "@") {
			return nil, errors.New("invalid email recipient")
		}
		out = append(out, a.Address)
	}
	return out, nil
}
func validateSMTP(d Data) error {
	if !flag(d, "smtp_enabled") {
		return nil
	}
	host := str(d, "smtp_host")
	if e := validateWebhookURL("https://" + net.JoinHostPort(host, "443")); e != nil || !validHost(host) {
		return errors.New("SMTP requires a public hostname")
	}
	if num(d, "smtp_port") < 1 || num(d, "smtp_port") > 65535 || !oneOf(str(d, "smtp_tls_mode"), "tls", "starttls") {
		return errors.New("invalid SMTP port or TLS mode")
	}
	addresses, e := emailRecipients(str(d, "smtp_from"))
	if e != nil || len(addresses) != 1 {
		return errors.New("SMTP requires one sender address")
	}
	if len(str(d, "smtp_username")) > 256 || strings.ContainsAny(str(d, "smtp_username"), "\r\n\x00") {
		return errors.New("invalid SMTP username")
	}
	if str(d, "smtp_username") != "" && str(d, "_smtp_password") == "" {
		return errors.New("SMTP username requires a password")
	}
	return nil
}
func emailMessage(from string, recipients []string, payload []byte, deliveryID string) []byte {
	d, _ := decode(payload)
	subject := mime.QEncoding.Encode("utf-8", "VeloRay · "+str(d, "event"))
	body := fmt.Sprintf("VeloRay event\r\n\r\nAction: %s\r\nTarget: %s\r\nActor: %s\r\nAt: %s\r\nDelivery: %s\r\n", str(d, "event"), str(d, "target"), str(d, "actor"), str(d, "occurred_at"), deliveryID)
	return []byte("From: " + from + "\r\nTo: " + strings.Join(recipients, ", ") + "\r\nSubject: " + subject + "\r\nMessage-ID: <veloray-" + deliveryID + "@" + strings.Split(from, "@")[1] + ">\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n" + body)
}
func (s *Server) deliverEmail(ctx context.Context, target string, payload []byte, id string) error {
	recipients, e := emailRecipients(target)
	if e != nil {
		return e
	}
	cfg, e := settings(ctx, s.Store.Pool)
	if e != nil {
		return e
	}
	if !flag(cfg, "smtp_enabled") {
		return errors.New("email delivery is disabled")
	}
	if e = validateSMTP(cfg); e != nil {
		return e
	}
	sender, _ := emailRecipients(str(cfg, "smtp_from"))
	from := sender[0]
	if s.EmailSend != nil {
		return s.EmailSend(ctx, from, recipients, emailMessage(from, recipients, payload, id))
	}
	password, e := unseal(s.Config.FieldKey, str(cfg, "_smtp_password"))
	if e != nil {
		return e
	}
	transport := webhookHTTPClient().Transport.(*http.Transport)
	return sendSMTP(ctx, cfg, from, recipients, password, emailMessage(from, recipients, payload, id), transport.DialContext, &tls.Config{ServerName: str(cfg, "smtp_host"), MinVersion: tls.VersionTLS12})
}
func sendSMTP(ctx context.Context, cfg Data, from string, recipients []string, password string, message []byte, dial func(context.Context, string, string) (net.Conn, error), tlsConfig *tls.Config) error {
	host := str(cfg, "smtp_host")
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	conn, e := dial(ctx, "tcp", net.JoinHostPort(host, strconv.FormatInt(num(cfg, "smtp_port"), 10)))
	if e != nil {
		return errors.New("SMTP connection unavailable")
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	if str(cfg, "smtp_tls_mode") == "tls" {
		secured := tls.Client(conn, tlsConfig)
		if e = secured.HandshakeContext(ctx); e != nil {
			return errors.New("SMTP TLS failed")
		}
		conn = secured
	}
	c, e := smtp.NewClient(conn, host)
	if e != nil {
		return errors.New("SMTP greeting failed")
	}
	defer c.Close()
	if str(cfg, "smtp_tls_mode") == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("SMTP STARTTLS is not advertised")
		}
		if e = c.StartTLS(tlsConfig); e != nil {
			return errors.New("SMTP STARTTLS failed")
		}
	}
	if user := str(cfg, "smtp_username"); user != "" {
		if e = c.Auth(smtp.PlainAuth("", user, password, host)); e != nil {
			return errors.New("SMTP authentication failed")
		}
	}
	if e = c.Mail(from); e != nil {
		return errors.New("SMTP sender rejected")
	}
	for _, to := range recipients {
		if e = c.Rcpt(to); e != nil {
			return errors.New("SMTP recipient rejected")
		}
	}
	writer, e := c.Data()
	if e != nil {
		return errors.New("SMTP data rejected")
	}
	_, e = io.WriteString(writer, string(message))
	if e != nil {
		writer.Close()
		return errors.New("SMTP write failed")
	}
	if e = writer.Close(); e != nil {
		return errors.New("SMTP message rejected")
	}
	// Successful DATA is the delivery boundary. A failed QUIT must not resend it.
	_ = c.Quit()
	return nil
}
