(() => {
  const root = document.documentElement;
  const choices = [...document.querySelectorAll("[data-theme-choice]")];
  let theme = root.dataset.theme || "system";
  try { const saved = localStorage.getItem("veloray.sub.theme.v1"); if (["system", "light", "dark"].includes(saved)) theme = saved; } catch { /* Storage may be disabled. */ }
  function applyTheme(value) { root.dataset.theme = value; choices.forEach(button => button.setAttribute("aria-pressed", String(button.dataset.themeChoice === value))); }
  applyTheme(theme);
  choices.forEach(button => button.addEventListener("click", () => { applyTheme(button.dataset.themeChoice); try { localStorage.setItem("veloray.sub.theme.v1", button.dataset.themeChoice); } catch {} }));
  const tabs = [...document.querySelectorAll("[data-sub-tab]")];
  function selectTab(tab, focus = false) {
    tabs.forEach(item => { const active = item === tab; item.setAttribute("aria-selected", String(active)); item.tabIndex = active ? 0 : -1; });
    document.querySelectorAll("[data-sub-panel]").forEach(panel => { panel.hidden = panel.dataset.subPanel !== tab.dataset.subTab; });
    if (focus) tab.focus();
  }
  tabs.forEach((tab, index) => {
    tab.addEventListener("click", () => selectTab(tab));
    tab.addEventListener("keydown", event => {
      let next;
      const direction = root.dir === "rtl" ? -1 : 1;
      if (event.key === "ArrowRight") next = (index + direction + tabs.length) % tabs.length;
      if (event.key === "ArrowLeft") next = (index - direction + tabs.length) % tabs.length;
      if (event.key === "Home") next = 0;
      if (event.key === "End") next = tabs.length - 1;
      if (next !== undefined) { event.preventDefault(); selectTab(tabs[next], true); }
    });
  });
  if (tabs[0]) selectTab(tabs[0]);
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
      if (!(input instanceof HTMLInputElement) && !(input instanceof HTMLTextAreaElement)) return;
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
  const search = document.getElementById("profile-search");
  search?.addEventListener("input", () => {
    const query = search.value.trim().toLocaleLowerCase();
    document.querySelectorAll(".profiles li").forEach(row => {
      row.hidden = !(row.dataset.search || "").toLocaleLowerCase().includes(query);
    });
  });
  const dialog = document.getElementById("qr-dialog");
  const qrImage = document.getElementById("subscription-qr");
  const subscriptionQR = qrImage?.src;
  document.getElementById("show-qr")?.addEventListener("click", () => { if(qrImage) qrImage.src = subscriptionQR; dialog?.showModal(); });
  document.querySelectorAll("[data-profile-qr]").forEach(button => button.addEventListener("click", () => { if(qrImage) qrImage.src = button.dataset.profileQr; dialog?.showModal(); }));
  dialog?.addEventListener("click", event => { if (event.target === dialog) dialog.close(); });
})();
