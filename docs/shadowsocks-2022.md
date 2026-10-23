# Shadowsocks 2022 multiuser

Choose **Shadowsocks → 2022 AES-128-GCM** or **2022 AES-256-GCM** on Xray or sing-box. The panel creates a correctly sized server key when blank, and a distinct 16/32-byte base64 key for each customer. Batch creation, shared customers, quotas, expiry and multi-format subscriptions work with these inbounds. Duplicate keys on one inbound are rejected.

The client password is `SERVER_KEY:USER_KEY`. The URI uses the percent-encoded AEAD-2022 SIP002 form; it does not base64-wrap the whole userinfo. Mihomo/Clash, sing-box and Xray exports include the combined key. Server configurations retain only the per-user key in their users/clients list and use the matching per-user traffic identity. Aggregate inbound counters are ignored for these managed users to prevent double charging.

The pinned Xray version accepts the user array as `clients`; current upstream also accepts that compatibility field. The integration test validates generated configurations on the real bundled cores, authenticates two separate keys and rejects another key. Keep remote agents updated before assigning new protocols.

Classic AEAD inbounds retain their one-user accounting behavior. AEAD-2022 ChaCha multiuser is not offered: the pinned Xray implementation supports multiuser identity headers for the AES methods. Changing key size on an existing inbound requires compatible user keys; create another inbound or explicitly rotate incompatible credentials first.

Sources: [Xray configuration](https://xtls.github.io/en/config/inbounds/shadowsocks.html), [pinned Xray schema](https://github.com/XTLS/Xray-core/blob/v26.3.27/infra/conf/shadowsocks.go), [sing-box](https://sing-box.sagernet.org/configuration/inbound/shadowsocks/), [SIP002](https://shadowsocks.org/doc/sip002.html).
