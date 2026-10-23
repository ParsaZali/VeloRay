type MiniApp = {
  initData: string;
  ready: () => void;
  expand: () => void;
  openInvoice: (url: string, callback: (status: string) => void) => void;
};
declare global {
  interface Window {
    Telegram?: { WebApp?: MiniApp };
  }
}
let sdk: Promise<MiniApp> | undefined;
export function miniAppData() {
  return (
    window.Telegram?.WebApp?.initData ||
    new URLSearchParams(location.hash.slice(1)).get("tgWebAppData") ||
    ""
  );
}
export function loadMiniApp(): Promise<MiniApp> {
  if (window.Telegram?.WebApp) return Promise.resolve(window.Telegram.WebApp);
  if (sdk) return sdk;
  sdk = new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.src = "https://telegram.org/js/telegram-web-app.js";
    script.async = true;
    const timer = setTimeout(
      () => reject(new Error("Telegram could not be loaded")),
      10000,
    );
    script.onload = () => {
      clearTimeout(timer);
      const app = window.Telegram?.WebApp;
      if (app) {
        app.ready();
        app.expand();
        resolve(app);
      } else reject(new Error("Telegram could not be loaded"));
    };
    script.onerror = () => {
      clearTimeout(timer);
      reject(new Error("Telegram could not be loaded"));
    };
    document.head.append(script);
  });
  return sdk;
}
export async function openStarsInvoice(url: string, onStatus: () => void) {
  const app = await loadMiniApp();
  app.openInvoice(url, () => onStatus());
}
