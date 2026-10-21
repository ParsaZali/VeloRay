/* VeloRay legacy subscription template adapter. No eval or external QR service. */
(function () {
  "use strict";
  window.VeloRayTemplate = {
    escapeHTML: function (value) {
      return String(value == null ? "" : value).replace(
        /[&<>"']/g,
        function (ch) {
          return {
            "&": "&amp;",
            "<": "&lt;",
            ">": "&gt;",
            '"': "&quot;",
            "'": "&#39;",
          }[ch];
        },
      );
    },
  };
  // The supplied legacy dashboard keeps its layout, but reads real token-scoped
  // history rather than spreading lifetime traffic over invented daily weights.
  var historyPoints = null,
    historyPending = false,
    historyReadAt = 0;
  window.VeloRayTemplate.drawHistory = function () {
    var holder = document.getElementById("interactiveChartContainer");
    if (!holder) return;
    if (document.body.dataset.vrHistoryEnabled === "false") {
      holder.hidden = true;
      return;
    }
    var start = document.getElementById("chartStartDateLabel");
    var end = document.getElementById("chartEndDateLabel");
    var line = document.getElementById("chartLinePath");
    var area = document.getElementById("chartAreaPath");
    var marks = document.getElementById("chartPointsGroup");
    var fa = document.documentElement.lang === "fa";
    function notice(text) {
      line.setAttribute("d", "");
      area.setAttribute("d", "");
      marks.replaceChildren();
      start.textContent = "";
      end.textContent = text;
    }
    function draw() {
      if (!historyPoints.length) {
        notice(
          fa
            ? "هنوز مصرفی در این بازه ثبت نشده است."
            : "No recorded usage in this period.",
        );
        return;
      }
      var points = historyPoints,
        first = points[0].at,
        last = points[points.length - 1].at;
      var max = Math.max.apply(
        null,
        [1].concat(
          points.map(function (p) {
            return p.bytes;
          }),
        ),
      );
      var x = function (p) {
        return first === last ? 50 : 2 + ((p.at - first) / (last - first)) * 96;
      };
      var y = function (p) {
        return 22 - (p.bytes / max) * 16;
      };
      var path = points
        .map(function (p, i) {
          return (i ? "L " : "M ") + x(p) + " " + y(p);
        })
        .join(" ");
      line.setAttribute("d", path);
      area.setAttribute(
        "d",
        path +
          " L " +
          x(points[points.length - 1]) +
          " 25 L " +
          x(points[0]) +
          " 25 Z",
      );
      marks.replaceChildren();
      var formatter = new Intl.DateTimeFormat(fa ? "fa-IR" : "en-US", {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
      points.forEach(function (p) {
        var circle = document.createElementNS(
          "http://www.w3.org/2000/svg",
          "circle",
        );
        circle.setAttribute("cx", x(p));
        circle.setAttribute("cy", y(p));
        circle.setAttribute("r", "1.5");
        circle.setAttribute("fill", "var(--color-chart)");
        var title = document.createElementNS(
          "http://www.w3.org/2000/svg",
          "title",
        );
        title.textContent =
          formatter.format(p.at) +
          " · " +
          new Intl.NumberFormat(fa ? "fa-IR" : "en-US").format(p.bytes) +
          " B";
        circle.appendChild(title);
        marks.appendChild(circle);
      });
      start.textContent = formatter.format(first);
      end.textContent = formatter.format(last);
    }
    if (historyPoints !== null) draw();
    if (historyPending || Date.now() - historyReadAt < 30000) return;
    historyPending = true;
    if (historyPoints === null)
      notice(fa ? "در حال دریافت مصرف…" : "Loading usage history…");
    var address = new URL(
      document.body.dataset.vrChartUrl || location.href,
      location.href,
    );
    address.searchParams.set("format", "stats");
    address.searchParams.set("range", "7d");
    var controller = new AbortController(),
      timeout = setTimeout(function () {
        controller.abort();
      }, 12000);
    fetch(address, {
      cache: "no-store",
      signal: controller.signal,
      headers: { Accept: "application/json" },
    })
      .then(function (response) {
        if (!response.ok) throw new Error("History unavailable");
        return response.json();
      })
      .then(function (data) {
        if (!Array.isArray(data.points)) throw new Error("Invalid history");
        historyPoints = data.points
          .filter(function (p) {
            return Number.isFinite(Date.parse(p.at));
          })
          .map(function (p) {
            return {
              at: Date.parse(p.at),
              bytes: Number.isFinite(Number(p.bytes))
                ? Math.max(0, Number(p.bytes))
                : 0,
            };
          })
          .sort(function (a, b) {
            return a.at - b.at;
          });
        draw();
      })
      .catch(function () {
        if (historyPoints === null)
          notice(
            fa
              ? "دریافت تاریخچه ناموفق بود؛ برای تلاش دوباره کلیک کنید."
              : "History unavailable. Click to retry.",
          );
      })
      .finally(function () {
        clearTimeout(timeout);
        historyPending = false;
        historyReadAt = Date.now();
      });
    if (!holder.dataset.vrRetryBound) {
      holder.dataset.vrRetryBound = "true";
      var retry = function () {
        if (historyPoints === null && !historyPending) {
          historyReadAt = 0;
          window.VeloRayTemplate.drawHistory();
        }
      };
      end.setAttribute("role", "button");
      end.tabIndex = 0;
      end.addEventListener("click", retry);
      end.addEventListener("keydown", function (event) {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          retry();
        }
      });
    }
  };
  function qrURL(value) {
    var url = new URL(location.href);
    url.search = "";
    url.hash = "";
    url.searchParams.set("format", "qr");
    url.searchParams.set("data", String(value || ""));
    return url.href;
  }
  if (!window.QRious) {
    window.QRious = function (options) {
      this.element = options.element;
      this.size = Math.max(64, Math.min(1024, options.size || 220));
      if (options.value) this.value = options.value;
    };
    Object.defineProperty(window.QRious.prototype, "value", {
      get: function () {
        return this._value || "";
      },
      set: function (value) {
        var self = this,
          image = new Image();
        this._value = value;
        image.onload = function () {
          if (self._value !== value) return;
          self.element.width = self.size;
          self.element.height = self.size;
          var context = self.element.getContext("2d");
          if (context) {
            context.imageSmoothingEnabled = false;
            context.drawImage(image, 0, 0, self.size, self.size);
          }
        };
        image.src = qrURL(value);
      },
    });
  }
  if (!window.QRCode) {
    window.QRCode = function (holder, options) {
      this.holder =
        typeof holder === "string" ? document.getElementById(holder) : holder;
      this.options = options || {};
      this.makeCode(typeof options === "string" ? options : this.options.text);
    };
    window.QRCode.CorrectLevel = { L: 1, M: 0, Q: 3, H: 2 };
    window.QRCode.prototype.clear = function () {
      this.holder.replaceChildren();
    };
    window.QRCode.prototype.makeCode = function (value) {
      this.clear();
      var image = new Image();
      image.alt = "QR code";
      image.width = this.options.width || 200;
      image.height = this.options.height || 200;
      image.src = qrURL(value);
      this.holder.appendChild(image);
    };
  }
  var allowed = new Set([
    "copyLink",
    "copyText",
    "showQR",
    "openQRModal",
    "closeModal",
  ]);
  function argumentsFor(source, element) {
    var values = [],
      at = 0;
    while (at < source.length) {
      while (/\s/.test(source[at] || "") && at < source.length) at++;
      if (
        source.slice(at, at + 4) === "this" &&
        /^(?:\s*,|\s*$)/.test(source.slice(at + 4))
      ) {
        values.push(element);
        at += 4;
      } else {
        var quote = source[at++];
        if (quote !== '"' && quote !== "'") return null;
        var value = "",
          closed = false;
        while (at < source.length) {
          var ch = source[at++];
          if (ch === quote && /^(?:\s*,|\s*$)/.test(source.slice(at))) {
            closed = true;
            break;
          }
          if (ch === "\\") {
            var escaped = source[at++];
            if (
              escaped === "u" &&
              /^[a-f\d]{4}$/i.test(source.slice(at, at + 4))
            ) {
              ch = String.fromCharCode(parseInt(source.slice(at, at + 4), 16));
              at += 4;
            } else ch = { n: "\n", r: "\r", t: "\t" }[escaped] || escaped;
          }
          value += ch;
        }
        if (!closed) return null;
        values.push(value);
      }
      while (/\s/.test(source[at] || "") && at < source.length) at++;
      if (at < source.length && source[at++] !== ",") return null;
      if (values.length > 2) return null;
    }
    return values;
  }
  document.addEventListener("click", function (event) {
    for (
      var element = event.target instanceof Element ? event.target : null;
      element;
      element = element.parentElement
    ) {
      var action = element.getAttribute("data-vr-click");
      if (!action) continue;
      if (action.trim() === "event.stopPropagation()") {
        event.stopPropagation();
        return;
      }
      var match = /^([A-Za-z]\w*)\(([\s\S]*)\)\s*;?$/.exec(action.trim());
      if (
        !match ||
        !allowed.has(match[1]) ||
        typeof window[match[1]] !== "function"
      )
        continue;
      var args = argumentsFor(match[2], element);
      if (args) window[match[1]].apply(element, args);
    }
  });
})();
