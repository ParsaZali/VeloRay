package panel

// An account can contain app-specific transports (Telegram/AmneziaWG alongside
// VLESS, for example). One unsupported member must not disable every other member.
func compatibleFeedConnections(connections []subscriptionConnection, format string) ([]subscriptionConnection, error) {
	selected := []subscriptionConnection{}
	var first error
	for _, item := range connections {
		var err error
		switch format {
		case "xray":
			_, err = xrayClientConfig(item.client, item.inbound, item.node)
		case "singbox", "sing-box":
			_, err = singClientOutbounds(item.client, item.inbound, item.node)
		default:
			_, err = clashConfig(item.client, item.inbound, item.node)
		}
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		selected = append(selected, item)
	}
	return selected, first
}
