package panel

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Keys, actions and placeholders are bounded data; no templates execute code.
func salesDefaults() Data {
	return Data{
		"locale": "fa", "menu_mode": "inline", "welcome": "", "welcome_image": "", "catalog_text": "", "catalog_image": "",
		"empty_text": "", "delivery_text": "", "trial_text": "", "support_text": "", "tutorial_text": "", "tutorial_image": "",
		"buttons": []any{
			Data{"action": "buy", "label": "🛍 خرید اشتراک", "visible": true, "row": 0},
			Data{"action": "services", "label": "📦 سرویس‌های من", "visible": true, "row": 0},
			Data{"action": "trial", "label": "🎁 تست رایگان", "visible": true, "row": 1},
			Data{"action": "account", "label": "👤 حساب من", "visible": true, "row": 1},
			Data{"action": "tutorial", "label": "📚 آموزش اتصال", "visible": true, "row": 2},
			Data{"action": "support", "label": "💬 پشتیبانی", "visible": true, "row": 2},
			Data{"action": "miniapp", "label": "🌐 فروشگاه", "visible": true, "row": 3},
		},
		"custom_buttons": []any{}, "tutorials": []any{},
		"trial_enabled": false, "trial_mode": "once", "trial_service_id": 0, "trial_traffic_bytes": int64(1073741824), "trial_hours": 24,
		"trial_channel": "", "trial_channel_url": "", "trial_paid_only": false,
		"allow_toggle": false, "allow_rotate": false,
		"card_enabled": false, "card_number": "", "card_holder": "", "card_instructions": "",
		"usdt_enabled": false, "usdt_network": "TRC20", "usdt_address": "", "usdt_instructions": "",
	}
}
func salesConfig(store Data) Data { return merge(salesDefaults(), obj(store, "bot")) }

var cardPattern = regexp.MustCompile(`^[0-9]{16}$`)
var tronAddress = regexp.MustCompile(`^T[1-9A-HJ-NP-Za-km-z]{33}$`)
var evmAddress = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
var transactionHash = regexp.MustCompile(`^(0x)?[0-9a-fA-F]{64}$`)
var channelName = regexp.MustCompile(`^@[A-Za-z][A-Za-z0-9_]{4,31}$`)

