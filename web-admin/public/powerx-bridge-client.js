(function () {
  if (typeof window === "undefined") {
    return;
  }

  var subscribers = new Set();
  var lastSyncAt = 0;

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
      if (payload.ctx) {
        ls.setItem("px_ctx", payload.ctx);
        setCookie("px_ctx", payload.ctx);
      }
      if (payload.ctxSig) {
        ls.setItem("px_ctx_sig", payload.ctxSig);
        setCookie("px_ctx_sig", payload.ctxSig);
      }
      if (payload.ctxJwt) {
        ls.setItem("px_ctx_jwt", payload.ctxJwt);
        setCookie("px_ctx_jwt", payload.ctxJwt);
      }
      if (payload.tenant_uuid) {
        ls.setItem("px_current_tenant_uuid", payload.tenant_uuid);
        setCookie("px_current_tenant_uuid", payload.tenant_uuid);
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

  function handleMessage(event) {
    if (!event || !event.data || event.data.source !== "powerx") {
      return;
    }

    if (event.data.type === "auth-token") {
      applyAuthPayload(event.data);
    } else if (event.data.type === "sync") {
      lastSyncAt = Date.now();
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
