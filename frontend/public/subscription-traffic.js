(() => {
  "use strict";
  const root = document.querySelector("[data-chart-url]");
  const chart = document.getElementById("traffic-chart");
  if (!root || !chart) return;
  const byId = (id) => document.getElementById(id);
  const labels = {
    en: {
      trafficOverview: "Traffic history",
      totalTraffic: "Total",
      uploadTraffic: "Upload",
      downloadTraffic: "Download",
      loadingTraffic: "Loading usage history…",
      noHistory: "No usage recorded in this period yet.",
      actualUsage: "Recorded server traffic",
      downloadCsv: "Download CSV",
      error:
        "Usage history could not load. Your connection link is still available.",
      retry: "Try again",
      table: "View recorded values",
      date: "Date / time",
    },
    fa: {
      trafficOverview: "نمودار مصرف ترافیک",
      totalTraffic: "کل مصرف",
      uploadTraffic: "آپلود",
      downloadTraffic: "دانلود",
      loadingTraffic: "در حال دریافت مصرف…",
      noHistory: "هنوز مصرفی در این بازه ثبت نشده است.",
      actualUsage: "مصرف ثبت‌شده سرور",
      downloadCsv: "دریافت CSV",
      error: "دریافت تاریخچه ناموفق بود. لینک اتصال همچنان در دسترس است.",
      retry: "تلاش دوباره",
      table: "مشاهده مقادیر ثبت‌شده",
      date: "تاریخ و ساعت",
    },
    ru: {
      trafficOverview: "История трафика",
      totalTraffic: "Всего",
      uploadTraffic: "Отправлено",
      downloadTraffic: "Получено",
      loadingTraffic: "Загрузка истории…",
      noHistory: "Нет данных за этот период.",
      actualUsage: "Данные сервера",
      downloadCsv: "Скачать CSV",
      error: "Не удалось загрузить историю. Ссылка подключения доступна.",
      retry: "Повторить",
      table: "Просмотр данных",
      date: "Дата / время",
    },
    zh: {
      trafficOverview: "流量概览",
      totalTraffic: "总计",
      uploadTraffic: "上传",
      downloadTraffic: "下载",
      loadingTraffic: "正在加载用量…",
      noHistory: "该时间段没有记录。",
      actualUsage: "服务器记录的流量",
      downloadCsv: "下载 CSV",
      error: "无法加载历史记录。订阅链接仍然可用。",
      retry: "重试",
      table: "查看记录值",
      date: "日期 / 时间",
    },
  };
  const locale = () => document.documentElement.lang || "en";
  const dict = () => labels[locale()] || labels.en;
  const bytes = (value) => {
    const units = ["B", "KB", "MB", "GB", "TB", "PB"];
    const exponent =
      value > 0 ? Math.min(5, Math.floor(Math.log(value) / Math.log(1024))) : 0;
    return (
      new Intl.NumberFormat(locale(), { maximumFractionDigits: 2 }).format(
        value / 1024 ** exponent,
      ) +
      " " +
      units[exponent]
    );
  };
  const time = (value) =>
    new Intl.DateTimeFormat(locale(), {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    }).format(new Date(value));
  const ns = "http://www.w3.org/2000/svg";
  function svg(tag, attributes, text) {
    const node = document.createElementNS(ns, tag);
    Object.entries(attributes).forEach(([key, value]) =>
      node.setAttribute(key, String(value)),
    );
    if (text !== undefined) node.textContent = text;
    return node;
  }
  chart.setAttribute("viewBox", "0 0 660 220");
  chart.setAttribute("preserveAspectRatio", "none");
  chart.setAttribute("dir", "ltr");
  const grid = svg("g", { class: "traffic-grid", "aria-hidden": "true" });
  chart.prepend(grid);
  const marks = svg("g", { id: "chart-points" });
  chart.append(marks);
  const empty = byId("chart-empty"),
    loading = byId("chart-loading");
  const retry = document.createElement("button");
  retry.type = "button";
  retry.id = "chart-retry";
  retry.hidden = true;
  empty.after(retry);
  const details = document.createElement("details");
  details.className = "traffic-table";
  details.hidden = true;
  const summary = document.createElement("summary"),
    table = document.createElement("table");
  const caption = document.createElement("caption");
  caption.className = "sr-only";
  table.append(caption);
  details.append(summary, table);
  chart.closest(".traffic-panel").append(details);
  let points = [],
    metric = "bytes",
    range = "7d",
    state = "loading",
    controller;
  const metricName = () =>
    dict()[
      metric === "bytes"
        ? "totalTraffic"
        : metric === "upload_bytes"
          ? "uploadTraffic"
          : "downloadTraffic"
    ];
  function render() {
    const m = dict();
    chart
      .closest(".traffic-panel")
      .querySelectorAll("[data-i18n]")
      .forEach((node) => {
        const value = m[node.dataset.i18n];
        if (value) node.textContent = value;
      });
    loading.hidden = state !== "loading";
    empty.hidden = state !== "empty" && state !== "error";
    empty.textContent = state === "error" ? m.error : m.noHistory;
    retry.hidden = state !== "error";
    retry.textContent = m.retry;
    chart.toggleAttribute("hidden", state !== "ready");
    details.hidden = state !== "ready";
    summary.textContent = m.table;
    byId("chart-total").textContent = "";
    if (state !== "ready") return;
    const values = points.map((p) => p[metric]);
    const max = Math.max(1, ...values);
    const from = points[0].at,
      to = points.at(-1).at;
    const width = Math.max(240, chart.getBoundingClientRect().width || 660);
    const right = width - 12;
    chart.setAttribute("viewBox", `0 0 ${width} 220`);
    const x = (point) =>
      from === to
        ? (64 + right) / 2
        : 64 + ((point.at - from) / (to - from)) * (right - 64);
    const y = (value) => 180 - (value / max) * 150;
    const path = points
      .map(
        (point, i) =>
          `${i ? "L" : "M"}${x(point).toFixed(2)} ${y(point[metric]).toFixed(2)}`,
      )
      .join(" ");
    byId("chart-line").setAttribute("d", path);
    byId("chart-fill").setAttribute(
      "d",
      path + ` L${x(points.at(-1))} 180 L${x(points[0])} 180 Z`,
    );
    grid.replaceChildren();
    marks.replaceChildren();
    for (const value of [0, max / 2, max]) {
      grid.append(
        svg("line", { x1: 64, x2: right, y1: y(value), y2: y(value) }),
        svg(
          "text",
          { x: 54, y: y(value) + 4, "text-anchor": "end" },
          bytes(value),
        ),
      );
    }
    grid.append(
      svg("text", { x: 64, y: 208, "text-anchor": "start" }, time(from)),
    );
    if (from !== to)
      grid.append(
        svg("text", { x: right, y: 208, "text-anchor": "end" }, time(to)),
      );
    // A single bucket has no line segment; always give it a visible marker.
    if (points.length <= 90)
      points.forEach((point) => {
        const circle = svg("circle", {
          cx: x(point),
          cy: y(point[metric]),
          r: points.length === 1 ? 5 : 3,
        });
        circle.append(
          svg("title", {}, `${time(point.at)} · ${bytes(point[metric])}`),
        );
        marks.append(circle);
      });
    const total = values.reduce((sum, value) => sum + value, 0);
    chart.setAttribute("aria-label", `${metricName()}: ${bytes(total)}`);
    byId("chart-total").textContent = bytes(total);
    caption.textContent = m.trafficOverview;
    table.querySelectorAll("thead, tbody").forEach((node) => node.remove());
    const head = document.createElement("thead"),
      heading = document.createElement("tr");
    [m.date, metricName()].forEach((label) => {
      const cell = document.createElement("th");
      cell.scope = "col";
      cell.textContent = label;
      heading.append(cell);
    });
    head.append(heading);
    const body = document.createElement("tbody");
    points.forEach((point) => {
      const row = document.createElement("tr");
      [time(point.at), bytes(point[metric])].forEach((value) => {
        const cell = document.createElement("td");
        cell.textContent = value;
        row.append(cell);
      });
      body.append(row);
    });
    table.append(head, body);
  }
  async function load(nextRange) {
    controller?.abort();
    const request = new AbortController();
    controller = request;
    range = nextRange;
    state = "loading";
    render();
    document.querySelectorAll("[data-range]").forEach((button) => {
      const selected = button.dataset.range === range;
      button.classList.toggle("is-selected", selected);
      button.setAttribute("aria-pressed", String(selected));
    });
    const timeout = setTimeout(() => request.abort(), 12000);
    try {
      const address = new URL(root.dataset.chartUrl, location.href);
      address.searchParams.set("range", range);
      const csv = byId("download-traffic");
      if (csv) {
        const url = new URL(address);
        url.searchParams.set("export", "csv");
        csv.href = url.href;
      }
      const response = await fetch(address, {
        signal: request.signal,
        cache: "no-store",
        headers: { Accept: "application/json" },
      });
      if (!response.ok) throw new Error("History unavailable");
      const data = await response.json();
      if (!Array.isArray(data.points)) throw new Error("Invalid history");
      if (controller !== request) return;
      points = data.points
        .filter((point) => Number.isFinite(Date.parse(point.at)))
        .map((point) => ({
          at: Date.parse(point.at),
          ...Object.fromEntries(
            ["bytes", "upload_bytes", "download_bytes"].map((key) => [
              key,
              Number.isFinite(Number(point[key]))
                ? Math.max(0, Number(point[key]))
                : 0,
            ]),
          ),
        }))
        .sort((a, b) => a.at - b.at);
      state = points.length ? "ready" : "empty";
    } catch {
      if (controller !== request) return;
      state = "error";
    } finally {
      clearTimeout(timeout);
      if (controller === request) render();
    }
  }
  document
    .querySelectorAll("[data-range]")
    .forEach((button) =>
      button.addEventListener("click", () => load(button.dataset.range)),
    );
  document.querySelectorAll("[data-metric]").forEach((button) =>
    button.addEventListener("click", () => {
      metric = button.dataset.metric;
      document.querySelectorAll("[data-metric]").forEach((option) => {
        const selected = option === button;
        option.classList.toggle("is-selected", selected);
        option.setAttribute("aria-pressed", String(selected));
      });
      render();
    }),
  );
  retry.addEventListener("click", () => load(range));
  new MutationObserver(render).observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["lang"],
  });
  new ResizeObserver(render).observe(chart.parentElement);
  load(range);
})();
