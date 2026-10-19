(() => {
  const root = document.documentElement;
  const select = document.getElementById("appearance");
  let theme = root.dataset.theme || "system";
  try {
    const saved = localStorage.getItem("veloray.sub.theme.v1");
    if (["system", "light", "dark"].includes(saved)) theme = saved;
  } catch { /* Storage may be disabled in an embedded browser. */ }
  root.dataset.theme = theme;
  if (select) {
    select.value = theme;
    select.addEventListener("change", () => {
      root.dataset.theme = select.value;
      try { localStorage.setItem("veloray.sub.theme.v1", select.value); } catch {}
    });
  }
  const time = document.getElementById("expiry");
  if (time?.dateTime) {
    const date = new Date(time.dateTime);
    if (!Number.isNaN(date.getTime())) {
      try { time.textContent = new Intl.DateTimeFormat(root.lang === "fa" ? "fa-IR" : root.lang, {year:"numeric",month:"short",day:"numeric"}).format(date); } catch {}
    }
  }
  document.querySelectorAll("[data-copy]").forEach(button => {
    button.addEventListener("click", async () => {
      const input = document.getElementById(button.dataset.copy);
      if (!(input instanceof HTMLInputElement)) return;
      let success = false;
      try { await navigator.clipboard.writeText(input.value); success = true; } catch {}
      if (!success) {
        input.focus(); input.select();
        try { success = document.execCommand("copy"); } catch {}
      }
      const feedback = document.getElementById("copy-feedback");
      if (feedback) feedback.textContent = success ? button.dataset.success : button.dataset.error;
    });
  });
  const dialog = document.getElementById("qr-dialog");
  document.getElementById("show-qr")?.addEventListener("click", () => dialog?.showModal());
})();
