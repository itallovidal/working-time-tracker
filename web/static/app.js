// Working Time Tracker: utilitários compartilhados pelas páginas.
// Carregado com defer antes do Alpine; cada página registra seus componentes no evento alpine:init.
(function () {
  'use strict';

  class ApiError extends Error {
    constructor(message, status) {
      super(message);
      this.status = status;
    }
  }

  // api chama a API JSON do próprio servidor. Em erro, lança ApiError com a
  // mensagem que o servidor mandou em {"error": "..."}.
  async function api(method, path, body) {
    const opts = { method, credentials: 'same-origin', headers: { Accept: 'application/json' } };
    if (body !== undefined) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    let res;
    try {
      res = await fetch(path, opts);
    } catch (e) {
      throw new ApiError('Sem conexão com o servidor. Confira a rede e tente de novo.', 0);
    }
    if (res.status === 401 && !path.startsWith('/api/auth/')) {
      location.href = '/login?next=' + encodeURIComponent(location.pathname + location.search);
      throw new ApiError('Sua sessão expirou. Entre de novo.', 401);
    }
    if (res.status === 204) return null;
    const text = await res.text();
    let data = null;
    if (text) {
      try { data = JSON.parse(text); } catch (e) { data = null; }
    }
    if (!res.ok) {
      const message = (data && (data.error || data.message)) || 'O servidor respondeu com erro ' + res.status + '.';
      throw new ApiError(message, res.status);
    }
    return data;
  }

  // form é um mixin para componentes com formulários: guarda qual ação está em
  // andamento e a mensagem de erro de cada uma, para mostrar ao lado do formulário certo.
  function form() {
    return {
      pending: null,
      errors: {},
      async run(key, fn) {
        if (this.pending) return undefined;
        this.pending = key;
        this.errors[key] = '';
        try {
          return await fn();
        } catch (e) {
          this.errors[key] = e && e.message ? e.message : String(e);
          return undefined;
        } finally {
          this.pending = null;
        }
      },
    };
  }

  const pad = (n) => String(n).padStart(2, '0');

  const fmt = {
    // 3725 -> "01:02:05"
    clock(seconds) {
      const s = Math.max(0, Math.floor(seconds || 0));
      return pad(Math.floor(s / 3600)) + ':' + pad(Math.floor((s % 3600) / 60)) + ':' + pad(s % 60);
    },
    // 3725 -> "1h 02min"
    hours(seconds) {
      const s = Math.max(0, Math.round(seconds || 0));
      const h = Math.floor(s / 3600);
      const m = Math.floor((s % 3600) / 60);
      if (h === 0) return m + 'min';
      return h + 'h ' + pad(m) + 'min';
    },
    date(iso) {
      if (!iso) return '';
      return new Date(iso).toLocaleDateString('pt-BR', { day: '2-digit', month: 'short', year: 'numeric' });
    },
    dateTime(iso) {
      if (!iso) return '';
      return new Date(iso).toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
    },
    time(iso) {
      if (!iso) return '';
      return new Date(iso).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
    },
    // ISO -> "2026-09-28" no fuso local, para <input type="date">
    dateInput(iso) {
      if (!iso) return '';
      const d = new Date(iso);
      return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate());
    },
    // "2026-09-28" -> ISO no fim daquele dia, no fuso local
    fromDateInput(value) {
      if (!value) return null;
      return new Date(value + 'T23:59:00').toISOString();
    },
    initials(name) {
      return (name || '?').trim().split(/\s+/).slice(0, 2).map((p) => p[0].toUpperCase()).join('');
    },
    role(role) {
      return role === 'admin' ? 'Admin' : 'Membro';
    },
  };

  async function copyText(text) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch (e) {
      return false;
    }
  }

  window.WTT = { api, ApiError, form, fmt, copyText, boot: window.BOOT || {} };

  document.addEventListener('alpine:init', () => {
    Alpine.store('toast', {
      items: [],
      seq: 0,
      show(message, kind) {
        const id = ++this.seq;
        this.items.push({ id, message, kind: kind || 'info' });
        setTimeout(() => this.dismiss(id), kind === 'error' ? 6000 : 3500);
      },
      error(message) {
        this.show(message, 'error');
      },
      dismiss(id) {
        this.items = this.items.filter((t) => t.id !== id);
      },
    });

    Alpine.data('logoutButton', () => ({
      busy: false,
      async logout() {
        this.busy = true;
        try {
          await api('POST', '/api/auth/logout');
        } finally {
          location.href = '/login';
        }
      },
    }));
  });
})();
