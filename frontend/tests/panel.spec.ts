async function choose(
  page: import("@playwright/test").Page,
  label: string,
  value: string,
) {
  await page.getByRole("combobox", { name: label, exact: true }).click();
  await page.locator(`.select-menu [data-value="${value}"]`).click();
}
import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";
test("login, navigation, client dialog, mobile menu and logout", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await page.getByLabel("Username", { exact: true }).fill("admin");
  await page
    .getByLabel("Password", { exact: true })
    .fill(process.env.VELORAY_E2E_PASSWORD || "Test-password-very-long");
  await page.getByRole("button", { name: "Login", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Dashboard", exact: true }),
  ).toBeVisible();
  await expect(page.locator("#main-content")).not.toContainText("undefined");
  await expect(
    page
      .locator(".metric-value")
      .filter({ hasText: /[0-9]+(?:\.[0-9]+)? (?:B|KB|MB|GB|TB)/ })
      .first(),
  ).toBeVisible();
  await page.screenshot({
    path: "test-results/dashboard-desktop.png",
    fullPage: true,
  });
  for (const name of [
    "Nodes",
    "Inbounds",
    "Clients",
    "Outbounds",
    "Routing",
    "Xray Core",
    "Security",
    "API",
    "Settings",
    "Accounts",
  ]) {
    await page.getByRole("button", { name, exact: true }).first().click();
    await expect(page.locator("#main-content")).toBeVisible();
    await expect(
      page.getByRole("heading", {
        name: name === "API" ? "API reference" : name,
        exact: true,
      }),
    ).toBeVisible();
    await expect(page.getByText("This page could not load")).toHaveCount(0);
  }
  await page
    .getByRole("button", { name: "Clients", exact: true })
    .first()
    .click();
  await page.getByRole("button", { name: "Add client", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "Add client" })).toBeVisible();
  await page.getByLabel("Client name", { exact: true }).fill("browser-client");
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Open navigation" }).click();
  await page
    .getByRole("button", { name: "Dashboard", exact: true })
    .last()
    .click();
  await expect(
    page.getByRole("heading", { name: "Dashboard", exact: true }),
  ).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    )
    .toBe(true);
  await page.screenshot({
    path: "test-results/dashboard-mobile.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(
    page.getByRole("button", { name: "Login", exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});
test("login is accessible in Persian", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "Change login language" }).click();
  await expect(page.locator("main")).toHaveAttribute("dir", "rtl");
  await expect(page.locator("main")).toHaveAttribute("lang", "fa");
  await page.evaluate(() =>
    document.fonts.load('500 14px "Vazirmatn"', "ورود"),
  );
  expect(
    await page
      .locator("main")
      .evaluate((el) => getComputedStyle(el).fontFamily),
  ).toMatch(/^Vazirmatn/);
  await expect(
    page.getByRole("button", { name: "ورود", exact: true }),
  ).toBeVisible();
  await page.screenshot({ path: "test-results/login-fa.png", fullPage: true });
});

