// Working Time Tracker: utilitários compartilhados pelas páginas.
// Carregado com defer antes do Alpine; cada página registra seus componentes no evento alpine:init.
(function () {
  'use strict';

  // ---------- Textos ----------
  // window.I18N vem de /i18n/<idioma>.js (o mesmo YAML do servidor). Uma chave é
  // um texto ou, quando tem plural, um objeto com as formas one/other.
  const i18n = window.I18N || { lang: document.documentElement.lang || 'pt-BR', messages: {} };
  const lang = i18n.lang;
  const pluralRules = new Intl.PluralRules(lang);

  function lookup(key) {
    let node = i18n.messages;
    for (const part of key.split('.')) {
      if (node === null || typeof node !== 'object' || !(part in node)) return undefined;
      node = node[part];
    }
    return node;
  }

  // t traduz uma chave. Os placeholders são {{.nome}}; o parâmetro count também
  // escolhe a forma do plural: t('x', { count: 3 }). Chave que não existe volta como está.
  function t(key, params) {
    let text = lookup(key);
    if (text !== null && typeof text === 'object') {
      text = text[pluralRules.select(Number(params && params.count))] ?? text.other;
    }
    if (typeof text !== 'string') {
      console.warn('i18n: chave sem texto:', key);
      return key;
    }
    return text.replace(/\{\{\s*\.(\w+)\s*\}\}/g, (match, name) => (params && params[name] !== undefined ? params[name] : match));
  }

  // errorText devolve a mensagem de um erro da API, que chega como {code, params}. O
  // texto é errors.<código> no catálogo. O parâmetro field, quando existe, é o
  // nome de um campo da API, e o rótulo dele é fields.<campo>.
  function errorText(err) {
    const params = { ...(err && err.params) };
    if (params.field) params.field = t('fields.' + params.field);
    const key = 'errors.' + (err && err.code);
    if (lookup(key) === undefined) {
      console.warn('i18n: código de erro sem texto:', err && err.code);
      return t('errors.unknown');
    }
    return t(key, params);
  }

  // ApiError é o erro de uma chamada à API. message já vem no idioma da pessoa; code
  // e params são os que o servidor mandou, para quem precisa decidir pelo código.
  class ApiError extends Error {
    constructor(message, status, code, params) {
      super(message);
      this.status = status;
      this.code = code;
      this.params = params;
    }
  }

  // api chama a API JSON do próprio servidor (headers são cabeçalhos a mais, como o Authorization do login
  // pelo Clerk). Em erro, lança ApiError: o servidor
  // manda só um código, em {"error": {"code": "...", "params": {...}}}, e a mensagem
  // é montada aqui, no idioma da página.
  async function api(method, path, body, headers) {
    const opts = { method, credentials: 'same-origin', headers: { Accept: 'application/json', ...headers } };
    if (body !== undefined) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    let res;
    try {
      res = await fetch(path, opts);
    } catch (e) {
      throw new ApiError(t('errors.no_connection'), 0);
    }
    if (res.status === 401 && !path.startsWith('/api/auth/')) {
      location.href = '/login?next=' + encodeURIComponent(location.pathname + location.search);
      throw new ApiError(t('errors.session_expired'), 401);
    }
    if (res.status === 204) return null;
    const text = await res.text();
    let data = null;
    if (text) {
      try { data = JSON.parse(text); } catch (e) { data = null; }
    }
    if (!res.ok) {
      const err = data && data.error;
      if (err && typeof err === 'object' && err.code) throw new ApiError(errorText(err), res.status, err.code, err.params);
      throw new ApiError(t('errors.http_status', { status: res.status }), res.status);
    }
    return data;
  }

  // limits são os tamanhos máximos que o servidor confere (internal/validate/limits.go), vindos em BOOT.limits. O
  // maxlength dos templates sai da função `limit`; aqui ficam os que o JavaScript confere antes de enviar.
  const limits = (window.BOOT && window.BOOT.limits) || {};

  // aliases ligam o nome de um campo que o servidor devolve ao campo da tela quando os dois não coincidem.
  const fieldAliases = { long_description: 'description' };

  // rules são as conferências de campo da tela, as mesmas do servidor. Cada regra recebe o valor e o nome do campo
  // e devolve o texto do erro (o mesmo errors.request.field_* do servidor) ou '' quando está certo. Campo vazio só
  // erra em required: o resto confere o que veio preenchido.
  const rules = (() => {
    const msg = (code, field, params) => errorText({ code: 'request.' + code, params: { field, ...params } });
    const text = (v) => String(v === null || v === undefined ? '' : v).trim();
    const chars = (v) => Array.from(v).length;
    return {
      required: (v, field) => (text(v) === '' ? msg('field_required', field) : ''),
      // max(n) aceita o tamanho em número ou o nome de um limite: rules.max('name').
      max: (n) => (v, field) => {
        const max = typeof n === 'string' ? limits[n] : n;
        return chars(text(v)) > max ? msg('field_too_long', field, { max }) : '';
      },
      email: (v, field) => {
        const x = text(v);
        return x === '' || /^[^\s@]+@[^\s@.]+(\.[^\s@.]+)+$/.test(x) ? '' : msg('field_invalid', field);
      },
      // url aceita o que o servidor aceita: http ou https, com o esquema opcional e um host com ponto.
      url: (v, field) => {
        const x = text(v);
        if (x === '') return '';
        if (/[\s\u0000-\u001f]/.test(x)) return msg('field_invalid', field);
        try {
          const u = new URL(x.includes('://') ? x : 'https://' + x);
          const ok = (u.protocol === 'http:' || u.protocol === 'https:') && !u.username && u.hostname.includes('.')
            && !u.hostname.startsWith('.') && !u.hostname.endsWith('.');
          return ok ? '' : msg('field_invalid', field);
        } catch (e) {
          return msg('field_invalid', field);
        }
      },
      phone: (v, field) => {
        const x = text(v);
        if (x === '') return '';
        const digits = (x.match(/\d/g) || []).length;
        return /^[0-9+() .-]{8,32}$/.test(x) && digits >= (limits.phone_min_digits || 7) ? '' : msg('field_invalid', field);
      },
      // money confere um valor digitado: só dígitos e separador, de 0 até o teto do servidor.
      money: (v, field) => {
        if (text(v) === '') return '';
        const cents = toCents(v, { strict: true });
        return cents === null || cents < 0 || cents > (limits.max_cents || 100000000) ? msg('field_invalid', field) : '';
      },
      // integer(min, max) confere um número inteiro, escrito só com dígitos.
      integer: (min, max) => (v, field) => {
        const x = text(v);
        if (x === '') return '';
        const n = Number(x);
        return /^\d+$/.test(x) && n >= min && n <= max ? '' : msg('field_invalid', field);
      },
      date: (v, field) => (text(v) === '' || /^\d{4}-\d{2}-\d{2}$/.test(text(v)) ? '' : msg('field_invalid', field)),
      time: (v, field) => (text(v) === '' || /^([01]\d|2[0-3]):[0-5]\d$/.test(text(v)) ? '' : msg('field_invalid', field)),
    };
  })();

  // form é um mixin para componentes com formulários: guarda qual ação está em andamento, a mensagem de erro de
  // cada uma (errors, o aviso do formulário) e o erro de cada campo (fieldErrors, embaixo do campo certo).
  //
  // O erro da API que diz o campo (params.field) vai para o campo, e só o que não diz fica no aviso do
  // formulário. check() confere os campos antes de enviar com as mesmas regras do servidor, mostra todos os erros
  // e leva o foco ao primeiro. Quem escreve o campo no template põe data-field="nome", @input="clearField('nome')"
  // e o partial field_error.
  function form() {
    return {
      pending: null,
      errors: {},
      fieldErrors: {},
      setField(name, message) {
        this.fieldErrors = { ...this.fieldErrors, [fieldAliases[name] || name]: message };
      },
      clearField(name) {
        if (!this.fieldErrors[name]) return;
        const next = { ...this.fieldErrors };
        delete next[name];
        this.fieldErrors = next;
      },
      clearFields() {
        this.fieldErrors = {};
      },
      // focusField leva o foco ao campo `name` que está à vista (o mesmo nome pode existir em outro modal).
      fieldEl(name) {
        return Array.from(document.querySelectorAll('[data-field="' + name + '"]')).find((e) => e.offsetParent !== null) || null;
      },
      focusField(name) {
        const el = this.fieldEl(name);
        if (el) el.focus();
      },
      // check confere os campos: { nome: [valor, regra, regra...] }. Devolve true quando todos estão certos. A
      // ordem das chaves é a ordem dos campos na tela, e o primeiro com erro recebe o foco.
      check(spec) {
        const found = {};
        for (const [name, [value, ...list]] of Object.entries(spec)) {
          for (const rule of list) {
            const message = rule(value, name);
            if (message) {
              found[name] = message;
              break;
            }
          }
        }
        this.fieldErrors = found;
        const first = Object.keys(found)[0];
        if (first) this.$nextTick(() => this.focusField(first));
        return first === undefined;
      },
      async run(key, fn) {
        if (this.pending) return undefined;
        this.pending = key;
        this.errors[key] = '';
        try {
          return await fn();
        } catch (e) {
          // Um erro que diz o campo vai para o campo, quando a tela o desenha (data-field); senão fica no aviso.
          const field = e && e.params && e.params.field;
          const name = field && (fieldAliases[field] || field);
          if (name && e.code && this.fieldEl(name)) {
            this.setField(name, e.message);
            this.focusField(name);
          } else {
            this.errors[key] = e && e.message ? e.message : String(e);
          }
          return undefined;
        } finally {
          this.pending = null;
        }
      },
    };
  }

  const pad = (n) => String(n).padStart(2, '0');

  // Em todas as listas de rótulos o value é o código do backend e só o label é traduzido.
  const weekdays = [
    { value: 'monday', label: t('labels.weekday.monday') },
    { value: 'tuesday', label: t('labels.weekday.tuesday') },
    { value: 'wednesday', label: t('labels.weekday.wednesday') },
    { value: 'thursday', label: t('labels.weekday.thursday') },
    { value: 'friday', label: t('labels.weekday.friday') },
    { value: 'saturday', label: t('labels.weekday.saturday') },
    { value: 'sunday', label: t('labels.weekday.sunday') },
  ];

  // markdown transforma o texto de uma descrição em HTML seguro para x-html. O marked
  // gera o HTML e o DOMPurify é a única defesa contra XSS: só as tags da lista passam,
  // sem imagem nem HTML cru, e os links só vão para http, https e mailto, abrem em outra
  // aba e não passam o referenciador. Uma imagem vira o texto alternativo. Sem as
  // bibliotecas (a página não as carregou, ou falharam), devolve o texto escapado.
  const escapeHTML = (s) => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  const markdownTags = ['p', 'br', 'strong', 'em', 'del', 'code', 'pre', 'blockquote', 'ul', 'ol', 'li', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'a', 'hr', 'table', 'thead', 'tbody', 'tr', 'th', 'td'];
  let markdownReady = false;
  function markdown(text) {
    const source = String(text || '');
    if (!source.trim()) return '';
    if (!window.marked || !window.DOMPurify) return escapeHTML(source);
    if (!markdownReady) {
      markdownReady = true;
      window.DOMPurify.addHook('afterSanitizeAttributes', (node) => {
        if (node.tagName === 'A') {
          node.setAttribute('target', '_blank');
          node.setAttribute('rel', 'noopener noreferrer');
        }
      });
    }
    const renderer = new window.marked.Renderer();
    renderer.image = (token) => escapeHTML(token.text || '');
    const html = window.marked.parse(source, { gfm: true, breaks: true, async: false, renderer });
    return window.DOMPurify.sanitize(html, {
      ALLOWED_TAGS: markdownTags,
      ALLOWED_ATTR: ['href', 'title', 'align'],
      ALLOWED_URI_REGEXP: /^(?:https?|mailto):/i,
      ALLOW_DATA_ATTR: false,
    });
  }

  // As prioridades de uma tarefa, da mais para a menos urgente. O value é o código do backend.
  const priorities = ['urgent', 'high', 'medium', 'low', 'none'].map((value) => ({ value, label: t('tasks.priority.' + value) }));

  // Os status de uma tarefa, na ordem em que ela costuma passar por eles. O value é o código do backend.
  const taskStatuses = ['backlog', 'in_progress', 'awaiting_closure', 'closed'].map((value) => ({ value, label: t('tasks.status.' + value) }));

  // Os regimes de trabalho que o backend aceita (internal/domain/organization).
  const workModes = [
    { value: 'remote', label: t('labels.work_mode.remote') },
    { value: 'hybrid', label: t('labels.work_mode.hybrid') },
    { value: 'onsite', label: t('labels.work_mode.onsite') },
  ];

  // As moedas que o backend aceita (internal/domain/organization).
  const currencies = [
    { value: 'BRL', label: t('labels.currency.BRL') },
    { value: 'USD', label: t('labels.currency.USD') },
    { value: 'EUR', label: t('labels.currency.EUR') },
  ];

  // countries lê o cadastro de países que o servidor manda em BOOT.countries (internal/country): o documento fiscal, o
  // código postal, os estados e os textos de cada país, no idioma da página. A tela se desenha a partir dele, então um
  // país novo no servidor aparece sem mexer aqui. Os nomes dos países vêm do navegador (Intl.DisplayNames).
  const countries = (() => {
    const empty = { list: [], generic: { legal_id: { label: '' } }, iso: [] };
    const registry = () => (window.BOOT && window.BOOT.countries) || empty;
    let displayNames = null;
    try { displayNames = new Intl.DisplayNames([lang], { type: 'region' }); } catch (e) { /* o código serve de nome */ }
    const name = (code) => {
      try { return (displayNames && displayNames.of(code)) || code; } catch (e) { return code; }
    };
    const get = (code) => registry().list.find((c) => c.code === code) || null;
    return {
      list: () => registry().list,
      get,
      name,
      // forLang sugere o país de um idioma da interface ("pt-BR" -> BR, "en" -> US); sem correspondência, o primeiro.
      forLang(l) {
        const tag = String(l || lang).toLowerCase().split(/[-_]/)[0];
        const hit = registry().list.find((c) => c.langs.includes(tag));
        return hit ? hit.code : (registry().list[0] ? registry().list[0].code : 'BR');
      },
      // legalId é o documento fiscal do país. Fora do cadastro não há regra: é um texto livre, sem máscara.
      legalId(code) {
        const c = get(code);
        return c ? c.legal_id : { field: null, label: registry().generic.legal_id.label, placeholder: '', mask: '' };
      },
      // others são os países ISO fora do cadastro, por nome: o país de um cliente pode ser qualquer um.
      others() {
        const known = new Set(registry().list.map((c) => c.code));
        return registry().iso.filter((c) => !known.has(c)).map((code) => ({ code, name: name(code) }))
          .sort((a, b) => a.name.localeCompare(b.name, lang));
      },
    };
  })();

  // Separadores de número do idioma, para ler o que a pessoa digita em um campo de valor.
  const numberParts = new Intl.NumberFormat(lang).formatToParts(1234.5);
  const groupSeparator = (numberParts.find((p) => p.type === 'group') || {}).value || ',';

  // As durações de sprint que as telas oferecem. A API aceita de 1 a 90 dias, e um
  // projeto que já tem outra duração continua com ela (sprintChoices).
  const sprintOptions = [7, 14, 30].map((days) => ({
    value: days,
    label: t('labels.sprint.long_' + days),
    short: t('labels.sprint.short_' + days),
  }));

  // sprintChoices são as opções do campo de duração da sprint. Quando o projeto
  // tem uma duração fora da lista, ela entra como opção, para salvar não a trocar.
  function sprintChoices(current) {
    if (!current || sprintOptions.some((o) => o.value === current)) return sprintOptions;
    return [...sprintOptions, { value: current, label: fmt.sprint(current) }].sort((a, b) => a.value - b.value);
  }

  // A daily, a weekly e a reunião com o cliente são decisões do projeto: ele pode ter
  // qualquer uma, todas ou nenhuma. O formulário guarda isso numa marca (has_daily,
  // has_weekly, has_meeting) e mantém o horário digitado quando a marca é desligada,
  // para religar sem perder o que estava lá. A reunião é com o cliente: sem cliente
  // escolhido no rascunho (customer_id) ela não vai.
  const routine = {
    blank: () => ({
      has_daily: false, daily_time: '', has_weekly: false, weekly_sync_day: '', weekly_sync_time: '',
      has_meeting: false, customer_meeting_day: '', customer_meeting_time: '',
    }),
    fromProject: (p) => ({
      has_daily: !!p.daily_time, daily_time: p.daily_time || '',
      has_weekly: !!p.weekly_sync_day, weekly_sync_day: p.weekly_sync_day || '', weekly_sync_time: p.weekly_sync_time || '',
      has_meeting: !!p.customer_meeting_day, customer_meeting_day: p.customer_meeting_day || '', customer_meeting_time: p.customer_meeting_time || '',
    }),
    // As conferências do rascunho para WTT.form().check, na ordem da tela: o dia e o horário ligados (a marca) são
    // obrigatórios, e a reunião com o cliente só vale com cliente escolhido (o servidor recusa sem ele).
    rules: (d) => {
      const on = (flag, ...list) => (flag ? list : []);
      const meeting = !!(d.has_meeting && d.customer_id);
      return {
        customer_meeting_day: [d.customer_meeting_day, ...on(meeting, rules.required)],
        customer_meeting_time: [d.customer_meeting_time, ...on(meeting, rules.required), rules.time],
        daily_time: [d.daily_time, ...on(d.has_daily, rules.required), rules.time],
        weekly_sync_day: [d.weekly_sync_day, ...on(d.has_weekly, rules.required)],
        weekly_sync_time: [d.weekly_sync_time, ...on(d.has_weekly, rules.required), rules.time],
      };
    },
    // Texto vazio é o que apaga na API; o horário de cada dia sai junto com ele.
    payload: (d) => {
      const meeting = d.has_meeting && d.customer_id;
      return {
        daily_time: d.has_daily ? d.daily_time : '',
        weekly_sync_day: d.has_weekly ? d.weekly_sync_day : '',
        weekly_sync_time: d.has_weekly ? d.weekly_sync_time : '',
        customer_meeting_day: meeting ? d.customer_meeting_day : '',
        customer_meeting_time: meeting ? d.customer_meeting_time : '',
      };
    },
  };

  const fmt = {
    // 3725 -> "01:02:05"
    clock(seconds) {
      const s = Math.max(0, Math.floor(seconds || 0));
      return pad(Math.floor(s / 3600)) + ':' + pad(Math.floor((s % 3600) / 60)) + ':' + pad(s % 60);
    },
    // 9000 -> "2h30"; 3600 -> "1h"; 2700 -> "45min"; menos de um minuto, vazio. Para o rótulo curto de uma barra.
    hoursShort(seconds) {
      const m = Math.round(Math.max(0, seconds || 0) / 60);
      if (m < 1) return '';
      if (m < 60) return m + 'min';
      return Math.floor(m / 60) + 'h' + (m % 60 ? pad(m % 60) : '');
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
      return new Date(iso).toLocaleDateString(lang, { day: '2-digit', month: 'short', year: 'numeric' });
    },
    dateTime(iso) {
      if (!iso) return '';
      return new Date(iso).toLocaleString(lang, { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
    },
    time(iso) {
      if (!iso) return '';
      return new Date(iso).toLocaleTimeString(lang, { hour: '2-digit', minute: '2-digit' });
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
    // ISO -> "2026-09-28T14:30" no fuso local, para <input type="datetime-local">
    dateTimeInput(iso) {
      if (!iso) return '';
      const d = new Date(iso);
      return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + 'T' + pad(d.getHours()) + ':' + pad(d.getMinutes());
    },
    // "2026-09-28T14:30" -> ISO; vazio é null
    fromDateTimeInput(value) {
      return value ? new Date(value).toISOString() : null;
    },
    initials(name) {
      return (name || '?').trim().split(/\s+/).slice(0, 2).map((p) => p[0].toUpperCase()).join('');
    },
    role(role) {
      return t(role === 'admin' ? 'roles.admin' : 'roles.member');
    },
    priority(value) {
      const p = priorities.find((x) => x.value === value);
      return p ? p.label : value;
    },
    taskStatus(value) {
      const s = taskStatuses.find((x) => x.value === value);
      return s ? s.label : value;
    },
    weekday(value) {
      const d = weekdays.find((w) => w.value === value);
      return d ? d.label : value;
    },
    // ("friday", "14:00") -> "Sexta às 14:00"; weekly antiga, sem horário, fica só com o dia
    weeklySlot(day, time) {
      return time ? t('labels.weekly_slot', { day: fmt.weekday(day), time }) : fmt.weekday(day);
    },
    // 14 -> "14 dias"; 30 -> "1 mês"
    sprint(days) {
      const o = sprintOptions.find((x) => x.value === days);
      return o ? o.short : t('labels.sprint.days', { count: days });
    },
    // 40 -> "40h por semana"; sem jornada, texto vazio
    weeklyHours(hours) {
      return hours ? t('labels.weekly_hours', { hours }) : '';
    },
    // "2026-10-05" -> "05 de out. de 2026": um dia de calendário, formatado sem passar por fuso. Não use fmt.date para
    // isso: o navegador converte o instante para o fuso dele e desloca o dia.
    day(value, opts) {
      if (!value) return '';
      const [y, m, d] = String(value).slice(0, 10).split('-').map(Number);
      return new Date(y, m - 1, d, 12).toLocaleDateString(lang, opts || { day: '2-digit', month: 'short', year: 'numeric' });
    },
    // {frequency: 'monthly', day: 5} -> "Mensal · dia 5"; sem regra, texto vazio
    paymentRule(rule) {
      if (!rule || !rule.frequency) return '';
      if (rule.frequency === 'monthly') return t('labels.payment_monthly', { day: rule.day });
      return t('labels.payment_biweekly', { date: fmt.day(rule.start, { day: '2-digit', month: '2-digit' }) });
    },
    // mask aplica uma máscara do cadastro de países: 9 é um dígito, * é um dígito ou uma letra, o resto é literal.
    // "11222333000181" com "**.***.***/****-99" -> "11.222.333/0001-81". Vale também enquanto a pessoa digita: o que
    // ainda não tem valor fica de fora (sem hífen sobrando), e o que passa do formato é cortado.
    mask(value, pattern) {
      const text = value === null || value === undefined ? '' : String(value);
      if (!pattern) return text;
      const raw = text.toUpperCase().replace(/[^0-9A-Z]/g, '');
      let out = '';
      let i = 0;
      for (const p of pattern) {
        if (i >= raw.length) break;
        if (p === '9' || p === '*') {
          if (p === '9' && !/[0-9]/.test(raw[i])) break;
          out += raw[i++];
        } else {
          out += p;
        }
      }
      return out;
    },
    // taxId mostra o documento fiscal pela máscara do país; um país sem regra mostra o texto como veio.
    taxId(value, code) {
      const id = countries.legalId(code);
      return id.mask ? fmt.mask(value, id.mask) : (value || '');
    },
    currency(value) {
      const c = currencies.find((x) => x.value === value);
      return c ? c.label : value;
    },
    workMode(value) {
      const m = workModes.find((x) => x.value === value);
      return m ? m.label : value;
    },
    // Centavos na moeda da organização: 2050 -> "R$ 20,50". Sem valor, um travessão.
    money(cents) {
      if (cents === null || cents === undefined) return '—';
      return new Intl.NumberFormat(lang, { style: 'currency', currency: orgCurrency() }).format(cents / 100);
    },
    // Centavos para um campo de texto, com o separador do idioma: 2050 -> "20,50" (pt-BR) ou "20.50" (en)
    moneyInput(cents) {
      if (cents === null || cents === undefined) return '';
      return new Intl.NumberFormat(lang, { minimumFractionDigits: 2, maximumFractionDigits: 2, useGrouping: false }).format(cents / 100);
    },
    // O símbolo da moeda da organização, para o rótulo dos campos: "R$"
    moneyUnit() {
      const parts = new Intl.NumberFormat(lang, { style: 'currency', currency: orgCurrency() }).formatToParts(0);
      const symbol = parts.find((p) => p.type === 'currency');
      return symbol ? symbol.value : orgCurrency();
    },
  };

  function orgCurrency() {
    const me = window.BOOT && window.BOOT.me;
    return (me && me.organization_currency) || 'BRL';
  }

  // toCents lê um valor digitado e devolve os centavos, ou null quando o texto
  // não é um valor. Aceita "20", "20,5", "20.50", "1.234,56" e "1,234.56". Com os dois
  // separadores, o último é o decimal. Com um só: repetido ou seguido de três dígitos
  // é milhar quando for o separador de milhar do idioma ("1.234" em pt-BR, "1,234" em
  // en); nos outros casos é decimal.
  //
  // No modo estrito ({ strict: true }) só dígitos, ponto, vírgula e espaços valem: "-20", "12abc" e "1e3" são
  // recusados em vez de lidos como 20, 12 e 13.
  function toCents(text, opts) {
    const raw = String(text === null || text === undefined ? '' : text).trim();
    if (opts && opts.strict && !/^[\d.,\s]*$/.test(raw)) return null;
    let v = raw.replace(/[^\d.,]/g, '');
    if (v === '') return null;
    const dot = v.lastIndexOf('.');
    const comma = v.lastIndexOf(',');
    let decimal = null;
    if (dot >= 0 && comma >= 0) {
      decimal = dot > comma ? '.' : ',';
    } else if (dot >= 0 || comma >= 0) {
      const sep = dot >= 0 ? '.' : ',';
      const repeated = v.split(sep).length > 2;
      const thousands = v.length - v.lastIndexOf(sep) - 1 === 3 && sep === groupSeparator;
      if (!repeated && !thousands) decimal = sep;
    }
    if (decimal) {
      const i = v.lastIndexOf(decimal);
      v = v.slice(0, i).replace(/[.,]/g, '') + '.' + v.slice(i + 1).replace(/[.,]/g, '');
    } else {
      v = v.replace(/[.,]/g, '');
    }
    const n = Number(v);
    return Number.isFinite(n) ? Math.round(n * 100) : null;
  }

  async function copyText(text) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch (e) {
      return false;
    }
  }

  // O que as telas mostram no lugar de um campo de cadastro sem valor.
  const notInformed = t('labels.not_informed');

  // can diz se quem está logado pode o que a permissão libera, no projeto da página (as do
  // projeto) ou na organização. O servidor confere de novo em toda rota: isto só esconde o
  // que a pessoa não pode usar.
  const granted = (window.BOOT && window.BOOT.can) || [];
  const can = (key) => granted.includes(key);

  // priorityClass e statusClass dão a cor de uma prioridade e de um status (`.tone` e a variante,
  // em app.css), no selo, no chip do filtro e no select. Uma tarefa sem valor conhecido cai em
  // "sem prioridade" e em "backlog", os padrões da API.
  const priorityClass = (p) => 'tone prio-' + (['urgent', 'high', 'medium', 'low'].includes(p) ? p : 'none');
  const statusClass = (s) => 'tone status-' + (['in_progress', 'awaiting_closure', 'closed'].includes(s) ? s : 'backlog');

  // Uma tarefa sem prazo guarda o tempo zero do Go (ano 1, uma tarefa importada de uma issue nasce assim):
  // antes de 1971 não é um prazo.
  const hasDeadlineDate = (iso) => !!iso && new Date(iso).getFullYear() >= 1971;
  // deadlineInfo descreve o prazo de uma tarefa para o badge: atrasada, vencendo
  // nas próximas 48 horas ou só a data.
  function deadlineInfo(iso) {
    if (!hasDeadlineDate(iso)) return { label: t('tasks.no_deadline'), cls: '' };
    const diff = new Date(iso).getTime() - Date.now();
    if (diff < 0) return { label: t('tasks.overdue', { date: fmt.date(iso) }), cls: 'badge-danger' };
    if (diff < 2 * 24 * 60 * 60 * 1000) return { label: t('tasks.due_soon', { date: fmt.date(iso) }), cls: 'badge-warn' };
    return { label: fmt.date(iso), cls: '' };
  }

  // payments são as contas de tela dos pagamentos, iguais no perfil da pessoa e na tela da equipe.
  const payments = {
    // "05 out. – 05 nov.": os dois dias são inclusivos, em dias de calendário.
    period(p) {
      return fmt.day(p.start, { day: '2-digit', month: 'short' }) + ' – ' + fmt.day(p.pay_date, { day: '2-digit', month: 'short' });
    },
    // Quanto falta para o pagamento do período corrente: "Paga hoje" ou "em 3 dias".
    daysLeft(cur) {
      if (!cur || cur.days_left === undefined) return '';
      return cur.days_left === 0 ? t('payments.today') : t('payments.days_left', { count: cur.days_left });
    },
    // O andamento do período contra a meta da jornada, em porcentagem.
    goalPercent(cur) {
      return cur && cur.goal_seconds ? Math.round(cur.seconds / cur.goal_seconds * 100) : 0;
    },
  };

  window.WTT = { can, t, lang, priorityClass, statusClass, hasDeadlineDate, deadlineInfo, errorText, api, ApiError, form, rules, limits, fmt, payments, toCents, copyText, notInformed, weekdays, priorities, taskStatuses, markdown, routine, sprintOptions, sprintChoices, workModes, currencies, countries, boot: window.BOOT || {} };

  // Onde flash() deixa a mensagem para a página seguinte.
  const flashKey = 'wtt:flash';

  document.addEventListener('alpine:init', () => {
    // $t('chave', { nome: valor }) nas expressões do Alpine (x-text, :title...).
    Alpine.magic('t', () => t);

    Alpine.store('toast', {
      items: [],
      seq: 0,
      // Mostra a mensagem que a página anterior deixou com flash().
      init() {
        let saved = null;
        try {
          saved = sessionStorage.getItem(flashKey);
          sessionStorage.removeItem(flashKey);
        } catch (e) {
          // Sem sessionStorage não há mensagem para mostrar.
        }
        if (!saved) return;
        let flash;
        try {
          flash = JSON.parse(saved);
        } catch (e) {
          flash = { message: saved };
        }
        this.show(flash.message, flash.kind);
      },
      show(message, kind) {
        const id = ++this.seq;
        this.items.push({ id, message, kind: kind || 'info' });
        setTimeout(() => this.dismiss(id), kind === 'error' ? 6000 : 3500);
      },
      error(message) {
        this.show(message, 'error');
      },
      // flash guarda a mensagem para aparecer na próxima página: é para quem salva e
      // redireciona em seguida.
      flash(message, kind) {
        try {
          sessionStorage.setItem(flashKey, JSON.stringify({ message, kind }));
        } catch (e) {
          // Sem sessionStorage a página seguinte só deixa de mostrar o toast.
        }
      },
      dismiss(id) {
        this.items = this.items.filter((t) => t.id !== id);
      },
    });

    // modal é o único modal da aplicação (partials/modal.gohtml). A página teleporta o
    // conteúdo para #modal-root e o mostra com x-show="$store.modal.name === '<nome>'".
    // name e title não são limpos ao fechar: o conteúdo fica lá até o fim da transição.
    let opener = null; // quem tinha o foco ao abrir, para devolver ao fechar
    let canClose = null; // a página pode impedir o fechamento, por exemplo enquanto salva
    const lockPage = (on) => {
      const root = document.documentElement;
      // Sem scroll a barra de rolagem some: o espaço dela vira padding para a página não pular.
      root.style.paddingRight = on ? (window.innerWidth - root.clientWidth) + 'px' : '';
      root.classList.toggle('modal-open', on);
      // inert prende o foco no modal. Os toasts ficam de fora para continuarem sendo lidos.
      document.querySelectorAll('body > .topbar, body > main').forEach((el) => { el.inert = on; });
    };
    Alpine.store('modal', {
      name: null,
      title: '',
      isOpen: false,
      open(name, title, guard) {
        if (!this.isOpen) opener = document.activeElement; // antes do inert, que tira o foco
        canClose = guard || null;
        this.name = name;
        this.title = title;
        this.isOpen = true;
        lockPage(true);
        // O Alpine segura o nextTick até o painel aparecer, então o campo já aceita foco.
        Alpine.nextTick(() => {
          const panel = document.querySelector('.modal-panel');
          const field = [...panel.querySelectorAll('[data-autofocus]')].find((el) => el.offsetParent);
          (field || panel).focus();
        });
      },
      // dismiss é o fechamento pedido pela pessoa: Esc, clique no fundo, X e Cancelar.
      dismiss() {
        if (this.isOpen && (!canClose || canClose())) this.close();
      },
      close() {
        if (!this.isOpen) return;
        this.isOpen = false;
        lockPage(false);
        const el = opener;
        opener = null;
        // Depois de o x-for reordenar a lista: mover a linha tiraria o foco do botão.
        Alpine.nextTick(() => el && el.isConnected && el.focus());
      },
    });

    // clock guarda a sessão de trabalho aberta da pessoa logada e um relógio que
    // avança a cada segundo. O balão da sessão e a tela de ponto leem daqui.
    // A sessão tem tarefas (tasks), cada uma com o intervalo em que esteve nela; as que
    // estão em andamento são as que ainda não têm fim (until_at).
    const ms = (iso) => new Date(iso).getTime();
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
        const end = s.end_at ? ms(s.end_at) : this.now;
        return Math.max(0, (end - ms(s.start_at)) / 1000);
      },
      // linkElapsed é o tempo de um intervalo de tarefa dentro da sessão, como o servidor
      // calcula: o intervalo recortado entre o início e o fim da sessão (ou agora).
      linkElapsed(session, link) {
        const sessionEnd = session.end_at ? ms(session.end_at) : this.now;
        const start = Math.max(ms(link.from_at), ms(session.start_at));
        const end = Math.min(link.until_at ? ms(link.until_at) : sessionEnd, sessionEnd);
        return Math.max(0, end - start) / 1000;
      },
      // taskElapsed é o tempo de uma tarefa na sessão, somando os intervalos dela.
      taskElapsed(session, taskId) {
        return (session.tasks || []).filter((l) => l.task_id === taskId)
          .reduce((sum, l) => sum + this.linkElapsed(session, l), 0);
      },
      // running são as tarefas em andamento na sessão aberta.
      running(session) {
        const s = session || this.session;
        return s && !s.end_at ? (s.tasks || []).filter((l) => !l.until_at) : [];
      },
      isRunning(taskId) {
        return this.running().some((l) => l.task_id === taskId);
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
      // addTask põe uma tarefa na sessão aberta, que passa a contá-la daqui em diante.
      async addTask(taskId) {
        if (!this.session) return;
        await api('POST', '/api/projects/' + this.session.project_id + '/work-sessions/' + this.session.id + '/tasks', { task_id: taskId });
        await this.refresh();
        window.dispatchEvent(new CustomEvent('wtt:sessions-changed'));
      },
      // stopTask para uma tarefa só: o intervalo dela fecha agora e a sessão segue com as outras. A sessão
      // aberta guarda sempre uma tarefa em andamento (o servidor recusa parar a última por aqui), então
      // parar a única em andamento é encerrar o ponto. O aviso é daqui porque todas as telas chamam.
      async stopTask(taskId) {
        const link = this.running().find((l) => l.task_id === taskId);
        if (!link) return;
        if (this.running().length === 1) {
          await this.clockOut();
          Alpine.store('toast').show(t('session.stopped'));
          return;
        }
        await api('PATCH', '/api/projects/' + this.session.project_id + '/work-sessions/' + this.session.id + '/tasks/' + link.id, { stop: true });
        await this.refresh();
        window.dispatchEvent(new CustomEvent('wtt:sessions-changed'));
        Alpine.store('toast').show(t('session.modal.stopped_task', { name: link.task.name }));
      },
      async clockOut() {
        if (!this.session) return;
        await api('POST', '/api/projects/' + this.session.project_id + '/work-sessions/clock-out', {});
        this.session = null;
        window.dispatchEvent(new CustomEvent('wtt:sessions-changed'));
      },
    });

    // presence guarda quem está com o ponto aberto na organização, para o dono e os admins: a bolinha da
    // lista de tarefas e a coluna "Agora" dos colaboradores leem daqui. Quem não é admin não chama nada
    // (o servidor responde 403 de qualquer jeito). A página que usa chama watch(); daí em diante a lista
    // se atualiza de 30 em 30 segundos enquanto a aba está à vista, e ao voltar para ela. Se uma chamada
    // falha, vale a última lista.
    Alpine.store('presence', {
      enabled: false,
      byPerson: {},
      timer: null,
      init() {
        const me = window.WTT.boot.me;
        this.enabled = !!me && me.role === 'admin';
      },
      watch() {
        if (!this.enabled || this.timer) return;
        this.refresh();
        this.timer = setInterval(() => { if (document.visibilityState === 'visible') this.refresh(); }, 30000);
        document.addEventListener('visibilitychange', () => { if (document.visibilityState === 'visible') this.refresh(); });
        // O ponto que o próprio admin bate ou fecha também muda a bolinha dele.
        window.addEventListener('wtt:sessions-changed', () => this.refresh());
      },
      async refresh() {
        try {
          const list = await api('GET', '/api/orgs/' + window.WTT.boot.me.organization_id + '/working-now');
          this.byPerson = Object.fromEntries(list.map((p) => [p.person_id, p]));
        } catch (e) {
          // A bolinha só deixa de mudar; nada a avisar.
        }
      },
      // of é o que a pessoa faz agora (working_on[] com tarefa e projeto), ou null se o ponto está fechado.
      of(personId) {
        return this.byPerson[personId] || null;
      },
      working(personId) {
        return !!this.of(personId);
      },
      // label é a dica da bolinha: em que a pessoa está, ou que o ponto está fechado.
      label(personId) {
        const p = this.of(personId);
        if (!p) return t('home.team.idle');
        if (p.working_on.length === 0) return t('home.team.working');
        const [first, ...rest] = p.working_on;
        const lines = [t('presence.working_on', { task: first.task.name, project: first.project.name })];
        if (rest.length > 0) lines.push(t('home.team.more_tasks', { count: rest.length }));
        return lines.join('\n');
      },
    });

    // sessionView é o modal de uma sessão (partials/session_modal.gohtml): o resumo, as tarefas
    // com o intervalo de cada uma e, para quem bateu o ponto e para os admins, o que muda as
    // tarefas. Abre de qualquer tela com Alpine.store('sessionView').open(sessao); depois de
    // cada mudança avisa as páginas com wtt:sessions-changed.
    const sessionApi = (s) => '/api/projects/' + s.project_id + '/work-sessions/' + s.id;
    Alpine.store('sessionView', {
      ...form(),
      session: null,
      canEdit: false,
      view: 'overview', // overview, ou interval quando edita o intervalo de uma tarefa
      link: null, // a tarefa da sessão que o interval edita
      draft: { from: '', until: '' },
      start: { from: '', until: '' }, // o rascunho como abriu: só o que mudou vai ao servidor
      confirmRemove: false,
      search: '',
      results: [],
      pick: '',
      open(session) {
        const me = window.WTT.boot.me;
        this.session = JSON.parse(JSON.stringify(session));
        this.canEdit = !!me && (me.role === 'admin' || me.id === session.person_id);
        this.view = 'overview';
        this.link = null;
        this.confirmRemove = false;
        this.search = '';
        this.errors = {};
        this.results = [];
        this.pick = '';
        Alpine.store('modal').open('session', t('session.modal.title', { date: fmt.date(session.start_at) }), () => !this.pending);
        if (this.canEdit) this.find();
      },
      // Os valores da sessão: a pessoa vê o que ganhou; quem vê o dos outros, o custo e a receita.
      // Vazio quando não há valor ou ele não é visível. A sessão aberta acompanha o cronômetro.
      amounts() {
        const s = this.session;
        if (!s) return [];
        const live = (kind) => {
          const rate = s[kind + '_rate_cents'];
          if (rate === null || rate === undefined) return null;
          return s.end_at ? s[kind + '_amount_cents'] : Math.round(Alpine.store('clock').elapsed(s) * rate / 3600);
        };
        const me = window.WTT.boot.me;
        const own = me && me.id === s.person_id;
        const rows = own
          ? [{ label: t('time.your_value'), value: live(s.owner_hours ? 'bill' : 'pay') }]
          : [{ label: t('time.cost'), value: live('pay') }, { label: t('time.revenue'), value: live('bill') }];
        return rows.filter((r) => r.value !== null && r.value !== undefined);
      },
      // A barra de cada tarefa: onde ela começa e termina dentro da sessão, em por cento.
      bar(l) {
        const s = this.session;
        const start = ms(s.start_at);
        const end = s.end_at ? ms(s.end_at) : Alpine.store('clock').now;
        const total = Math.max(1, end - start);
        const from = Math.min(Math.max(ms(l.from_at), start), end);
        const to = Math.max(from, Math.min(l.until_at ? ms(l.until_at) : end, end));
        const left = (from - start) / total * 100;
        const width = Math.min(100 - left, Math.max(1.5, (to - from) / total * 100));
        return 'left:' + left + '%;width:' + width + '%';
      },
      isRunning(l) {
        return !this.session.end_at && !l.until_at;
      },
      range(l) {
        const s = this.session;
        const to = l.until_at ? fmt.time(l.until_at) : (s.end_at ? fmt.time(s.end_at) : t('session.modal.now'));
        return fmt.time(l.from_at) + ' – ' + to;
      },
      // Não dá para tirar nem parar a última tarefa: a sessão guarda sempre uma e, aberta, uma em andamento.
      otherRunning(l) {
        return this.session.tasks.some((x) => x.id !== l.id && !x.until_at);
      },
      canStop(l) {
        return !this.session.end_at && !l.until_at && this.otherRunning(l);
      },
      canRemove(l) {
        return this.session.tasks.length > 1 && (!!this.session.end_at || !!l.until_at || this.otherRunning(l));
      },
      // Tarefas do projeto que ainda não estão na sessão, para o Adicionar. Quem já está tem o
      // intervalo para editar.
      async find() {
        const s = this.session;
        const q = this.search.trim();
        try {
          const page = await api('GET', '/api/projects/' + s.project_id + '/tasks?page=1&per_page=20' + (q ? '&q=' + encodeURIComponent(q) : ''));
          const inSession = new Set(s.tasks.map((l) => l.task_id));
          this.results = (page.items || []).filter((x) => !inSession.has(x.id));
          this.pick = this.results.length ? this.results[0].id : '';
        } catch (e) {
          this.errors.add = e.message;
        }
      },
      // changed guarda a sessão que o servidor devolveu, atualiza o relógio (a sessão aberta é a
      // do cabeçalho) e avisa as páginas.
      async changed(session) {
        this.session = session;
        await Alpine.store('clock').refresh();
        window.dispatchEvent(new CustomEvent('wtt:sessions-changed'));
        if (this.canEdit) await this.find();
      },
      add() {
        return this.run('add', async () => {
          const added = this.results.find((x) => x.id === this.pick);
          await this.changed(await api('POST', sessionApi(this.session) + '/tasks', { task_id: this.pick }));
          Alpine.store('toast').show(t('session.modal.added', { name: added ? added.name : '' }));
        });
      },
      edit(l) {
        this.link = l;
        this.draft = { from: fmt.dateTimeInput(l.from_at), until: fmt.dateTimeInput(l.until_at) };
        this.start = { ...this.draft };
        this.confirmRemove = false;
        this.errors = {};
        this.clearFields();
        this.view = 'interval';
      },
      back() {
        this.view = 'overview';
        this.errors = {};
        this.clearFields();
      },
      // checkInterval confere o intervalo como o servidor (validateInterval): o começo cabe na sessão, o fim vem
      // depois do começo e nenhum passa do fim da sessão (ou de agora, se aberta). O que não mudou já valeu antes.
      // O store não é um componente (sem $nextTick), então o foco vai pelo Alpine.nextTick.
      checkInterval() {
        const s = this.session;
        const l = this.link;
        const fromChanged = this.draft.from !== this.start.from;
        const untilChanged = this.draft.until !== this.start.until;
        this.clearFields();
        if (!fromChanged && !untilChanged) return true;
        const found = {};
        const message = (code, field) => window.WTT.errorText({ code, params: { field } });
        const limit = s.end_at ? ms(s.end_at) : Date.now();
        const from = fromChanged ? (this.draft.from ? new Date(this.draft.from).getTime() : NaN) : ms(l.from_at);
        const until = untilChanged ? (this.draft.until ? new Date(this.draft.until).getTime() : null) : (l.until_at ? ms(l.until_at) : null);
        if (Number.isNaN(from)) {
          found.from_at = message('request.field_required', 'from_at');
        } else if (fromChanged && (from < ms(s.start_at) || from > limit || (s.end_at && from >= limit))) {
          found.from_at = message('work_session.invalid_interval', 'from_at');
        } else if (until !== null && (until <= from || until > limit)) {
          found.until_at = message('work_session.invalid_interval', 'until_at');
        }
        this.fieldErrors = found;
        const first = Object.keys(found)[0];
        if (first) Alpine.nextTick(() => this.focusField(first));
        return first === undefined;
      },
      saveInterval() {
        if (!this.checkInterval()) return undefined;
        return this.run('save', async () => {
          const body = {};
          if (this.draft.from !== this.start.from) body.from_at = fmt.fromDateTimeInput(this.draft.from);
          if (this.draft.until !== this.start.until) body.until_at = fmt.fromDateTimeInput(this.draft.until);
          if (Object.keys(body).length > 0) {
            await this.changed(await api('PATCH', sessionApi(this.session) + '/tasks/' + this.link.id, body));
            Alpine.store('toast').show(t('session.modal.saved'));
          }
          this.back();
        });
      },
      stopNow() {
        return this.run('save', async () => {
          await this.changed(await api('PATCH', sessionApi(this.session) + '/tasks/' + this.link.id, { stop: true }));
          Alpine.store('toast').show(t('session.modal.stopped_task', { name: this.link.task.name }));
          this.back();
        });
      },
      remove() {
        return this.run('save', async () => {
          await this.changed(await api('DELETE', sessionApi(this.session) + '/tasks/' + this.link.id));
          Alpine.store('toast').show(t('session.modal.removed'));
          this.back();
        });
      },
    });

    // tip é a dica de um número ou de um rótulo: um botão pequeno abre a
    // explicação num balão, que fecha com Esc, com um clique fora e quando a
    // janela muda de tamanho. O balão nasce alinhado à esquerda do bloco que
    // o contém; place() o puxa de volta quando ele passaria da borda direita
    // da fileira em que o bloco está, ou da tela.
    Alpine.data('tip', () => ({
      open: false,
      toggle() {
        this.open = !this.open;
        if (this.open) this.$nextTick(() => this.place());
      },
      place() {
        const pop = this.$refs.pop;
        pop.style.left = '0px';
        const row = this.$root.parentElement.getBoundingClientRect();
        const limit = Math.min(row.right, document.documentElement.clientWidth - 16);
        const over = pop.getBoundingClientRect().right - limit;
        if (over > 0) pop.style.left = -over + 'px';
      },
    }));

    // sessionDock é o balão da sessão aberta (partials/active_session.gohtml): aberto, lista as tarefas
    // em andamento; Tarefas da sessão e Parar ficam no rodapé dele. Ao acabar a sessão, volta a fechado.
    Alpine.data('sessionDock', () => ({
      open: false,
      busy: false,
      init() {
        this.$watch('$store.clock.session', (s) => { if (!s) this.open = false; });
        // Os avisos sobem acima do balão, que cresce com as tarefas: a altura vai para o CSS.
        new ResizeObserver(() => document.documentElement.style.setProperty('--dock-h', this.$root.offsetHeight + 'px')).observe(this.$root);
      },
      details() {
        Alpine.store('sessionView').open(Alpine.store('clock').session);
      },
      async stop() {
        this.busy = true;
        try {
          await Alpine.store('clock').clockOut();
          Alpine.store('toast').show(t('session.stopped'));
        } catch (e) {
          Alpine.store('toast').error(e.message);
        } finally {
          this.busy = false;
        }
      },
      // stopTask para só a tarefa do cartão; sendo a única em andamento, é o mesmo que Parar.
      async stopTask(l) {
        this.busy = true;
        try {
          await Alpine.store('clock').stopTask(l.task_id);
        } catch (e) {
          Alpine.store('toast').error(e.message);
        } finally {
          this.busy = false;
        }
      },
    }));

    // O menu da conta, no canto da barra superior: o perfil, a ajuda, o idioma e a saída.
    Alpine.data('userMenu', () => ({
      open: false,
      busy: false,
      toggle() {
        this.open = !this.open;
      },
      close() {
        this.open = false;
      },
      async logout() {
        this.busy = true;
        try {
          await api('POST', '/api/auth/logout');
        } finally {
          // out=1 avisa o login que foi uma saída: com o Clerk ligado, ele também desconecta de lá.
          location.href = '/login?out=1';
        }
      },
    }));
  });
})();
