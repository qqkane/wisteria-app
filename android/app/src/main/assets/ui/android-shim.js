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
          let raw, v;
          try {
            raw = n.Call(name, args === undefined ? "[]" : JSON.stringify(args));
          } catch (e) {
            return reject(e);
          }
          try {
            v = JSON.parse(raw);
          } catch (e) {
            // Wails JSON-encodes Go strings; the raw bridge returns them
            // verbatim (Version). Treat an unparseable reply as a string.
            v = typeof raw === "string" && raw !== "" ? raw : null;
          }
          if (v && v.__error) return reject(new Error(v.__error));
          resolve(v === undefined ? null : v);
        } catch (e) {
          reject(e);
        }
      }, 0);
    });
  }

  // Selecting a config (a profile row calls SetActive; the big button and
  // switch-while-connected call Connect) jumps to the main screen. Per user
  // request subscription ops (import / add / refresh) must NOT navigate.
  const AUTO_NAV = { SetActive: 1, Connect: 1 };
  const activeTab = () =>
    document.querySelector(".sidebar-nav-item.active")?.dataset.tab || "vpn";
  window.go = {
    main: {
      App: new Proxy({}, {
        get(_, name) {
          return (...args) =>
            call(name, args).then((res) => {
              if (AUTO_NAV[name] && activeTab() !== "vpn") {
                setTimeout(() => { if (activeTab() !== "vpn") navTo("vpn"); }, 350);
              }
              return res;
            });
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

  // ---------- inline icons (feather-style, match the app's icon set) ----------
  const stroke = 'fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"';
  const svg = (body) => `<svg viewBox="0 0 24 24" ${stroke}>${body}</svg>`;
  const ICONS = {
    home: svg('<path d="M12 2v10"/><path d="M18.4 6.6a9 9 0 1 1-12.77.04"/>'),
    back: svg('<line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/>'),
    logs: svg('<polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/>'),
    diag: svg('<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>'),
    settings: svg('<line x1="4" y1="21" x2="4" y2="14"/><line x1="4" y1="10" x2="4" y2="3"/><line x1="12" y1="21" x2="12" y2="12"/><line x1="12" y1="8" x2="12" y2="3"/><line x1="20" y1="21" x2="20" y2="16"/><line x1="20" y1="12" x2="20" y2="3"/><line x1="1" y1="14" x2="7" y2="14"/><line x1="9" y1="8" x2="15" y2="8"/><line x1="17" y1="16" x2="23" y2="16"/>'),
    chev: svg('<path d="M9 6l6 6-6 6"/>'),
  };

  // ---------- bottom navigation ----------
  // Five items across the top of a phone are hard to reach. The bottom bar
  // has two tabs: «Главная» (VPN view) and «Настройки» (settings view, which
  // carries quick links to Логи and Диагностика). Clicks are forwarded to
  // the real sidebar buttons so SPA state stays desktop-identical.
  const navTo = (tab) => {
    const b = document.querySelector(`.sidebar-nav-item[data-tab="${tab}"]`);
    if (b) b.click();
  };

  function buildBottomNav() {
    if (document.getElementById("android-bottom")) return;
    const app = document.getElementById("app");
    if (!app) return;
    const bar = document.createElement("nav");
    bar.id = "android-bottom";
    bar.innerHTML =
      `<button class="ab-item" id="ab-home">${ICONS.home}<span data-i18n="ab.home">Главная</span></button>` +
      `<button class="ab-item" id="ab-settings">${ICONS.settings}<span data-i18n="nav.settings">Настройки</span></button>`;
    app.appendChild(bar);
    bar.querySelector("#ab-home").addEventListener("click", () => navTo("vpn"));
    bar.querySelector("#ab-settings").addEventListener("click", () => navTo("settings"));
    // Mirror the SPA's active view onto the bottom tabs; the hidden sidebar
    // nav still owns the source of truth. Логи/Диагностика count as the
    // settings section (they are reached from it).
    const sync = () => {
      const cur = document.querySelector(".sidebar-nav-item.active");
      const homeActive = !cur || cur.dataset.tab === "vpn";
      bar.querySelector("#ab-home").classList.toggle("active", homeActive);
      bar.querySelector("#ab-settings").classList.toggle("active", !homeActive);
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

  // One delegated click handler for the shim-built navigation cards; they
  // are re-injected whenever the SPA re-renders the view.
  document.addEventListener("click", (ev) => {
    const b = ev.target.closest("[data-abgoto]");
    if (b) navTo(b.dataset.abgoto);
  });

  // While connecting, the big button becomes an active Cancel (the shared
  // frontend just disables it). The SPA's connecting marker is the button's
  // disabled attribute (renderVpn); "connected" adds the connected class.
  const isConnectingBtn = (btn) =>
    !!btn && btn.disabled && !btn.classList.contains("connected");

  document.addEventListener("click", (ev) => {
    const btn = ev.target.closest("#btn-toggle");
    if (!btn) return;
    if (btn.dataset.abCancel === "1" || isConnectingBtn(btn)) {
      ev.preventDefault();
      ev.stopImmediatePropagation();
      call("Disconnect").catch(() => {});
      // Instant feedback: do not wait for the service's state event (the
      // user saw a dead button when that round-trip stalled). The next
      // state event re-syncs the real picture anyway. "Connect" is the
      // button label in both dictionaries by design.
      delete btn.dataset.abCancel;
      btn.classList.remove("ab-cancel");
      btn.disabled = false;
      const txt = btn.querySelector(".connect-text");
      if (txt) {
        delete txt.dataset.abCancel;
        txt.textContent = "Connect";
      }
    }
  }, true);

  // Keep the language toggle's highlight in sync with the select: the SPA
  // rewrites the form (fillSettingsForm) without firing a DOM mutation.
  document.addEventListener("change", (ev) => {
    if (!ev.target || ev.target.id !== "s-lang") return;
    document.querySelectorAll(".ab-lang button").forEach((b) =>
      b.classList.toggle("active", b.dataset.lang === ev.target.value));
  });

  // The settings form is re-rendered on every visit; re-apply each time:
  // - drop the Windows-only switch pairs (start minimized / confirm exit)
  // - relabel the autostart toggle (it maps to Android boot autostart)
  // - quick links to Логи/Диагностика at the top of the settings view
  // - segmented RU/EN toggle instead of the native <select> dialog
  const killPairs = ["s-startmin", "s-confirmexit"];
  let abLangWanted = null; // language picked via the segmented toggle
  function secCard(tab) {
    // the boot-time i18n pass has already run by the time these render, so
    // take the label text from the (hidden, already-translated) sidebar nav
    const label = document.querySelector(`.sidebar-nav-item[data-tab="${tab}"]`)?.textContent?.trim() || tab;
    return `<button class="ab-sec-card" data-abgoto="${tab}">${ICONS[tab]}` +
      `<span data-i18n="nav.${tab}">${label}</span><span class="ab-chev">${ICONS.chev}</span></button>`;
  }
  new MutationObserver(() => {
    document.body.classList.toggle("ab-modal", !!document.querySelector(".modal-overlay.show"));

    // «Отмена» instead of a dead disabled button while connecting. The SPA
    // marks connecting with the button's disabled attr; once we take over,
    // our own abCancel marker keeps the state stable across observer runs
    // (the SPA re-render on state change drops it with the old DOM). Every
    // write below is guarded: this observer watches attributes on the whole
    // document, and an unguarded same-value write would re-trigger it on
    // every pass — a self-sustaining microtask loop, i.e. a frozen app.
    const btn = document.getElementById("btn-toggle");
    if (btn) {
      const spaConnecting = btn.disabled && !btn.classList.contains("connected");
      if (spaConnecting || btn.dataset.abCancel === "1") {
        const txt = btn.querySelector(".connect-text");
        if (btn.dataset.abCancel !== "1") btn.dataset.abCancel = "1";
        if (btn.disabled) btn.disabled = false;
        if (!btn.classList.contains("ab-cancel")) btn.classList.add("ab-cancel");
        if (txt && txt.dataset.abCancel !== "1") {
          txt.dataset.abCancel = "1";
          txt.textContent = document.documentElement.lang === "en" ? "Cancel" : "Отмена";
        }
      }
    }

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

    const title = document.querySelector(".settings-title");
    if (title && !document.getElementById("ab-sec-grid")) {
      const grid = document.createElement("div");
      grid.id = "ab-sec-grid";
      grid.innerHTML = secCard("logs") + secCard("diag");
      title.after(grid);
    }

    // «Назад» on the Логи/Диагностика screens: the phone has no sidebar,
    // and the bottom bar's second tab is not an obvious way back.
    const curTab = document.querySelector(".sidebar-nav-item.active")?.dataset.tab;
    if ((curTab === "logs" || curTab === "diag") &&
        !document.getElementById("ab-back") && !document.getElementById("content-wrapper")?.querySelector(".modal-overlay")) {
      const host = document.querySelector(".logs-header");
      if (host) {
        const back = document.createElement("button");
        back.id = "ab-back";
        back.className = "ab-sec-card";
        back.innerHTML = `${ICONS.back}<span>${document.documentElement.lang === "en" ? "Back" : "Назад"}</span>` +
          `<span class="ab-chev">${ICONS.chev}</span>`;
        back.addEventListener("click", () => navTo("settings"));
        host.before(back);
      }
    }

    const langSel = document.getElementById("s-lang");
    if (langSel && !(langSel.nextElementSibling && langSel.nextElementSibling.classList.contains("ab-lang"))) {
      const seg = document.createElement("div");
      seg.className = "ab-lang";
      seg.innerHTML = '<button type="button" data-lang="ru">Русский</button>' +
        '<button type="button" data-lang="en">English</button>';
      langSel.after(seg);
      // The re-rendered template select resets to "ru"; fillSettingsForm
      // applies the real value only after its async GetSettings, so prefer
      // the language the toggle itself picked (abLangWanted).
      const sync = () => seg.querySelectorAll("button").forEach((b) =>
        b.classList.toggle("active", b.dataset.lang === (abLangWanted || langSel.value || "ru")));
      seg.querySelectorAll("button").forEach((b) => b.addEventListener("click", () => {
        if (langSel.value === b.dataset.lang && !abLangWanted) return;
        abLangWanted = b.dataset.lang;
        langSel.value = b.dataset.lang;
        // the frontend listens for change on #s-lang: re-renders everything
        // in the new language; pressing «Сохранить настройки» persists it
        langSel.dispatchEvent(new Event("change", { bubbles: true }));
        setTimeout(sync, 200); // re-sync after fillSettingsForm settled
      }));
      sync();
    }
    // attributes too: .modal-overlay.show is a class change, not a DOM one
  }).observe(document.documentElement, { childList: true, subtree: true, attributes: true });

  // Cold-start safety net: if the SPA boot chain died before its first
  // render (any bridge hiccup leaves an empty content area), replay the
  // boot's data loads directly into S and re-render — a plain state re-emit
  // is not enough when an earlier await aborted the chain. A full reload is
  // the last resort.
  setTimeout(() => {
    const cw = document.getElementById("content-wrapper");
    if (!cw || cw.children.length > 0 || document.querySelector(".modal-overlay")) return;
    Promise.all([
      call("GetState").catch(() => null),
      call("ListProfiles").catch(() => null),
      call("ListSubscriptions").catch(() => null),
    ]).then(([st, profiles, subs]) => {
      if (window.S) {
        if (profiles) S.profiles = profiles;
        if (subs) S.subs = subs;
      }
      if (st) window.__wisteriaEmit("state", JSON.stringify(st));
      // renderState is wired to "state"; if the view is STILL empty the
      // renderer itself died — reload once.
      setTimeout(() => {
        const w = document.getElementById("content-wrapper");
        if (!w || w.children.length > 0) return;
        try {
          if (!sessionStorage.getItem("ab-reloaded")) {
            sessionStorage.setItem("ab-reloaded", "1");
            location.reload();
          }
        } catch (e) { /* private mode */ }
      }, 1200);
    });
  }, 2500);
})();
