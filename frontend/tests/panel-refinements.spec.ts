import { readFileSync, mkdirSync } from "node:fs";
import { dirname, join, resolve, extname } from "node:path";
import { fileURLToPath } from "node:url";
import { test, expect, type Page } from "@playwright/test";
import {
  outboundTemplate,
  setConfigPath,
  routingPreset,
  parsePorts,
} from "../src/lib/network-config";

const node = {
  id: 1,
  name: "Local",
  is_local: true,
  enabled: true,
  status: "ok",
  config_patch: {},
};
test("sales workspace publishes an explicit-currency offer and preserves the sidebar", async ({
  page,
}) => {
  const offer = {
    id: 1,
    owner_id: 1,
    name: "Monthly",
    description: "",
    enabled: true,
    service_id: 7,
    amount: 10000,
    currency: "IRT",
    duration_days: 30,
    traffic_bytes: 107374182400,
    cost_amount: 2000,
  };
  await panel(page, "/services", {
    services: {
      json: [
        { id: 7, name: "Primary", note: "", inbound_ids: [], defaults: {} },
      ],
    },
    "admin/users": { json: [{ id: 1, username: "admin" }] },
    "commerce/settings": {
      json: {
        payments_enabled: false,
        merchant_configured: false,
        sandbox: false,
      },
    },
    "commerce/offers": { json: [offer] },
    "commerce/orders": { json: [] },
    "commerce/wallets": {
      json: [{ currency: "IRT", balance: 70000, credit_limit: 0 }],
    },
    "commerce/ledger": { json: [] },
    "commerce/store": {
      json: {
        enabled: true,
        name: "My store",
        description: "",
        support_url: "",
        logo_url: "",
        url: "http://127.0.0.1:8861/shop?store=1",
      },
    },
    "commerce/reports": {
      json: {
        summary: [
          {
            currency: "IRT",
            revenue: 10000,
            orders: 1,
            unit_cost: 2000,
            operating_cost: 1000,
            profit: 7000,
          },
        ],
        by_offer: [],
        by_node: [],
      },
    },
  });
  await page
    .getByRole("button", { name: "Sales & billing", exact: true })
    .click();
  await expect(page.getByText("7,000 toman", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Offers", exact: true }).click();
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Price", { exact: true }).fill("15000");
  let posted: any;
  await page.route("**/api/commerce/offers/1?owner=1", async (route) => {
    posted = route.request().postDataJSON();
    await route.fulfill({ json: { ...offer, ...posted } });
  });
  await dialog.getByRole("button", { name: "Save", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  expect(posted.amount).toBe(15000);
  expect(posted.currency).toBe("IRT");
  expect(posted).not.toHaveProperty("id");
  expect(posted).not.toHaveProperty("owner_id");
  await page
    .getByRole("button", { name: "Wallet & ledger", exact: true })
    .click();
  await expect(
    page.getByText("70,000 toman", { exact: true }).first(),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "test-results/commerce-wallet-mobile.png",
    fullPage: true,
  });
});
test("public storefront supports Persian, light theme and private order delivery", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.addInitScript(() => {
    localStorage.setItem("veloray-locale", "en");
    localStorage.setItem("veloray-theme", "dark");
    sessionStorage.setItem(
      "veloray-order:order_abcdefghijklmnopqrstuvwx",
      "private-order-token-1234567890123456789",
    );
  });
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/shop/catalog")
      return route.fulfill({
        json: {
          enabled: true,
          name: "Green Connect",
          description: "Simple subscriptions",
          logo_url: "",
          support_url: "https://support.example",
          payments_enabled: true,
          offers: [
            {
              id: 1,
              name: "Monthly",
              amount: 10000,
              currency: "IRT",
              duration_days: 30,
              traffic_bytes: 107374182400,
            },
          ],
        },
      });
    if (path.includes("/api/shop/orders/")) {
      expect(route.request().headers().authorization).toContain(
        "private-order-token",
      );
      return route.fulfill({
        json: {
          id: "order_abcdefghijklmnopqrstuvwx",
          amount: 10000,
          currency: "IRT",
          state: "fulfilled",
          snapshot: { name: "Monthly" },
          subscription_url:
            "https://sub.example:100/sub/private-customer-token",
        },
      });
    }
    return route.fulfill({ status: 404, json: { detail: "unexpected route" } });
  });
  await page.goto("/shop?store=1");
  await expect(
    page.getByRole("heading", { name: "Choose your subscription" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Continue to payment" }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "Change theme" }).click();
  await expect(page.locator("html")).toHaveClass(/light/);
  await page.getByRole("button", { name: "Change language" }).click();
  await expect(page.locator("html")).toHaveAttribute("dir", "rtl");
  await expect(
    page.getByRole("heading", { name: "اشتراک خود را انتخاب کنید" }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "test-results/storefront-persian-mobile.png",
    fullPage: true,
  });
  await page.goto("/shop?store=1&order=order_abcdefghijklmnopqrstuvwx");
  await expect(
    page.locator(
      'input[value="https://sub.example:100/sub/private-customer-token"]',
    ),
  ).toBeVisible();
  expect(errors).toEqual([]);
});
async function panel(
  page: Page,
  path: string,
  overrides: Record<string, { json: any; status?: number }> = {},
) {
  const outbounds: any[] = [],
    rules: any[] = [];
  await page.addInitScript(() => {
    localStorage.setItem("veloray-theme", "dark");
    localStorage.setItem("veloray-locale", "en");
  });
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url()),
      resource = url.pathname.replace(/^\/api\//, "").replace(/\/$/, "");
    let json: any = {};
    if (overrides[resource]) {
      await route.fulfill(overrides[resource]);
      return;
    }
    if (resource === "auth/me")
      json = { id: 1, username: "admin", is_superuser: true, permissions: [] };
    else if (resource === "settings")
      json = {
        site_name: "VeloRay",
        accent: "emerald",
        default_locale: "en",
        dashboard_refresh_seconds: 60,
      };
    else if (resource === "nodes") json = [node];
    else if (/^nodes\/\d+\/balancing$/.test(resource))
      json = {
        balancers: [],
        outbound_tags: ["direct", "blocked"],
        revision: "a".repeat(64),
        observatory_configured: false,
      };
    else if (
      resource === "inbounds" ||
      resource === "clients/inbound-options" ||
      resource === "services" ||
      resource.startsWith("admin/")
    )
      json = [];
    else if (resource === "health")
      json = {
        status: "ok",
        database: "ok",
        version: "1.0",
        uptime_seconds: 95,
        started_at: new Date(Date.now() - 95000).toISOString(),
      };
    else if (resource === "system-info")
      json = {
        hostname: "Local",
        uptime_seconds: 9876543,
        panel_uptime_seconds: 95,
        cpu_percent: 0,
        cpu_count: 2,
        memory_total_bytes: 100,
        memory_used_bytes: 50,
        memory_available_bytes: 50,
        memory_percent: 50,
        swap_total_bytes: 0,
        swap_used_bytes: 0,
        swap_free_bytes: 0,
        swap_percent: 0,
        disk_total_bytes: 100,
        disk_used_bytes: 50,
        disk_free_bytes: 50,
        disk_percent: 50,
        load_average: [0, 0, 0],
      };
    else if (resource.endsWith("/sni-scan"))
      json = {
        results: Array.from({ length: 14 }, (_, index) => ({
          domain: index === 0 ? "www.cloudflare.com" : `edge-${index}.example`,
          valid: index % 2 === 0,
          latency_ms: 30 + index,
          tls_version: "TLS 1.3",
          alpn: "h2",
          error: "Certificate validation failed",
        })),
      };
    else if (
      resource.startsWith("outbounds") ||
      resource.startsWith("routing_rules")
    ) {
      const entries = resource.startsWith("outbounds") ? outbounds : rules;
      if (route.request().method() === "POST") {
        json = { ...route.request().postDataJSON(), id: entries.length + 1 };
        entries.push(json);
      } else if (route.request().method() === "PATCH") {
        const item = entries.find((i) => i.id === +resource.split("/")[1]);
        Object.assign(item, route.request().postDataJSON());
        json = item;
      } else if (route.request().method() === "DELETE") {
        entries.splice(
          entries.findIndex((i) => i.id === +resource.split("/")[1]),
          1,
        );
        json = {};
      } else json = entries;
    } else if (resource === "overview")
      json = {
        nodes_total: 1,
        nodes_online: 1,
        clients_total: 1,
        clients_active: 1,
        clients_expiring_7d: 0,
        inbounds_total: 1,
        protocols: [],
        traffic_used_bytes: 9 * 1024 ** 3,
        traffic_raw_bytes: 3 * 1024 ** 3,
        traffic_upload_bytes: 1024 ** 3,
        traffic_download_bytes: 2 * 1024 ** 3,
      };
    else if (resource === "clients") json = [];
    else if (resource === "traffic-history")
      json = [0, 1, 2].map((i) => ({
        at: new Date(Date.now() - (2 - i) * 300000).toISOString(),
        raw_upload_traffic_bytes: i * 1024 ** 3,
        raw_download_traffic_bytes: i * 2 * 1024 ** 3,
        upload_bytes: 1024 ** 3,
        download_bytes: 2 * 1024 ** 3,
        interval_seconds: 300,
      }));
    await route.fulfill({ json });
  });
  await page.goto(path);
  await expect(page.locator("#main-content")).toBeVisible();
}
async function choose(page: Page, label: string, value: string, dialog = true) {
  const root = dialog
    ? page.getByRole("dialog")
    : page.locator("#main-content");
  await root.getByRole("combobox", { name: label, exact: true }).click();
  await page.locator(`.select-menu [data-value="${value}"]`).click();
}

test("installed geo assets load on demand, filter and create a rule", async ({
  page,
}) => {
  const requests: URL[] = [];
  await panel(page, "/routing");
  await page.route("**/api/nodes/1/geo-assets?*", async (route) => {
    const url = new URL(route.request().url());
    requests.push(url);
    await route.fulfill({
      json: {
        kind: "geosite",
        sha256: "a".repeat(64),
        size: 1048576,
        modified: "2026-10-20T12:00:00Z",
        total: 200,
        matched: url.searchParams.get("q") === "none" ? 0 : 1,
        entries:
          url.searchParams.get("q") === "none"
            ? []
            : [{ tag: "ir", reference: "geosite:ir", count: 45 }],
      },
    });
  });
  expect(requests).toHaveLength(0);
  await page
    .getByText("Installed GeoIP and Geosite lists", { exact: true })
    .click();
  await expect(page.getByText("geosite:ir", { exact: true })).toBeVisible();
  await page
    .getByRole("button", { name: "Create rule geosite:ir", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await dialog
    .getByRole("button", { name: "Save & deploy", exact: true })
    .click();
  await expect(dialog).not.toBeVisible();
  await expect(page.locator("#main-content")).toContainText("geosite:ir");
  await page.getByLabel("Search tags", { exact: true }).fill("none");
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await expect(
    page.getByText("No matching tags.", { exact: true }),
  ).toBeVisible();
  expect(requests.at(-1)?.searchParams.get("q")).toBe("none");
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});

test("readable dark and light palette, raw upload and download", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await panel(page, "/dashboard", {
    settings: {
      json: {
        site_name: "VeloRay",
        accent: "amber",
        default_locale: "en",
        dashboard_refresh_seconds: 60,
      },
    },
  });
  expect(
    await page.evaluate(() =>
      getComputedStyle(document.documentElement)
        .getPropertyValue("--brand")
        .trim(),
    ),
  ).toBe("#67e8a5");
  const traffic = page.locator(".traffic-directions");
  await expect(traffic.getByText("1.00 GB", { exact: true })).toBeVisible();
  await expect(traffic.getByText("2.00 GB", { exact: true })).toBeVisible();
  await expect(
    page.locator("#main-content").getByText("9.00 GB", { exact: true }),
  ).toHaveCount(0);
  await expect(
    page.locator('[aria-label="Traffic chart"] canvas'),
  ).toBeVisible();
  await preview(page, "dashboard-dark");
  expect(
    await page.evaluate(() =>
      getComputedStyle(document.documentElement)
        .getPropertyValue("--bg")
        .trim(),
    ),
  ).toBe("#101318");
  await page.getByRole("button", { name: "Theme: dark" }).click();
  await expect(page.locator("html")).toHaveClass(/light/);
  expect(
    await page.evaluate(() =>
      getComputedStyle(document.documentElement)
        .getPropertyValue("--bg")
        .trim(),
    ),
  ).toBe("#f5f6f8");
  expect(
    await page.evaluate(() =>
      getComputedStyle(document.documentElement)
        .getPropertyValue("--brand")
        .trim(),
    ),
  ).toBe("#177444");
  await page
    .getByRole("button", { name: "Transfer rate", exact: true })
    .click();
  await page.screenshot({
    path: "test-results/refined-dashboard-light.png",
    fullPage: true,
  });
  expect(errors).toEqual([]);
});
test("outbound defaults, guided creation, duplicate, status and delete", async ({
  page,
}) => {
  await panel(page, "/outbounds");
  await expect(page.locator(".network-builtin-row")).toHaveCount(2);
  await expect(
    page.getByRole("button", { name: "Inspect direct", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "New outbound", exact: true }).click();
  const form = page.getByRole("dialog");
  await form.getByLabel("Name", { exact: true }).fill("Edge");
  await form.getByLabel("Tag", { exact: true }).fill("edge");
  await choose(page, "Protocol", "vless");
  await form
    .getByLabel("Server address", { exact: true })
    .fill("edge.example.com");
  await form
    .getByLabel("UUID", { exact: true })
    .fill("11111111-1111-4111-8111-111111111111");
  await form.getByLabel("SNI", { exact: true }).fill("edge.example.com");
  await choose(page, "Next hop (optional)", "direct");
  const request = page.waitForRequest(
    (r) => r.method() === "POST" && r.url().includes("/api/outbounds/"),
  );
  await form
    .getByRole("button", { name: "Save & deploy", exact: true })
    .click();
  const data = (await request).postDataJSON();
  expect(data.config.settings.vnext[0].address).toBe("edge.example.com");
  expect(data.config.streamSettings.sockopt.dialerProxy).toBe("direct");
  expect(data.config.streamSettings.tlsSettings.serverName).toBe(
    "edge.example.com",
  );
  await expect(
    page.locator("tbody").getByText("Edge", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Disable Edge", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Enable Edge", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Duplicate", exact: true }).click();
  await expect(
    page.getByRole("dialog").getByLabel("Tag", { exact: true }),
  ).toHaveValue("edge-copy");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Cancel", exact: true })
    .click();
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Delete", exact: true })
    .click();
  await expect(page.locator("tbody tr:not(.network-builtin-row)")).toHaveCount(
    0,
  );
});
test("routing defaults, presets and sing-box block action", async ({
  page,
}) => {
  await panel(page, "/routing");
  await expect(
    page
      .locator(".network-system-card")
      .getByText("Panel API", { exact: true }),
  ).toBeVisible();
  await choose(page, "Core", "singbox", false);
  await expect(page.locator(".network-system-card")).toHaveCount(1);
  await page.getByRole("button", { name: "New rule", exact: true }).click();
  const form = page.getByRole("dialog");
  await form.getByLabel("Name", { exact: true }).fill("Private IPs");
  await form
    .getByRole("button", { name: "Block private IPs", exact: true })
    .click();
  await form.getByLabel("Ports", { exact: true }).fill("443,8000-9000");
  const request = page.waitForRequest(
    (r) => r.method() === "POST" && r.url().includes("/api/routing_rules/"),
  );
  await form
    .getByRole("button", { name: "Save & deploy", exact: true })
    .click();
  const data = (await request).postDataJSON();
  expect(data.config).toMatchObject({
    action: "reject",
    ip_is_private: true,
    port: [443],
    port_range: ["8000:9000"],
  });
  expect(data.config.outbound).toBeUndefined();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: "test-results/refined-routing-mobile.png",
    fullPage: true,
  });
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
});
test("Xray balancing deploys groups, keeps conflict drafts and selects a routing destination", async ({
  page,
}) => {
  await panel(page, "/routing");
  let state = {
    balancers: [] as any[],
    outbound_tags: ["direct", "edge-a", "edge-b"],
    revision: "b".repeat(64),
    observatory_configured: false,
  };
  let conflict = false;
  await page.route("**/api/nodes/1/balancing", async (route) => {
    if (route.request().method() === "PUT") {
      if (conflict) {
        await route.fulfill({
          status: 409,
          json: {
            detail: "balancing configuration changed; reload before saving",
          },
        });
        return;
      }
      const input = route.request().postDataJSON();
      expect(input.revision).toBe(state.revision);
      state = {
        ...state,
        balancers: input.balancers,
        revision: "c".repeat(64),
      };
    }
    await route.fulfill({ json: state });
  });
  await page.getByText("Xray load balancing", { exact: true }).click();
  await page
    .getByRole("button", { name: "Reload balancing", exact: true })
    .click();
  await page.getByRole("button", { name: "Add balancer", exact: true }).click();
  await page.getByLabel("Balancer tag", { exact: true }).fill("pool");
  await page.getByLabel("Selector prefixes", { exact: true }).fill("edge-");
  await choose(page, "Balancing strategy", "roundrobin", false);
  await choose(page, "Fallback outbound", "direct", false);
  await expect(
    page.getByText("Matching outbounds: edge-a, edge-b", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Save balancing & deploy", exact: true })
    .click();
  await expect(page.getByRole("status")).toHaveText(
    "Balancing saved and deployed.",
  );
  expect(state.balancers).toEqual([
    {
      tag: "pool",
      selector: ["edge-"],
      strategy: { type: "roundrobin" },
      fallbackTag: "direct",
    },
  ]);
  await page.getByRole("button", { name: "New rule", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Name", { exact: true }).fill("Balanced TCP");
  await choose(page, "Destination", "balancer:pool");
  await choose(page, "Network", "tcp");
  const request = page.waitForRequest(
    (r) => r.method() === "POST" && r.url().includes("/api/routing_rules/"),
  );
  await dialog
    .getByRole("button", { name: "Save & deploy", exact: true })
    .click();
  const data = (await request).postDataJSON();
  expect(data.config).toMatchObject({ balancerTag: "pool", network: "tcp" });
  expect(data.config.outboundTag).toBeUndefined();
  await expect(dialog).not.toBeVisible();
  await expect(
    page.getByRole("cell", { name: "pool", exact: true }),
  ).toBeVisible();
  conflict = true;
  await page.getByLabel("Balancer tag", { exact: true }).fill("my-draft");
  await page
    .getByRole("button", { name: "Save balancing & deploy", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText("configuration changed");
  await expect(page.getByLabel("Balancer tag", { exact: true })).toHaveValue(
    "my-draft",
  );
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
});

test("inbound fallback editor preserves fields and hides unsupported transports", async ({
  page,
}) => {
  await panel(page, "/inbounds");
  await page.getByRole("button", { name: "New inbound", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await choose(page, "Connection preset", "trojan");
  await dialog
    .getByRole("button", { name: "Add fallback", exact: true })
    .click();
  await dialog
    .getByLabel("Destination", { exact: true })
    .fill("127.0.0.1:8080");
  await dialog
    .getByLabel("TLS certificate", { exact: true })
    .fill("/etc/cert.pem");
  await dialog
    .getByLabel("TLS private key", { exact: true })
    .fill("/etc/key.pem");
  const request = page.waitForRequest(
    (r) => r.method() === "POST" && r.url().includes("/api/inbounds/"),
  );
  await dialog
    .getByRole("button", { name: "Create & deploy", exact: true })
    .click();
  expect((await request).postDataJSON().protocol_settings.fallbacks).toEqual([
    { dest: "127.0.0.1:8080", xver: 0 },
  ]);
  await expect(dialog).not.toBeVisible();
  await page.getByRole("button", { name: "New inbound", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Add fallback", exact: true }),
  ).toHaveCount(0);
  await choose(page, "Connection preset", "websocket");
  await expect(
    page.getByRole("button", { name: "Add fallback", exact: true }),
  ).toHaveCount(0);
});

test("editing credentials preserves advanced fields and templates remain isolated", () => {
  const config = outboundTemplate("xray", "vless");
  config.streamSettings.sockopt = { tcpFastOpen: true };
  const changed = setConfigPath(
    config,
    ["settings", "vnext", 0, "address"],
    "edge.example.com",
  );
  expect(changed.streamSettings.sockopt).toEqual({ tcpFastOpen: true });
  expect(config.settings.vnext[0].address).toBe("");
  expect(
    outboundTemplate("xray", "vless").streamSettings.sockopt,
  ).toBeUndefined();
  expect(routingPreset("singbox", "private")).toEqual({
    action: "reject",
    ip_is_private: true,
  });
  expect(() => parsePorts("9000-8000")).toThrow();
  expect(() => parsePorts("65536")).toThrow();
});

test("client IP and registered device dialogs on desktop and mobile", async ({
  page,
}) => {
  await page.addInitScript(() => {
    localStorage.setItem("veloray-theme", "dark");
    localStorage.setItem("veloray-locale", "en");
  });
  let unavailable = false,
    fail = false;
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/\/$/, "");
    let json: any = {};
    if (path === "/api/auth/me")
      json = { id: 1, username: "admin", is_superuser: true, permissions: [] };
    else if (path === "/api/settings")
      json = {
        site_name: "VeloRay",
        default_locale: "en",
        default_expiry_days: 30,
      };
    else if (
      path === "/api/inbounds" ||
      path === "/api/clients/inbound-options"
    )
      json = [{ id: 1, name: "main", supports_clients: true }];
    else if (path === "/api/services") json = [];
    else if (path === "/api/clients")
      json = [
        {
          id: 7,
          name: "alice",
          inbound: 1,
          inbound_name: "main",
          node_name: "Local",
          enabled: true,
          online: true,
          used_traffic_bytes: 0,
          traffic_limit_bytes: 0,
          usage_metered: true,
          device_limit: 0,
          device_count: 0,
          account_id: 0,
          protocol: "vless",
          expires_at: null,
          subscription_url: "/sub/test",
          share_link: "vless://test",
          effective_traffic_multiplier: 1,
        },
      ];
    else if (path === "/api/clients/7/ips") {
      if (fail) {
        await route.fulfill({
          status: 503,
          json: { detail: "Connection service unavailable" },
        });
        return;
      }
      json = {
        count: unavailable ? 0 : 1,
        available: !unavailable,
        observed_at: new Date().toISOString(),
        items: unavailable
          ? []
          : [
              {
                ip: "203.0.113.7",
                last_seen_at: new Date().toISOString(),
                connections: [
                  {
                    node: 1,
                    node_name: "Local",
                    inbound: 1,
                    inbound_name: "main",
                    core: "xray",
                  },
                ],
              },
            ],
      };
    } else if (path === "/api/clients/7/devices")
      json = {
        count: 1,
        items: [
          {
            id: 1,
            hwid: "device-123",
            device_model: "iPhone 14",
            device_os: "iOS",
            os_version: "18.3",
            user_agent: "TestApp/1.0",
            first_seen_at: "2026-10-11T10:00:00Z",
            last_seen_at: "2026-10-11T11:00:00Z",
          },
        ],
      };
    else if (path === "/api/admin/users") json = [];
    await route.fulfill({ json });
  });
  await page.goto("/clients");
  await page.getByPlaceholder("Search clients...").fill("alice");
  await page.getByRole("button", { name: "Actions for alice" }).click();
  const menu = page.getByRole("menu");
  expect(
    await menu.evaluate((element) => element.parentElement === document.body),
  ).toBeTruthy();
  const box = await menu.boundingBox();
  expect(box!.y).toBeGreaterThanOrEqual(0);
  expect(box!.y + box!.height).toBeLessThanOrEqual(1000);
  await page.screenshot({ path: "test-results/client-menu-overlay.png" });
  await page
    .getByRole("menuitem", { name: "IP's", exact: true })
    .filter({ visible: true })
    .click();
  let dialog = page.getByRole("dialog");
  await expect(dialog.getByText("203.0.113.7", { exact: true })).toBeVisible();
  unavailable = true;
  await dialog.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(
    dialog.getByText("Some connection data is unavailable"),
  ).toBeVisible();
  await expect(dialog.getByText("203.0.113.7", { exact: true })).toHaveCount(0);
  await expect(dialog.getByText("No connected IPs")).toHaveCount(0);
  fail = true;
  await dialog.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText(
    "Connection service unavailable",
  );
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await page.getByRole("button", { name: "Actions for alice" }).click();
  await page
    .getByRole("menuitem", { name: "Device's", exact: true })
    .filter({ visible: true })
    .click();
  dialog = page.getByRole("dialog");
  await expect(dialog.getByText("iPhone 14", { exact: true })).toBeVisible();
  await expect(dialog.getByText("device-123", { exact: true })).toBeVisible();
  await expect(
    dialog.getByText("Last subscription update", { exact: true }),
  ).toBeVisible();
  await expect(dialog.getByText("Online", { exact: true })).toHaveCount(0);
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await page.setViewportSize({ width: 390, height: 844 });
  await page
    .getByRole("button", { name: "Device's", exact: true })
    .filter({ visible: true })
    .click();
  await expect(
    page.getByRole("dialog").getByText("iPhone 14", { exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
});

test("SNI and configuration dialogs have bounded size, padding and nested focus", async ({
  page,
}) => {
  await panel(page, "/outbounds");
  await page
    .getByRole("button", { name: "Inspect direct", exact: true })
    .click();
  const config = page.getByRole("dialog", {
    name: "Configuration",
    exact: true,
  });
  expect(
    await config.evaluate((el) => parseFloat(getComputedStyle(el).paddingLeft)),
  ).toBeGreaterThanOrEqual(18);
  await page.screenshot({ path: "test-results/configuration-dialog.png" });
  await page.keyboard.press("Escape");
  await page
    .getByRole("button", { name: "Inbounds", exact: true })
    .filter({ visible: true })
    .click();
  await page.getByRole("button", { name: "New inbound", exact: true }).click();
  const parent = page.getByRole("dialog", { name: "New inbound", exact: true });
  await parent.getByRole("button", { name: "Scan SNI", exact: true }).click();
  const scanner = page.getByRole("dialog", {
    name: "SNI scanner",
    exact: true,
  });
  await scanner.getByRole("button", { name: "Find SNI", exact: true }).click();
  await expect(scanner.locator(".sni-result")).toHaveCount(14);
  const box = await scanner.boundingBox();
  expect(box!.width).toBeLessThanOrEqual(540);
  expect(box!.height).toBeLessThanOrEqual(880);
  expect(
    await scanner.evaluate((el) =>
      parseFloat(getComputedStyle(el).paddingLeft),
    ),
  ).toBeGreaterThanOrEqual(18);
  await page.screenshot({ path: "test-results/sni-dialog-dark.png" });
  await page.keyboard.press("Escape");
  await expect(scanner).toHaveCount(0);
  await expect(parent).toBeVisible();
  expect(await page.evaluate(() => document.body.style.overflow)).toBe(
    "hidden",
  );
  await page.keyboard.press("Escape");
  await expect(parent).toHaveCount(0);
  expect(await page.evaluate(() => document.body.style.overflow)).not.toBe(
    "hidden",
  );
  await page.getByRole("button", { name: "Theme: dark" }).click();
  await expect(page.locator("html")).toHaveClass(/light/);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "New inbound", exact: true }).click();
  await parent.getByRole("button", { name: "Scan SNI", exact: true }).click();
  await scanner.getByRole("button", { name: "Find SNI", exact: true }).click();
  await expect(scanner.locator(".sni-result")).toHaveCount(14);
  const small = await scanner.boundingBox();
  expect(small!.x).toBeGreaterThanOrEqual(12);
  expect(small!.x + small!.width).toBeLessThanOrEqual(378);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
  await page.screenshot({ path: "test-results/sni-dialog-light-mobile.png" });
});

test("Settings reports database and panel uptime even if host information fails", async ({
  page,
}) => {
  await panel(page, "/settings", {
    "system-info": {
      status: 503,
      json: { detail: "Host metrics unavailable" },
    },
  });
  await expect(page.getByText("Connected", { exact: true })).toBeVisible();
  await expect(page.getByText("1m", { exact: true })).toBeVisible();
  await expect(page.getByText("Checking…", { exact: true })).toHaveCount(0);
  await page.screenshot({ path: "test-results/settings-status.png" });
});

test("custom chart range supports keyboard and pointer controls", async ({
  page,
}) => {
  await panel(page, "/dashboard");
  const start = page.getByRole("slider", { name: "Chart start", exact: true }),
    end = page.getByRole("slider", { name: "Chart end", exact: true });
  // Traffic loads asynchronously; focusing a disabled handle is a no-op.
  await expect(start).toBeEnabled();
  await expect(end).toBeEnabled();
  await start.focus();
  await page.keyboard.press("ArrowRight");
  await expect(start).toHaveAttribute("aria-valuenow", "1");
  await end.focus();
  await page.keyboard.press("ArrowLeft");
  await expect(end).toHaveAttribute("aria-valuenow", "99");
  const box = await page.locator(".chart-window-track").boundingBox();
  await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
  await page.mouse.down();
  await page.mouse.move(box!.x + box!.width * 0.55, box!.y + box!.height / 2);
  await page.mouse.up();
  await page.getByRole("button", { name: "Full range", exact: true }).click();
  await expect(start).toHaveAttribute("aria-valuenow", "0");
  await expect(end).toHaveAttribute("aria-valuenow", "100");
  await expect(page.locator('input[type="range"]')).toHaveCount(0);
  await page.screenshot({ path: "test-results/chart-range.png" });
});

test("compact subscription portal fits desktop and mobile while preserving connection details", async ({
  page,
}) => {
  const file = process.env.VELORAY_TEST_PORTAL_HTML;
  test.skip(!file, "portal render fixture is required");
  const html = readFileSync(file!, "utf8");
  await page.route("**/portal-preview", (route) =>
    route.fulfill({ contentType: "text/html", body: html }),
  );
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/portal-preview");
  const copyButton = page.getByRole("button", {
    name: "Copy link",
    exact: true,
  });
  await expect(copyButton).toBeVisible();
  await copyButton.click();
  await expect(page.getByRole("status")).toHaveText("Link copied");
  for (const viewport of [
    { width: 1280, height: 720 },
    { width: 390, height: 844 },
    { width: 320, height: 568 },
  ]) {
    await page.setViewportSize(viewport);
    expect(
      await page.evaluate(
        () =>
          document.documentElement.scrollWidth <= innerWidth &&
          document.documentElement.scrollHeight <= innerHeight,
      ),
    ).toBeTruthy();
  }
  await expect(page.locator("details[open]")).toHaveCount(0);
  await expect(
    page.getByRole("link", { name: "Base64", exact: true }),
  ).toHaveAttribute("href", /format=base64$/);
  await page.getByText("Connection URI", { exact: true }).click();
  await expect(
    page.getByRole("textbox", { name: "Connection URI", exact: true }),
  ).toHaveValue("vless://example");
  await page.getByText("Connection URI", { exact: true }).click();
  await page.emulateMedia({ colorScheme: "light" });
  await page.screenshot({ path: "test-results/subscription-light-mobile.png" });
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.emulateMedia({ colorScheme: "dark" });
  await page.screenshot({ path: "test-results/subscription-dark-desktop.png" });
});

test("theme reveals a circular palette and respects reduced motion", async ({
  page,
}) => {
  await panel(page, "/dashboard");
  await page.evaluate(() => {
    const original = Element.prototype.animate;
    Element.prototype.animate = function (keyframes, options) {
      if (
        typeof options === "object" &&
        options.pseudoElement === "::view-transition-new(root)"
      )
        document.documentElement.dataset.themeReveal =
          JSON.stringify(keyframes);
      return original.call(this, keyframes, options);
    };
  });
  await page.getByRole("button", { name: "Theme: dark", exact: true }).click();
  await expect(page.locator("html")).toHaveClass(/light/);
  await expect(page.locator("html")).toHaveAttribute(
    "data-theme-reveal",
    /circle\(0px/,
  );
  expect(
    await page
      .locator('nav [aria-current="page"]')
      .evaluate((el) => getComputedStyle(el).boxShadow),
  ).toBe("none");
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.evaluate(
    () => delete document.documentElement.dataset.themeReveal,
  );
  await page.getByRole("button", { name: "Theme: light", exact: true }).click();
  await expect(page.locator("html")).toHaveClass(/dark/);
  await expect(page.locator("html")).not.toHaveAttribute("data-theme-reveal");
});

test("subscription dashboard supports chart, four languages, search, QR and mobile", async ({
  page,
}) => {
  const fixture = process.env.VELORAY_TEST_MODERN_PORTAL_HTML;
  test.skip(!fixture, "Modern subscription HTML fixture is required");
  const html = readFileSync(fixture!, "utf8").replaceAll(
    "https://panel.example/sub/",
    "http://127.0.0.1:8861/sub/",
  );
  await page.addInitScript(() => {
    localStorage.setItem("veloray-subscription-lang", "en");
    localStorage.setItem("veloray-subscription-theme", "light");
  });
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.route("**/portal-modern-preview", (route) =>
    route.fulfill({ contentType: "text/html", body: html }),
  );
  // Use a deterministic same-origin response so rendering is tested without
  // depending on the development-server proxy in this HTML fixture test.
  await page.addInitScript(() => {
    const originalFetch = window.fetch.bind(window);
    window.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
      const path = new URL(
        typeof input === "string"
          ? input
          : input instanceof Request
            ? input.url
            : String(input),
        location.href,
      );
      if (
        path.pathname.startsWith("/sub/") &&
        path.searchParams.get("format") === "stats"
      ) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              range: "7d",
              history_available: true,
              points: [
                {
                  at: "2026-10-15T00:00:00Z",
                  bytes: 1048576,
                  upload_bytes: 524288,
                  download_bytes: 524288,
                },
                {
                  at: "2026-10-16T00:00:00Z",
                  bytes: 2097152,
                  upload_bytes: 1048576,
                  download_bytes: 1048576,
                },
              ],
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        );
      }
      return originalFetch(input, init);
    };
  });
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/portal-modern-preview");
  await expect(page.locator('[data-portal-version="2"]')).toBeVisible();
  await expect(page.getByRole("heading", { name: "Arman" })).toBeVisible();
  await expect(page.locator(".stat-date")).toHaveText("No expiry");
  await expect(page.locator("#traffic-chart")).toBeVisible();
  await expect(page.locator("#chart-total")).toContainText("MB");
  await page.locator('[data-metric="upload_bytes"]').click();
  await expect(page.locator('[data-metric="upload_bytes"]')).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await expect(page.locator("#traffic-chart")).toHaveAttribute(
    "aria-label",
    /Upload/,
  );
  await page.locator('[data-metric="download_bytes"]').click();
  await expect(page.locator("#traffic-chart")).toHaveAttribute(
    "aria-label",
    /Download/,
  );
  await page.locator('[data-metric="bytes"]').click();
  await expect(page.locator("#download-traffic")).toHaveAttribute(
    "href",
    /(?=.*export=csv)(?=.*range=7d)/,
  );
  const copy = page.getByRole("button", { name: "Copy link" });
  await copy.click();
  await expect(page.locator("#copy-feedback")).toHaveText("Link copied");
  await expect(
    page.getByRole("button", { name: "QR code" }).first(),
  ).toBeVisible();
  await page.getByRole("button", { name: "QR code" }).first().click();
  await expect(page.locator("#qr-dialog")).toBeVisible();
  await page.locator("#close-qr").click();
  await page
    .getByPlaceholder("Search name, protocol or server")
    .fill("missing");
  await expect(page.locator(".profile-item:visible")).toHaveCount(0);
  await expect(page.locator("#profiles-empty")).toBeVisible();
  await page
    .getByPlaceholder("Search name, protocol or server")
    .fill("primary");
  await expect(page.locator(".profile-item:visible")).toHaveCount(1);
  await page.locator("#portal-language").selectOption("fa");
  await expect(page.locator("html")).toHaveAttribute("dir", "rtl");
  await expect(
    page.getByRole("heading", { name: "نمودار مصرف ترافیک", exact: true }),
  ).toBeVisible();
  await page.locator("#portal-language").selectOption("ru");
  await expect(
    page.getByRole("heading", { name: "История трафика", exact: true }),
  ).toBeVisible();
  await page.locator("#portal-language").selectOption("zh");
  await expect(
    page.getByRole("heading", { name: "流量概览", exact: true }),
  ).toBeVisible();
  await page.locator("#toggle-theme").click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  for (const width of [390, 320]) {
    await page.setViewportSize({ width, height: 700 });
    await expect
      .poll(() =>
        page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      )
      .toBe(true);
  }
  await page.screenshot({
    path: "test-results/modern-subscription-mobile.png",
    fullPage: true,
  });
  await page.locator("#portal-language").selectOption("fa");
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: "test-results/subscription-dashboard-fa-dark.png",
    fullPage: true,
  });
  await page.locator("#portal-language").selectOption("en");
  await page.locator("#toggle-theme").click();
  await page.setViewportSize({ width: 1280, height: 1000 });
  await page.screenshot({
    path: "test-results/subscription-dashboard-en-light.png",
    fullPage: true,
  });
  expect(errors).toEqual([]);
});

test("simple subscription page has persistent themes, copying, QR and no horizontal overflow", async ({
  page,
}) => {
  const file = process.env.VELORAY_TEST_SIMPLE_PORTAL_HTML;
  test.skip(!file, "simple portal render fixture is required");
  const html = readFileSync(file!, "utf8");
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.route("**/simple-preview", (route) =>
    route.fulfill({ contentType: "text/html", body: html }),
  );
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.route("**/sub/**", (route) =>
    route.fulfill({
      json: {
        points: [
          {
            at: "2026-10-21T00:00:00Z",
            bytes: 1048576,
            upload_bytes: 262144,
            download_bytes: 786432,
          },
        ],
      },
    }),
  );
  await page.goto("/simple-preview");
  await expect(page.locator("#chart-points circle")).toHaveCount(1);
  await expect(page.locator("#traffic-chart")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Arman" })).toBeVisible();
  await expect(page.locator("#traffic-chart")).toHaveCount(1);
  await page
    .getByRole("group", { name: "Appearance", exact: true })
    .getByRole("button", { name: "Dark", exact: true })
    .click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.reload();
  await expect(
    page
      .getByRole("group", { name: "Appearance", exact: true })
      .getByRole("button", { name: "Dark", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await page
    .getByRole("group", { name: "Appearance", exact: true })
    .getByRole("button", { name: "Light", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Copy link", exact: true })
    .first()
    .click();
  await expect(page.getByRole("status")).toHaveText("Link copied");
  await page.getByRole("button", { name: "QR", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).not.toBeVisible();
  for (const width of [1280, 390, 320]) {
    await page.setViewportSize({ width, height: 800 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: `test-results/simple-subscription-${width}-light.png`,
      fullPage: true,
    });
  }
  await page
    .getByRole("group", { name: "Appearance", exact: true })
    .getByRole("button", { name: "Dark", exact: true })
    .click();
  await page.screenshot({
    path: "test-results/simple-subscription-mobile-dark.png",
    fullPage: true,
  });
  expect(errors).toEqual([]);
});

test("subscription settings validate custom file, edit hosts and retain device policy", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await panel(page, "/settings", {
    "subscription-template/validate": { json: { valid: true, bytes: 500 } },
    inbounds: { json: [{ id: 1, name: "Main", node_name: "Local" }] },
  });
  await page.getByRole("button", { name: "Subscription", exact: true }).click();
  const template = page.getByRole("combobox", {
    name: "Template",
    exact: true,
  });
  await template.click();
  await page
    .getByRole("option", { name: "Custom HTML file", exact: true })
    .click();
  await page
    .getByLabel("HTML template file")
    .fill("/etc/veloray/templates/custom/index.html");
  await page.getByRole("button", { name: "Validate template" }).click();
  await expect(
    page.getByText("Template is valid. Save settings to activate it."),
  ).toBeVisible();
  await page.getByText("Subscription hosts (0)", { exact: true }).click();
  await page.getByRole("button", { name: "Add host" }).click();
  const fieldset = page.locator("fieldset").filter({
    has: page.getByRole("textbox", { name: "Public address", exact: true }),
  });
  await fieldset
    .getByRole("textbox", { name: "Name", exact: true })
    .fill("CDN");
  await fieldset
    .getByRole("textbox", { name: "Public address", exact: true })
    .fill("edge.example");
  await fieldset
    .getByRole("combobox", { name: "Inbound", exact: true })
    .click();
  await page.getByRole("option", { name: "Main · Local" }).click();
  await page.getByText("Subscription device policy", { exact: true }).click();
  await page.getByLabel("Default HWID device limit").fill("2");
  const saved = page.waitForRequest(
    (request) =>
      request.url().endsWith("/api/settings") && request.method() === "PATCH",
  );
  await page.getByRole("button", { name: "Save settings" }).click();
  const payload = (await saved).postDataJSON();
  expect(payload.subscription_template).toBe("custom");
  expect(payload.subscription_hwid_limit).toBe(2);
  expect(payload.subscription_hosts[0]).toMatchObject({
    name: "CDN",
    address: "edge.example",
    inbound: 1,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  expect(errors).toEqual([]);
});

async function managementPanel(page: Page, path: string) {
  const inbound = (id: number, node_name: string) => ({
    id,
    node: id,
    node_name,
    name: `Edge ${id}`,
    protocol: "vless",
    core: "xray",
    enabled: true,
    supports_clients: true,
  });
  const data: Record<string, any[]> = {
    customers: [],
    clients: [],
    services: [],
    profiles: [],
    snippets: [],
    "subscription-policies": [],
  };
  await panel(page, path, {
    inbounds: { json: [inbound(1, "North"), inbound(2, "South")] },
    "clients/inbound-options": {
      json: [inbound(1, "North"), inbound(2, "South")],
    },
    ...Object.fromEntries(Object.keys(data).map((k) => [k, { json: [] }])),
  });
  await page.route("**/api/**", async (route) => {
    const resource = new URL(route.request().url()).pathname
        .replace(/^\/api\//, "")
        .replace(/\/$/, ""),
      [kind, id, action] = resource.split("/"),
      rows = data[kind];
    if (resource === "clients/inbound-options") {
      await route.fulfill({ json: [inbound(1, "North"), inbound(2, "South")] });
      return;
    }
    if (!rows) {
      await route.fallback();
      return;
    }
    const method = route.request().method(),
      item = rows.find((i) => i.id === +id);
    if (action === "assign" && method === "POST") {
      Object.assign(item, route.request().postDataJSON());
      await route.fulfill({ json: { node_ids: item.node_ids } });
      return;
    }
    if (action === "revisions") {
      await route.fulfill({
        json: [{ revision: item.revision, created_at: "2026-10-18T12:00:00Z" }],
      });
      return;
    }
    if (method === "POST") {
      const v = {
        ...route.request().postDataJSON(),
        id: rows.length + 1,
        revision: 1,
        used_traffic_bytes: 123,
        subscription_url: "https://vpn.example/sub/stable-example-token",
        connections: [{}, {}],
      };
      rows.push(v);
      await route.fulfill({ json: v, status: 201 });
      return;
    }
    if (method === "PATCH") {
      Object.assign(item, route.request().postDataJSON());
      item.revision++;
      await route.fulfill({ json: item });
      return;
    }
    if (method === "DELETE") {
      rows.splice(rows.indexOf(item), 1);
      await route.fulfill({ status: 204 });
      return;
    }
    await route.fulfill({
      json: id
        ? item
        : rows.map(({ body, ...metadata }) =>
            kind === "profiles" || kind === "snippets"
              ? metadata
              : { ...metadata, body },
          ),
    });
  });
  await page.reload();
  await expect(page.getByRole("status", { name: "Loading…" })).toHaveCount(0);
  return data;
}
test("services keep plan defaults and managed fields in CRUD forms", async ({
  page,
}) => {
  const data = await managementPanel(page, "/services");
  await page.getByRole("button", { name: "Add service", exact: true }).click();
  let dialog = page.getByRole("dialog");
  await dialog.getByLabel("Name", { exact: true }).fill("Standard");
  await dialog.getByLabel("Edge 1 · North · vless").check();
  await dialog.getByLabel("Traffic quota (GiB)").fill("10");
  await dialog.getByLabel("HWID limit", { exact: true }).fill("2");
  await dialog.getByRole("combobox", { name: "Calendar renewal" }).click();
  await page.getByRole("option", { name: "Monthly", exact: true }).click();
  await dialog.getByLabel("Timezone", { exact: true }).fill("Asia/Tehran");
  await dialog.getByRole("button", { name: "Save", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(data.services[0].defaults.traffic_limit_bytes).toBe(10 * 1073741824);
  expect(data.services[0].defaults.renewal_period).toBe("month");
});
test("profile editor supports reusable snippets, node assignment and history", async ({
  page,
}) => {
  const data = await managementPanel(page, "/profiles");
  await page.getByRole("button", { name: "Snippets", exact: true }).click();
  await page.getByRole("button", { name: "Add snippet", exact: true }).click();
  let dialog = page.getByRole("dialog");
  await dialog.getByLabel("Name", { exact: true }).fill("Shared DNS");
  await dialog
    .getByLabel("Policy JSON", { exact: true })
    .fill('{"dns":{"queryStrategy":"UseIPv4"}}');
  await dialog.getByRole("button", { name: "Save", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await page.getByRole("button", { name: "Profiles", exact: true }).click();
  await page.getByRole("button", { name: "Add profile", exact: true }).click();
  dialog = page.getByRole("dialog");
  await dialog.getByLabel("Name", { exact: true }).fill("Regional policy");
  await dialog.getByLabel("Shared DNS", { exact: true }).check();
  await dialog
    .getByLabel("Xray policy JSON", { exact: true })
    .fill('{"routing":{"domainStrategy":"IPIfNonMatch"}}');
  await dialog.getByRole("button", { name: "Save", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(data.profiles[0].body.snippet_ids).toEqual([1]);
  await page.getByRole("button", { name: "Assign nodes", exact: true }).click();
  dialog = page.getByRole("dialog");
  await dialog.getByLabel("Local", { exact: true }).check();
  await dialog.getByRole("button", { name: "Save", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(data.profiles[0].node_ids).toEqual([1]);
  await page.getByRole("button", { name: "History", exact: true }).click();
  await expect(
    page.getByRole("dialog").getByText("Revision 1", { exact: false }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
});
test("management forms remain usable at 390px with a light theme", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await managementPanel(page, "/services");
  await page.evaluate(() => document.documentElement.classList.remove("dark"));
  await page.getByRole("button", { name: "Add service", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("removed customer URL redirects to the unified users screen", async ({
  page,
}) => {
  await managementPanel(page, "/customers");
  await expect(page).toHaveURL(/\/clients$/);
  await expect(
    page.getByRole("button", { name: "Customers", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Add client", exact: true }),
  ).toBeVisible();
});

test("outbound traffic stays scoped to the selected core and handles unavailable telemetry", async ({
  page,
}) => {
  const calls: string[] = [];
  await panel(page, "/outbounds", {
    outbounds: { json: [] },
    "outbound-traffic": {
      json: {
        available: true,
        collected_at: "2026-10-19T10:00:00Z",
        outbounds: [],
      },
    },
  });
  await page.route("**/api/outbound-traffic?**", async (route) => {
    const url = new URL(route.request().url());
    calls.push(url.search);
    const sing = url.searchParams.get("core") === "singbox";
    await route.fulfill({
      json: {
        available: !sing,
        collected_at: "2026-10-19T10:00:00Z",
        outbounds: sing
          ? []
          : [
              {
                tag: "direct",
                upload_bytes: 3 * 1073741824,
                download_bytes: 5 * 1073741824,
                period_upload_bytes: 1073741824,
                period_download_bytes: 2 * 1073741824,
              },
            ],
      },
    });
  });
  await page.getByText("Outbound traffic", { exact: true }).click();
  const table = page.getByRole("table", { name: "Outbound traffic · xray" });
  await expect(table).toBeVisible();
  await expect(
    table.getByRole("row", { name: "direct 1.000 2.000 3.000 5.000" }),
  ).toBeVisible();
  expect(calls[0]).toContain("core=xray&hours=24");
  await page
    .getByRole("combobox", { name: "Traffic period", exact: true })
    .click();
  await page.getByRole("option", { name: "7 days", exact: true }).click();
  await expect.poll(() => calls.at(-1)).toContain("hours=168");
  await page.getByRole("combobox", { name: "Core", exact: true }).click();
  await page.getByRole("option", { name: "sing-box", exact: true }).click();
  await expect(table).toHaveCount(0);
  await page.getByText("Outbound traffic", { exact: true }).click();
  await expect(
    page.getByText(
      "Current outbound telemetry is unavailable. Upgrade the node agent and apply its configuration.",
      { exact: true },
    ),
  ).toBeVisible();
  expect(calls.at(-1)).toContain("core=singbox&hours=24");
});

test("sing-box group editor exposes probe and switching controls", async ({
  page,
}) => {
  await panel(page, "/outbounds");
  await page.getByRole("combobox", { name: "Core", exact: true }).click();
  await page.getByRole("option", { name: "sing-box", exact: true }).click();
  await page.getByRole("button", { name: "New outbound", exact: true }).click();
  const form = page.getByRole("dialog");
  await form.getByLabel("Name", { exact: true }).fill("Auto edge");
  await form.getByLabel("Tag", { exact: true }).fill("auto-edge");
  await choose(page, "Protocol", "urltest");
  await form.getByLabel("Outbound tags", { exact: true }).fill("direct");
  await form
    .getByLabel("Probe URL", { exact: true })
    .fill("https://probe.example/generate_204");
  await form.getByLabel("Probe interval", { exact: true }).fill("1m");
  await form.getByLabel("Latency tolerance (ms)", { exact: true }).fill("120");
  await form.getByLabel("Probe idle timeout", { exact: true }).fill("15m");
  const interrupt = form.getByLabel(
    "Interrupt existing connections when switching",
    { exact: true },
  );
  await expect(interrupt).not.toBeChecked();
  await interrupt.check();
  const request = page.waitForRequest(
    (r) => r.method() === "POST" && r.url().includes("/api/outbounds/"),
  );
  await form
    .getByRole("button", { name: "Save & deploy", exact: true })
    .click();
  const body = (await request).postDataJSON();
  expect(body.config).toEqual({
    outbounds: ["direct"],
    url: "https://probe.example/generate_204",
    interval: "1m",
    tolerance: 120,
    idle_timeout: "15m",
    interrupt_exist_connections: true,
  });
  await expect(form).toHaveCount(0);
  await page.getByRole("button", { name: "New outbound", exact: true }).click();
  await choose(page, "Protocol", "direct");
  await expect(
    page.getByRole("combobox", { name: "Next hop (optional)", exact: true }),
  ).toHaveCount(0);
  await choose(page, "Protocol", "selector");
  await choose(page, "Default candidate", "direct");
  await expect(
    page.getByRole("combobox", { name: "Default candidate", exact: true }),
  ).toContainText("direct");
});

test("subscription policies assign customers and services without leaking external links in the list", async ({
  page,
}) => {
  const data = await managementPanel(page, "/subscription-policies");
  data.customers.push({
    id: 1,
    name: "Shared buyer",
    note: "",
    inbound_ids: [1],
    service_ids: [],
  });
  data.services.push({
    id: 1,
    name: "Standard",
    inbound_ids: [1],
    defaults: {},
  });
  await page.reload();
  await page.getByRole("button", { name: "Add policy", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Name", { exact: true }).fill("Regional feed");
  await dialog
    .getByLabel("External feed URL", { exact: true })
    .fill("https://feed.example/sub/private-url-token");
  await dialog
    .getByLabel("External connection links", { exact: true })
    .fill(
      "vless://955cc471-391e-4f25-bc37-c8336ed2be5b@edge.example:443?security=tls#External",
    );
  await dialog.getByLabel("Shared buyer", { exact: true }).check();
  await dialog.getByLabel("Standard", { exact: true }).check();
  await dialog.getByLabel("hiddify", { exact: true }).check();
  await dialog.getByText("Client templates", { exact: true }).click();
  await dialog
    .getByRole("textbox", { name: "clash", exact: true })
    .fill('{"mixed-port":7890}');
  await dialog.getByRole("button", { name: "Save", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  const row = data["subscription-policies"][0];
  expect(row.customer_ids).toEqual([1]);
  expect(row.service_ids).toEqual([1]);
  expect(row.body.apps).toEqual(["hiddify"]);
  expect(row.body.templates.clash).toBe('{"mixed-port":7890}');
  await expect(
    page.getByText("https://feed.example/sub/private-url-token", {
      exact: true,
    }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Edit Regional feed", exact: true })
    .click();
  await expect(
    page.getByRole("dialog").getByLabel("External feed URL", { exact: true }),
  ).toHaveValue("https://feed.example/sub/private-url-token");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("calendar preferences preserve Persian New Year and timezone boundaries", async ({
  page,
}) => {
  await panel(page, "/settings");
  await page.getByRole("button", { name: "System", exact: true }).click();
  await page.getByRole("combobox", { name: "Calendar", exact: true }).click();
  await page
    .getByRole("option", { name: "Persian (Jalali)", exact: true })
    .click();
  await page
    .getByLabel("Display timezone", { exact: true })
    .fill("Asia/Tehran");
  await page
    .getByRole("button", { name: "Save date preferences", exact: true })
    .click();
  await expect(page.getByRole("status")).toContainText("Saved");
  const output = await page.evaluate(async () => {
    const { formatDate } = await import("/src/lib/dates.ts");
    return formatDate("2024-03-19T21:00:00Z");
  });
  expect(output).toContain("1403");
  expect(output).toContain("01/01");
  await page
    .getByLabel("Display timezone", { exact: true })
    .fill("Not/A_Timezone");
  await page
    .getByRole("button", { name: "Save date preferences", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText("valid IANA timezone");
  expect(
    await page.evaluate(() => localStorage.getItem("veloray-timezone")),
  ).toBe("Asia/Tehran");
});

test("notification destinations support independent event routing and do not submit parent settings", async ({
  page,
}) => {
  const rows: any[] = [];
  await panel(page, "/settings");
  let settingsSaves = 0;
  await page.route("**/api/settings", async (route) => {
    if (route.request().method() !== "GET") settingsSaves++;
    await route.fallback();
  });
  await page.route("**/api/webhooks/endpoints**", async (route) => {
    const id = new URL(route.request().url()).pathname.split("/")[4];
    if (route.request().method() === "POST") {
      const row = { ...route.request().postDataJSON(), id: rows.length + 1 };
      rows.push(row);
      await route.fulfill({ status: 201, json: row });
      return;
    }
    await route.fulfill({
      json: id
        ? rows.find((r) => r.id === +id)
        : rows.map(({ url, signing_secret, ...rest }) => rest),
    });
  });
  await page.getByRole("button", { name: "Webhooks", exact: true }).click();
  await page
    .getByRole("button", { name: "Add destination", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Name", { exact: true }).fill("Operations");
  await dialog.getByRole("combobox", { name: "Channel", exact: true }).click();
  await page.getByRole("option", { name: "Discord", exact: true }).click();
  await dialog
    .getByLabel("HTTPS destination URL", { exact: true })
    .fill("https://discord.com/api/webhooks/test/private-token");
  await dialog
    .getByLabel("Event filters", { exact: true })
    .fill("node.*\nclient.quota");
  await dialog.getByLabel("Enabled", { exact: true }).check();
  await dialog.getByRole("button", { name: "Save", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(settingsSaves).toBe(0);
  expect(rows[0].events).toEqual(["node.*", "client.quota"]);
  await expect(
    page.getByText("https://discord.com/api/webhooks/test/private-token", {
      exact: true,
    }),
  ).toHaveCount(0);
});

test("GitHub and Telegram login availability preserves password login and handles provider failure", async ({
  page,
}) => {
  let enabled = true;
  let starts = 0;
  await page.addInitScript(() => {
    localStorage.setItem("veloray-locale", "en");
    localStorage.setItem("veloray-login-locale", "en");
  });
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/auth/me")
      await route.fulfill({
        status: 401,
        json: { detail: "Authentication required" },
      });
    else if (path === "/api/auth/oidc")
      await route.fulfill({
        json: { enabled, telegram: enabled, github_enabled: enabled },
      });
    else if (path === "/api/auth/github/start") {
      starts++;
      expect(route.request().method()).toBe("POST");
      await route.fulfill({
        status: 502,
        json: { detail: "Provider unavailable" },
      });
    } else await route.fulfill({ json: {} });
  });
  await page.goto("/");
  await expect(
    page.getByRole("button", { name: "Sign in with Telegram", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Sign in with GitHub", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText("Provider unavailable");
  expect(starts).toBe(1);
  await expect(page.getByLabel("Password", { exact: true })).toBeVisible();
  enabled = false;
  await page.reload();
  await expect(
    page.getByRole("button", { name: "Sign in with GitHub", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Sign in with Telegram", exact: true }),
  ).toHaveCount(0);
  await expect(page.getByLabel("Password", { exact: true })).toBeVisible();
});

test("OIDC login keeps the local second factor and password fallback", async ({
  page,
}) => {
  let completed = false;
  const attempts: unknown[] = [];
  await page.addInitScript(() => {
    localStorage.setItem("veloray-locale", "en");
    localStorage.setItem("veloray-login-locale", "en");
  });
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/auth/me") {
      await route.fulfill(
        completed
          ? {
              json: {
                id: 1,
                username: "admin",
                is_superuser: true,
                permissions: [],
              },
            }
          : { status: 401, json: { detail: "Authentication required" } },
      );
    } else if (/^\/api\/(nodes|clients|traffic-history)\/?$/.test(path))
      await route.fulfill({ json: [] });
    else if (path === "/api/overview")
      await route.fulfill({
        json: {
          nodes_total: 0,
          nodes_online: 0,
          clients_total: 0,
          clients_active: 0,
          clients_expiring_7d: 0,
          inbounds_total: 0,
          protocols: [],
          traffic_used_bytes: 0,
          traffic_raw_bytes: 0,
          traffic_upload_bytes: 0,
          traffic_download_bytes: 0,
        },
      });
    else if (path === "/api/auth/oidc")
      await route.fulfill({ json: { enabled: true } });
    else if (path === "/api/auth/oidc/complete") {
      attempts.push(route.request().postDataJSON());
      if (attempts.length === 1)
        await route.fulfill({
          status: 400,
          json: { detail: "Invalid authentication code" },
        });
      else {
        completed = true;
        await route.fulfill({
          json: { id: 1, username: "admin", is_superuser: true },
        });
      }
    } else if (path === "/api/settings")
      await route.fulfill({
        json: { site_name: "VeloRay", default_locale: "en" },
      });
    else if (path.endsWith("/auth/csrf"))
      await route.fulfill({
        json: {},
        headers: { "Set-Cookie": "csrftoken=test-csrf; Path=/; SameSite=Lax" },
      });
    else await route.fulfill({ json: {} });
  });
  await page.goto("/?oidc=totp");
  await expect(
    page.getByRole("heading", { name: "Verify your identity", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("textbox", { name: "Username", exact: true }),
  ).toHaveCount(0);
  await page
    .getByLabel("Authenticator / recovery code", { exact: true })
    .fill("111111");
  await page
    .getByRole("button", { name: "Verify and continue", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText(
    "Invalid authentication code",
  );
  await page
    .getByLabel("Authenticator / recovery code", { exact: true })
    .fill("222222");
  await page
    .getByRole("button", { name: "Verify and continue", exact: true })
    .click();
  await expect(page.locator("#main-content")).toBeVisible();
  expect(attempts).toEqual([{ totp_code: "111111" }, { totp_code: "222222" }]);
  await expect(page).toHaveURL(/\/dashboard$/);
  completed = false;
  await page.goto("/?oidc=totp");
  await page
    .getByRole("button", { name: "Back to login", exact: true })
    .click();
  await expect(
    page.getByRole("textbox", { name: "Username", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Sign in with SSO", exact: true }),
  ).toBeVisible();
});

test("operations manage monitors, private billing and diagnostics on mobile", async ({
  page,
}) => {
  const entries: any[] = [];
  let billing: any = {
    enabled: true,
    provider: "",
    region: "",
    reference: "",
    currency: "USD",
    price_minor: 0,
    budget_bytes: 0,
    warning_percent: 80,
    reset_day: 1,
    timezone: "UTC",
  };
  await panel(page, "/operations", { monitors: { json: [] } });
  await page.route("**/api/monitors**", async (route) => {
    const request = route.request(),
      path = new URL(request.url()).pathname,
      id = Number(path.split("/")[3]);
    if (path.endsWith("/probe")) {
      entries[0].last_result = {
        ok: true,
        latency_ms: 12,
        status_code: 204,
        checked_at: "2026-10-19T12:00:00Z",
      };
      await route.fulfill({ json: entries[0].last_result });
      return;
    }
    if (path.endsWith("/history")) {
      await route.fulfill({
        json: [
          {
            ok: true,
            latency_ms: 12,
            status_code: 204,
            at: "2026-10-19T12:00:00Z",
          },
        ],
      });
      return;
    }
    if (request.method() === "POST") {
      entries.push({
        ...request.postDataJSON(),
        id: 1,
        target_host: "probe.example",
      });
      await route.fulfill({ status: 201, json: entries[0] });
      return;
    }
    if (request.method() === "DELETE") {
      entries.splice(0);
      await route.fulfill({ status: 204 });
      return;
    }
    await route.fulfill({
      json: id
        ? entries.find((r) => r.id === id)
        : entries.map(({ url, ...rest }) => rest),
    });
  });
  await page.route("**/api/operations/nodes/1/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/diagnostics")) {
      await route.fulfill({
        json: {
          xray: {
            core: "xray",
            status: { running: true },
            operations: [
              {
                operation_id: "test-operation",
                state: "applied",
                at: "2026-10-19T12:00:00Z",
                config_hash: "public-hash",
              },
            ],
          },
        },
      });
      return;
    }
    if (route.request().method() === "PATCH")
      billing = route.request().postDataJSON();
    await route.fulfill({
      json: {
        config: billing,
        usage: {
          used_bytes: 1024 ** 3,
          lifetime_bytes: 1024 ** 3,
          cycle_start: "2026-10-01T00:00:00Z",
          cycle_end: "2026-11-01T00:00:00Z",
        },
        history: [],
      },
    });
  });
  await page.reload();
  await page.getByRole("button", { name: "Add monitor", exact: true }).click();
  let dialog = page.getByRole("dialog", {
    name: "Health monitor",
    exact: true,
  });
  await dialog.getByLabel("Name", { exact: true }).fill("Regional route");
  await dialog
    .getByLabel("HTTPS probe URL", { exact: true })
    .fill("https://probe.example/private-source-path");
  await dialog
    .getByRole("button", { name: "Save monitor", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  expect(entries[0].node).toBe(1);
  await expect(
    page.getByText("https://probe.example/private-source-path", {
      exact: true,
    }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Run probe", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Probe succeeded");
  await page.getByRole("button", { name: "History", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("12 ms");
  await page.keyboard.press("Escape");
  await page.getByRole("tab", { name: "Node billing", exact: true }).click();
  await page
    .getByLabel("Hosting provider", { exact: true })
    .fill("Host company");
  await page
    .getByLabel("Account reference", { exact: true })
    .fill("private-contract");
  await page.getByLabel("Traffic budget (GiB)", { exact: true }).fill("200");
  await page.getByRole("button", { name: "Save billing", exact: true }).click();
  await expect(page.getByRole("status")).toContainText(
    "Billing settings saved",
  );
  expect(billing.budget_bytes).toBe(200 * 1024 ** 3);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("tab", { name: "Diagnostics", exact: true }).click();
  await expect(page.getByText("public-hash", { exact: false })).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  // Failed node reads must not leave another node's private billing editable.
  await page.route("**/api/nodes", (route) =>
    route.fulfill({ json: [node, { ...node, id: 2, name: "Second" }] }),
  );
  await page.route("**/api/operations/nodes/2/**", (route) =>
    route.fulfill({
      status: 502,
      json: { detail: "Node billing unavailable" },
    }),
  );
  await page.reload();
  await page.getByRole("tab", { name: "Node billing", exact: true }).click();
  await expect(
    page.getByLabel("Account reference", { exact: true }),
  ).toHaveValue("private-contract");
  await choose(page, "Node", "2", false);
  await expect(page.getByRole("alert")).toContainText(
    "Node billing unavailable",
  );
  await expect(
    page.getByRole("button", { name: "Save billing", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByLabel("Account reference", { exact: true }),
  ).toHaveCount(0);
});

test("SMTP settings preserve write-only password and email endpoint recipient input", async ({
  page,
}) => {
  let payload: any;
  await panel(page, "/settings", { "webhooks/endpoints": { json: [] } });
  await page.route("**/api/settings", async (route) => {
    if (route.request().method() === "PATCH") {
      payload = route.request().postDataJSON();
      await route.fulfill({
        json: {
          ...payload,
          smtp_password: undefined,
          smtp_configured: true,
          site_name: "VeloRay",
          default_locale: "en",
        },
      });
    } else await route.fallback();
  });
  await page.getByRole("button", { name: "Webhooks", exact: true }).click();
  await page.getByLabel("Enable SMTP email", { exact: true }).check();
  await page
    .getByLabel("SMTP hostname", { exact: true })
    .fill("smtp.example.com");
  await page
    .getByLabel("Sender email", { exact: true })
    .fill("notify@example.com");
  await page.getByLabel("SMTP username", { exact: true }).fill("operator");
  await page
    .getByLabel("SMTP password", { exact: true })
    .fill("private-smtp-secret");
  await page.getByLabel("Quota warning (%)", { exact: true }).fill("85");
  await page
    .getByRole("button", { name: "Save settings", exact: true })
    .click();
  await expect(page.getByLabel("SMTP password", { exact: true })).toHaveValue(
    "",
  );
  expect(payload.smtp_password).toBe("private-smtp-secret");
  expect(payload.alert_quota_percent).toBe(85);
  expect(payload.smtp_configured).toBeUndefined();
  await page
    .getByRole("button", { name: "Add destination", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Notification destination",
    exact: true,
  });
  await dialog.getByRole("combobox", { name: "Channel", exact: true }).click();
  await page.getByRole("option", { name: "Email", exact: true }).click();
  await expect(
    dialog.getByLabel("Recipient emails", { exact: true }),
  ).toHaveAttribute("type", "text");
  await expect(
    dialog.getByLabel("HMAC signing secret", { exact: true }),
  ).toHaveCount(0);
});

test("Persian picker maps leap Esfand and Nowruz to Gregorian API values with keyboard navigation", async ({
  page,
}) => {
  const data = await managementPanel(page, "/clients");
  data.clients.push({
    id: 1,
    account_id: 0,
    inbound: 1,
    name: "Calendar user",
    inbound_name: "Edge 1",
    node_name: "North",
    protocol: "vless",
    credential: "test-key",
    enabled: true,
    online: false,
    usage_metered: true,
    traffic_limit_bytes: 0,
    used_traffic_bytes: 0,
    raw_used_traffic_bytes: 0,
    traffic_multiplier: 1,
    expires_at: "2027-01-01T00:00:00Z",
    duration_days: 30,
    note: "",
  });
  await page.addInitScript(() =>
    localStorage.setItem("veloray-calendar", "persian"),
  );
  await page.reload();
  const dates = await page.evaluate(async () => {
    const c = await import("/src/lib/calendar.ts");
    return {
      leap: c.calendarMonth("2025-03-20", "persian", "en-US"),
      newYear: c.calendarMonth("2025-03-21", "persian", "en-US"),
    };
  });
  expect(dates.leap.dates).toHaveLength(30);
  expect(dates.leap.dates.at(-1)).toBe("2025-03-20");
  expect(dates.leap.next).toBe("2025-03-21");
  expect(dates.newYear.first).toBe("2025-03-21");
  await page
    .getByRole("button", { name: "Actions for Calendar user", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "Edit", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Edit client", exact: true });
  await dialog.getByRole("button", { name: "Expires at", exact: true }).click();
  const calendar = page.getByRole("dialog", {
    name: "Choose date",
    exact: true,
  });
  await expect(calendar).toBeVisible();
  const focused = calendar.locator("[data-date]:focus");
  const before = await focused.getAttribute("data-date");
  await page.keyboard.press("ArrowRight");
  const after = await calendar
    .locator("[data-date]:focus")
    .getAttribute("data-date");
  expect(after).not.toBe(before);
  await page.keyboard.press("Enter");
  await expect(
    dialog.getByRole("button", { name: "Expires at", exact: true }),
  ).not.toHaveText("Select date");
  await page.keyboard.press("Escape");
  await expect(calendar).toHaveCount(0);
  await expect(dialog).toBeVisible();
});

for (const legacyName of [
  "modernX.html",
  "sub-dashboard.html",
  "sub-fish.html",
]) {
  test(`supplied ${legacyName} supports real CSP, copy, QR, dates and themes without CDNs`, async ({
    page,
  }) => {
    const directory =
      process.env.VELORAY_TEST_LEGACY_PORTAL_DIR ||
      (process.env.VELORAY_TEST_SIMPLE_PORTAL_HTML
        ? join(
            dirname(process.env.VELORAY_TEST_SIMPLE_PORTAL_HTML),
            "veloray-legacy-portals",
          )
        : "");
    test.skip(!directory, "Rendered legacy fixtures are required");
    const html = readFileSync(join(directory, legacyName), "utf8");
    const rawHeaders = JSON.parse(
      readFileSync(join(directory, legacyName + ".headers.json"), "utf8"),
    );
    const headers = Object.fromEntries(
      Object.entries(rawHeaders).map(([k, v]) => [
        k,
        (v as string[]).join(", "),
      ]),
    );
    const errors: string[] = [],
      external: string[] = [];
    let qrRequests = 0,
      infoRequests = 0;
    page.on("pageerror", (e) => errors.push(e.message));
    page.on("request", (r) => {
      if (
        !r.url().startsWith("https://panel.example/") &&
        !r.url().startsWith("data:")
      )
        external.push(r.url());
    });
    await page
      .context()
      .grantPermissions(["clipboard-read", "clipboard-write"]);
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.addInitScript(() => {
      (window as any).__cspViolations = [];
      document.addEventListener("securitypolicyviolation", (e) =>
        (window as any).__cspViolations.push(e.violatedDirective),
      );
    });
    await page.route("https://panel.example/**", async (route) => {
      const url = new URL(route.request().url());
      if (url.searchParams.get("format") === "stats") {
        await route.fulfill({
          json: {
            points: [
              {
                at: "2026-10-21T00:00:00Z",
                bytes: 1048576,
                upload_bytes: 262144,
                download_bytes: 786432,
              },
            ],
          },
        });
        return;
      }
      if (url.searchParams.get("format") === "info") {
        infoRequests++;
        await route.fulfill({
          json: {
            downloadByte: 256 * 1024 ** 2,
            uploadByte: 128 * 1024 ** 2,
            totalByte: 5 * 1024 ** 3,
            usedByte: 1024 ** 3,
            expire: Date.now() + 30 * 86400000,
            lastOnline: Date.now() - 60000,
            isOnline: true,
            enabled: true,
          },
        });
        return;
      }
      if (url.searchParams.get("format") === "qr") {
        qrRequests++;
        expect(url.searchParams.get("data")).not.toContain("&amp;");
        expect(url.searchParams.get("data")).toMatch(
          /^(vless:\/\/|https:\/\/panel.example\/sub\/)/,
        );
        await route.fulfill({
          contentType: "image/png",
          body: readFileSync(join(directory, legacyName + ".qr.png")),
        });
        return;
      }
      if (url.pathname.startsWith("/sub/")) {
        await route.fulfill({ headers, body: html });
        return;
      }
      const publicRoot = fileURLToPath(
          new URL("../public/", import.meta.url),
        ).replace(/\/$/, ""),
        file = resolve(publicRoot, "." + url.pathname);
      if (!file.startsWith(publicRoot + "/")) {
        await route.abort();
        return;
      }
      const type = (
        {
          ".js": "text/javascript",
          ".css": "text/css",
          ".woff2": "font/woff2",
        } as Record<string, string>
      )[extname(file)];
      await route.fulfill({
        contentType: type || "application/octet-stream",
        body: readFileSync(file),
      });
    });
    await page.goto(
      "https://panel.example/sub/" + "s".repeat(32) + "?format=portal",
    );
    if (legacyName === "modernX.html") {
      await expect(
        page.locator("#configsContainer [data-vr-click]"),
      ).toHaveCount(2);
      const initial = await page.locator("html").getAttribute("data-theme");
      await page.locator("#themeToggleBtn").click();
      await expect(page.locator("html")).not.toHaveAttribute(
        "data-theme",
        initial!,
      );
      await page.locator('[data-vr-click="copyLink(this)"]').click();
      expect(
        await page.evaluate(() => navigator.clipboard.readText()),
      ).toContain("vless://");
      await page.locator('[data-vr-click^="openQRModal"]').click();
      await expect(page.locator("#qrModal")).toHaveClass(/active/);
      await page.locator("#qrModal .modal-close-btn").click();
      await expect(page.locator("#qrModal")).not.toHaveClass(/active/);
      await expect(page.locator("#daysLeftBadge")).toContainText("روز");
    } else if (legacyName === "sub-dashboard.html") {
      await expect(page.locator("#chartPointsGroup circle")).toHaveCount(1);
      await expect(page.locator("#chartPointsGroup title")).toContainText("B");
      await expect(page.locator("#usedStatValue")).toHaveText(
        /^[۱1](?:[٫.][۰0])?$/,
      );
      await page.locator("#copyMainBtn").click();
      expect(
        await page.evaluate(() => navigator.clipboard.readText()),
      ).toContain("/sub/");
      await page.locator("#qrMainBtn").click();
      await expect(page.locator("#qrModalImg")).toHaveClass(/loaded/);
      await page.locator("#closeModalBtn").click();
      await page.locator('[data-target="tab-configs"]').click();
      await page
        .locator("#configListContainer [data-vr-click^=copyText]")
        .first()
        .click();
      expect(
        await page.evaluate(() => navigator.clipboard.readText()),
      ).toContain("format=xray");
      await page
        .locator("#configListContainer [data-vr-click^=showQR]")
        .first()
        .click();
      await expect(page.locator("#qrModalImg")).toHaveClass(/loaded/);
      await page.locator("#closeModalBtn").click();
      const initial = await page.locator("html").getAttribute("data-theme");
      await page.locator("#themeToggleBtn").click();
      await expect(page.locator("html")).not.toHaveAttribute(
        "data-theme",
        initial!,
      );
    } else {
      await page.locator("#copySub").click();
      expect(
        await page.evaluate(() => navigator.clipboard.readText()),
      ).toContain("/sub/");
      await page.locator("#qrSub").click();
      await expect(page.locator("#qrHolder img")).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(page.locator("#jsonBtn")).toHaveAttribute(
        "href",
        /format=xray/,
      );
      await expect.poll(() => infoRequests).toBeGreaterThan(0);
      await expect(page.locator("#daysText")).toContainText("روز");
      await expect(page.locator("#usedNum")).toContainText("۱");
      const initial = await page.locator("html").getAttribute("data-theme");
      await page.locator("#themeBtn").click();
      await expect(page.locator("html")).not.toHaveAttribute(
        "data-theme",
        initial!,
      );
    }
    await expect.poll(() => qrRequests).toBeGreaterThan(0);
    await page.setViewportSize({ width: 390, height: 844 });
    await expect
      .poll(() =>
        page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      )
      .toBe(true);
    await page.screenshot({
      path: `test-results/legacy-${legacyName}.png`,
      fullPage: true,
    });
    expect(errors).toEqual([]);
    expect(external).toEqual([]);
    expect(await page.evaluate(() => (window as any).__cspViolations)).toEqual(
      [],
    );
    await expect(page.locator("img[src=Route]")).toHaveCount(0);
  });
}

test("LDAP login keeps its provider through second factor and offers local fallback", async ({
  page,
}) => {
  let enabled = true;
  const attempts: any[] = [];
  await page.addInitScript(() => {
    localStorage.setItem("veloray-locale", "en");
    localStorage.setItem("veloray-login-locale", "en");
  });
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/auth/me")
      await route.fulfill({
        status: 401,
        json: { detail: "Authentication required" },
      });
    else if (path === "/api/auth/oidc")
      await route.fulfill({ json: { enabled: false, ldap_enabled: enabled } });
    else if (path === "/api/auth/login") {
      const body = route.request().postDataJSON();
      attempts.push(body);
      if (!body.totp_code && body.provider === "ldap")
        await route.fulfill({
          status: 202,
          json: { two_factor_required: true },
        });
      else
        await route.fulfill({
          status: 401,
          json: { detail: "Test rejected credentials" },
        });
    } else await route.fulfill({ json: {} });
  });
  await page.goto("/");
  const method = page.getByRole("combobox", { name: "Sign-in method" });
  await expect(method).toBeVisible();
  await method.click();
  await page
    .getByRole("option", { name: "LDAP directory", exact: true })
    .click();
  await page.getByLabel("Username", { exact: true }).fill("alice");
  await page.getByLabel("Password", { exact: true }).fill("directory-secret");
  await page.getByRole("button", { name: "Login", exact: true }).click();
  await expect(page.getByLabel("Authenticator / recovery code")).toBeVisible();
  await page.getByLabel("Authenticator / recovery code").fill("RECOVERY-ONCE");
  await page
    .getByRole("button", { name: "Verify and continue", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText(
    "Test rejected credentials",
  );
  expect(attempts.map((x) => x.provider)).toEqual(["ldap", "ldap"]);
  expect(attempts[1]).toMatchObject({
    username: "alice",
    password: "directory-secret",
    totp_code: "RECOVERY-ONCE",
  });
  await page
    .getByRole("button", { name: "Back to login", exact: true })
    .click();
  await method.click();
  await page
    .getByRole("option", { name: "Local account", exact: true })
    .click();
  await expect(page.getByLabel("Password", { exact: true })).toHaveValue("");
  await page.getByLabel("Password", { exact: true }).fill("local-secret");
  await page.getByRole("button", { name: "Login", exact: true }).click();
  await expect.poll(() => attempts.length).toBe(3);
  expect(attempts[2].provider).toBe("local");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
  enabled = false;
  await page.reload();
  await expect(method).toHaveCount(0);
  await expect(page.getByLabel("Password", { exact: true })).toBeVisible();
});

test("administrator edits LDAP binding while preserving local role and SSO fields", async ({
  page,
}) => {
  const user = {
    id: 2,
    username: "alice",
    email: "",
    is_active: true,
    is_staff: true,
    is_superuser: false,
    role: "viewer",
    ldap_dn: "uid=old,dc=example,dc=com",
    oidc_issuer: "https://github.com",
    oidc_subject: "42",
  };
  const saves: any[] = [];
  await panel(page, "/admin", { "admin/users": { json: [user] } });
  await page.route("**/api/admin/users/2/", async (route) => {
    saves.push(route.request().postDataJSON());
    await route.fulfill({ json: { ...user, ...saves.at(-1) } });
  });
  await page
    .getByRole("row")
    .filter({ hasText: "alice" })
    .getByTitle("Edit", { exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Account",
    exact: true,
  });
  const dn = dialog.getByRole("textbox", {
    name: "LDAP distinguished name (optional)",
    exact: true,
  });
  await expect(dn).toHaveValue(user.ldap_dn);
  await dn.fill("uid=alice,ou=people,dc=example,dc=com");
  await dialog
    .getByRole("button", { name: "Save account", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  expect(saves).toHaveLength(1);
  expect(saves[0]).toMatchObject({
    ldap_dn: "uid=alice,ou=people,dc=example,dc=com",
    oidc_issuer: "https://github.com",
    oidc_subject: "42",
    role: "viewer",
  });
  expect(saves[0].password).toBeUndefined();
});

test("ShadowTLS form sends a paired TCP configuration and preserves its keys on edit", async ({
  page,
}) => {
  const inbound = {
    id: 4,
    node: 1,
    name: "Wrapped",
    core: "singbox",
    protocol: "shadowtls",
    transport: "raw",
    security: "none",
    listen: "0.0.0.0",
    port: 443,
    enabled: true,
    protocol_settings: {
      handshake_server: "tls.example",
      handshake_port: 8443,
      psk: "MDEyMzQ1Njc4OWFiY2RlZg==",
    },
  };
  await panel(page, "/inbounds", { inbounds: { json: [inbound] } });
  await page.getByRole("button", { name: "New inbound", exact: true }).click();
  await choose(page, "Core", "singbox");
  await choose(page, "Protocol", "shadowtls");
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("TCP only", { exact: false })).toBeVisible();
  await expect(
    dialog.getByLabel("TLS certificate", { exact: true }),
  ).toHaveCount(0);
  await dialog
    .getByLabel("Handshake server", { exact: true })
    .fill("tls.example");
  await dialog.getByLabel("Handshake port", { exact: true }).fill("8443");
  const created = page.waitForRequest(
    (r) => r.method() === "POST" && r.url().includes("/api/inbounds/"),
  );
  await dialog
    .getByRole("button", { name: "Create & deploy", exact: true })
    .click();
  expect((await created).postDataJSON()).toMatchObject({
    core: "singbox",
    protocol: "shadowtls",
    transport: "raw",
    security: "none",
    protocol_settings: {
      handshake_server: "tls.example",
      handshake_port: 8443,
      psk: "",
    },
  });
  await expect(dialog).not.toBeVisible();
  await page.getByRole("button", { name: "Edit", exact: true }).first().click();
  await expect(
    dialog.getByLabel("Handshake port", { exact: true }),
  ).toHaveValue("8443");
  await expect(
    dialog.getByLabel("Inner server key", { exact: true }),
  ).toHaveValue(inbound.protocol_settings.psk);
  const patched = page.waitForRequest(
    (r) => r.method() === "PATCH" && r.url().includes("/api/inbounds/4/"),
  );
  await dialog
    .getByRole("button", { name: "Save & deploy", exact: true })
    .click();
  expect((await patched).postDataJSON().protocol_settings).toEqual(
    inbound.protocol_settings,
  );
});

test("separate subscription port applies independently and preserves the previous address while pending", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await panel(page, "/settings");
  let endpoint = {
    managed: true,
    panel_origin: "https://panel.example:8443",
    subscription_origin: "https://panel.example:8444",
    port: 8444,
    applied_port: 8444,
    revision: "revision-one",
    status: "ready",
    error: "",
  };
  let complete = false;
  let received: unknown;
  await page.route("**/api/subscription-endpoint", async (route) => {
    if (route.request().method() === "PUT") {
      received = route.request().postDataJSON();
      endpoint = {
        ...endpoint,
        port: 9444,
        revision: "revision-two",
        status: "pending",
      };
      await route.fulfill({ status: 202, json: endpoint });
      return;
    }
    if (complete)
      endpoint = {
        ...endpoint,
        applied_port: 9444,
        subscription_origin: "https://panel.example:9444",
        status: "ready",
      };
    await route.fulfill({ json: endpoint });
  });
  await page.getByRole("button", { name: "Subscription", exact: true }).click();
  const section = page.getByRole("region", {
    name: "Separate subscription port",
  });
  await expect(
    section.getByText("https://panel.example:8444", { exact: true }),
  ).toBeVisible();
  await section.getByLabel("Subscription port", { exact: true }).fill("9444");
  await section
    .getByRole("button", { name: "Apply port", exact: true })
    .click();
  await expect(section.getByRole("status")).toHaveText("Applying");
  expect(received).toEqual({ port: 9444, revision: "revision-one" });
  await expect(
    section.getByText("https://panel.example:8444", { exact: true }),
  ).toBeVisible();
  await expect(
    section.getByLabel("Subscription port", { exact: true }),
  ).toBeDisabled();
  complete = true;
  await expect(
    section.getByText("https://panel.example:9444", { exact: true }),
  ).toBeVisible({ timeout: 10000 });
  await expect(section.getByRole("status")).toHaveText("Active");
  await expect(
    section.getByRole("link", { name: "Check subscription service" }),
  ).toHaveAttribute("href", "https://panel.example:9444/api/health");
  await page.evaluate(() => scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/settings-subscription-dark-desktop.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Theme: dark", exact: true }).click();
  await page.evaluate(() => scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/settings-subscription-light-desktop.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Change language" }).click();
  await expect(
    page.getByRole("heading", { name: "پورت جداگانهٔ اشتراک" }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.evaluate(() => scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/settings-subscription-fa-mobile.png",
    fullPage: true,
  });
  expect(errors).toEqual([]);
});

test("subscription port failure retains the applied URL and allows retry", async ({
  page,
}) => {
  await panel(page, "/settings", {
    "subscription-endpoint": {
      json: {
        managed: true,
        panel_origin: "https://panel.example:8443",
        subscription_origin: "https://panel.example:8444",
        port: 9444,
        applied_port: 8444,
        revision: "revision-failed",
        status: "error",
        error:
          "The subscription listener could not be applied. The previous endpoint remains active. Check that the port is free and the subscription service is running.",
      },
    },
  });
  await page.getByRole("button", { name: "Subscription", exact: true }).click();
  const section = page.getByRole("region", {
    name: "Separate subscription port",
  });
  await expect(section.getByRole("alert")).toContainText(
    "previous endpoint remains active",
  );
  await expect(
    section.getByText("https://panel.example:8444", { exact: true }),
  ).toBeVisible();
  await expect(
    section.getByRole("button", { name: "Apply port", exact: true }),
  ).toBeEnabled();
  // An invalid independent port draft must not block saving the HTML template.
  await section.getByLabel("Subscription port", { exact: true }).fill("0");
  const saved = page.waitForRequest(
    (request) =>
      request.url().endsWith("/api/settings") && request.method() === "PATCH",
  );
  await page
    .getByRole("button", { name: "Save settings", exact: true })
    .click();
  expect((await saved).postDataJSON()).not.toHaveProperty("port");
});

test("dashboard history survives unrelated API failures and retries history independently", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.emulateMedia({ reducedMotion: "reduce" });
  await panel(page, "/dashboard", {
    "system-info": { json: { detail: "Server info unavailable" }, status: 503 },
  });
  const canvas = page.locator('[aria-label="Traffic chart"] canvas');
  await expect(canvas).toBeVisible();
  // Check rendered series pixels, not merely a blank canvas element.
  await expect
    .poll(() =>
      canvas.evaluate((el: HTMLCanvasElement) => {
        const ctx = el.getContext("2d")!;
        const pixels = ctx.getImageData(0, 0, el.width, el.height).data;
        let colored = 0;
        for (let i = 0; i < pixels.length; i += 4)
          if (
            pixels[i + 3] > 100 &&
            (pixels[i + 1] > pixels[i] + 35 || pixels[i + 2] > pixels[i] + 35)
          )
            colored++;
        return colored;
      }),
    )
    .toBeGreaterThan(100);
  await expect(page.getByText("No traffic recorded yet.")).toHaveCount(0);
  await page.route("**/api/traffic-history?*", (route) =>
    route.fulfill({
      status: 503,
      json: { detail: "History temporarily unavailable" },
    }),
  );
  await page
    .getByRole("button", { name: "Refresh", exact: true })
    .first()
    .click();
  await expect(page.getByText("History temporarily unavailable")).toBeVisible();
  await expect(canvas).toBeVisible();
  await page.route("**/api/traffic-history?*", (route) =>
    route.fulfill({
      json: [
        {
          at: new Date().toISOString(),
          used_traffic_bytes: 1048576,
          bytes: 1048576,
          upload_bytes: 0,
          download_bytes: 0,
          interval_seconds: 300,
        },
      ],
    }),
  );
  await page
    .getByRole("button", { name: "Refresh", exact: true })
    .last()
    .click();
  await expect(page.getByText("History temporarily unavailable")).toHaveCount(
    0,
  );
  await expect(page.locator('[aria-label="Traffic chart"]')).toBeVisible();
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/dashboard-recovered-history.png",
    fullPage: true,
  });
  expect(errors).toEqual([]);
});

for (const variant of ["simple", "modern"]) {
  test(`${variant} subscription chart handles one bucket, zero, empty, retry and app formats under CSP`, async ({
    page,
  }) => {
    const file =
      variant === "simple"
        ? process.env.VELORAY_TEST_SIMPLE_PORTAL_HTML
        : process.env.VELORAY_TEST_MODERN_PORTAL_HTML;
    test.skip(!file, "render fixture required");
    const html = readFileSync(file!, "utf8");
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    const csp =
      "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'";
    await page.route("**/subscription-regression", (route) =>
      route.fulfill({
        contentType: "text/html",
        headers: { "Content-Security-Policy": csp },
        body: html,
      }),
    );
    let status = 200,
      points = [
        {
          at: "2026-10-21T00:00:00Z",
          bytes: 1048576,
          upload_bytes: 262144,
          download_bytes: 786432,
        },
      ];
    const addresses: string[] = [];
    await page.route("**/sub/**", (route) => {
      addresses.push(route.request().url());
      return route.fulfill({ status, json: { points } });
    });
    await page.goto("/subscription-regression");
    await expect(page.locator("#traffic-chart")).toBeVisible();
    await expect(page.locator("#chart-points circle")).toHaveCount(1);
    await expect(page.locator("#chart-total")).toContainText("MB");
    expect(new URL(addresses[0]).origin).toBe("http://127.0.0.1:8861");
    await page.locator("#client-format").selectOption("clash");
    await expect(page.locator("#subscription")).toHaveValue(/format=clash$/);
    if (variant === "simple")
      await expect(page.locator("#subscription-qr")).toHaveAttribute(
        "src",
        /format=qr.*data=/,
      );
    else
      await expect(
        page.locator(".subscription-card .qr-trigger"),
      ).toHaveAttribute("data-qr", /format=qr.*data=/);
    points = [
      {
        at: "2026-10-21T00:00:00Z",
        bytes: 0,
        upload_bytes: 0,
        download_bytes: 0,
      },
    ];
    await page.locator('[data-range="24h"]').click();
    await expect(page.locator("#chart-total")).toHaveText(/[0۰] B/);
    await expect(page.locator(".traffic-grid")).not.toContainText("undefined");
    await expect(page.locator("#chart-points circle")).toHaveCount(1);
    points = [];
    await page.locator('[data-range="1h"]').click();
    await expect(page.locator("#chart-empty")).toBeVisible();
    await expect(page.locator("#traffic-chart")).toBeHidden();
    status = 503;
    await page.locator('[data-range="12h"]').click();
    await expect(page.locator("#chart-retry")).toBeVisible();
    status = 200;
    points = [
      {
        at: "2026-10-21T01:00:00Z",
        bytes: 2097152,
        upload_bytes: 1048576,
        download_bytes: 1048576,
      },
    ];
    await page.locator("#chart-retry").click();
    await expect(page.locator("#traffic-chart")).toBeVisible();
    await expect(page.locator("#chart-retry")).toBeHidden();
    await page.locator(".traffic-table summary").click();
    await expect(page.locator(".traffic-table tbody tr")).toHaveCount(1);
    for (const width of [390, 320]) {
      await page.setViewportSize({ width, height: 844 });
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
    }
    expect(errors).toEqual([]);
  });
}

for (const protocol of ["mtproto", "amneziawg"]) {
  test(`native ${protocol} form sends protocol settings without TLS certificate fields`, async ({
    page,
  }) => {
    await panel(page, "/inbounds", {
      "nodes/1/wireguard-keypair": {
        json: {
          private_key: "private-test-key",
          public_key: "public-test-key",
        },
      },
    });
    await page
      .getByRole("button", { name: "New inbound", exact: true })
      .click();
    await choose(page, "Core", "native");
    if (protocol === "amneziawg") await choose(page, "Protocol", protocol);
    const dialog = page.getByRole("dialog");
    await expect(
      dialog.getByLabel("TLS certificate", { exact: true }),
    ).toHaveCount(0);
    if (protocol === "mtproto")
      await dialog.getByLabel("FakeTLS hostname").fill("www.example.com");
    else {
      await dialog
        .getByRole("button", { name: "Generate", exact: true })
        .click();
      await expect(
        dialog.getByLabel("Private key", { exact: true }),
      ).toHaveValue("private-test-key");
      await dialog.getByLabel("H1", { exact: true }).fill("912345678");
      await expect(dialog.getByLabel("MTU", { exact: true })).toHaveValue(
        "1280",
      );
    }
    const request = page.waitForRequest(
      (r) => r.method() === "POST" && r.url().includes("/api/inbounds/"),
    );
    await dialog
      .getByRole("button", { name: "Create & deploy", exact: true })
      .click();
    const body = (await request).postDataJSON();
    expect(body).toMatchObject({
      core: "native",
      protocol,
      transport: "raw",
      security: "none",
      flow: "",
    });
    if (protocol === "mtproto")
      expect(body.protocol_settings.fake_tls_host).toBe("www.example.com");
    else
      expect(body.protocol_settings).toMatchObject({
        secret_key: "private-test-key",
        public_key: "public-test-key",
        h1: "912345678",
        jc: 4,
        s4: 16,
        mtu: 1280,
      });
    await expect(dialog).not.toBeVisible();
  });
}

test("client connection check runs the chosen delivery host and displays measured latency", async ({
  page,
}) => {
  await panel(page, "/clients", {
    clients: {
      json: [
        {
          id: 1,
          name: "Alice",
          inbound: 1,
          inbound_name: "Main",
          node_name: "Local",
          enabled: true,
          traffic_limit_bytes: 0,
          used_traffic_bytes: 0,
          raw_used_traffic_bytes: 0,
          traffic_multiplier: 1,
          subscription_url: "/sub/test",
          credential: "test",
          protocol_settings: {},
        },
      ],
    },
    "clients/1/delivery-check": {
      json: {
        hwid_required: false,
        hwid_limit: 0,
        template_available: true,
        checks: [
          {
            inbound: 1,
            host_index: 2,
            name: "CDN",
            protocol: "vless",
            eligible: true,
            reasons: [],
            probe_available: true,
            formats: { raw: { available: true } },
          },
        ],
      },
    },
    "clients/1/subscription-history": { json: [] },
    "clients/1/connection-test": {
      json: {
        ok: true,
        network_tested: true,
        vantage: "node",
        latency_ms: 81,
        status_code: 204,
      },
    },
  });
  await page
    .getByRole("button", { name: "Actions for Alice", exact: true })
    .click();
  await page
    .getByRole("menuitem", { name: "Check subscription", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("CDN", { exact: false })).toBeVisible();
  const request = page.waitForRequest((r) =>
    r.url().includes("/clients/1/connection-test/"),
  );
  await dialog
    .getByRole("button", { name: "Test connection", exact: true })
    .click();
  expect((await request).postDataJSON()).toMatchObject({
    inbound: 1,
    host_index: 2,
  });
  await expect(dialog.getByRole("status")).toContainText(
    "Connection successful · 81 ms 204",
  );
});

async function preview(page: Page, name: string) {
  if (!process.env.VELORAY_SCREENSHOTS) return;
  mkdirSync(process.env.VELORAY_SCREENSHOTS, { recursive: true });
  await page.screenshot({
    path: join(process.env.VELORAY_SCREENSHOTS, `${name}.png`),
    animations: "disabled",
  });
}
test("original sidebar keeps its grouping, footer and usable expanded/collapsed navigation", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.setViewportSize({ width: 1440, height: 768 });
  await panel(page, "/settings", { project: { json: { stars: 1 } } });
  await expect(page.locator(".project-stars")).toHaveCount(0);
  await expect(
    page
      .locator("aside .sidebar-footer")
      .getByRole("link", { name: "VeloRay GitHub" }),
  ).toBeVisible();
  const navigation = page.locator("aside nav").first();
  for (const group of ["Overview", "Management", "Infrastructure", "System"])
    await expect(navigation.getByText(group, { exact: true })).toBeVisible();
  await navigation
    .getByRole("button", { name: "Settings", exact: true })
    .scrollIntoViewIfNeeded();
  await expect(
    navigation.getByRole("button", { name: "Settings", exact: true }),
  ).toBeInViewport();
  await page
    .getByRole("button", { name: "Collapse navigation", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Expand navigation", exact: true }),
  ).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("button", { name: "Expand navigation", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Expand navigation", exact: true })
    .click();
  await expect(
    page
      .locator("aside .sidebar-footer")
      .getByRole("link", { name: "VeloRay GitHub" }),
  ).toBeVisible();
  await expect(
    page.getByText(
      "Runtime policy, subscriptions, automation, security and host health.",
    ),
  ).toHaveCount(0);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await expect(page.locator(".settings-status")).toBeVisible();
  await navigation.evaluate((el) => {
    el.scrollTop = 0;
  });
  await page.mouse.move(1300, 90);
  await preview(page, "settings-dark");
  await page.getByRole("button", { name: "Theme: dark" }).click();
  await expect(page.locator("html")).toHaveClass(/light/);
  await preview(page, "settings-light");
  await page
    .locator(".settings-nav")
    .getByRole("button", { name: "Subscription", exact: true })
    .click();
  await page.getByLabel("Template", { exact: true }).click();
  await expect(
    page.getByRole("option", { name: "Simple subscription page", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("option", { name: "Custom HTML file", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("option")).toHaveCount(2);
  await page.keyboard.press("Escape");
  await expect(
    page.getByLabel("Subscription path", { exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});
test("core manager enables official downloads and tracks a selected version job", async ({
  page,
}) => {
  await panel(page, "/dashboard");
  let enabled = false,
    installing = false,
    installed = false,
    requested: any;
  await page.route("**/api/nodes/1/core-*/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.includes("core-releases")) {
      await route.fulfill({
        json: {
          releases: [
            {
              tag: "v26.3.27",
              sha256: "a".repeat(64),
              url: "https://github.com/XTLS/Xray-core/releases/tag/v26.3.27",
            },
          ],
        },
      });
      return;
    }
    if (url.pathname.includes("core-policy"))
      enabled = route.request().postDataJSON().enabled;
    if (url.pathname.includes("core-install")) {
      requested = route.request().postDataJSON();
      installing = true;
    }
    const state = {
      managed: true,
      enabled,
      cores: {
        xray: {
          tag: installed ? "v26.3.27" : "bundled",
          source: installed ? "official" : "bundled",
          sha256: "b".repeat(64),
          can_rollback: installed,
          user_ip_attribution: true,
        },
      },
      job: installing
        ? { core: "xray", tag: "v26.3.27", state: "running" }
        : installed
          ? { core: "xray", tag: "v26.3.27", state: "success" }
          : undefined,
    };
    await route.fulfill({ json: state });
    if (installing) {
      installing = false;
      installed = true;
    }
  });
  await page.getByRole("button", { name: "Cores", exact: true }).click();
  const manager = page.locator(".core-manager");
  await manager
    .getByRole("button", { name: "Check releases", exact: true })
    .click();
  await expect(
    manager.getByRole("button", { name: "Install version", exact: true }),
  ).toBeDisabled();
  await manager.getByLabel("Allow official core downloads").click();
  await expect(
    manager.getByLabel("Allow official core downloads"),
  ).toBeChecked();
  page.once("dialog", (dialog) => dialog.accept());
  await manager
    .getByRole("button", { name: "Install version", exact: true })
    .click();
  await expect(
    manager.getByText("Core version changed successfully."),
  ).toBeVisible();
  expect(requested).toEqual({ core: "xray", tag: "v26.3.27", rollback: false });
  await expect(
    manager.getByRole("button", { name: "Rollback", exact: true }),
  ).toBeEnabled();
});
test("login preview and subscription tabs stay accessible on mobile", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.route("**/api/**", (route) => {
    if (route.request().url().includes("auth/me"))
      return route.fulfill({ status: 401, json: { detail: "unauthorized" } });
    return route.fulfill({ json: { enabled: false } });
  });
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Login to your account" }),
  ).toBeVisible();
  await preview(page, "login");
  const file = process.env.VELORAY_TEST_SIMPLE_PORTAL_HTML;
  test.skip(!file, "render fixture required");
  await page.route("**/subscription-preview", (route) =>
    route.fulfill({
      contentType: "text/html",
      body: readFileSync(file!, "utf8"),
    }),
  );
  await page.route("**/sub/**", (route) =>
    route.fulfill({
      json: {
        points: [
          {
            at: "2026-10-21T00:00:00Z",
            bytes: 1048576,
            upload_bytes: 262144,
            download_bytes: 786432,
          },
          {
            at: "2026-10-22T00:00:00Z",
            bytes: 2097152,
            upload_bytes: 524288,
            download_bytes: 1572864,
          },
        ],
      },
    }),
  );
  await page.goto("/subscription-preview");
  await page.setViewportSize({ width: 900, height: 1150 });
  await expect(page.locator("#traffic-chart")).toBeVisible();
  await preview(page, "subscription");
  const tabs = page.getByRole("tab");
  await tabs.nth(1).click();
  await expect(page.locator("#profile-search")).toBeVisible();
  await expect(page.locator(".connection")).toBeHidden();
  await tabs.nth(1).press("ArrowRight");
  await expect(page.locator(".app-cards")).toBeVisible();
  await tabs.nth(2).press("Home");
  await expect(page.locator(".connection")).toBeVisible();
  for (const width of [390, 320]) {
    await page.setViewportSize({ width, height: 844 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  }
});
