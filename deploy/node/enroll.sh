#!/usr/bin/env bash
# Loaded by the node installer after local TLS and health checks succeed.
vr_node_enroll() {
 [[ -n "${VELORAY_ENROLL_URL:-}" ]]||return 0
 [[ "$VELORAY_ENROLL_URL" =~ ^https://[^/?#]+/api/node-enrollment/complete$ ]]||{ vr_say 'Invalid HTTPS enrollment URL.' 'نشانی HTTPS ثبت نود معتبر نیست.' >&2;return 2; }
 [[ "${VELORAY_ENROLL_TOKEN:-}" =~ ^[A-Za-z0-9_-]{40,100}$ ]]||{ vr_say 'Missing or invalid enrollment token.' 'توکن ثبت نود معتبر نیست.' >&2;return 2; }
 local fingerprint payload response
 local -a trust=()
 [[ -z "${VELORAY_ENROLL_CA_FILE:-}" ]]||trust=(--cacert "$VELORAY_ENROLL_CA_FILE")
 fingerprint="$(openssl x509 -in "$VELORAY_NODE_CERT" -outform DER | sha256sum | awk '{print $1}')"
 payload="$(jq -nc --arg url "https://$node_url_host:$node_port" --arg token "$VELORAY_AGENT_TOKEN" --arg fingerprint "$fingerprint" '{agent_url:$url,agent_token:$token,certificate_sha256:$fingerprint}')"
 if ! response="$(curl --proto '=https' --tlsv1.2 --fail --silent --show-error --connect-timeout 10 --max-time 30 "${trust[@]}" -H 'Content-Type: application/json' -H "Authorization: Bearer $VELORAY_ENROLL_TOKEN" --data-binary @- "$VELORAY_ENROLL_URL" <<<"$payload")";then
  vr_say 'Node is installed, but enrollment failed. Issue a new token if expired; check panel HTTPS trust and agent reachability.' 'نود نصب شده ولی ثبت خودکار ناموفق بود. اعتبار توکن، گواهی HTTPS پنل و دسترسی پنل به نود را بررسی کنید.' >&2
  return 1
 fi
 jq -e '.registered==true and (.node_id|type)=="number"' <<<"$response" >/dev/null||return 1
 unset VELORAY_ENROLL_TOKEN payload
 vr_say 'Node identity and certificate verified. Enrollment token consumed.' 'هویت و گواهی نود تأیید شد. توکن یک‌بارمصرف استفاده شد.'
}
