// Android WebView bridge: exposes the same shapes the Wails runtime gives
// the desktop frontend — window.go.main.App.* (promise-returning binding
// proxies) and window.runtime.EventsOn. Every call is a JSON round-trip to
// the Kotlin bridge object (window.WisteriaAndroid, addJavascriptInterface):
// arguments are one JSON array string, the reply is either the JSON-encoded
// result or {"__error":"..."} which turns into a rejected promise — the
// same contract the frontend's try/catch expects.
"use strict";
(function () {
  // ---------- async bridge ----------
  // CallAsync returns immediately and answers through __wisteriaResolve();
  // Call (synchronous, holds the JS thread) stays as a fallback for an
  // older Kotlin side. Every binding the SPA issues goes through the async
  // path, so package enumeration, a 15 s subscription fetch or an HTTP probe
  // can never freeze the interface again.
  const pending = new Map();
  const CALL_TIMEOUT_MS = 60000;
  let reqSeq = 0;

  function settle(reqId, replyJson, fail) {
    const p = pending.get(reqId);
    if (!p) return;
    pending.delete(reqId);
    clearTimeout(p.timer);
    if (fail) return p.reject(fail);
    let v;
    try {
      v = JSON.parse(replyJson);
    } catch (e) {
      // Unparseable reply: treat it as a bare string (Version used to come
      // back unquoted). An empty reply means null.
      v = typeof replyJson === "string" && replyJson !== "" ? replyJson : null;
    }
    if (v && v.__error) return p.reject(new Error(v.__error));
    p.resolve(v === undefined ? null : v);
  }

  // The Kotlin side evaluates this for every finished async call.
  window.__wisteriaResolve = function (reqId, replyJson) {
    settle(reqId, replyJson, null);
  };

  function callSync(name, args) {
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

  function call(name, args) {
    const n = window.WisteriaAndroid;
    if (!n) return Promise.reject(new Error("мост ещё не готов"));
    if (typeof n.CallAsync !== "function") return callSync(name, args);
    return new Promise((resolve, reject) => {
      const id = "r" + (++reqSeq);
      // A wedged binding must not lock the SPA's busy guard forever.
      const timer = setTimeout(() => {
        pending.delete(id);
        reject(new Error("таймаут вызова " + name));
      }, CALL_TIMEOUT_MS);
      pending.set(id, { resolve, reject, timer });
      try {
        n.CallAsync(id, name, args === undefined ? "[]" : JSON.stringify(args));
      } catch (e) {
        pending.delete(id);
        clearTimeout(timer);
        reject(e);
      }
    });
  }

  // Selecting a config (a profile row calls SetActive; the big button and
  // switch-while-connected call Connect) jumps to the main screen. Per user
  // request subscription ops (import / add / refresh) must NOT navigate.
  // Consecutive calls (switch-while-connected does SetActive then Connect)
  // are debounced into a single navigation.
  const AUTO_NAV = { SetActive: 1, Connect: 1 };
  let autoNavTimer = null;
  const activeTab = () =>
    document.querySelector(".sidebar-nav-item.active")?.dataset.tab || "vpn";
  window.go = {
    main: {
      App: new Proxy({}, {
        get(_, name) {
          return (...args) =>
            call(name, args).then((res) => {
              if (AUTO_NAV[name] && activeTab() !== "vpn") {
                clearTimeout(autoNavTimer);
                autoNavTimer = setTimeout(() => {
                  autoNavTimer = null;
                  if (activeTab() !== "vpn") navTo("vpn");
                }, 350);
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

  // Android Back button: consume it for the SPA's own layers — an open
  // modal first, then a non-VPN tab — and only let it exit the app from
  // the main VPN screen. MainActivity asks via evaluateJavascript and
  // calls super.onBackPressed() unless this returns "true".
  window.__wisteriaBack = function () {
    const open = document.querySelector(".modal-overlay.show");
    if (open) {
      // Route through the modal's own close path: #cf-cancel resolves the
      // askConfirm promise, .modal-close runs UI.closeModal().
      const closer = open.querySelector(".modal-close, #cf-cancel");
      if (closer) closer.click(); else open.classList.remove("show");
      return "true";
    }
    const cur = document.querySelector(".sidebar-nav-item.active")?.dataset.tab;
    if (cur && cur !== "vpn") { navTo("vpn"); return "true"; }
    return "false";
  };

  window.__PLATFORM = "android";
  document.documentElement.classList.add("platform-android");

  // ---------- inline icons (feather-style, match the app's icon set) ----------
  const stroke = 'fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"';
  const svg = (body) => `<svg viewBox="0 0 24 24" ${stroke}>${body}</svg>`;
  const ICONS = {
    home: svg('<path d="M12 2v10"/><path d="M18.4 6.6a9 9 0 1 1-12.77.04"/>'),
    back: svg('<line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/>'),
    profiles: svg('<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>'),
    logs: svg('<polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/>'),
    diag: svg('<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>'),
    settings: svg('<line x1="4" y1="21" x2="4" y2="14"/><line x1="4" y1="10" x2="4" y2="3"/><line x1="12" y1="21" x2="12" y2="12"/><line x1="12" y1="8" x2="12" y2="3"/><line x1="20" y1="21" x2="20" y2="16"/><line x1="20" y1="12" x2="20" y2="3"/><line x1="1" y1="14" x2="7" y2="14"/><line x1="9" y1="8" x2="15" y2="8"/><line x1="17" y1="16" x2="23" y2="16"/>'),
    chev: svg('<path d="M9 6l6 6-6 6"/>'),
  };

  // ---------- bottom navigation ----------
  // Five items across the top of a phone are hard to reach. The bottom bar
  // has three tabs: «Главная» (VPN view), «Профили» (profiles, centered —
  // the mission's main nav entry) and «Настройки» (settings view, which
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
      `<button class="ab-item" id="ab-profiles">${ICONS.profiles}<span data-i18n="nav.profiles">Профили</span></button>` +
      `<button class="ab-item" id="ab-settings">${ICONS.settings}<span data-i18n="nav.settings">Настройки</span></button>`;
    app.appendChild(bar);
    bar.querySelector("#ab-home").addEventListener("click", () => navTo("vpn"));
    bar.querySelector("#ab-profiles").addEventListener("click", () => navTo("profiles"));
    bar.querySelector("#ab-settings").addEventListener("click", () => navTo("settings"));
    // Mirror the SPA's active view onto the bottom tabs; the hidden sidebar
    // nav still owns the source of truth. Логи/Диагностика count as the
    // settings section (they are reached from it).
    const sync = () => {
      const cur = document.querySelector(".sidebar-nav-item.active")?.dataset.tab || "vpn";
      bar.querySelector("#ab-home").classList.toggle("active", cur === "vpn");
      bar.querySelector("#ab-profiles").classList.toggle("active", cur === "profiles");
      bar.querySelector("#ab-settings").classList.toggle("active", cur === "settings" || cur === "logs" || cur === "diag");
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
  // - quick links to Логи/Диагностика at the top of the settings view
  // - segmented RU/EN toggle instead of the native <select> dialog
  // (The desktop-only rows — start minimized / confirm exit / autoping — and
  // the Android autostart wording are handled by app.js itself, gated on
  // window.__PLATFORM: mutating rendered DOM from a global observer was both
  // fragile and expensive.)
  let abLangWanted = null; // language picked via the segmented toggle
  function secCard(tab) {
    // the boot-time i18n pass has already run by the time these render, so
    // take the label text from the (hidden, already-translated) sidebar nav
    const label = document.querySelector(`.sidebar-nav-item[data-tab="${tab}"]`)?.textContent?.trim() || tab;
    return `<button class="ab-sec-card" data-abgoto="${tab}">${ICONS[tab]}` +
      `<span data-i18n="nav.${tab}">${label}</span><span class="ab-chev">${ICONS.chev}</span></button>`;
  }
  // ---------- targeted observers ----------
  // The old code watched EVERY attribute on documentElement (attributes:
  // true on the subtree root): every CSS animation class toggle and every
  // button state change re-ran a document-wide scan, which is exactly the
  // work that makes the WebView drop frames while the connect animation
  // plays. The observers below watch only the nodes they need.

  // Modals: `.modal-overlay.show` is a class change, not a DOM one.
  const modalSeen = new WeakSet();
  function syncModalClass() {
    document.body.classList.toggle("ab-modal", !!document.querySelector(".modal-overlay.show"));
  }
  function observeModals() {
    document.querySelectorAll(".modal-overlay").forEach((m) => {
      if (modalSeen.has(m)) return;
      modalSeen.add(m);
      new MutationObserver(syncModalClass).observe(m, { attributes: true, attributeFilter: ["class"] });
    });
  }

  // «Отмена» instead of a dead disabled button while connecting. The SPA
  // marks connecting with the button's disabled attr; once we take over,
  // our own abCancel marker keeps the state stable across sync runs (the
  // SPA re-render on state change replaces the node, and ensureBtnObserver
  // re-binds to the fresh one). Every write below is guarded: this runs
  // from an attribute observer on the button itself, and an unguarded
  // same-value write would retrigger it on every pass — a self-sustaining
  // microtask loop, i.e. a frozen app.
  window.__wisteriaSyncBtn = function () {
    const btn = document.getElementById("btn-toggle");
    if (!btn) return;
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
  };

  // The connect button is recreated by every structural SPA render, so the
  // attribute observer is re-bound whenever the referenced node changes.
  let btnObserver = null;
  let btnObserved = null;
  function ensureBtnObserver() {
    const btn = document.getElementById("btn-toggle");
    if (btn === btnObserved && btnObserver) return;
    if (btnObserver) {
      btnObserver.disconnect();
      btnObserver = null;
    }
    btnObserved = btn;
    if (!btn) return;
    btnObserver = new MutationObserver(() => window.__wisteriaSyncBtn());
    btnObserver.observe(btn, { attributes: true, attributeFilter: ["disabled", "class"] });
  }

  observeModals();
  syncModalClass();
  ensureBtnObserver();
  window.__wisteriaSyncBtn();

  // ---------- the one remaining DOM pass ----------
  // It runs only when #content-wrapper's direct children change, i.e. on a
  // tab switch or a full view re-render — NOT on subtitle/log/uptime text
  // updates deep inside the view. The old observer watched
  // documentElement+subtree, so the 1 s uptime tick and every appended log
  // line re-ran a document-wide scan several times per second.
  let shimSyncTimer = 0;
  function shimSync() {
    syncModalClass();
    observeModals();
    ensureBtnObserver();
    // The SPA re-creates the button on state changes; sync the fresh node.
    window.__wisteriaSyncBtn();

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
  }

  // Coalesce bursts (a tab switch fires several childList records) into one
  // pass, and never run more than a few times a second.
  function scheduleShimSync() {
    if (shimSyncTimer) return;
    shimSyncTimer = setTimeout(() => {
      shimSyncTimer = 0;
      try { shimSync(); } catch (e) { /* never let the shim break the SPA */ }
    }, 80);
  }

  const contentHost = document.getElementById("content-wrapper");
  if (contentHost) {
    new MutationObserver(scheduleShimSync).observe(contentHost, { childList: true });
  }

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
