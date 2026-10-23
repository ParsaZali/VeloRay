import { test, expect } from "@playwright/test";

test("native WebAuthn creates a discoverable passkey, signs in and revokes it in the security UI", async ({
  page,
  context,
}) => {
  await page.addInitScript(() => {
    localStorage.setItem("veloray-locale", "en");
    localStorage.setItem("veloray-theme", "dark");
  });
  const cdp = await context.newCDPSession(page);
  await cdp.send("WebAuthn.enable", { enableUI: false });
  const { authenticatorId } = await cdp.send(
    "WebAuthn.addVirtualAuthenticator",
    {
      options: {
        protocol: "ctap2",
        transport: "internal",
        hasResidentKey: true,
        hasUserVerification: true,
        isUserVerified: true,
        automaticPresenceSimulation: true,
      },
    },
  );
  let logged = true,
    registered: any,
    assertion: any,
    deleted: any;
  let keys: {
    id: string;
    name: string;
    created_at: string;
    last_used_at?: string;
  }[] = [];
  const challenge = Buffer.from(
      "challenge-with-at-least-thirty-two-bytes",
    ).toString("base64url"),
    handle = Buffer.from("opaque-user-handle-for-the-native-test").toString(
      "base64url",
    );
  const user = {
    id: 1,
    username: "admin",
    is_superuser: true,
    is_staff: true,
    permissions: [],
    two_factor_setup_required: false,
  };
  await page.route("**/api/**", async (route) => {
    const req = route.request(),
      url = new URL(req.url()),
      path = url.pathname.replace(/^\/api\//, "");
    let json: any = {};
    if (path === "auth/me")
      return logged
        ? route.fulfill({ json: user })
        : route.fulfill({ status: 401, json: { detail: "sign in required" } });
    if (path === "settings")
      json = { site_name: "VeloRay", default_locale: "en", accent: "emerald" };
    else if (path === "auth/2fa/status")
      json = { enabled: false, recovery_codes_remaining: 0 };
    else if (path === "auth/passkeys")
      json = { enabled: true, credentials: keys };
    else if (path === "auth/passkey/status")
      json = { enabled: true, rp_id: "localhost" };
    else if (path === "auth/csrf") json = { csrfToken: "test" };
    else if (path === "auth/oidc") json = { enabled: false };
    else if (path === "auth/passkeys/register/begin") {
      expect(req.postDataJSON()).toMatchObject({
        name: "Laptop",
        password: "local-password",
      });
      json = {
        publicKey: {
          rp: { id: "localhost", name: "VeloRay" },
          user: { id: handle, name: "admin", displayName: "admin" },
          challenge,
          pubKeyCredParams: [{ type: "public-key", alg: -7 }],
          timeout: 30000,
          attestation: "none",
          authenticatorSelection: {
            residentKey: "required",
            requireResidentKey: true,
            userVerification: "required",
          },
        },
      };
    } else if (path === "auth/passkeys/register/finish") {
      registered = req.postDataJSON();
      keys = [
        {
          id: registered.id,
          name: "Laptop",
          created_at: new Date().toISOString(),
        },
      ];
      json = { id: registered.id, name: "Laptop" };
    } else if (path === "auth/passkey/login/begin")
      json = {
        publicKey: {
          challenge,
          rpId: "localhost",
          timeout: 30000,
          userVerification: "required",
        },
      };
    else if (path === "auth/passkey/login/finish") {
      assertion = req.postDataJSON();
      logged = true;
      json = user;
    } else if (path.startsWith("auth/passkeys/") && req.method() === "DELETE") {
      deleted = req.postDataJSON();
      expect(path).toBe(`auth/passkeys/${registered.id}`);
      keys = [];
      return route.fulfill({ status: 204 });
    } else if (
      [
        "nodes",
        "inbounds",
        "clients",
        "events",
        "traffic-history",
        "outbound-traffic",
      ].includes(path)
    )
      json = [];
    return route.fulfill({ json });
  });
  await page.goto("http://localhost:8861/security");
  await expect(page.getByLabel("Passkey name", { exact: true })).toHaveCSS(
    "border-top-style",
    "solid",
  );
  expect(
    (await page.getByLabel("Passkey name", { exact: true }).boundingBox())!
      .height,
  ).toBeGreaterThanOrEqual(40);
  await page.getByLabel("Passkey name", { exact: true }).fill("Laptop");
  await page
    .getByLabel("Local password", { exact: true })
    .fill("local-password");
  await page.getByRole("button", { name: "Add passkey", exact: true }).click();
  await expect(page.getByText("Passkey added", { exact: true })).toBeVisible();
  const registration = JSON.parse(
    Buffer.from(registered.response.clientDataJSON, "base64url").toString(),
  );
  expect(registration).toMatchObject({
    type: "webauthn.create",
    origin: "http://localhost:8861",
    challenge,
  });
  expect(registered.response.attestationObject).toBeTruthy();
  const credentials = await cdp.send("WebAuthn.getCredentials", {
    authenticatorId,
  });
  expect(credentials.credentials).toHaveLength(1);
  expect(credentials.credentials[0].isResidentCredential).toBe(true);
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "test-results/passkeys-mobile.png",
    fullPage: true,
  });
  logged = false;
  await page.goto("http://localhost:8861/");
  await page
    .getByRole("button", { name: "Sign in with passkey", exact: true })
    .click();
  await expect.poll(() => !!assertion).toBe(true);
  expect(assertion.id).toBe(registered.id);
  expect(assertion.response.userHandle).toBe(handle);
  expect(assertion.response.signature).toBeTruthy();
  expect(
    JSON.parse(
      Buffer.from(assertion.response.clientDataJSON, "base64url").toString(),
    ),
  ).toMatchObject({
    type: "webauthn.get",
    challenge,
    origin: "http://localhost:8861",
  });
  await page.goto("http://localhost:8861/security");
  await page
    .getByLabel("Local password", { exact: true })
    .fill("local-password");
  await page
    .getByRole("button", { name: "Remove Laptop", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Remove passkey",
    exact: true,
  });
  await dialog.getByRole("button", { name: "Remove", exact: true }).click();
  await expect(
    page.getByText("No passkeys registered", { exact: true }),
  ).toBeVisible();
  expect(deleted).toMatchObject({ password: "local-password" });
  await cdp.send("WebAuthn.removeVirtualAuthenticator", { authenticatorId });
});
