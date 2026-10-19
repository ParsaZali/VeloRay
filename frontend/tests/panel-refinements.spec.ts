import { readFileSync } from "node:fs";
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
    else if (resource === "inbounds" || resource.startsWith("admin/"))
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

test("original dark and light palette, raw upload and download", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await panel(page, "/dashboard");
  const traffic = page.locator(".traffic-directions");
  await expect(traffic.getByText("1.00 GB", { exact: true })).toBeVisible();
  await expect(traffic.getByText("2.00 GB", { exact: true })).toBeVisible();
  await expect(
    page.locator("#main-content").getByText("9.00 GB", { exact: true }),
  ).toHaveCount(0);
  await expect(
    page.locator('[aria-label="Traffic chart"] canvas'),
  ).toBeVisible();
  expect(
    await page.evaluate(() =>
      getComputedStyle(document.documentElement)
        .getPropertyValue("--bg")
        .trim(),
    ),
  ).toBe("#000000");
  await page.getByRole("button", { name: "Theme: dark" }).click();
  await expect(page.locator("html")).toHaveClass(/light/);
  expect(
    await page.evaluate(() =>
      getComputedStyle(document.documentElement)
        .getPropertyValue("--bg")
        .trim(),
    ),
  ).toBe("#f3f6f8");
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
    else if (path === "/api/inbounds")
      json = [{ id: 1, name: "main", supports_clients: true }];
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

test("PasarGuard-inspired portal supports chart, four languages, search, QR and mobile", async ({
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
    /export=csv.*range=7d/,
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
  await expect(page.getByText("نمودار مصرف ترافیک")).toBeVisible();
  await page.locator("#portal-language").selectOption("ru");
  await expect(page.getByText("Использование трафика")).toBeVisible();
  await page.locator("#portal-language").selectOption("zh");
  await expect(page.getByText("流量概览")).toBeVisible();
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
  await page.goto("/simple-preview");
  await expect(page.getByRole("heading", { name: "Arman" })).toBeVisible();
  await expect(page.locator("#traffic-chart")).toHaveCount(0);
  await page.getByLabel("Appearance").selectOption("dark");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.reload();
  await expect(page.getByLabel("Appearance")).toHaveValue("dark");
  await page.getByLabel("Appearance").selectOption("light");
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
  await page.getByLabel("Appearance").selectOption("dark");
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

async function managementPanel(page:Page,path:string){
 const inbound=(id:number,node_name:string)=>({id,node:id,node_name,name:`Edge ${id}`,protocol:"vless",core:"xray",enabled:true,supports_clients:true});
 const data:Record<string,any[]>={customers:[],services:[],profiles:[],snippets:[]};
 await panel(page,path,{inbounds:{json:[inbound(1,"North"),inbound(2,"South")]},...Object.fromEntries(Object.keys(data).map(k=>[k,{json:[]}]))});
 await page.route("**/api/**",async route=>{
  const resource=new URL(route.request().url()).pathname.replace(/^\/api\//,"").replace(/\/$/,""),[kind,id,action]=resource.split("/"),rows=data[kind];
  if(!rows){await route.fallback();return}
  const method=route.request().method(),item=rows.find(i=>i.id===+id);
  if(action==="assign"&&method==="POST"){Object.assign(item,route.request().postDataJSON());await route.fulfill({json:{node_ids:item.node_ids}});return}
  if(action==="revisions"){await route.fulfill({json:[{revision:item.revision,created_at:"2026-10-18T12:00:00Z"}]});return}
  if(method==="POST"){const v={...route.request().postDataJSON(),id:rows.length+1,revision:1,used_traffic_bytes:123,subscription_url:"https://vpn.example/sub/stable-example-token",connections:[{},{}]};rows.push(v);await route.fulfill({json:v,status:201});return}
  if(method==="PATCH"){Object.assign(item,route.request().postDataJSON());item.revision++;await route.fulfill({json:item});return}
  if(method==="DELETE"){rows.splice(rows.indexOf(item),1);await route.fulfill({status:204});return}
  await route.fulfill({json:id?item:rows.map(({body,...metadata})=>kind==="profiles"||kind==="snippets"?metadata:{...metadata,body})});
 });
 await page.reload();await expect(page.getByRole("status",{name:"Loading…"})).toHaveCount(0);
 return data;
}
test("services and customers keep shared policy and managed fields in CRUD forms",async({page})=>{
 const data=await managementPanel(page,"/services");
 await page.getByRole("button",{name:"Add service",exact:true}).click();
 let dialog=page.getByRole("dialog");await dialog.getByLabel("Name",{exact:true}).fill("Standard");await dialog.getByLabel("Edge 1 · North · vless").check();
 await dialog.getByLabel("Traffic quota (GiB)").fill("10");await dialog.getByLabel("HWID limit",{exact:true}).fill("2");
 await dialog.getByRole("combobox",{name:"Calendar renewal"}).click();await page.getByRole("option",{name:"Monthly",exact:true}).click();await dialog.getByLabel("Timezone",{exact:true}).fill("Asia/Tehran");
 await dialog.getByRole("button",{name:"Save",exact:true}).click();await expect(dialog).toHaveCount(0);
 expect(data.services[0].defaults.traffic_limit_bytes).toBe(10*1073741824);expect(data.services[0].defaults.renewal_period).toBe("month");
 await page.getByRole("button",{name:"Customers",exact:true}).click();await page.getByRole("button",{name:"Add customer",exact:true}).click();dialog=page.getByRole("dialog");
 await dialog.getByLabel("Name",{exact:true}).fill("Shared buyer");await dialog.getByLabel("Standard",{exact:true}).check();
 await dialog.getByRole("combobox",{name:"Apply plan defaults"}).click();await page.getByRole("option",{name:"Standard",exact:true}).click();await expect(dialog.getByLabel("Traffic quota (GiB)")).toHaveValue("10");
 await dialog.getByLabel("Edge 2 · South · vless").check();await dialog.getByRole("button",{name:"Save",exact:true}).click();await expect(dialog).toHaveCount(0);
 expect(data.customers[0].service_ids).toEqual([1]);expect(data.customers[0].inbound_ids).toEqual([2]);expect(data.customers[0].hwid_limit).toBe(2);
 await page.getByRole("button",{name:"Edit Shared buyer",exact:true}).click();await page.getByRole("dialog").getByLabel("Note",{exact:true}).fill("Keeps usage and token");
 const request=page.waitForRequest(r=>r.method()==="PATCH"&&r.url().includes("/customers/1"));await page.getByRole("dialog").getByRole("button",{name:"Save",exact:true}).click();const body=(await request).postDataJSON();expect(body.used_traffic_bytes).toBeUndefined();expect(body.subscription_token).toBeUndefined();
 await expect(page.getByRole("dialog")).toHaveCount(0);
});
test("profile editor supports reusable snippets, node assignment and history",async({page})=>{
 const data=await managementPanel(page,"/profiles");
 await page.getByRole("button",{name:"Snippets",exact:true}).click();await page.getByRole("button",{name:"Add snippet",exact:true}).click();let dialog=page.getByRole("dialog");await dialog.getByLabel("Name",{exact:true}).fill("Shared DNS");await dialog.getByLabel("Policy JSON",{exact:true}).fill('{"dns":{"queryStrategy":"UseIPv4"}}');await dialog.getByRole("button",{name:"Save",exact:true}).click();await expect(dialog).toHaveCount(0);
 await page.getByRole("button",{name:"Profiles",exact:true}).click();await page.getByRole("button",{name:"Add profile",exact:true}).click();dialog=page.getByRole("dialog");await dialog.getByLabel("Name",{exact:true}).fill("Regional policy");await dialog.getByLabel("Shared DNS",{exact:true}).check();await dialog.getByLabel("Xray policy JSON",{exact:true}).fill('{"routing":{"domainStrategy":"IPIfNonMatch"}}');await dialog.getByRole("button",{name:"Save",exact:true}).click();await expect(dialog).toHaveCount(0);expect(data.profiles[0].body.snippet_ids).toEqual([1]);
 await page.getByRole("button",{name:"Assign nodes",exact:true}).click();dialog=page.getByRole("dialog");await dialog.getByLabel("Local",{exact:true}).check();await dialog.getByRole("button",{name:"Save",exact:true}).click();await expect(dialog).toHaveCount(0);expect(data.profiles[0].node_ids).toEqual([1]);
 await page.getByRole("button",{name:"History",exact:true}).click();await expect(page.getByRole("dialog").getByText("Revision 1",{exact:false})).toBeVisible();await page.keyboard.press("Escape");await expect(page.getByRole("dialog")).toHaveCount(0);
});
test("management forms remain usable at 390px with a light theme",async({page})=>{
 await page.setViewportSize({width:390,height:844});await managementPanel(page,"/customers");await page.evaluate(()=>document.documentElement.classList.remove("dark"));await page.getByRole("button",{name:"Add customer",exact:true}).click();await expect(page.getByRole("dialog")).toBeVisible();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);await page.keyboard.press("Escape");await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("customer service filtering and bulk edits preserve selection and require explicit fields",async({page})=>{
 const data=await managementPanel(page,"/customers");
 data.services.push({id:1,name:"North plan",inbound_ids:[1],defaults:{}},{id:2,name:"South plan",inbound_ids:[2],defaults:{}});
 data.customers.push(...[{id:1,name:"Alice",service_ids:[1]},{id:2,name:"Bob",service_ids:[2]},{id:3,name:"Cora",service_ids:[1]}].map(c=>({...c,note:"",inbound_ids:[1],enabled:true,used_traffic_bytes:123,traffic_limit_bytes:1073741824,connections:[{}]})));
 await page.reload();await expect(page.getByRole("heading",{name:"Alice",exact:true})).toBeVisible();
 await page.getByRole("combobox",{name:"Filter by service"}).click();await page.getByRole("option",{name:"North plan",exact:true}).click();
 await expect(page.getByRole("heading",{name:"Bob",exact:true})).toHaveCount(0);await page.getByRole("button",{name:"Select visible",exact:true}).click();
 await page.getByRole("combobox",{name:"Filter by service"}).click();await page.getByRole("option",{name:"All services",exact:true}).click();
 await expect(page.getByRole("checkbox",{name:"Select Bob",exact:true})).not.toBeChecked();
 await page.getByRole("button",{name:"Bulk actions (2)",exact:true}).click();const dialog=page.getByRole("dialog");
 await dialog.getByRole("combobox",{name:"Action",exact:true}).click();await page.getByRole("option",{name:"Edit quota and access",exact:true}).click();
 const apply=dialog.getByRole("button",{name:"Apply to selected",exact:true});await expect(apply).toBeDisabled();
 await dialog.getByLabel("Replace traffic quota",{exact:true}).check();await dialog.getByLabel("Traffic quota (GiB)").fill("5");
 let calls=0;await page.route("**/api/customers/bulk",async route=>{calls++;const body=route.request().postDataJSON();expect(body.ids).toEqual([1,3]);expect(body.patch).toEqual({traffic_limit_bytes:5*1073741824});if(calls===1){await route.fulfill({status:409,json:{detail:"Node unavailable; changes rolled back"}});return}for(const row of data.customers)if(body.ids.includes(row.id))Object.assign(row,body.patch);await route.fulfill({json:{count:2,status:"applied"}})});
 await apply.click();await expect(dialog.getByRole("alert")).toContainText("changes rolled back");await expect(dialog).toBeVisible();await apply.click();await expect(dialog).toHaveCount(0);
 expect(data.customers[0].used_traffic_bytes).toBe(123);expect(data.customers[1].traffic_limit_bytes).toBe(1073741824);expect(data.customers[2].traffic_limit_bytes).toBe(5*1073741824);
 await expect(page.getByRole("button",{name:"Bulk actions (0)",exact:true})).toBeDisabled();
});

test("outbound traffic stays scoped to the selected core and handles unavailable telemetry",async({page})=>{
 const calls:string[]=[];
 await panel(page,"/outbounds",{outbounds:{json:[]},"outbound-traffic":{json:{available:true,collected_at:"2026-10-19T10:00:00Z",outbounds:[]}}});
 await page.route("**/api/outbound-traffic?**",async route=>{const url=new URL(route.request().url());calls.push(url.search);const sing=url.searchParams.get("core")==="singbox";await route.fulfill({json:{available:!sing,collected_at:"2026-10-19T10:00:00Z",outbounds:sing?[]:[{tag:"direct",upload_bytes:3*1073741824,download_bytes:5*1073741824,period_upload_bytes:1073741824,period_download_bytes:2*1073741824}]}})});
 await page.getByText("Outbound traffic",{exact:true}).click();const table=page.getByRole("table",{name:"Outbound traffic · xray"});await expect(table).toBeVisible();await expect(table.getByRole("row",{name:"direct 1.000 2.000 3.000 5.000"})).toBeVisible();expect(calls[0]).toContain("core=xray&hours=24");
 await page.getByRole("combobox",{name:"Traffic period",exact:true}).click();await page.getByRole("option",{name:"7 days",exact:true}).click();await expect.poll(()=>calls.at(-1)).toContain("hours=168");
 await page.getByRole("combobox",{name:"Core",exact:true}).click();await page.getByRole("option",{name:"sing-box",exact:true}).click();await expect(table).toHaveCount(0);await page.getByText("Outbound traffic",{exact:true}).click();await expect(page.getByText("Current outbound telemetry is unavailable. Upgrade the node agent and apply its configuration.",{exact:true})).toBeVisible();expect(calls.at(-1)).toContain("core=singbox&hours=24");
});

test("sing-box group editor exposes probe and switching controls",async({page})=>{
 await panel(page,"/outbounds");await page.getByRole("combobox",{name:"Core",exact:true}).click();await page.getByRole("option",{name:"sing-box",exact:true}).click();await page.getByRole("button",{name:"New outbound",exact:true}).click();const form=page.getByRole("dialog");await form.getByLabel("Name",{exact:true}).fill("Auto edge");await form.getByLabel("Tag",{exact:true}).fill("auto-edge");await choose(page,"Protocol","urltest");
 await form.getByLabel("Outbound tags",{exact:true}).fill("direct");await form.getByLabel("Probe URL",{exact:true}).fill("https://probe.example/generate_204");await form.getByLabel("Probe interval",{exact:true}).fill("1m");await form.getByLabel("Latency tolerance (ms)",{exact:true}).fill("120");await form.getByLabel("Probe idle timeout",{exact:true}).fill("15m");const interrupt=form.getByLabel("Interrupt existing connections when switching",{exact:true});await expect(interrupt).not.toBeChecked();await interrupt.check();
 const request=page.waitForRequest(r=>r.method()==="POST"&&r.url().includes("/api/outbounds/"));await form.getByRole("button",{name:"Save & deploy",exact:true}).click();const body=(await request).postDataJSON();expect(body.config).toEqual({outbounds:["direct"],url:"https://probe.example/generate_204",interval:"1m",tolerance:120,idle_timeout:"15m",interrupt_exist_connections:true});
 await expect(form).toHaveCount(0);await page.getByRole("button",{name:"New outbound",exact:true}).click();await choose(page,"Protocol","direct");await expect(page.getByRole("combobox",{name:"Next hop (optional)",exact:true})).toHaveCount(0);await choose(page,"Protocol","selector");await choose(page,"Default candidate","direct");await expect(page.getByRole("combobox",{name:"Default candidate",exact:true})).toContainText("direct");
});
