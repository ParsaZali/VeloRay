import { test, expect, type Page } from "@playwright/test";

async function fixture(
  page: Page,
  { fa = false, unavailable = false, admin = true } = {},
) {
  await page.addInitScript(
    ({ fa }) => {
      localStorage.setItem("veloray-locale", fa ? "fa" : "en");
      localStorage.setItem("veloray-theme", fa ? "light" : "dark");
    },
    { fa },
  );
  let enabled = false,
    running = false,
    updated = false,
    polls = 0,
    reads = 0;
  const submitted: unknown[] = [];
  const asset = (kind: string) => ({
    available: true,
    tag: updated ? "20261023064708" : "installed",
    source: updated ? "v2fly/domain-list-community" : "installed",
    repository:
      kind === "geoip" ? "v2fly/geoip" : "v2fly/domain-list-community",
    sha256: (updated ? "b" : "a").repeat(64),
    can_rollback: updated,
  });
  const geoState = () => ({
    managed: true,
    enabled,
    assets: { geoip: asset("geoip"), geosite: asset("geosite") },
    job: running
      ? { kind: "geosite", tag: "20261023064708", state: "running" }
      : updated
        ? { kind: "geosite", tag: "20261023064708", state: "success" }
        : undefined,
  });
  await page.route("**/api/**", async (route) => {
    const req = route.request();
    const path = new URL(req.url()).pathname
      .replace(/^\/api\//, "")
      .replace(/\/$/, "");
    let json: unknown = {};
    if (path === "auth/me")
      json = {
        id: 1,
        username: "admin",
        is_superuser: admin,
        is_staff: true,
        role: admin ? "admin" : "custom",
        permissions: ["xray.read", "nodes.read"],
      };
    else if (path === "settings")
      json = {
        site_name: "VeloRay",
        accent: "emerald",
        default_locale: fa ? "fa" : "en",
      };
    else if (path === "nodes")
      json = [
        {
          id: 1,
          name: "Local",
          is_local: true,
          enabled: true,
          status: "ok",
          config_patch: {},
        },
      ];
    else if (path === "nodes/1/config-preview")
      json = { node: "Local", config: { inbounds: [] } };
    else if (path === "nodes/1/ports") json = { ports: [] };
    else if (path.endsWith("xray-status") || path.endsWith("singbox-status"))
      json = { installed: true, running: true, version: "Xray 26.3.27" };
    else if (path === "nodes/1/core-policy") {
      enabled = req.postDataJSON().enabled;
    } else if (path === "nodes/1/geo-releases")
      json = {
        repository: "v2fly/domain-list-community",
        releases: [
          {
            tag: "20261023064708",
            sha256: "b".repeat(64),
            url: "https://github.com/v2fly/domain-list-community/releases/tag/20261023064708",
          },
        ],
      };
    else if (path === "nodes/1/geo-install") {
      submitted.push(req.postDataJSON());
      running = true;
      polls = 0;
      json = geoState();
    } else if (path === "nodes/1/geo-manager") {
      reads++;
      if (unavailable)
        return route.fulfill({
          status: 503,
          json: { detail: "Update the node agent to manage geo assets." },
        });
      if (running && ++polls >= 2) {
        running = false;
        updated = true;
      }
      json = geoState();
    }
    if (path === "nodes/1/core-manager" || path === "nodes/1/core-policy")
      json = {
        managed: true,
        enabled,
        cores: {
          xray: {
            tag: "bundled",
            source: "bundled",
            sha256: "c".repeat(64),
            can_rollback: false,
            user_ip_attribution: true,
          },
        },
        geo_job: { state: running ? "running" : "" },
      };
    await route.fulfill({ json });
  });
  await page.goto("/xray");
  await expect(page.locator("#main-content")).toBeVisible();
  return { submitted, reads: () => reads };
}

test("geo updates load on demand, honor core download policy, confirm, poll and roll back", async ({
  page,
}) => {
  const state = await fixture(page);
  const manager = page.getByRole("region", { name: "Geo asset updates" });
  await expect(manager).toBeVisible();
  expect(state.reads()).toBe(0);
  await manager.getByRole("button", { name: "GeoIP / Geosite" }).click();
  const sites = manager.getByRole("article", { name: "Geosite", exact: true });
  await sites.getByRole("button", { name: "Check releases" }).click();
  await expect(
    sites.getByRole("button", { name: "Update", exact: true }),
  ).toBeDisabled();
  await page.getByLabel("Allow official core downloads").click();
  await expect(
    sites.getByRole("button", { name: "Update", exact: true }),
  ).toBeEnabled();
  await sites.getByRole("button", { name: "Update", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Confirm geo update" });
  await expect(dialog).toBeVisible();
  expect(state.submitted).toHaveLength(0);
  await dialog.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(
    manager.getByText("Geo asset updated successfully."),
  ).toBeVisible();
  expect(state.submitted).toEqual([
    { kind: "geosite", tag: "20261023064708", rollback: false },
  ]);
  await sites.getByRole("button", { name: "Rollback", exact: true }).click();
  await dialog.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect.poll(() => state.submitted.length).toBe(2);
  expect(state.submitted[1]).toEqual({
    kind: "geosite",
    tag: "",
    rollback: true,
  });
  await expect(
    manager.getByText("Geo asset updated successfully."),
  ).toBeVisible();
  await expect(page.locator("aside").first()).not.toContainText(/github stars/i);
});

test("geo controls preserve readable light Persian layout on mobile", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.setViewportSize({ width: 390, height: 844 });
  await fixture(page, { fa: true });
  const manager = page.getByRole("region", {
    name: "به‌روزرسانی فایل‌های Geo",
  });
  await manager.getByRole("button", { name: "GeoIP / Geosite" }).click();
  await manager
    .getByRole("article", { name: "Geosite", exact: true })
    .getByRole("button", { name: "بررسی نسخه‌ها" })
    .click();
  await expect(
    manager.getByRole("combobox", { name: "Geosite نسخه" }),
  ).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("dir", "rtl");
  await expect(page.locator("html")).toHaveClass(/light/);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "test-results/geo-mobile-persian.png",
    fullPage: true,
  });
  expect(errors).toEqual([]);
});

test("an older or unavailable agent leaves the rest of the cores page usable", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await fixture(page, { unavailable: true });
  const manager = page.getByRole("region", { name: "Geo asset updates" });
  await manager.getByRole("button", { name: "GeoIP / Geosite" }).click();
  await expect(manager.getByRole("alert")).toContainText(
    "Update the node agent",
  );
  await expect(page.locator(".core-manager")).toBeVisible();
  await expect(page.getByLabel("Allow official core downloads")).toBeEnabled();
  expect(errors).toEqual([]);
});

test("limited accounts do not fetch or see core and geo mutation controls", async ({
  page,
}) => {
  const state = await fixture(page, { admin: false });
  await expect(
    page.getByRole("region", { name: "Geo asset updates" }),
  ).toHaveCount(0);
  await expect(page.getByLabel("Allow official core downloads")).toHaveCount(0);
  expect(state.reads()).toBe(0);
});
