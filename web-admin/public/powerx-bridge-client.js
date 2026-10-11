(function () {
  if (typeof window === "undefined") {
    return;
  }

  var subscribers = new Set();
  var lastSyncAt = 0;

  var protocolVersion = 2;
  var instanceId = '';
  var sequence = 0;
  var navigatorAdapter = null;
  var navigationAbort = null;
  var lastAuthPayload = null;
  var pluginMatch = window.location.pathname.match(/\/_p\/([^/]+)\/admin(?:\/|$)/);
  var pluginId = pluginMatch ? pluginMatch[1] : '';
  var hostOrigin = '';
  try { hostOrigin = new URL(document.referrer).origin; } catch (_ignore) {}

  function send(payload) {
    if (!hostOrigin || window.parent === window) return;
    window.parent.postMessage(Object.assign({}, payload, {
      source: 'plugin', protocolVersion: protocolVersion, pluginId: pluginId, instanceId: instanceId
    }), hostOrigin);
  }

  function applyAuthPayload(payload) {
    if (!Number.isSafeInteger(payload.sequence) || payload.sequence <= sequence) return;
    sequence = payload.sequence;
    lastAuthPayload = payload;
    if (payload.type === 'auth-cleared' && navigationAbort) navigationAbort.abort();
    // This consumer never writes the host's authoritative session.
    consumeAuth(payload);
  }

  function consumeAuth(payload) {
    if (!subscribers.size) return;
    var bindingId = instanceId;
    Promise.all(Array.from(subscribers).map(function (cb) { return Promise.resolve().then(function () { return cb(payload); }); }))
      .then(function () {
        if (payload.type === 'auth-token' && lastAuthPayload === payload && bindingId === instanceId) {
          send({ type: 'auth:applied', sequence: payload.sequence, sessionId: payload.sessionId, revision: payload.revision });
        }
      }).catch(function () {
        if (lastAuthPayload === payload && bindingId === instanceId) send({ type: 'auth:rejected', sequence: payload.sequence });
      });
  }

  function normalizeNavigatePath(path) {
    if (typeof path !== "string") throw new Error('PLUGIN_NAVIGATION_INVALID');
    var p = path.trim();
    if (!p || /^[a-z][a-z0-9+.-]*:/i.test(p) || p.startsWith('//') || p.includes('\\')) {
      throw new Error('PLUGIN_NAVIGATION_INVALID');
    }
    if (!p.startsWith("/")) p = "/" + p;

    // Nuxt 水合后会删除 __NUXT__；挂载范围由 iframe 入口确认的插件 ID 决定。
    // 已带命名空间的地址交给下面的严格范围校验，不能重新包装跨插件地址。
    if (p.startsWith('/_p/')) return p;
    return '/_p/' + pluginId + '/admin' + p;
  }

  function comparablePath(path) {
    var url = new URL(path, window.location.origin);
    url.searchParams.delete('__px_iframe');
    url.searchParams.delete('__px_retry');
    return url.pathname.replace(/\/$/, '') + url.search + url.hash;
  }

  function applyNavigate(payload) {
    if (navigationAbort) navigationAbort.abort();
    navigationAbort = new AbortController();
    var signal = navigationAbort.signal;
    var bindingId = instanceId;
    var acknowledge = function (ok, code) {
      if (!signal.aborted && bindingId === instanceId) {
        var result = { type: 'navigation:result', requestId: payload.requestId, ok: ok };
        if (!ok) result.code = code;
        send(result);
      }
    };
    var target;
    try {
      if (typeof payload.path !== 'string' || typeof payload.requestId !== 'string' || !payload.requestId) throw new Error('PLUGIN_NAVIGATION_INVALID');
      target = normalizeNavigatePath(payload.path);
      var url = new URL(target, window.location.origin);
      var mount = '/_p/' + pluginId + '/admin';
      if (url.origin !== window.location.origin || !(url.pathname === mount || url.pathname.startsWith(mount + '/'))) throw new Error('PLUGIN_NAVIGATION_INVALID');
    } catch (_ignore) { acknowledge(false, 'PLUGIN_NAVIGATION_INVALID'); return; }
    if (comparablePath(target) === comparablePath(window.location.href)) { acknowledge(true); return; }
    if (!navigatorAdapter) { acknowledge(false, 'PLUGIN_NAVIGATION_UNSUPPORTED'); return; }
    Promise.resolve().then(function () { if (signal.aborted) return; return navigatorAdapter(target, { signal: signal, requestId: payload.requestId, instanceId: bindingId }); }).then(function () {
      var matches = comparablePath(target) === comparablePath(window.location.href);
      acknowledge(matches, 'PLUGIN_NAVIGATION_ROUTE_MISMATCH');
    }).catch(function () { acknowledge(false, 'PLUGIN_NAVIGATION_FAILED'); });
  }

  function handleMessage(event) {
    if (!event || !event.data || event.data.source !== 'powerx') return;
    var payload = event.data;
    if (event.source !== window.parent || !hostOrigin || event.origin !== hostOrigin ||
      payload.protocolVersion !== protocolVersion || payload.pluginId !== pluginId || !payload.instanceId) {
      event.stopImmediatePropagation(); return;
    }
    if (payload.type === 'sync') {
      if (instanceId !== payload.instanceId) {
        sequence = 0; lastAuthPayload = null;
        if (navigationAbort) navigationAbort.abort();
      }
      instanceId = payload.instanceId;
      lastSyncAt = Date.now();
      send({ type: 'bridge:ready', navigationReady: Boolean(navigatorAdapter) });
    } else if (payload.instanceId !== instanceId) {
      event.stopImmediatePropagation();
    } else if (payload.type === 'auth-token' || payload.type === 'auth-cleared') {
      if (!Number.isSafeInteger(payload.sequence) || payload.sequence <= sequence) {
        event.stopImmediatePropagation(); return;
      }
      applyAuthPayload(payload);
    } else if (payload.type === 'navigate') {
      applyNavigate(payload);
    }
  }

  window.addEventListener('message', handleMessage, true);
  send({ type: 'bridge:hello' });
  window.PowerXBridgeClient = {
    protocolVersion: protocolVersion,
    onAuthToken: function (cb) {
      if (typeof cb === 'function') {
        subscribers.add(cb);
        if (lastAuthPayload && (lastAuthPayload.type === 'auth-cleared' || lastAuthPayload.expiresAt > Date.now())) consumeAuth(lastAuthPayload);
        else send({ type: 'auth-token:request' });
      }
      return function () { subscribers.delete(cb); };
    },
    setNavigator: function (adapter) {
      if (typeof adapter !== 'function') throw new Error('PLUGIN_NAVIGATOR_REQUIRED');
      navigatorAdapter = adapter;
      send({ type: 'bridge:hello' });
      return function () { navigatorAdapter = null; };
    },
    requestSync: function () { send({ type: 'request-sync' }); },
    requestAuth: function () { send({ type: 'auth-token:request' }); },
    getLastSyncAt: function () { return lastSyncAt; },
  };
})();
