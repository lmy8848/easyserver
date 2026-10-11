// EasyServer 插件 Host SDK（文档 §9.3）
// 插件前端页面（iframe 内）通过本 SDK 与面板主前端通信；所有需要权限的
// 操作由主前端代理执行，iframe 不持有 plugin_token。
(function () {
  'use strict';
  if (window.EasyServerPlugin) return; // 防止重复注入

  var seq = 0;
  var pending = {};

  function send(type, payload) {
    return new Promise(function (resolve, reject) {
      var id = 'sdk-' + Date.now() + '-' + (++seq);
      pending[id] = { resolve: resolve, reject: reject };
      window.parent.postMessage(
        { __esplugin: true, sdk: true, id: id, type: type, payload: payload },
        window.location.origin
      );
      // 超时保护
      setTimeout(function () {
        if (pending[id]) {
          delete pending[id];
          reject(new Error('Host SDK 请求超时: ' + type));
        }
      }, 15000);
    });
  }

  window.addEventListener('message', function (e) {
    var data = e.data;
    if (!data || !data.__esplugin_reply || !data.id || !pending[data.id]) return;
    var p = pending[data.id];
    delete pending[data.id];
    if (data.err) {
      p.reject(new Error(data.err));
    } else {
      p.resolve(data.result);
    }
  });

  window.EasyServerPlugin = {
    /** 面板内导航 */
    navigate: function (path) {
      return send('navigate', { path: String(path) });
    },
    /** 发送面板通知 */
    notify: function (title, content) {
      return send('notify', { title: title, content: content });
    },
    /** 受限 GET 请求（经主前端代理，带面板登录态） */
    request: function (path) {
      return send('request', { method: 'GET', url: path });
    },
  };
})();
