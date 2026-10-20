# Installed GeoIP and Geosite browser

In **Routing**, choose a node and Xray, then open **Installed GeoIP and Geosite
lists**. Search installed tags, inspect counts, modification time and SHA-256,
copy an exact reference, or create a new rule. New rules select the correct node
and `ip`/`domain` field, with `direct` as their destination; choose the intended
outbound and save/deploy normally.

Both UI and API require an administrator. An administrator's read-scoped API key
may browse without modifying settings. There is no upload, arbitrary file path,
downloader or privileged shell operation. Older node agents return a bounded
error asking the operator to update the agent.

`GET /api/nodes/{id}/geo-assets?kind=geosite&q=ir&limit=100` returns metadata,
`total`, `matched`, and up to 100 `{tag, reference, count}` entries. `kind` is
`geoip` or `geosite`; search is a case-insensitive tag substring of at most 96
bytes. No individual IP ranges or domain lists are returned. Xray `.dat`
references are not sing-box `.srs` rule sets.

The agent reads only `geoip.dat` and `geosite.dat` under `XRAY_LOCATION_ASSET`
(native default `/usr/local/share/xray`, container `/opt/veloray/xray`). Files
must be regular and at most 64 MiB. The protobuf envelope is independently
indexed with bounded tag/entry counts; malformed indexes are rejected. Cache
entries expire on size/mtime changes and at least every five minutes. No
persistent index is written. Verified asset updates remain an operator action.

Tests inspect actual bundled assets, malformed/duplicate tags, searches, cache
replacement, administrator/read-key boundaries and browser rule creation,
including a mobile viewport.
