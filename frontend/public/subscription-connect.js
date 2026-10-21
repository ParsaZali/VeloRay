(() => {
  "use strict";
  const input = document.getElementById("subscription"),
    picker = document.getElementById("client-format");
  if (!input || !picker || !input.dataset.baseUrl) return;
  picker.addEventListener("change", () => {
    const address = new URL(input.dataset.baseUrl);
    if (picker.value) address.searchParams.set("format", picker.value);
    input.value = address.href;
    const base = new URL(input.dataset.baseUrl);
    const qr = new URL(base.pathname, location.href);
    qr.searchParams.set("format", "qr");
    qr.searchParams.set("data", address.href);
    const image = document.getElementById("subscription-qr");
    if (image) image.src = qr.href;
    const trigger = document.querySelector(".subscription-card .qr-trigger");
    if (trigger) trigger.dataset.qr = qr.href;
    const feedback = document.getElementById("copy-feedback");
    if (feedback) feedback.textContent = "";
  });
})();
