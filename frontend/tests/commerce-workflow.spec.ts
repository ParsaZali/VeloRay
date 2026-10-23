import { test, expect, type Page } from "@playwright/test";

async function workspace(page: Page) {
  const state = {
    store: {
      enabled: true,
      name: "Green Store",
      description: "",
      logo_url: "",
      support_url: "",
      url: "/shop?store=1",
      subscription_branding: false,
      subscription_origin: "",
      subscription_verified: false,
      domain_record: undefined as
        | { name: string; value: string; verified: boolean }
        | undefined,
    },
    children: [] as Record<string, unknown>[],
    writes: [] as { resource: string; body: any }[],
  };
  await page.addInitScript(() => {
    localStorage.setItem("veloray-locale", "en");
    localStorage.setItem("veloray-theme", "dark");
  });
  await page.route("**/api/**", async (route) => {
    const req = route.request(),
      path = new URL(req.url()).pathname
        .replace(/^\/api\//, "")
        .replace(/\/$/, ""),
      method = req.method();
    let json: any = {};
    if (method !== "GET" && req.postData()) {
      state.writes.push({ resource: path, body: req.postDataJSON() });
    }
    if (path === "auth/me")
      json = { id: 1, username: "admin", is_superuser: true, permissions: [] };
    else if (path === "settings")
      json = { site_name: "VeloRay", default_locale: "en", accent: "emerald" };
    else if (path === "auth/csrf") json = { csrfToken: "test" };
    else if (path === "services")
      json = [
        { id: 7, name: "Primary", note: "", inbound_ids: [1], defaults: {} },
      ];
    else if (path === "inbounds")
      json = [
        {
          id: 1,
          name: "Edge",
          node: 1,
          node_name: "Local",
          protocol: "vless",
          core: "xray",
          enabled: true,
        },
      ];
    else if (path === "admin/users") json = [{ id: 1, username: "admin" }];
    else if (path === "commerce/settings")
      json = {
        payments_enabled: false,
        merchant_configured: false,
        sandbox: false,
      };
    else if (path === "commerce/reports")
      json = { summary: [], by_offer: [], by_node: [], by_seller: [] };
    else if (path === "commerce/store") {
      if (method === "PATCH") {
        Object.assign(state.store, req.postDataJSON());
        if (state.store.subscription_origin)
          state.store.domain_record = {
            name: "_veloray.sub.shop.example",
            value: "veloray-site-verification=demo-proof",
            verified: false,
          };
      }
      json = state.store;
    } else if (path === "commerce/store/verify-domain") {
      state.store.subscription_verified = true;
      json = state.store;
    } else if (path.startsWith("commerce/")) json = [];
    else if (path === "resellers/policy")
      json = {
        id: 1,
        username: "admin",
        allowed_inbounds: [],
        is_active: true,
        may_delegate: true,
        unrestricted_inbounds: true,
        max_clients: 0,
        traffic_budget_bytes: 0,
        clients_count: 0,
        reserved_clients: 0,
        reserved_traffic_bytes: 0,
      };
    else if (path === "resellers") {
      if (method === "POST") {
        const child = {
          ...req.postDataJSON(),
          id: 2,
          parent_id: 1,
          clients_count: 0,
          allocated_traffic_bytes: 0,
          reserved_clients: 0,
          reserved_traffic_bytes: 0,
        };
        state.children.push(child);
        json = { ...child, generated_password: "Demo-reseller-password-2026" };
      } else json = state.children;
    } else if (path === "resellers/transfer")
      json = { debit: { balance: 800 }, credit: { balance: 200 } };
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
    else if (path === "monitors") json = [];
    else if (path === "operations/nodes/1/diagnostics")
      json = { operation: { config_hash: "public-hash" } };
    else if (path === "cluster")
      return route.fulfill({
        status: 503,
        json: { detail: "Panel status unavailable" },
      });
    else if (path === "health")
      json = { status: "ok", database: "ok", version: "1.0" };
    else if (path === "services/7/impact")
      json = {
        revision: "a".repeat(64),
        customers: 3,
        nodes: [1, 2],
        connections_added: 3,
        connections_removed: 0,
        connections_preserved: 3,
      };
    else if (path === "services/7" && method === "PATCH")
      json = { id: 7, ...req.postDataJSON() };
    await route.fulfill({ json });
  });
  return state;
}
test("subreseller creation shows a generated password once and transfers real-parent credit", async ({
  page,
}) => {
  const state = await workspace(page);
  await page.goto("/services");
  await page
    .getByRole("button", { name: "Sales & billing", exact: true })
    .click();
  await page.getByRole("button", { name: "Team", exact: true }).click();
  await page
    .getByRole("button", { name: "Add subreseller", exact: true })
    .click();
  let dialog = page.getByRole("dialog");
  await dialog.getByLabel("Username", { exact: true }).fill("branch-one");
  await dialog.getByLabel("Client limit", { exact: true }).fill("3");
  await dialog.getByLabel("Traffic budget (GiB)", { exact: true }).fill("5");
  await dialog.getByLabel("Edge · Local", { exact: true }).check();
  await dialog.getByRole("button", { name: "Save", exact: true }).click();
  dialog = page.getByRole("dialog");
  await expect(dialog.getByLabel("Password", { exact: true })).toHaveValue(
    "Demo-reseller-password-2026",
  );
  expect(
    state.writes.find((w) => w.resource === "resellers")?.body,
  ).toMatchObject({
    username: "branch-one",
    max_clients: 3,
    traffic_budget_bytes: 5 * 1073741824,
    allowed_inbounds: [1],
  });
  await page.keyboard.press("Escape");
  await page
    .getByRole("button", { name: "Transfer credit", exact: true })
    .click();
  dialog = page.getByRole("dialog");
  await dialog.getByLabel("Amount", { exact: true }).fill("200");
  await dialog
    .getByRole("button", { name: "Transfer credit", exact: true })
    .click();
  await expect(dialog).not.toBeVisible();
  const transfer = state.writes.find(
    (w) => w.resource === "resellers/transfer",
  )?.body;
  expect(transfer).toMatchObject({ user_id: 2, amount: 200, currency: "IRT" });
  expect(transfer.reference.length).toBeGreaterThan(12);
  expect(transfer).not.toHaveProperty("owner_id");
});
test("store branding saves before DNS and HTTPS verification and remains usable on mobile", async ({
  page,
}) => {
  const state = await workspace(page);
  await page.goto("/services");
  await page
    .getByRole("button", { name: "Sales & billing", exact: true })
    .click();
  await page.getByRole("button", { name: "Storefront", exact: true }).click();
  await page
    .getByLabel(
      "Use this store's name, logo and support on customer subscriptions",
      { exact: true },
    )
    .check();
  await page
    .getByLabel("Custom subscription origin (optional)", { exact: true })
    .fill("https://sub.shop.example:100");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByLabel("TXT record name", { exact: true })).toHaveValue(
    "_veloray.sub.shop.example",
  );
  await page
    .getByRole("button", { name: "Verify domain", exact: true })
    .click();
  await expect(
    page.getByText("Domain verified", { exact: true }),
  ).toBeVisible();
  expect(
    state.writes.find((w) => w.resource === "commerce/store")?.body,
  ).toMatchObject({
    subscription_branding: true,
    subscription_origin: "https://sub.shop.example:100",
  });
  expect(
    state.writes.find((w) => w.resource.endsWith("verify-domain"))?.body.origin,
  ).toBe("https://sub.shop.example:100");
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "test-results/store-domain-mobile.png",
    fullPage: true,
  });
});
test("service review sends a revision only after the impact has been displayed", async ({
  page,
}) => {
  const state = await workspace(page);
  await page.goto("/services");
  await page.getByRole("button", { name: "Edit Primary", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Name", { exact: true }).fill("Primary updated");
  await dialog
    .getByRole("button", { name: "Review changes", exact: true })
    .click();
  await expect(
    dialog.getByRole("region", { name: "Change impact", exact: true }),
  ).toBeVisible();
  expect(state.writes.filter((w) => w.resource === "services/7")).toHaveLength(
    0,
  );
  await dialog
    .getByRole("button", { name: "Save & deploy", exact: true })
    .click();
  await expect(dialog).not.toBeVisible();
  expect(
    state.writes.find((w) => w.resource === "services/7")?.body
      .expected_revision,
  ).toBe("a".repeat(64));
});
test("cluster status failure does not hide deployment diagnostics", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await workspace(page);
  await page.goto("/operations");
  await page.getByRole("tab", { name: "Diagnostics", exact: true }).click();
  await expect(page.getByText("public-hash", { exact: false })).toBeVisible();
  await expect(
    page.getByText("Panel status unavailable", { exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});
