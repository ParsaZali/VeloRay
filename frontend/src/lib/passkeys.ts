import { api } from "@/lib/api";
import { t } from "@/lib/i18n";

function binary(value: string): ArrayBuffer {
  const raw = atob(value.replace(/-/g, "+").replace(/_/g, "/"));
  return Uint8Array.from(raw, (c) => c.charCodeAt(0)).buffer;
}
function encoded(value: ArrayBuffer) {
  return btoa(String.fromCharCode(...new Uint8Array(value)))
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
}
function credentialJSON(credential: PublicKeyCredential) {
  const r = credential.response;
  const response: Record<string, unknown> = {
    clientDataJSON: encoded(r.clientDataJSON),
  };
  if (r instanceof AuthenticatorAttestationResponse) {
    response.attestationObject = encoded(r.attestationObject);
    response.transports = r.getTransports?.() || [];
  } else {
    const assertion = r as AuthenticatorAssertionResponse;
    response.authenticatorData = encoded(assertion.authenticatorData);
    response.signature = encoded(assertion.signature);
    response.userHandle = assertion.userHandle
      ? encoded(assertion.userHandle)
      : null;
  }
  return {
    id: credential.id,
    rawId: encoded(credential.rawId),
    type: credential.type,
    response,
    clientExtensionResults: credential.getClientExtensionResults(),
    authenticatorAttachment: credential.authenticatorAttachment,
  };
}
export function passkeyError(error: unknown) {
  if (
    error instanceof DOMException &&
    ["NotAllowedError", "AbortError"].includes(error.name)
  )
    return t(
      "Passkey cancelled or unavailable. Try again.",
      "عملیات لغو شد یا کلید در دسترس نیست؛ دوباره امتحان کنید.",
    );
  return error instanceof Error ? error.message : String(error);
}
export function supportsPasskeys() {
  return (
    window.isSecureContext &&
    typeof PublicKeyCredential !== "undefined" &&
    !!navigator.credentials
  );
}
export async function signInWithPasskey() {
  await api("/auth/csrf");
  const options = await api<{
    publicKey: Omit<
      PublicKeyCredentialRequestOptions,
      "challenge" | "allowCredentials"
    > & {
      challenge: string;
      allowCredentials?: {
        id: string;
        type: PublicKeyCredentialType;
        transports?: AuthenticatorTransport[];
      }[];
    };
  }>("/auth/passkey/login/begin", { method: "POST", body: "{}" });
  const publicKey: PublicKeyCredentialRequestOptions = {
    ...options.publicKey,
    challenge: binary(options.publicKey.challenge),
    allowCredentials: options.publicKey.allowCredentials?.map((c) => ({
      ...c,
      id: binary(c.id),
    })),
  };
  const credential = (await navigator.credentials.get({
    publicKey,
  })) as PublicKeyCredential | null;
  if (!credential)
    throw new Error(t("No passkey was selected.", "کلیدی انتخاب نشد."));
  return api("/auth/passkey/login/finish", {
    method: "POST",
    body: JSON.stringify(credentialJSON(credential)),
  });
}
export async function registerPasskey(
  name: string,
  password: string,
  code: string,
) {
  const options = await api<{
    publicKey: Omit<
      PublicKeyCredentialCreationOptions,
      "challenge" | "user" | "excludeCredentials"
    > & {
      challenge: string;
      user: Omit<PublicKeyCredentialUserEntity, "id"> & { id: string };
      excludeCredentials?: {
        id: string;
        type: PublicKeyCredentialType;
        transports?: AuthenticatorTransport[];
      }[];
    };
  }>("/auth/passkeys/register/begin", {
    method: "POST",
    body: JSON.stringify({ name, password, code }),
  });
  const publicKey: PublicKeyCredentialCreationOptions = {
    ...options.publicKey,
    challenge: binary(options.publicKey.challenge),
    user: { ...options.publicKey.user, id: binary(options.publicKey.user.id) },
    excludeCredentials: options.publicKey.excludeCredentials?.map((c) => ({
      ...c,
      id: binary(c.id),
    })),
  };
  const credential = (await navigator.credentials.create({
    publicKey,
  })) as PublicKeyCredential | null;
  if (!credential)
    throw new Error(t("No passkey was created.", "کلیدی ساخته نشد."));
  return api("/auth/passkeys/register/finish", {
    method: "POST",
    body: JSON.stringify(credentialJSON(credential)),
  });
}
