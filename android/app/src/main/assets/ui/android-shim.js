// Android WebView bridge: exposes the same shapes the Wails runtime gives
// the desktop frontend — window.go.main.App.* (promise-returning binding
// proxies) and window.runtime.EventsOn. Every call is a JSON round-trip to
// the Kotlin bridge object (window.WisteriaAndroid, addJavascriptInterface):
// arguments are one JSON array string, the reply is either the JSON-encoded
// result or {"__error":"..."} which turns into a rejected promise — the
// same contract the frontend's try/catch expects.
"use strict";
(function () {
  function call(name, args) {
    const n = window.WisteriaAndroid;
    if (!n) return Promise.reject(new Error("мост ещё не готов"));
    return new Promise((resolve, reject) => {
      setTimeout(() => {
        try {
          let v;
          try {
            v = JSON.parse(n.Call(name, args === undefined ? "[]" : JSON.stringify(args)));
          } catch (e) {
            return reject(new Error("мост: некорректный ответ"));
          }
          if (v && v.__error) return reject(new Error(v.__error));
          resolve(v === undefined ? null : v);
        } catch (e) {
          reject(e);
        }
      }, 0);
    });
  }

  window.go = {
    main: {
      App: new Proxy({}, {
        get(_, name) {
          return (...args) => call(name, args);
        },
      }),
    },
  };

  const handlers = {};
  window.runtime = {
    EventsOn(event, cb) {
      (handlers[event] = handlers[event] || []).push(cb);
    },
    EventsOff(event) {
      delete handlers[event];
    },
    EventsEmit() {}, // the frontend never emits; keep the shape complete
  };

  // The Kotlin side evaluates this for every pushed event.
  window.__wisteriaEmit = function (event, payloadJson) {
    let data = null;
    try { data = payloadJson == null ? null : JSON.parse(payloadJson); } catch (e) { /* null */ }
    (handlers[event] || []).forEach((cb) => {
      try { cb(data); } catch (e) { console.error(e); }
    });
  };

  window.__PLATFORM = "android";
  document.documentElement.classList.add("platform-android");

  // ---------- bottom navigation (Hiddify-style) ----------
  // Five items across the top of a phone are hard to reach; navigation moves
  // to a fixed bottom bar: «Главная» opens the VPN view directly, «Ещё»
  // opens a menu with the four secondary views. Clicks are forwarded to the
  // real sidebar buttons so SPA state stays exactly as the desktop drives it.
  function buildBottomNav() {
    if (document.getElementById("android-bottom")) return;
    const app = document.getElementById("app");
    if (!app) return;

    const stroke = 'fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"';
    const svg = (body) => `<svg viewBox="0 0 24 24" ${stroke}>${body}</svg>`;
    const ICONS = {
      home: svg('<path d="M12 2v10"/><path d="M18.4 6.6a9 9 0 1 1-12.77.04"/>'),
      more: svg('<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>'),
      profiles: svg('<path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/>'),
      logs: svg('<polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/>'),
      diag: svg('<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>'),
      settings: svg('<line x1="4" y1="21" x2="4" y2="14"/><line x1="4" y1="10" x2="4" y2="3"/><line x1="12" y1="21" x2="12" y2="12"/><line x1="12" y1="8" x2="12" y2="3"/><line x1="20" y1="21" x2="20" y2="16"/><line x1="20" y1="12" x2="20" y2="3"/><line x1="1" y1="14" x2="7" y2="14"/><line x1="9" y1="8" x2="15" y2="8"/><line x1="17" y1="16" x2="23" y2="16"/>'),
      chev: svg('<path d="M9 6l6 6-6 6"/>'),
    };

    const bar = document.createElement("nav");
    bar.id = "android-bottom";
    bar.innerHTML =
      `<button class="ab-item" id="ab-home">${ICONS.home}<span data-i18n="ab.home">Главная</span></button>` +
      `<button class="ab-item" id="ab-more">${ICONS.more}<span data-i18n="ab.more">Ещё</span></button>`;
    app.appendChild(bar);

    const menu = document.createElement("div");
    menu.id = "android-menu";
    const row = (tab) =>
      `<button class="am-item" data-goto="${tab}">${ICONS[tab]}<span data-i18n="nav.${tab}"></span><span class="am-chev">${ICONS.chev}</span></button>`;
    menu.innerHTML =
      '<div class="am-scrim"></div><div class="am-card">' +
      '<div class="am-title" data-i18n="ab.more">Ещё</div>' +
      row("profiles") + row("logs") + row("diag") + row("settings") +
      "</div>";
    app.appendChild(menu);

    const navTo = (tab) => {
      const b = document.querySelector(`.sidebar-nav-item[data-tab="${tab}"]`);
      if (b) b.click();
      menu.classList.remove("open");
    };
    bar.querySelector("#ab-home").addEventListener("click", () => navTo("vpn"));
    bar.querySelector("#ab-more").addEventListener("click", () => menu.classList.add("open"));
    menu.querySelector(".am-scrim").addEventListener("click", () => menu.classList.remove("open"));
    menu.querySelectorAll(".am-item").forEach((b) =>
      b.addEventListener("click", () => navTo(b.dataset.goto)));

    // Mirror the SPA's active view onto the two bottom tabs; the sidebar
    // nav itself is display:none but still owns the source of truth.
    const sync = () => {
      const cur = document.querySelector(".sidebar-nav-item.active");
      const homeActive = !cur || cur.dataset.tab === "vpn";
      bar.querySelector("#ab-home").classList.toggle("active", homeActive);
      bar.querySelector("#ab-more").classList.toggle("active", !homeActive);
    };
    new MutationObserver(sync).observe(document.querySelector(".sidebar-nav"), {
      subtree: true, attributes: true, attributeFilter: ["class"],
    });
    sync();
  }
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", buildBottomNav);
  } else {
    buildBottomNav();
  }

  // The settings form is re-rendered on every visit; drop the Windows-only
  // switch pairs (start minimized / confirm exit) each time they appear, and
  // relabel the autostart toggle: s-autostart maps to Android boot autostart.
  const killPairs = ["s-startmin", "s-confirmexit"];
  new MutationObserver(() => {
    document.body.classList.toggle("ab-modal", !!document.querySelector(".modal-overlay.show"));
    const as = document.querySelector("input#s-autostart");
    if (as) {
      const lbl = as.closest("label.switch")?.previousElementSibling;
      if (lbl && lbl.tagName === "LABEL" && lbl.textContent.includes("Windows"))
        lbl.textContent = lbl.textContent
          .replace("при входе в Windows", "при включении телефона")
          .replace("Start with Windows", "Start on device boot");
    }
    killPairs.forEach((id) => {
      const el = document.getElementById(id);
      if (!el) return;
      const ctl = el.closest("label.switch") || el;
      const lbl = ctl.previousElementSibling;
      if (lbl && lbl.tagName === "LABEL") lbl.remove();
      ctl.remove();
    });
    // attributes too: .modal-overlay.show is a class change, not a DOM one
  }).observe(document.documentElement, { childList: true, subtree: true, attributes: true });
})();
