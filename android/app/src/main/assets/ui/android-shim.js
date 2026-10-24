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

  // The settings form is re-rendered on every visit; drop the Windows-only
  // switch pairs (start minimized / confirm exit) each time they appear, and
  // relabel the autostart toggle: s-autostart maps to Android boot autostart.
  const killPairs = ["s-startmin", "s-confirmexit"];
  new MutationObserver(() => {
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
  }).observe(document.documentElement, { childList: true, subtree: true });
})();
