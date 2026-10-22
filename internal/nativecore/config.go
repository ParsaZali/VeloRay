// Package nativecore owns protocol listeners that are not provided by Xray or
// sing-box. It never opens a management listener or executes host commands.
package nativecore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/9seconds/mtg/v2/mtglib"
)

type Config struct {
	Inbounds []Inbound `json:"inbounds"`
}
type Inbound struct {
	Tag      string       `json:"tag"`
	Listen   string       `json:"listen"`
	Port     int          `json:"port"`
	Protocol string       `json:"protocol"`
	AWG      *AWGSettings `json:"awg,omitempty"`
	Users    []User       `json:"users"`
}
type User struct {
	Name         string   `json:"name"`
	Secret       string   `json:"secret,omitempty"`
	PublicKey    string   `json:"public_key,omitempty"`
	PresharedKey string   `json:"preshared_key,omitempty"`
	Address      string   `json:"address,omitempty"`
	ExpiresAt    string   `json:"expires_at,omitempty"`
	IPLimit      int      `json:"ip_limit,omitempty"`
	DeniedIPs    []string `json:"denied_ips,omitempty"`
}

var hostname = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$`)

func ValidHostname(value string) bool {
	value = strings.ToLower(value)
	if !hostname.MatchString(value) || !strings.Contains(value, ".") || strings.Contains(value, "..") || net.ParseIP(value) != nil {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
	}
	for _, suffix := range []string{".local", ".localhost", ".internal", ".home", ".lan"} {
		if strings.HasSuffix(value, suffix) {
			return false
		}
	}
	return true
}
func Decode(raw []byte) (Config, error) {
	var cfg Config
	if len(raw) > 4<<20 {
		return cfg, errors.New("native configuration exceeds 4 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, errors.New("invalid native configuration")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return cfg, errors.New("unexpected trailing configuration")
	}
	if cfg.Inbounds == nil {
		return cfg, errors.New("native configuration requires an inbounds array")
	}
	if len(cfg.Inbounds) > 512 {
		return cfg, errors.New("too many native listeners")
	}
	awgCount, totalUsers := 0, 0
	tags, addresses, names, keys := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, in := range cfg.Inbounds {
		if in.Tag == "" || len(in.Tag) > 256 || tags[in.Tag] {
			return cfg, errors.New("invalid or duplicate native tag")
		}
		tags[in.Tag] = true
		if net.ParseIP(in.Listen) == nil || in.Port < 1 || in.Port > 65535 {
			return cfg, errors.New("invalid native listen address")
		}
		for _, port := range []int{9191, 10085, 10086, 10087, 8610, 8611, 8612} {
			if in.Port == port {
				return cfg, errors.New("reserved native port")
			}
		}
		address := net.JoinHostPort(in.Listen, fmt.Sprint(in.Port))
		if addresses[address] {
			return cfg, errors.New("duplicate native listener")
		}
		addresses[address] = true
		if in.Protocol != "mtproto" && in.Protocol != "amneziawg" {
			return cfg, errors.New("unsupported native protocol")
		}
		if in.Protocol == "amneziawg" {
			awgCount++
			if awgCount > 16 {
				return cfg, errors.New("at most 16 AmneziaWG listeners are supported per agent")
			}
			if err := validateAWG(in); err != nil {
				return cfg, err
			}
		} else if in.AWG != nil {
			return cfg, errors.New("unexpected AWG settings")
		}
		totalUsers += len(in.Users)
		if totalUsers > 16384 {
			return cfg, errors.New("too many native users")
		}
		if len(in.Users) > 4096 {
			return cfg, errors.New("too many users on one native listener")
		}
		host := ""
		for _, u := range in.Users {
			if (!strings.HasPrefix(u.Name, "veloray:") && !strings.HasPrefix(u.Name, "veloray-native-")) || strings.Contains(u.Name, ">>>") || len(u.Name) > 1024 || names[u.Name] {
				return cfg, errors.New("invalid or duplicate native user")
			}
			names[u.Name] = true
			if u.IPLimit < 0 || u.IPLimit > 10000 {
				return cfg, errors.New("invalid MTProto IP limit")
			}
			if u.ExpiresAt != "" {
				if _, err := time.Parse(time.RFC3339, u.ExpiresAt); err != nil {
					return cfg, errors.New("invalid native expiry")
				}
			}
			if in.Protocol == "amneziawg" {
				continue
			}
			if u.PublicKey != "" || u.PresharedKey != "" || u.Address != "" {
				return cfg, errors.New("unexpected MTProto peer fields")
			}
			secret, err := mtglib.ParseSecret(u.Secret)
			if err != nil || !secret.Valid() || !ValidHostname(secret.Host) {
				return cfg, errors.New("invalid MTProto FakeTLS secret or public hostname")
			}
			key := address + ":" + fmt.Sprintf("%x", secret.Key)
			if keys[key] {
				return cfg, errors.New("duplicate MTProto key on listener")
			}
			keys[key] = true
			if host != "" && host != secret.Host {
				return cfg, errors.New("all MTProto users on a listener must share the FakeTLS hostname")
			}
			host = secret.Host
			for _, ip := range u.DeniedIPs {
				if net.ParseIP(ip) == nil {
					return cfg, errors.New("invalid denied client IP")
				}
			}
		}
	}
	return cfg, nil
}