func validateSalesConfig(store Data) error {
	input := obj(store, "bot")
	defaults := salesDefaults()
	keys := []string{}
	for k := range defaults {
		keys = append(keys, k)
	}
	if e := allowed(input, strings.Join(keys, " ")); e != nil {
		return fail(400, e.Error())
	}
	if e := typed(input, defaults); e != nil {
		return fail(400, e.Error())
	}
	b := salesConfig(store)
	if !oneOf(str(b, "locale"), "fa", "en") || !oneOf(str(b, "menu_mode"), "inline", "keyboard") || !oneOf(str(b, "trial_mode"), "once", "paid_only") {
		return fail(400, "invalid bot language, menu or trial mode")
	}
	for _, k := range strings.Fields("welcome catalog_text empty_text delivery_text trial_text support_text tutorial_text") {
		if utf8.RuneCountInString(str(b, k)) > 900 {
			return fail(400, k+" must be at most 900 characters")
		}
	}
	for _, k := range []string{"welcome_image", "catalog_image", "tutorial_image", "trial_channel_url"} {
		if len(str(b, k)) > 2048 || !commerceURL(str(b, k)) {
			return fail(400, "invalid HTTPS bot media or channel URL")
		}
	}
	if num(b, "trial_hours") < 1 || num(b, "trial_hours") > 168 || num(b, "trial_traffic_bytes") < 1048576 || num(b, "trial_traffic_bytes") > 100*1073741824 {
		return fail(400, "trial requires 1–168 hours and 1 MiB–100 GiB")
	}
	if str(b, "trial_channel") != "" && (!channelName.MatchString(str(b, "trial_channel")) || str(b, "trial_channel_url") == "") {
		return fail(400, "trial membership requires @channel and its HTTPS join URL")
	}
	if flag(b, "trial_enabled") && num(b, "trial_service_id") < 1 {
		return fail(400, "select a service for the free trial")
	}
	if len(array(b, "buttons")) > 12 || len(array(b, "custom_buttons")) > 12 || len(array(b, "tutorials")) > 24 {
		return fail(400, "too many bot buttons or tutorials")
	}
	seen := map[string]bool{}
	for _, v := range array(b, "buttons") {
		d := asData(v)
		action := str(d, "action")
		if e := allowed(d, "action label visible row"); e != nil {
			return fail(400, e.Error())
		}
		if e := typed(d, Data{"action": "", "label": "", "visible": false, "row": 0}); e != nil {
			return fail(400, e.Error())
		}
		if !oneOf(action, "buy", "services", "trial", "account", "tutorial", "support", "miniapp") || seen[action] || strings.TrimSpace(str(d, "label")) == "" || utf8.RuneCountInString(str(d, "label")) > 48 || num(d, "row") < 0 || num(d, "row") > 11 {
			return fail(400, "invalid or duplicate main menu button")
		}
		seen[action] = true
	}
	for _, v := range array(b, "custom_buttons") {
		d := asData(v)
		if e := typed(d, Data{"label": "", "url": "", "text": "", "image": ""}); e != nil {
			return fail(400, e.Error())
		}
		if len(str(d, "url")) > 2048 || len(str(d, "image")) > 2048 {
			return fail(400, "custom button URLs are too long")
		}
		if e := allowed(d, "label url text image"); e != nil {
			return fail(400, e.Error())
		}
		if str(d, "label") == "" || utf8.RuneCountInString(str(d, "label")) > 48 || utf8.RuneCountInString(str(d, "text")) > 900 || !commerceURL(str(d, "url")) || !commerceURL(str(d, "image")) || str(d, "url") == "" && str(d, "text") == "" {
			return fail(400, "custom buttons require a label and HTTPS URL or content")
		}
	}
	for _, v := range array(b, "tutorials") {
		d := asData(v)
		if e := typed(d, Data{"title": "", "url": "", "text": "", "image": ""}); e != nil {
			return fail(400, e.Error())
		}
		if len(str(d, "url")) > 2048 || len(str(d, "image")) > 2048 {
			return fail(400, "tutorial URLs are too long")
		}
		if e := allowed(d, "title text url image"); e != nil {
			return fail(400, e.Error())
		}
		if str(d, "title") == "" || utf8.RuneCountInString(str(d, "title")) > 80 || utf8.RuneCountInString(str(d, "text")) > 900 || !commerceURL(str(d, "url")) || !commerceURL(str(d, "image")) {
			return fail(400, "invalid tutorial")
		}
	}
	if flag(b, "card_enabled") && (!cardPattern.MatchString(str(b, "card_number")) || strings.TrimSpace(str(b, "card_holder")) == "" || len(str(b, "card_holder")) > 160) {
		return fail(400, "card payment requires a 16-digit card and holder name")
	}
	if !oneOf(str(b, "usdt_network"), "TRC20", "ERC20", "BEP20") {
		return fail(400, "choose TRC20, ERC20 or BEP20")
	}
	if flag(b, "usdt_enabled") {
		valid := evmAddress.MatchString(str(b, "usdt_address"))
		if str(b, "usdt_network") == "TRC20" {
			valid = tronAddress.MatchString(str(b, "usdt_address"))
		}
		if !valid {
			return fail(400, "invalid USDT address for the selected network")
		}
	}
	if len(str(b, "card_instructions")) > 1200 || len(str(b, "usdt_instructions")) > 1200 {
		return fail(400, "payment instructions are too long")
	}
	return nil
}
func salesText(store, cfg Data, key, en, fa string, user int64) string {
	b := salesConfig(store)
	text := str(b, key)
	if text == "" {
		text = botText(cfg, en, fa)
	}
	text = strings.NewReplacer("{store}", str(store, "name"), "{user_id}", fmt.Sprint(user)).Replace(text)
	r := []rune(text)
	if len(r) > 900 {
		text = string(r[:900])
	}
	return text
}
func manualMethods(store Data, currency string) []any {
	b := salesConfig(store)
	out := []any{}
	if flag(b, "card_enabled") && oneOf(currency, "IRT", "IRR") {
		out = append(out, Data{"method": "card", "card_number": str(b, "card_number"), "card_holder": str(b, "card_holder"), "instructions": str(b, "card_instructions")})
	}
	if flag(b, "usdt_enabled") && currency == "USDT" {
		out = append(out, Data{"method": "usdt", "network": str(b, "usdt_network"), "address": str(b, "usdt_address"), "instructions": str(b, "usdt_instructions")})
	}
	return out
}
func publicSalesConfig(store Data) Data {
	b := salesConfig(store)
	out := Data{}
	for _, k := range strings.Fields("welcome welcome_image catalog_text catalog_image empty_text delivery_text trial_text support_text tutorial_text tutorial_image buttons custom_buttons tutorials trial_enabled trial_channel_url allow_toggle allow_rotate locale") {
		out[k] = b[k]
	}
	return out
}
