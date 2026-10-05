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

  const weekdays = [
    { value: 'monday', label: 'Segunda-feira' },
    { value: 'tuesday', label: 'Terça-feira' },
    { value: 'wednesday', label: 'Quarta-feira' },
    { value: 'thursday', label: 'Quinta-feira' },
    { value: 'friday', label: 'Sexta-feira' },
    { value: 'saturday', label: 'Sábado' },
    { value: 'sunday', label: 'Domingo' },
  ];

  const orgSizes = [
    { value: '1-10', label: '1 a 10 pessoas' },
    { value: '11-50', label: '11 a 50 pessoas' },
    { value: '51-200', label: '51 a 200 pessoas' },
    { value: '201-500', label: '201 a 500 pessoas' },
    { value: '500+', label: 'Mais de 500 pessoas' },
  ];

  // As moedas que o backend aceita (internal/domain/organization).
  const currencies = [
    { value: 'BRL', label: 'Real (BRL)' },
    { value: 'USD', label: 'Dólar americano (USD)' },
    { value: 'EUR', label: 'Euro (EUR)' },
  ];

  const fmt = {
    // 3725 -> "01:02:05"
    clock(seconds) {
      const s = Math.max(0, Math.floor(seconds || 0));
      return pad(Math.floor(s / 3600)) + ':' + pad(Math.floor((s % 3600) / 60)) + ':' + pad(s % 60);
    },
    // 3725 -> "1h 02min"; 42 -> "42s"
    hours(seconds) {
      const s = Math.max(0, Math.floor(seconds || 0));
      const h = Math.floor(s / 3600);
      const m = Math.floor((s % 3600) / 60);
      if (h === 0 && m === 0) return s + 's';
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
    weekday(value) {
      const d = weekdays.find((w) => w.value === value);
      return d ? d.label : value;
    },
    // "11222333000181" -> "11.222.333/0001-81"
    cnpj(value) {
      const v = value || '';
      if (v.length !== 14) return v;
      return v.slice(0, 2) + '.' + v.slice(2, 5) + '.' + v.slice(5, 8) + '/' + v.slice(8, 12) + '-' + v.slice(12);
    },
    orgSize(value) {
      const s = orgSizes.find((x) => x.value === value);
      return s ? s.label : value;
    },
    currency(value) {
      const c = currencies.find((x) => x.value === value);
      return c ? c.label : value;
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

  // Tipos de integração e os campos de credencial de cada um. Um tipo novo no
  // backend (internal/adapter) precisa de uma entrada aqui para aparecer na tela.
  const integrationTypes = [
    {
      value: 'github',
      label: 'GitHub',
      fields: [
        { key: 'token', label: 'Token de acesso', type: 'password', hint: 'Token pessoal com permissão de leitura de issues.' },
        { key: 'repo', label: 'Repositório', type: 'text', placeholder: 'dono/repositorio' },
      ],
    },
    {
      value: 'gitlab',
      label: 'GitLab',
      fields: [
        { key: 'token', label: 'Token de acesso', type: 'password', hint: 'Token com escopo read_api.' },
        { key: 'project_url', label: 'Projeto', type: 'text', placeholder: 'grupo/projeto ou https://gitlab.com/grupo/projeto' },
      ],
    },
  ];

  window.WTT = { api, ApiError, form, fmt, copyText, weekdays, orgSizes, currencies, integrationTypes, boot: window.BOOT || {} };

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

    // clock guarda a sessão de trabalho aberta da pessoa logada e um relógio que
    // avança a cada segundo. O indicador do cabeçalho e a tela de ponto leem daqui.
    Alpine.store('clock', {
      session: null,
      now: Date.now(),
      ready: false,
      init() {
        if (!window.WTT.boot.me) return;
        this.refresh();
        setInterval(() => { this.now = Date.now(); }, 1000);
      },
      elapsed(session) {
        const s = session || this.session;
        if (!s) return 0;
        const end = s.end_at ? new Date(s.end_at).getTime() : this.now;
        return Math.max(0, (end - new Date(s.start_at).getTime()) / 1000);
      },
      async refresh() {
        try {
          this.session = await api('GET', '/api/work-sessions/active');
        } catch (e) {
          // O cabeçalho só deixa de mostrar o indicador; a tela de ponto mostra o erro.
        } finally {
          this.ready = true;
        }
      },
      async clockIn(projectId, taskId) {
        await api('POST', '/api/projects/' + projectId + '/work-sessions/clock-in', { task_id: taskId });
        await this.refresh();
        window.dispatchEvent(new CustomEvent('wtt:sessions-changed'));
      },
      async clockOut() {
        if (!this.session) return;
        await api('POST', '/api/projects/' + this.session.task.project_id + '/work-sessions/clock-out', {});
        this.session = null;
        window.dispatchEvent(new CustomEvent('wtt:sessions-changed'));
      },
    });

    Alpine.data('activeSession', () => ({
      busy: false,
      async stop() {
        this.busy = true;
        try {
          await Alpine.store('clock').clockOut();
          Alpine.store('toast').show('Ponto encerrado.');
        } catch (e) {
          Alpine.store('toast').error(e.message);
        } finally {
          this.busy = false;
        }
      },
    }));

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
