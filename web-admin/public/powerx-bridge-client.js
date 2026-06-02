(function () {
  if (typeof window === "undefined") {
    return;
  }

  var subscribers = new Set();
  var lastSyncAt = 0;

  function getCookie(name) {
    var escaped = name.replace(/[-[\]{}()*+?.,\\^$|#\s]/g, "\\$&");
    var m = document.cookie.match(new RegExp("(?:^|;\\s*)" + escaped + "=([^;]*)"));
    return m ? decodeURIComponent(m[1]) : "";
  }

  function setCookie(name, value) {
    var encoded =
      value === null || value === undefined || value === ""
        ? ""
        : encodeURIComponent(value);
    var expires =
      value === null || value === undefined || value === ""
        ? "Thu, 01 Jan 1970 00:00:00 GMT"
        : "";
    document.cookie =
      name +
      "=" +
      encoded +
      "; path=/; SameSite=Lax" +
      (expires ? "; expires=" + expires : "");
  }

  function applyAuthPayload(payload) {
    try {
      var ls = window.localStorage;
      if (payload.accessToken) {
        ls.setItem("access_token", payload.accessToken);
        setCookie("access_token", payload.accessToken);
      }
      if (payload.refreshToken) {
        ls.setItem("refresh_token", payload.refreshToken);
      }
      if (payload.tokenType) {
        ls.setItem("token_type", payload.tokenType);
      }
      if (payload.scope) {
        ls.setItem("scope", payload.scope);
      }
      if (payload.expiresAt) {
        ls.setItem("expires_at", String(payload.expiresAt));
      } else if (payload.expiresIn) {
        var exp = Date.now() + payload.expiresIn * 1000;
        ls.setItem("expires_at", String(exp));
      }
      var tenantUUID = payload.tenant_uuid || payload.tenantUuid || payload.tenantUUID;
      if (tenantUUID) {
        ls.setItem("px_current_tenant_uuid", tenantUUID);
        setCookie("px_current_tenant_uuid", tenantUUID);
      }

      subscribers.forEach(function (cb) {
        try {
          cb(payload);
        } catch (err) {
          console.warn("[PowerXBridgeClient] subscriber error", err);
        }
      });
    } catch (error) {
      console.warn("[PowerXBridgeClient] applyAuthPayload failed", error);
    }
  }

  function bootstrapAuthFromCookies() {
    try {
      var ls = window.localStorage;
      var token = getCookie("access_token");
      var refreshToken = getCookie("refresh_token");
      var tenantUUID = getCookie("px_current_tenant_uuid");

      if (token) {
        ls.setItem("access_token", token);
        ls.setItem("token_type", "Bearer");
        if (!ls.getItem("expires_at")) {
          try {
            var parts = token.split(".");
            if (parts.length >= 2) {
              var payload = JSON.parse(atob(parts[1].replace(/-/g, "+").replace(/_/g, "/")));
              var expSec = Number(payload && payload.exp);
              if (expSec > 0) {
                ls.setItem("expires_at", String(expSec * 1000));
              }
            }
          } catch (_ignore) {}
        }
      }
      if (refreshToken) {
        ls.setItem("refresh_token", refreshToken);
      }
      if (tenantUUID) {
        ls.setItem("px_current_tenant_uuid", tenantUUID);
      }
    } catch (error) {
      console.warn("[PowerXBridgeClient] bootstrapAuthFromCookies failed", error);
    }
  }

  function normalizeNavigatePath(path) {
    if (!path || typeof path !== "string") return "/";
    var p = path.trim();
    if (!p) return "/";
    if (!p.startsWith("/")) p = "/" + p;

    var adminBase =
      (window.__NUXT__ &&
        window.__NUXT__.config &&
        window.__NUXT__.config.public &&
        window.__NUXT__.config.public.pluginAdminBase) ||
      (window.__NUXT__ &&
        window.__NUXT__.config &&
        window.__NUXT__.config.app &&
        window.__NUXT__.config.app.baseURL) ||
      "";
    adminBase = String(adminBase || "").replace(/\/?$/, "/");

    var match = p.match(/^\/_p\/[^/]+\/admin(\/.*)?$/);
    if (match) {
      return p;
    }
    if (adminBase && adminBase.startsWith("/_p/") && !p.startsWith(adminBase)) {
      p = adminBase.replace(/\/$/, "") + p;
    }
    return p;
  }

  function applyNavigate(path) {
    try {
      var target = normalizeNavigatePath(path);
      var current = window.location.pathname + window.location.search + window.location.hash;
      if (target === current) return;

      window.history.pushState(window.history.state, "", target);
      try {
        window.dispatchEvent(new PopStateEvent("popstate", { state: window.history.state }));
      } catch (_ignore) {
        var evt = document.createEvent("Event");
        evt.initEvent("popstate", true, true);
        window.dispatchEvent(evt);
      }
    } catch (error) {
      console.warn("[PowerXBridgeClient] applyNavigate failed", error);
    }
  }

  function shouldInterceptAnchorClick(event, anchor) {
    if (!anchor) return false;
    if (event.defaultPrevented) return false;
    if (event.button !== 0) return false; // 仅拦截左键
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return false;
    if (anchor.target && anchor.target !== "_self") return false;
    if (anchor.hasAttribute("download")) return false;
    if (!anchor.href) return false;
    return true;
  }

  function installAnchorInterceptor() {
    document.addEventListener(
      "click",
      function (event) {
        try {
          var target = event.target;
          if (!target || !target.closest) return;
          var anchor = target.closest("a");
          if (!shouldInterceptAnchorClick(event, anchor)) return;

          var url = new URL(anchor.href, window.location.href);
          if (url.origin !== window.location.origin) return;
          if (!url.pathname.startsWith("/_p/")) return;
          if (!url.pathname.includes("/admin/")) return;

          var next = url.pathname + url.search + url.hash;
          var current = window.location.pathname + window.location.search + window.location.hash;
          if (next === current) return;

          event.preventDefault();
          applyNavigate(next);
        } catch (_ignore) {}
      },
      true
    );
  }

  function handleMessage(event) {
    if (!event || !event.data || event.data.source !== "powerx") {
      return;
    }

    if (event.data.type === "auth-token") {
      applyAuthPayload(event.data);
    } else if (event.data.type === "sync") {
      lastSyncAt = Date.now();
    } else if (event.data.type === "navigate") {
      applyNavigate(event.data.path);
    }
  }

  window.addEventListener("message", handleMessage);

  function requestSync() {
    window.parent &&
      window.parent.postMessage({ source: "plugin", type: "request-sync" }, "*");
  }

  function notifyReady() {
    window.parent &&
      window.parent.postMessage({ source: "plugin", type: "ready" }, "*");
  }

  bootstrapAuthFromCookies();
  installAnchorInterceptor();
  notifyReady();
  requestSync();

  window.PowerXBridgeClient = {
    onAuthToken: function (cb) {
      if (typeof cb === "function") {
        subscribers.add(cb);
      }
      return function () {
        subscribers.delete(cb);
      };
    },
    requestSync: requestSync,
    getLastSyncAt: function () {
      return lastSyncAt;
    },
  };
})();
