import { test, expect, type Page } from "@playwright/test";

const GiB = 1073741824;
async function resellerPanel(page: Page, admin = false) {
  const options = [
    { id: 1, name: "North", node_name: "Germany", protocol: "vless", core: "xray", supports_clients: true, supports_batch: true, usage_metered: true },
    { id: 2, name: "South", node_name: "Finland", protocol: "trojan", core: "singbox", supports_clients: true, supports_batch: true, usage_metered: true },
  ];
  const item = (id: number, account: number, inbound: number, name: string) => ({
    id, account_id: account, account_connections: account ? 2 : 1, inbound, name,
    inbound_name: options[inbound - 1].name, node_name: options[inbound - 1].node_name,
    protocol: options[inbound - 1].protocol, owner_id: 2, credential: `owned-secret-${id}`,
    enabled: true, frozen: false, online: false, connection_status: "offline",
    traffic_limit_bytes: 10 * GiB, used_traffic_bytes: GiB, raw_used_traffic_bytes: GiB,
    traffic_multiplier: 1, effective_traffic_multiplier: 1, usage_metered: true,
    expires_at: "2027-01-01T00:00:00Z", duration_days: 30, start_after_first_use: false,
    subscription_url: `https://sub.example:100/sub/${account ? "shared" : id}-private-token`,
    subscription_portal_url: `https://sub.example:100/sub/${account ? "shared" : id}-private-token`,
    note: "", hwid_limit: 0, device_limit: 0, renewal_interval_days: 0,
  });
  const state = {
    session: { id: 2, username: "seller", is_superuser: admin, role: admin ? "administrator" : "reseller",
      // Deliberately stale infrastructure grants must not expose those pages.
      permissions: ["dashboard.read", "clients.read", "clients.write", "clients.delete", "commerce.read", "commerce.write", "resellers.read", "traffic.read", "settings.read", "inbounds.read", "nodes.read", "xray.read", "routing_rules.read", "outbounds.read", "api.read"] },
    clients: [item(3, 0, 1, "Bob"), item(10, 7, 1, "Alice"), item(11, 7, 2, "Alice")],
    requests: [] as string[], writes: [] as { path: string; body: Record<string, any> }[],
    shared: { id: 7, name: "Alice", inbound_ids: [1, 2], service_ids: [], plan_id: 0 },
  };
  await page.addInitScript(() => {
    localStorage.setItem("veloray-locale", "en");
    localStorage.setItem("veloray-theme", "dark");
  });
  await page.route("**/api/**", async route => {
    const request = route.request(), method = request.method();
    const path = new URL(request.url()).pathname.replace(/^\/api\//, "").replace(/\/$/, "");
    state.requests.push(path);
    const body = request.postData() ? request.postDataJSON() : {};
    if (method !== "GET") state.writes.push({ path, body });
    if (/^(inbounds|nodes|system-info|xray|routing_rules|outbounds)(\/|$)/.test(path))
      return route.fulfill({ status: 403, json: { detail: "Infrastructure access is restricted" } });
    let json: any = {};
    if (path === "auth/me") json = state.session;
    else if (path === "auth/csrf") json = { csrfToken: "test" };
    else if (path === "settings") json = { site_name: "VeloRay", default_locale: "en", default_client_days: 30, default_traffic_gb: 10 };
    else if (path === "clients/inbound-options") json = options;
    else if (path === "clients" && method === "GET") json = state.clients;
    else if (path === "overview") json = { clients_total: 2, clients_active: 2, clients_expiring_7d: 0, traffic_raw_bytes: 2 * GiB, traffic_upload_bytes: GiB, traffic_download_bytes: GiB, allocated_traffic_bytes: 20 * GiB, traffic_budget_bytes: 100 * GiB, max_clients: 10, reserved_clients: 2, reserved_traffic_bytes: 10 * GiB, inbounds_total: 2, protocols: [] };
    else if (path === "traffic-history") json = [];
    else if (path === "services") json = [];
    else if (path === "customers/7" && method === "GET") json = state.shared;
    else if (path === "customers/7" && method === "PATCH") {
      Object.assign(state.shared, body);
      for (const client of state.clients) if (client.account_id === 7) Object.assign(client, body);
      json = state.shared;
    } else if (path === "customers" && method === "POST") {
      for (const [index, inbound] of body.inbound_ids.entries()) state.clients.push({ ...item(20 + index, 8, inbound, body.name), ...body });
      json = { id: 8, ...body };
    } else if (path === "clients/bulk") {
      state.clients = state.clients.filter(client => !body.ids.includes(client.id) && !(client.account_id === 7 && body.ids.includes(10)));
      json = { count: body.ids.length, account_count: 2, status: "applied" };
    } else if (path === "customers/7/freeze" || path === "customers/7/resume") {
      for (const client of state.clients) if (client.account_id === 7) { client.frozen = path.endsWith("freeze"); client.enabled = !client.frozen; }
    } else if (path === "customers/7/rotate-token") {
      for (const client of state.clients) if (client.account_id === 7) client.subscription_url = "https://sub.example:100/sub/rotated-private-token";
    } else if (path === "resellers") json = [];
    else if (path === "commerce/store") json = { enabled: false, name: "Seller", support_url: "", logo_url: "", description: "" };
    else if (path === "commerce/reports") json = { summary: [], by_offer: [], by_node: [], by_seller: [] };
    else if (path.startsWith("commerce/")) json = [];
    else if (path === "admin/users") json = [];
    else if (path === "health") json = { status: "ok", version: "1.0" };
    await route.fulfill({ json });
  });
  return state;
}

test("reseller has a focused navigation and cannot mount infrastructure via old URLs", async ({ page }) => {
  const state = await resellerPanel(page);
  for (const path of ["/inbounds", "/nodes", "/routing", "/settings", "/accounts"]) {
    await page.goto(path);
    await expect(page).toHaveURL(/\/dashboard$/);
    await expect(page.getByRole("heading", { name: "Dashboard", exact: true })).toBeVisible();
  }
  const nav = page.locator("aside:visible nav");
  await expect(nav.getByRole("button")).toHaveCount(4);
  for (const name of ["Dashboard", "Clients", "Sales & wallet", "Security"]) await expect(nav.getByRole("button", { name, exact: true })).toBeVisible();
  for (const name of ["Customers", "Inbounds", "Nodes", "Cores", "Routing", "Outbounds", "Settings"]) await expect(nav.getByRole("button", { name, exact: true })).toHaveCount(0);
  await expect(page.getByText("CPU", { exact: true })).toHaveCount(0);
  await expect(page.getByText("No nodes registered", { exact: true })).toHaveCount(0);
  await expect(page.getByText("70.00 GB remaining of 100.00 GB")).toBeVisible();
  expect(state.requests.filter(path => /^(inbounds|nodes|system-info|xray|routing_rules|outbounds)(\/|$)/.test(path))).toEqual([]);
  await page.screenshot({ path: "test-results/reseller-dashboard-dark.png", fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Open navigation" }).click();
  await expect(page.locator("aside:visible nav").getByRole("button")).toHaveCount(4);
  await page.screenshot({ path: "test-results/reseller-sidebar-mobile.png", fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("Customers URL opens unified users and a shared account remains editable once", async ({ page }) => {
  const state = await resellerPanel(page);
  await page.goto("/customers");
  await expect(page).toHaveURL(/\/clients$/);
  const row = page.getByRole("row").filter({ has: page.getByText("Alice", { exact: true }) });
  await expect(row).toHaveCount(1);
  await expect(row).toContainText("North · South");
  await row.getByRole("button", { name: "Actions for Alice" }).click();
  await page.getByRole("menuitem", { name: "Edit", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Edit client", exact: true });
  await expect(dialog.getByLabel("North · Germany · VLESS")).toBeChecked();
  await expect(dialog.getByLabel("South · Finland · TROJAN")).toBeChecked();
  await dialog.getByLabel("Client name", { exact: true }).fill("Alice updated");
  await dialog.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  const write = state.writes.find(write => write.path === "customers/7");
  expect(write?.body).toMatchObject({ name: "Alice updated", inbound_ids: [1, 2], service_ids: [] });
  expect(write?.body).not.toHaveProperty("subscription_token");
  expect(write?.body).not.toHaveProperty("used_traffic_bytes");
  expect(write?.body).not.toHaveProperty("credential");
  await expect(page.getByRole("row").filter({ has: page.getByText("Alice updated", { exact: true }) })).toHaveCount(1);
});

test("multi-connection user creation uses assigned choices without infrastructure requests", async ({ page }) => {
  const state = await resellerPanel(page);
  await page.goto("/clients");
  await page.getByRole("button", { name: "Add client", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add client", exact: true });
  await dialog.getByLabel("Client name", { exact: true }).fill("New buyer");
  await dialog.getByLabel("South · Finland · TROJAN").check();
  await dialog.getByLabel("Traffic limit (GB)").fill("15");
  await dialog.getByRole("button", { name: "Create client", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  expect(state.writes.find(write => write.path === "customers")?.body).toMatchObject({ name: "New buyer", inbound_ids: [1, 2], traffic_limit_bytes: 15 * GiB });
  await expect(page.getByRole("row").filter({ has: page.getByText("New buyer", { exact: true }) })).toHaveCount(1);
  expect(state.requests).not.toContain("inbounds");
});

test("unified users retain freeze, token replacement and mixed bulk deletion", async ({ page }) => {
  const state = await resellerPanel(page);
  await page.goto("/clients");
  await page.getByRole("button", { name: "Freeze subscription Alice", exact: true }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Freeze subscription", exact: true }).click();
  await expect(page.getByRole("button", { name: "Resume subscription Alice", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Resume subscription Alice", exact: true }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Resume subscription", exact: true }).click();
  await expect(page.getByRole("button", { name: "Freeze subscription Alice", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Actions for Alice", exact: true }).click();
  page.once("dialog", dialog => dialog.accept());
  await page.getByRole("menuitem", { name: "Replace subscription link", exact: true }).click();
  await expect.poll(() => state.writes.some(write => write.path === "customers/7/rotate-token")).toBe(true);
  await page.getByRole("checkbox", { name: "Select all visible clients", exact: true }).check();
  page.once("dialog", dialog => dialog.accept());
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await expect.poll(() => state.writes.some(write => write.path === "clients/bulk")).toBe(true);
  expect(state.writes.find(write => write.path === "clients/bulk")?.body).toEqual({ ids: [3, 10], action: "delete", whole_accounts: true });
  await expect(page.getByRole("row").filter({ has: page.getByText("Alice", { exact: true }) })).toHaveCount(0);
});

test("reseller sales opens directly without infrastructure plan editors", async ({ page }) => {
  const state = await resellerPanel(page);
  await page.goto("/services");
  await expect(page.getByRole("button", { name: "Wallet & ledger", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Add service", exact: true })).toHaveCount(0);
  expect(state.requests).not.toContain("inbounds");
  await page.getByRole("button", { name: "Wallet & ledger", exact: true }).click();
  await expect(page.getByText("Infrastructure access is restricted")).toHaveCount(0);
});

test("permission refresh removes an open page when access changes", async ({ page }) => {
  const state = await resellerPanel(page);
  await page.goto("/clients");
  await expect(page.getByRole("button", { name: "Add client", exact: true })).toBeVisible();
  state.session.permissions = [];
  await page.evaluate(() => window.dispatchEvent(new Event("veloray:session-update")));
  await expect(page).toHaveURL(/\/security$/);
  await expect(page.getByRole("heading", { name: "Security", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Add client", exact: true })).toHaveCount(0);
});
