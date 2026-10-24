import { test, expect, type Page } from "@playwright/test";
async function panel(page: Page, path: string) {
  const state = {
    settings: {
      site_name: "VeloRay",
      default_locale: "en",
      accent: "emerald",
      telegram_configured: true,
      telegram_owner_ids: [],
      telegram_enabled: true,
    },
    writes: [] as { path: string; body: any }[],
    botError: false,
    update: {
      version: "1.0",
      state: "available",
      installed_commit: "a".repeat(40),
      latest_commit: "b".repeat(40),
      managed: true,
      error: "",
      job: null as any,
      auto_update_enabled: false,
    },
    requests: [] as string[],
  };
  await page.addInitScript(() => {
    localStorage.setItem("veloray-locale", "en");
    localStorage.setItem("veloray-theme", "dark");
  });
  await page.route("**/api/**", async (route) => {
    const req = route.request(),
      p = new URL(req.url()).pathname
        .replace(/^\/api\//, "")
        .replace(/\/$/, "");
    state.requests.push(p);
    const body = req.postData() ? req.postDataJSON() : {};
    if (req.method() !== "GET") state.writes.push({ path: p, body });
    let json: any = [];
    if (p === "auth/me")
      json = {
        id: 1,
        username: "admin",
        is_superuser: true,
        role: "administrator",
        permissions: [],
      };
    else if (p === "auth/csrf") json = { csrfToken: "test" };
    else if (p === "settings") {
      if (req.method() === "PATCH") Object.assign(state.settings, body);
      json = state.settings;
    } else if (p === "version" || p === "health")
      json = { version: "1.0", status: "ok", database: "ok" };
    else if (p === "updates/status" || p === "updates/check")
      json = state.update;
    else if (p === "updates/install") {
      state.update.job = {
        id: 1,
        kind: "panel_update",
        target: body.commit,
        state: "queued",
        error: "",
      };
      json = state.update.job;
    } else if (p === "panel-endpoint")
      json =
        req.method() === "PUT"
          ? { id: 2, state: "queued", target: String(body.port) }
          : {
              managed: true,
              port: 8443,
              origin: "https://panel.example:8443",
              job: null,
            };
    else if (p === "subscription-endpoint")
      json = {
        managed: true,
        panel_origin: "https://panel.example:8443",
        subscription_origin: "https://panel.example:8444",
        port: 8444,
        applied_port: 8444,
        revision: "v1",
        status: "ready",
        error: "",
      };
    else if (p === "subscription-endpoint/random-port") json = { port: 54321 };
    else if (p === "nodes")
      json = [
        {
          id: 1,
          name: "Local",
          is_local: true,
          public_host: "panel.example",
          agent_url: "http://127.0.0.1:9191",
          enabled: true,
          status: "ok",
          xray_version: "Xray test",
          last_seen_at: "2026-10-24T00:00:00Z",
        },
      ];
    else if (p === "overview")
      json = {
        clients_total: 2,
        clients_active: 2,
        traffic_raw_bytes: 0,
        inbounds_total: 2,
        nodes_total: 1,
        nodes_online: 1,
        protocols: [],
      };
    else if (p === "system-info") json = {};
    else if (p === "telegram/status") {
      if (state.botError)
        return route.fulfill({
          status: 502,
          json: { detail: "Telegram connection failed" },
        });
      json = {
        username: "veloray_test_bot",
        owner_count: 2,
        enabled: true,
        configured: true,
      };
    }
    await route.fulfill({ json });
  });
  await page.goto(path);
  return state;
}
test("sidebar keeps green branding, moves commerce last and opens real update settings", async ({
  page,
}) => {
  await panel(page, "/dashboard");
  const nav = page.locator("aside:visible nav");
  await expect(nav.getByRole("button").last()).toHaveText("Services and plans");
  await expect(
    nav.getByRole("button", { name: "Config profiles", exact: true }),
  ).toHaveCount(0);
  await expect(
    nav.getByRole("button", { name: "Subscription policies", exact: true }),
  ).toHaveCount(0);
  await expect(page.getByText("Top consumers", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Client health", { exact: true })).toHaveCount(0);
  await expect(
    page.locator("aside:visible").getByText("v1.0", { exact: true }),
  ).toBeVisible();
  const update = page
    .locator("aside:visible")
    .getByRole("button", { name: "(Update available)", exact: true });
  await expect(update).toBeVisible();
  const color = await update.evaluate((el) => {
    const canvas = document.createElement("canvas");
    const ctx = canvas.getContext("2d")!;
    ctx.fillStyle = getComputedStyle(el).color;
    ctx.fillRect(0, 0, 1, 1);
    return Array.from(ctx.getImageData(0, 0, 1, 1).data);
  });
  expect(color[0]).toBeGreaterThan(180);
  expect(color[1]).toBeGreaterThan(100);
  expect(color[2]).toBeLessThan(140);
  await update.click();
  await expect(page).toHaveURL(/\/settings\/system$/);
  await expect(
    page.getByRole("heading", { name: "Panel updates", exact: true }),
  ).toBeVisible();
  await expect(
    page.locator("aside:visible").getByRole("link", { name: "VeloRay GitHub" }),
  ).toBeVisible();
  await page.screenshot({
    path: "test-results/panel-update-dark.png",
    fullPage: true,
  });
});
test("profile dialogs have full-width inputs, visible JSON editors and fit mobile", async ({
  page,
}) => {
  await panel(page, "/profiles");
  await expect(page).toHaveURL(/\/settings\/profiles$/);
  await page.getByRole("button", { name: "Add profile", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Profile", exact: true }),
    name = dialog.getByLabel("Name", { exact: true }),
    json = dialog.getByLabel("Xray policy JSON", { exact: true });
  await expect(name).toHaveCSS("border-top-width", "1px");
  expect((await name.boundingBox())!.width).toBeGreaterThan(500);
  expect((await json.boundingBox())!.height).toBeGreaterThanOrEqual(150);
  await name.fill("Regional policy");
  await json.fill('{"dns":{}}');
  await page.screenshot({
    path: "test-results/profile-dialog-dark.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(name).toBeVisible();
  expect(await dialog.evaluate((e) => e.scrollWidth <= e.clientWidth)).toBe(
    true,
  );
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "test-results/profile-dialog-mobile.png",
    fullPage: true,
  });
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Add profile", exact: true }),
  ).toBeFocused();
});
test("health monitor fields are styled without per-page classes", async ({
  page,
}) => {
  await panel(page, "/operations");
  await page.getByRole("button", { name: "Add monitor", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Health monitor" });
  await expect(dialog.getByLabel("Name", { exact: true })).toHaveCSS(
    "border-top-width",
    "1px",
  );
  expect(
    (await dialog
      .getByRole("combobox", { name: "Core", exact: true })
      .boundingBox())!.height,
  ).toBeGreaterThan(40);
  await page.screenshot({
    path: "test-results/monitor-dialog-dark.png",
    fullPage: true,
  });
});
test("Telegram owners retain separators while typing and Check shows success and failure notices", async ({
  page,
}) => {
  const state = await panel(page, "/settings");
  await page
    .getByRole("navigation", { name: "Settings sections" })
    .getByRole("button", { name: "Telegram", exact: true })
    .click();
  const owners = page.getByLabel("Owner IDs", { exact: true });
  await owners.pressSequentially("123, 456");
  await expect(owners).toHaveValue("123, 456");
  await page
    .getByRole("button", { name: "Save settings", exact: true })
    .click();
  await expect
    .poll(
      () =>
        state.writes.find((x) => x.path === "settings")?.body
          .telegram_owner_ids,
    )
    .toEqual(["123", "456"]);
  await page
    .getByRole("button", { name: "Check bot connection", exact: true })
    .click();
  await expect(
    page.getByRole("status").filter({ hasText: "Bot connection checked." }),
  ).toBeVisible();
  state.botError = true;
  await page
    .getByRole("button", { name: "Check bot connection", exact: true })
    .click();
  await expect(
    page.getByRole("alert").filter({ hasText: "Telegram connection failed" }),
  ).toBeVisible();
  await page.screenshot({
    path: "test-results/telegram-notice-dark.png",
    fullPage: true,
  });
});
test("random subscription port suggests a value without applying it", async ({
  page,
}) => {
  const state = await panel(page, "/settings");
  await page
    .getByRole("navigation", { name: "Settings sections" })
    .getByRole("button", { name: "Subscription", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Choose random port", exact: true })
    .click();
  await expect(
    page.getByLabel("Subscription port", { exact: true }),
  ).toHaveValue("54321");
  expect(
    state.writes.filter((x) => x.path === "subscription-endpoint"),
  ).toHaveLength(0);
  await expect(
    page.getByText("https://panel.example:8444", { exact: true }),
  ).toBeVisible();
});
test("panel port and verified update require concrete confirmation and send bounded payloads", async ({
  page,
}) => {
  const state = await panel(page, "/settings");
  await page.getByLabel("HTTPS port", { exact: true }).fill("12345");
  await page.getByRole("button", { name: "Apply port", exact: true }).click();
  expect(state.writes.filter((x) => x.path === "panel-endpoint")).toHaveLength(
    0,
  );
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Apply port", exact: true })
    .click();
  await expect(
    page.getByRole("link", { name: "https://panel.example:12345" }),
  ).toBeVisible();
  expect(state.writes.find((x) => x.path === "panel-endpoint")?.body).toEqual({
    port: 12345,
  });
  await page.goto("/settings/system");
  await page
    .getByRole("button", { name: "Update VeloRay", exact: true })
    .click();
  expect(state.writes.filter((x) => x.path === "updates/install")).toHaveLength(
    0,
  );
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Install update", exact: true })
    .click();
  await expect
    .poll(() => state.writes.find((x) => x.path === "updates/install")?.body)
    .toEqual({ commit: "b".repeat(40) });
  await expect(page.getByText("Update queued", { exact: true })).toBeVisible();
});
test("unknown update state never shows a green up-to-date badge", async ({
  page,
}) => {
  const state = await panel(page, "/dashboard");
  state.update.state = "unknown";
  await page.reload();
  await expect(
    page.locator("aside:visible").getByText("(Up to date)", { exact: true }),
  ).toHaveCount(0);
});