test("bundled fonts, black surfaces and visible compact navigation icons", async ({
  page,
}) => {
  // The panel must render when third-party font/CDN requests are unavailable.
  await page.route("**/*", async (route) => {
    const host = new URL(route.request().url()).hostname;
    if (host === "127.0.0.1" || host === "localhost") await route.continue();
    else await route.abort();
  });
  await page.goto("/");
  const fonts = await page.evaluate(async () => {
    const inter = await document.fonts.load('500 14px "Inter"', "Traffic 123");
    const vazirmatn = await document.fonts.load(
      '500 14px "Vazirmatn"',
      "مصرف ۱۲۳",
    );
    return [...inter, ...vazirmatn].map((font) => ({
      family: font.family,
      status: font.status,
    }));
  });
  expect(fonts).toEqual([
    { family: "Inter", status: "loaded" },
    { family: "Vazirmatn", status: "loaded" },
  ]);
  await expect(page.locator("body")).toHaveCSS(
    "background-color",
    "rgb(8, 11, 16)",
  );
  await page.getByLabel("Username", { exact: true }).fill("admin");
  await page
    .getByLabel("Password", { exact: true })
    .fill(process.env.VELORAY_E2E_PASSWORD || "Test-password-very-long");
  await page.getByRole("button", { name: "Login", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Dashboard", exact: true }),
  ).toBeVisible();
  await expect(page.locator("aside").first()).toHaveCSS(
    "background-color",
    "rgb(10, 14, 20)",
  );
  const dashboard = page
    .getByRole("button", { name: "Dashboard", exact: true })
    .first();
  await expect(dashboard.locator("svg")).toHaveCSS("width", "18px");
  await expect(dashboard.locator("svg")).toHaveAttribute("stroke-width", "2");
  await page.getByRole("button", { name: "Collapse navigation" }).click();
  await expect(dashboard).toBeVisible();
  await expect(dashboard.locator("svg")).toBeVisible();
  const icons = await page
    .locator("aside nav .vr-icon svg")
    .evaluateAll((elements) =>
      elements.map((el) => {
        const box = el.getBoundingClientRect();
        const style = getComputedStyle(el);
        return {
          width: box.width,
          height: box.height,
          color: style.color,
          opacity: style.opacity,
        };
      }),
    );
  expect(icons.length).toBe(11);
  for (const icon of icons) {
    expect(icon.width).toBe(18);
    expect(icon.height).toBe(18);
    expect(icon.color).not.toBe("rgb(0, 0, 0)");
    expect(icon.opacity).toBe("1");
  }
  await page.screenshot({
    path: "test-results/navigation-compact.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Theme: dark" }).click();
  await expect(page.locator("html")).toHaveClass("light");
  await expect(page.locator('meta[name="theme-color"]')).toHaveAttribute(
    "content",
    "#f3f6f8",
  );
  await page.screenshot({
    path: "test-results/dashboard-light.png",
    fullPage: true,
  });
  await page.reload();
  await expect(page.locator("html")).toHaveClass("light");
  await page.getByRole("button", { name: "Theme: light" }).click();
  await expect(page.locator("html")).toHaveClass("dark");
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Open navigation" }).click();
  const mobileNav = page.locator("aside").last();
  await expect(mobileNav.getByText("Dashboard", { exact: true })).toBeVisible();
  await expect(
    mobileNav
      .getByRole("button", { name: "Dashboard", exact: true })
      .locator("svg"),
  ).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
  await page.screenshot({
    path: "test-results/navigation-mobile.png",
    fullPage: true,
  });
});

test("runtime validation and private configuration download", async ({
  page,
}) => {
  await page.goto("/xray");
  await page.getByLabel("Username", { exact: true }).fill("admin");
  await page
    .getByLabel("Password", { exact: true })
    .fill(process.env.VELORAY_E2E_PASSWORD || "Test-password-very-long");
  await page.getByRole("button", { name: "Login", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Xray Core", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Running", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Validate", exact: true }).click();
  await expect(page.getByRole("status")).toContainText(
    "Configuration is valid",
  );
  const downloading = page.waitForEvent("download");
  await page.getByRole("button", { name: "Download", exact: true }).click();
  const download = await downloading;
  expect(download.suggestedFilename()).toMatch(/^veloray-node-\d+\.json$/);
  expect(await download.failure()).toBeNull();
  await page.screenshot({
    path: "test-results/runtime-desktop.png",
    fullPage: true,
  });
});

test("live port checks, batch creation and usage export", async ({ page }) => {
  test.skip(
    !process.env.VELORAY_E2E_OCCUPIED_PORT,
    "needs an isolated occupied-port fixture",
  );
  await page.goto("/inbounds");
  await page.getByLabel("Username", { exact: true }).fill("admin");
  await page
    .getByLabel("Password", { exact: true })
    .fill(process.env.VELORAY_E2E_PASSWORD || "Test-password-very-long");
  await page.getByRole("button", { name: "Login", exact: true }).click();
  await page.getByRole("button", { name: "New inbound", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Inbounds", exact: true }),
  ).toBeVisible();
  await choose(page, "Connection preset", "reality");
  await page
    .getByLabel("Port", { exact: true })
    .fill(process.env.VELORAY_E2E_OCCUPIED_PORT!);
  await page.getByRole("button", { name: "Check port", exact: true }).click();
  await expect(
    page
      .getByRole("alert")
      .filter({
        hasText: `TCP port ${process.env.VELORAY_E2E_OCCUPIED_PORT} is in use`,
      }),
  ).toContainText(`pid=${process.env.VELORAY_E2E_OCCUPIED_PID}`);
  await page.getByLabel("Port", { exact: true }).fill("2053");
  await page.getByRole("button", { name: "Check port", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "Port check passed" }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Clients", exact: true })
    .first()
    .click();
  await page.getByRole("button", { name: "Batch create", exact: true }).click();
  await page.getByLabel("Name prefix", { exact: true }).fill("browser-batch");
  await page.getByLabel("Number of clients").fill("3");
  await page.getByLabel("Traffic multiplier").fill("-0.5");
  await expect(page.locator("#multiplier-help")).toContainText("1G => 0.5G");
  await page
    .getByRole("button", { name: "Create clients", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(
    page.getByRole("row", { name: /^Select browser-batch-001 / }),
  ).toBeVisible();
  await expect(
    page.getByRole("row", { name: /^Select browser-batch-003 / }),
  ).toBeVisible();
  const downloading = page.waitForEvent("download");
  await page.getByRole("button", { name: "Export usage", exact: true }).click();
  const download = await downloading;
  const path = await download.path();
  expect(download.suggestedFilename()).toBe("veloray-client-usage.csv");
  const csv = readFileSync(path!, "utf8");
  expect(csv).toContain("browser-batch-003");
  expect(csv).not.toContain("credential");
  expect(csv).not.toContain("subscription_token");
  expect(csv).toContain("traffic_multiplier");
  const clients = await (await page.request.get("/api/clients/")).json();
  expect(
    clients
      .filter((c: any) => c.name.startsWith("browser-batch"))
      .map((c: any) => c.traffic_multiplier),
  ).toEqual([-0.5, -0.5, -0.5]);
  await page.getByLabel("Select all visible clients", { exact: true }).check();
  await page.getByRole("button", { name: "Disable", exact: true }).click();
  await expect(
    page.getByRole("row").filter({ hasText: "browser-batch-001" }),
  ).toContainText("Disabled");
  await page.getByLabel("Select all visible clients", { exact: true }).check();
  await page.getByRole("button", { name: "Enable", exact: true }).click();
  await expect(
    page.getByRole("row").filter({ hasText: "browser-batch-001" }),
  ).toContainText(/Online|Offline|Unknown/);
  await page
    .getByRole("button", { name: "Xray Core", exact: true })
    .first()
    .click();
  await expect(page.getByText("In use by Xray").first()).toBeVisible();
  const diagnostics = page.waitForEvent("download");
  await page.getByRole("button", { name: "Diagnostics", exact: true }).click();
  const report = await diagnostics;
  const data = readFileSync((await report.path())!, "utf8");
  for (const key of [
    "agent_token",
    "subscription_token",
    "credential",
    "reality_private_key",
  ])
    expect(data).not.toContain(key);
  await page.screenshot({
    path: "test-results/listener-checks-desktop.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() =>
      page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    )
    .toBe(true);
  await page.screenshot({
    path: "test-results/listener-checks-mobile.png",
    fullPage: true,
  });
});

test("negative multiplier creation, editing and mobile explanation", async ({
  page,
}) => {
  const clientName = `browser-discount-${Date.now()}`;
  await page.goto("/clients");
  await page.getByLabel("Username", { exact: true }).fill("admin");
  await page
    .getByLabel("Password", { exact: true })
    .fill(process.env.VELORAY_E2E_PASSWORD || "Test-password-very-long");
  await page.getByRole("button", { name: "Login", exact: true }).click();
  await page.getByRole("button", { name: "Add client", exact: true }).click();
  await page.getByLabel("Client name", { exact: true }).fill(clientName);
  const inbound = await (await page.request.get("/api/inbounds/")).json();
  await choose(
    page,
    "Inbound",
    String(inbound.find((i: any) => i.name === "main").id),
  );
  const multiplier = page.getByLabel("Traffic multiplier");
  await multiplier.fill("-0.5");
  await expect(page.locator("#multiplier-help")).toContainText("1G => 0.5G");
  await page.screenshot({
    path: "test-results/multiplier-desktop.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await multiplier.scrollIntoViewIfNeeded();
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
  await page.screenshot({
    path: "test-results/multiplier-mobile.png",
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Create client", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.setViewportSize({ width: 1440, height: 1000 });
  const row = page.getByRole("row").filter({
    hasText: clientName,
  });
  await row
    .getByRole("button", { name: `Actions for ${clientName}`, exact: true })
    .click();
  await row.getByRole("button", { name: "Edit", exact: true }).click();
  await expect(multiplier).toHaveValue("-0.5");
  await page.getByRole("button", { name: "Expires at", exact: true }).click();
  await page.getByRole("button", { name: "Next month", exact: true }).click();
  await page
    .locator(".calendar-grid")
    .getByRole("button", { name: "15", exact: true })
    .click();
  await choose(page, "Hour", "12");
  await page
    .locator(".calendar-actions")
    .getByRole("button", { name: "Done", exact: true })
    .click();

  await multiplier.fill("4");
  expect(
    await multiplier.evaluate(
      (input: HTMLInputElement) => input.validity.rangeOverflow,
    ),
  ).toBe(false);
  await multiplier.fill("25");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const clients = await (await page.request.get("/api/clients/")).json();
  expect(
    clients.find((c: any) => c.name === clientName).traffic_multiplier,
  ).toBe(25);
});

test("backup preview and disabled import preserve a shared account", async ({
  page,
}) => {
  test.skip(
    !process.env.VELORAY_E2E_IMPORT_DB,
    "needs a privately supplied backup fixture",
  );
  await page.goto("/imports");
  await page.getByLabel("Username", { exact: true }).fill("admin");
  await page
    .getByLabel("Password", { exact: true })
    .fill(process.env.VELORAY_E2E_PASSWORD || "Test-password-very-long");
  await page.getByRole("button", { name: "Login", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Settings", exact: true }),
  ).toBeVisible();
  await page
    .getByLabel("Backup file", { exact: true })
    .setInputFiles(process.env.VELORAY_E2E_IMPORT_DB!);
  const previewing = page.waitForResponse(
    (r) =>
      r.url().includes("/imports/preview") && r.request().method() === "POST",
  );
  await page
    .getByRole("button", { name: "Preview backup", exact: true })
    .click();
  const previewResponse = await previewing;
  expect(previewResponse.ok()).toBe(true);
  const preview = await previewResponse.json();
  expect(preview.accounts).toBe(1);
  expect(preview.client_links).toBe(5);
  const serialized = JSON.stringify(preview);
  expect(serialized).not.toContain('"credential"');
  expect(serialized).not.toContain('"subscription_token"');
  const commit = page.getByRole("button", {
    name: "Import selected data",
    exact: true,
  });
  await expect(commit).toBeDisabled();
  await page.screenshot({
    path: "test-results/import-preview-desktop.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Change language" }).click();
  await expect(page.getByRole("heading", { name: "تنظیمات" })).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
  await page.screenshot({
    path: "test-results/import-preview-mobile-fa.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "تغییر زبان" }).click();
  await page
    .getByRole("checkbox", {
      name: "I reviewed the warnings. Save selected inbounds disabled so I can verify them before activation.",
    })
    .check();
  await commit.click();
  await expect(
    page.getByRole("heading", { name: "Data imported" }),
  ).toBeVisible();
  const inbounds = await (await page.request.get("/api/inbounds/")).json();
  const imported = inbounds.filter((i: any) => i.name !== "main");
  expect(imported).toHaveLength(5);
  expect(imported.every((i: any) => i.enabled === false)).toBe(true);
  const clients = await (await page.request.get("/api/clients/")).json();
  const members = clients.filter((c: any) => c.account_id);
  expect(members).toHaveLength(5);
  expect(new Set(members.map((c: any) => c.account_id)).size).toBe(1);
  expect(new Set(members.map((c: any) => c.subscription_url)).size).toBe(1);
  expect(members.every((c: any) => c.used_traffic_bytes === 0)).toBe(true);
});

test("global language, black layout and chart tooltip stay consistent", async ({
  page,
}) => {
  await page.route("**/api/traffic-history?**", (route) =>
    route.fulfill({
      json: [
        {
          at: new Date(Date.now() - 3600000).toISOString(),
          used_traffic_bytes: 0,
          bytes: 0,
        },
        {
          at: new Date(Date.now() - 1800000).toISOString(),
          used_traffic_bytes: 5242880,
          bytes: 5242880,
        },
        {
          at: new Date().toISOString(),
          used_traffic_bytes: 8388608,
          bytes: 3145728,
        },
      ],
    }),
  );
  await page.goto("/dashboard");
  await page.getByLabel("Username", { exact: true }).fill("admin");
  await page
    .getByLabel("Password", { exact: true })
    .fill(process.env.VELORAY_E2E_PASSWORD || "Test-password-very-long");
  await page.getByRole("button", { name: "Login", exact: true }).click();
  await expect(page.locator("body")).not.toContainText("Network control");
  await expect(
    page
      .locator("nav")
      .getByRole("button", { name: "Subscriptions", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.locator("nav").getByRole("button", { name: "Audit log", exact: true }),
  ).toHaveCount(0);
  const chart = page.locator('[aria-label="Traffic chart"] canvas');
  const chartBox = await chart.boundingBox();
  await chart.hover({
    position: { x: chartBox!.width / 2, y: chartBox!.height / 2 },
  });
  await expect(page.locator(".chart-tooltip")).toBeVisible();
  const shell = await page
      .locator('[aria-label="Traffic chart"]')
      .boundingBox(),
    tip = await page.locator(".chart-tooltip").boundingBox();
  expect(tip!.y).toBeGreaterThanOrEqual(shell!.y);
  expect(tip!.y + tip!.height).toBeLessThanOrEqual(shell!.y + shell!.height);
  await expect(chart).toBeVisible();
  await page.getByRole("button", { name: "Change language" }).click();
  await expect(page.locator("html")).toHaveAttribute("dir", "rtl");
  await expect(
    page.getByRole("heading", { name: "داشبورد", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "کلاینت‌ها", exact: true })
    .first()
    .click();
  await expect(
    page.getByRole("heading", { name: "کلاینت‌ها", exact: true }),
  ).toBeVisible();
  await expect(page.getByPlaceholder("جست‌وجوی کلاینت‌ها…")).toBeVisible();
  await page
    .getByRole("button", { name: "افزودن کلاینت", exact: true })
    .click();
  await expect(page.getByLabel("ضریب مصرف", { exact: true })).toBeVisible();
  await expect(page.getByRole("dialog")).not.toContainText(
    "Traffic multiplier",
  );
  await page.getByRole("button", { name: "لغو", exact: true }).click();
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("lang", "fa");
  await page.getByRole("button", { name: "تغییر زبان" }).click();
  await expect(
    page.getByRole("heading", { name: "Clients", exact: true }),
  ).toBeVisible();
  await expect(
    page.locator("nav").getByRole("button", { name: "Accounts", exact: true }),
  ).toBeVisible();
  await expect(page.locator("#main-content")).not.toContainText("ضریب");
  const navBox = await page.locator("aside").boundingBox(),
    contentBox = await page.locator("#main-content").boundingBox();
  expect(contentBox!.x).toBeGreaterThanOrEqual(navBox!.x + navBox!.width);
  await page.screenshot({
    path: "test-results/clients-live-desktop.png",
    fullPage: true,
  });
});

test("client usage refreshes automatically and filter controls fit", async ({
  page,
}) => {
  await page.goto("/clients");
  await page.getByLabel("Username", { exact: true }).fill("admin");
  await page
    .getByLabel("Password", { exact: true })
    .fill(process.env.VELORAY_E2E_PASSWORD || "Test-password-very-long");
  await page.getByRole("button", { name: "Login", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Clients", exact: true }),
  ).toBeVisible();
  const source = await (await page.request.get("/api/clients/")).json();
  expect(source.length).toBeGreaterThan(0);
  let polls = 0;
  await page.route("**/api/clients/**", async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    polls++;
    await route.fulfill({
      json: source.map((c: any, i: number) =>
        i === 0
          ? {
              ...c,
              enabled: true,
              online: true,
              used_traffic_bytes: polls * 1024 ** 3,
              raw_used_traffic_bytes: polls * 1024 ** 3,
            }
          : c,
      ),
    });
  });
  const row = page
    .getByRole("row")
    .filter({ has: page.getByText(source[0].name, { exact: true }) });
  await expect.poll(() => polls, { timeout: 12000 }).toBeGreaterThanOrEqual(1);
  await expect(row).toContainText("Online");
  await expect.poll(() => polls, { timeout: 12000 }).toBeGreaterThanOrEqual(2);
  await expect(row).toContainText("2.00 GB");
  const search = await page.getByPlaceholder("Search clients...").boundingBox();
  const selectors = await page.locator(".client-filters .custom-select").all();
  for (const select of selectors) {
    const box = await select.boundingBox();
    expect(Math.abs(box!.y - search!.y)).toBeLessThan(2);
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
  await page.screenshot({
    path: "test-results/clients-live-mobile.png",
    fullPage: true,
  });
});

test("Accounts creates a reseller with enforced isolation", async ({
  page,
}) => {
  const resellerName = `browser-reseller-${Date.now()}`;
  await page.goto("/accounts");
  await page.getByLabel("Username", { exact: true }).fill("admin");
  await page
    .getByLabel("Password", { exact: true })
    .fill(process.env.VELORAY_E2E_PASSWORD || "Test-password-very-long");
  await page.getByRole("button", { name: "Login", exact: true }).click();
  await page.getByRole("button", { name: "Add account", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Username", { exact: true }).fill(resellerName);
  await dialog
    .getByLabel("Password", { exact: true })
    .fill("Reseller-test-password-42");
  await choose(page, "Role", "reseller");
  await dialog.getByLabel("Client limit", { exact: true }).fill("1");
  await dialog.getByLabel("Traffic budget (GB)", { exact: true }).fill("10");
  await dialog
    .getByRole("checkbox")
    .filter({ hasNot: page.locator("[disabled]") })
    .last()
    .check();
  await dialog
    .getByRole("button", { name: "Save account", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByRole("row").filter({ hasText: resellerName }),
  ).toContainText("Reseller");
  await page.getByRole("button", { name: "Sign out" }).click();
  await page.getByLabel("Username", { exact: true }).fill(resellerName);
  await page
    .getByLabel("Password", { exact: true })
    .fill("Reseller-test-password-42");
  await page.getByRole("button", { name: "Login", exact: true }).click();
  await expect(
    page.locator("header").getByText(resellerName, { exact: true }),
  ).toBeVisible();
  expect((await page.request.get("/api/admin/users/")).status()).toBe(403);
  expect((await page.request.get("/api/nodes/")).status()).toBe(403);
  expect(await (await page.request.get("/api/clients/")).json()).toEqual([]);
  await expect(
    page.locator("nav").getByRole("button", { name: "Accounts", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Clients", exact: true })
    .first()
    .click();
  await expect(
    page.getByText("No clients found", { exact: true }),
  ).toBeVisible();
});
