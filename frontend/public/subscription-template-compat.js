/* VeloRay legacy subscription template adapter. No eval or external QR service. */
(function () {
  'use strict';
  window.VeloRayTemplate = { escapeHTML: function(value) {
    return String(value == null ? '' : value).replace(/[&<>"']/g, function(ch) {
      return ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'})[ch];
    });
  }};
  function qrURL(value) {
    var url = new URL(location.href);
    url.search = ''; url.hash = '';
    url.searchParams.set('format', 'qr');
    url.searchParams.set('data', String(value || ''));
    return url.href;
  }
  if (!window.QRious) {
    window.QRious = function (options) {
      this.element = options.element;
      this.size = Math.max(64, Math.min(1024, options.size || 220));
      if (options.value) this.value = options.value;
    };
    Object.defineProperty(window.QRious.prototype, 'value', {
      get: function () { return this._value || ''; },
      set: function (value) {
        var self = this, image = new Image();
        this._value = value;
        image.onload = function () {
          if (self._value !== value) return;
          self.element.width = self.size; self.element.height = self.size;
          var context = self.element.getContext('2d');
          if (context) { context.imageSmoothingEnabled = false; context.drawImage(image, 0, 0, self.size, self.size); }
        };
        image.src = qrURL(value);
      }
    });
  }
  if (!window.QRCode) {
    window.QRCode = function (holder, options) {
      this.holder = typeof holder === 'string' ? document.getElementById(holder) : holder;
      this.options = options || {};
      this.makeCode(typeof options === 'string' ? options : this.options.text);
    };
    window.QRCode.CorrectLevel = { L: 1, M: 0, Q: 3, H: 2 };
    window.QRCode.prototype.clear = function () { this.holder.replaceChildren(); };
    window.QRCode.prototype.makeCode = function (value) {
      this.clear(); var image = new Image();
      image.alt = 'QR code'; image.width = this.options.width || 200; image.height = this.options.height || 200;
      image.src = qrURL(value); this.holder.appendChild(image);
    };
  }
  var allowed = new Set(['copyLink', 'copyText', 'showQR', 'openQRModal', 'closeModal']);
  function argumentsFor(source, element) {
    var values = [], at = 0;
    while (at < source.length) {
      while (/\s/.test(source[at] || '') && at < source.length) at++;
      if (source.slice(at, at + 4) === 'this' && /^(?:\s*,|\s*$)/.test(source.slice(at + 4))) { values.push(element); at += 4; }
      else {
        var quote = source[at++];
        if (quote !== '"' && quote !== "'") return null;
        var value = '', closed = false;
        while (at < source.length) {
          var ch = source[at++];
          if (ch === quote && /^(?:\s*,|\s*$)/.test(source.slice(at))) { closed = true; break; }
          if (ch === '\\') {
            var escaped = source[at++];
            if (escaped === 'u' && /^[a-f\d]{4}$/i.test(source.slice(at, at + 4))) { ch = String.fromCharCode(parseInt(source.slice(at, at + 4), 16)); at += 4; }
            else ch = ({ n: '\n', r: '\r', t: '\t' })[escaped] || escaped;
          }
          value += ch;
        }
        if (!closed) return null;
        values.push(value);
      }
      while (/\s/.test(source[at] || '') && at < source.length) at++;
      if (at < source.length && source[at++] !== ',') return null;
      if (values.length > 2) return null;
    }
    return values;
  }
  document.addEventListener('click', function (event) {
    for (var element = event.target instanceof Element ? event.target : null; element; element = element.parentElement) {
      var action = element.getAttribute('data-vr-click');
      if (!action) continue;
      if (action.trim() === 'event.stopPropagation()') { event.stopPropagation(); return; }
      var match = /^([A-Za-z]\w*)\(([\s\S]*)\)\s*;?$/.exec(action.trim());
      if (!match || !allowed.has(match[1]) || typeof window[match[1]] !== 'function') continue;
      var args = argumentsFor(match[2], element);
      if (args) window[match[1]].apply(element, args);
    }
  });
})();
