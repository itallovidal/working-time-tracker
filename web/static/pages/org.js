// Componentes das páginas da organização (a lista de projetos, as abas Sobre, Colaboradores,
// Clientes e Projetos e a tela de edição) e do perfil de quem está logado.
document.addEventListener('alpine:init', () => {
  const { api, form } = WTT;
  const me = WTT.boot.me;
  const orgId = me.organization_id;
  const toast = (msg, kind) => Alpine.store('toast').show(msg, kind);
  const setText = (selector, text) => document.querySelectorAll(selector).forEach((el) => { el.textContent = text; });

  // A organização vem pronta do servidor nas páginas dela (window.BOOT.org).
  const org = WTT.boot.org || { name: me.organization_name };

  const blankProject = () => ({
    name: '', description: '', sprint_duration_days: 14, ...WTT.routine.blank(),
    customer_id: '', rate: '',
  });

  const orgTexts = [
    'name', 'summary', 'description', 'industry', 'size',
    'website', 'contact_email', 'phone', 'linkedin_url', 'instagram_url',
    'legal_name', 'cnpj', 'address_line1', 'address_line2', 'city', 'state', 'postal_code', 'country',
    'work_mode', 'timezone', 'currency',
  ];
  const orgNumbers = ['founded_year'];

  // orgForm copia a organização para o formulário: campo sem valor vira texto vazio.
  function orgForm(o) {
    const f = {};
    orgTexts.concat(orgNumbers).forEach((k) => { f[k] = o[k] || ''; });
    f.cnpj = WTT.fmt.cnpj(f.cnpj);
    return f;
  }

  // timezoneOptions lista os fusos que o navegador conhece, sempre com o atual.
  function timezoneOptions(current) {
    let zones = [];
    try { zones = Intl.supportedValuesOf('timeZone'); } catch (e) { zones = []; }
    if (zones.length === 0) {
      zones = ['America/Sao_Paulo', 'America/Manaus', 'America/Recife', 'America/Rio_Branco', 'America/Noronha', 'UTC'];
    }
    return current && !zones.includes(current) ? [current, ...zones] : zones;
  }

  // rows monta as linhas de uma lista de [rótulo, valor, extras]. Uma linha sem
  // valor aparece como "Não informado", sem link.
  const rows = (list) => list.map((r) => (r[1]
    ? { label: r[0], value: r[1], ...(r[2] || {}) }
    : { label: r[0], value: WTT.notInformed, empty: true }));
  const bareURL = (url) => (url || '').replace(/^https?:\/\/(www\.)?/, '').replace(/\/$/, '');

  // projectCreation é o que a página inicial e a aba Projetos têm em comum: o modal de novo projeto
  // (o formulário é o parcial project_form) e a criação. Cada componente o espalha no seu objeto.
  const projectCreation = () => ({
    ...form(),
    customers: null, // só carregados quando o modal de novo projeto abre pela primeira vez
    draft: blankProject(),
    // openCreate abre o modal de novo projeto. Só admins criam projeto, e só eles podem
    // listar os clientes; por isso a lista vem aqui, e não no init.
    async openCreate() {
      this.draft = blankProject();
      this.errors.create = '';
      Alpine.store('modal').open('project', WTT.t('org.projects.new'), () => !this.pending);
      if (this.customers !== null) return;
      try {
        this.customers = (await api('GET', '/api/orgs/' + orgId + '/customers')) || [];
      } catch (e) {
        this.errors.create = WTT.t('org.projects.customers_load_failed', { error: e.message });
      }
    },
    // create cria o projeto e, se houver cliente ou valor, grava a cobrança logo em seguida.
    create() {
      return this.run('create', async () => {
        // O valor só vale com cliente (o campo some no projeto interno) e é conferido antes:
        // depois de criado, um erro na cobrança deixaria o projeto sem ela.
        const cents = this.draft.customer_id ? WTT.toCents(this.draft.rate) : null;
        if (this.draft.customer_id && cents === null && String(this.draft.rate).trim() !== '') throw new Error(WTT.t('org.projects.rate_invalid'));
        if (cents !== null && cents > 100000000) throw new Error(WTT.t('org.projects.rate_too_high'));
        const p = await api('POST', '/api/orgs/' + orgId + '/projects', {
          name: this.draft.name,
          description: this.draft.description,
          sprint_duration_days: Number(this.draft.sprint_duration_days) || 0,
          ...WTT.routine.payload(this.draft),
        });
        if (this.draft.customer_id) {
          try {
            await api('PUT', '/api/projects/' + p.id + '/billing', { customer_id: this.draft.customer_id || null, bill_rate_cents: cents });
          } catch (e) {
            // O projeto já existe: as configurações dele são o lugar de definir a cobrança de novo.
            Alpine.store('toast').flash(WTT.t('org.projects.created_no_billing', { error: e.message }), 'error');
            location.href = '/projects/' + p.id + '/management/settings';
            return;
          }
        }
        Alpine.store('toast').flash(WTT.t('org.projects.created'));
        location.href = '/projects/' + p.id;
      });
    },
  });

  // A aba Projetos da organização: todos os projetos numa tabela de gestão.
  Alpine.data('orgProjects', () => ({
    ...projectCreation(),
    loading: true,
    projects: [],
    async init() {
      try {
        this.projects = (await api('GET', '/api/orgs/' + orgId + '/projects')) || [];
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
    },
  }));

  // Tons dos cartões de projeto: só enfeite (as classes .hue-* do app.css), sorteados pelo id para
  // o projeto ter sempre a mesma cor, em qualquer lugar.
  const projectHues = ['teal', 'blue', 'purple', 'green', 'orange', 'gold'];
  const hueOf = (id) => {
    let h = 0;
    for (const c of String(id)) h = (h * 31 + c.charCodeAt(0)) >>> 0;
    return projectHues[h % projectHues.length];
  };
  // initialsOf são as iniciais das duas primeiras palavras, como a do canto da barra superior:
  // "Ana Souza" -> "AS".
  const initialsOf = (name) => String(name || '').trim().split(/\s+/).filter(Boolean).slice(0, 2).map((w) => Array.from(w)[0].toUpperCase()).join('') || '?';
  // markOf é a sigla do cartão do projeto. As palavras curtas e minúsculas que ligam o nome ("da",
  // "de", "do") ficam de fora, para "Site da Jatobá" dar "SJ" e não "SD"; um nome de uma palavra só
  // usa as duas primeiras letras dela.
  const markOf = (name) => {
    const words = String(name || '').trim().split(/\s+/).filter(Boolean);
    const main = words.filter((w) => !(w.length <= 3 && w === w.toLowerCase() && w !== w.toUpperCase()));
    const picked = main.length ? main : words;
    const letters = picked.length > 1 ? picked.slice(0, 2).map((w) => Array.from(w)[0]) : Array.from(picked[0] || '?').slice(0, 2);
    return letters.join('').toUpperCase();
  };

  const HOME_PROJECTS_PER_PAGE = 6;
  const HOME_TEAM_PER_PAGE = 5; // listas de pessoas ficam em cinco por vez
  // As três janelas da visão geral: a chave é a da API, o campo é o tempo da pessoa nela.
  const periodFields = { last_7_days: 'last_7_days_seconds', last_30_days: 'last_30_days_seconds', all_time: 'total_seconds' };

  // pageOf recorta uma página de uma lista e corrige o número dela se a lista encolheu.
  const pageOf = (rows, wanted, perPage) => {
    const pages = Math.max(1, Math.ceil(rows.length / perPage));
    const page = Math.min(Math.max(1, wanted), pages);
    const start = (page - 1) * perPage;
    return { rows: rows.slice(start, start + perPage), page, pages, total: rows.length, from: rows.length ? start + 1 : 0, to: Math.min(start + perPage, rows.length) };
  };

  // GUARDADO, SEM USO. A equipe por horas, de quem mais trabalhou na janela para quem menos, que a
  // página inicial deixou de mostrar em 7 out 2026: comparar o tempo de cada um dá a entender que
  // quem trabalhou mais é quem mais merece reconhecimento, e o valor entregue nem sempre vem das
  // horas. Vai com o parcial partials/team_hours.gohtml; para voltar a usar, espalhe
  // `...teamHours()` no orgHome e inclua o parcial na página.
  const teamHours = () => ({
    rankingAt: 1,
    secondsOf(p) {
      return p[periodFields[this.period]];
    },
    rankingRows() {
      return [...this.stats.by_person].sort((a, b) => this.secondsOf(b) - this.secondsOf(a) || a.person.name.localeCompare(b.person.name));
    },
    rankingView() {
      return pageOf(this.rankingRows(), this.rankingAt, HOME_TEAM_PER_PAGE);
    },
    rankingReset() {
      this.rankingAt = 1;
    },
    // A barra é a proporção de quem mais trabalhou; quem trabalhou algo nunca fica sem barra.
    barWidth(p) {
      const max = Math.max(0, ...this.stats.by_person.map((x) => this.secondsOf(x)));
      return max > 0 && this.secondsOf(p) > 0 ? Math.max(3, Math.round(this.secondsOf(p) / max * 100)) : 0;
    },
  });

  // A página inicial: para admins, a visão geral da organização (tempo e dinheiro de todos os
  // projetos) e, para todos, os projetos em cartões, seis por página.
  Alpine.data('orgHome', () => ({
    ...projectCreation(),
    loading: true,
    projects: [],
    total: 0,
    page: 1,
    stats: null,
    statsLoading: false,
    period: 'last_30_days',
    teamAt: 1,
    periods: [
      { key: 'last_7_days', label: WTT.t('home.stats.last_7') },
      { key: 'last_30_days', label: WTT.t('home.stats.last_30') },
      { key: 'all_time', label: WTT.t('home.stats.all_time') },
    ],
    hueOf,
    initialsOf,
    markOf,
    init() {
      const wanted = parseInt(new URLSearchParams(location.search).get('page'), 10);
      this.page = wanted > 0 ? wanted : 1;
      this.loadProjects();
      if (me.role === 'admin') this.loadStats();
    },

    // Os projetos: uma página por vez, vinda do servidor, que corrige uma página que não existe mais.
    loadProjects() {
      return this.run('page', async () => {
        this.errors.load = '';
        try {
          const res = await api('GET', '/api/orgs/' + orgId + '/projects?page=' + this.page + '&per_page=' + HOME_PROJECTS_PER_PAGE);
          this.projects = res.items || [];
          this.total = res.total;
          this.page = res.page;
          this.syncURL();
        } catch (e) {
          this.errors.load = e.message;
        } finally {
          this.loading = false;
        }
      });
    },
    go(page) {
      this.page = page;
      return this.loadProjects();
    },
    pages() {
      return Math.max(1, Math.ceil(this.total / HOME_PROJECTS_PER_PAGE));
    },
    summary() {
      return WTT.t('home.projects.summary', { page: this.page, pages: this.pages(), count: this.total });
    },
    // A página fica no endereço, para recarregar ou voltar cair onde a pessoa estava.
    syncURL() {
      const url = new URL(location.href);
      if (this.page > 1) url.searchParams.set('page', this.page);
      else url.searchParams.delete('page');
      history.replaceState(null, '', url);
    },

    // A visão geral: o servidor manda as três janelas de uma vez, e trocar de janela não pede nada.
    async loadStats() {
      this.statsLoading = true;
      this.errors.stats = '';
      try {
        this.stats = await api('GET', '/api/orgs/' + orgId + '/overview');
      } catch (e) {
        this.errors.stats = e.message;
      } finally {
        this.statsLoading = false;
      }
    },
    setPeriod(key) {
      this.period = key;
      if (this.rankingReset) this.rankingReset(); // só existe com o ranking por horas, hoje guardado
    },
    current() {
      return this.stats.periods[this.period];
    },
    scopeText() {
      return WTT.t({ last_7_days: 'home.stats.scope_7', last_30_days: 'home.stats.scope_30', all_time: 'home.stats.scope_all' }[this.period]);
    },
    // O tempo da equipe são as horas dos outros; somadas às suas dão o total de todos.
    teamSeconds() {
      const p = this.current();
      return Math.max(0, p.seconds - p.my_seconds);
    },
    mineShare() {
      const p = this.current();
      return p.seconds > 0 ? Math.round(p.my_seconds / p.seconds * 100) : null;
    },
    marginShare() {
      const m = this.current().money;
      return m.margin_cents === null || !m.bill_amount_cents ? null : Math.round(m.margin_cents / m.bill_amount_cents * 100);
    },
    marginNegative() {
      const m = this.current().money.margin_cents;
      return m !== null && m < 0;
    },
    workingNames() {
      return this.stats.by_person.filter((p) => p.working_now).map((p) => p.person.name).join(', ');
    },

    // A equipe: todas as pessoas, cinco por página, com quem trabalha agora primeiro e depois por
    // nome. Não há ordem por horas de propósito (veja teamHours).
    teamRows() {
      return [...this.stats.by_person].sort((a, b) => Number(b.working_now) - Number(a.working_now) || a.person.name.localeCompare(b.person.name));
    },
    teamView() {
      return pageOf(this.teamRows(), this.teamAt, HOME_TEAM_PER_PAGE);
    },
    // As outras tarefas da sessão, além da que aparece na linha, para a dica de quem tem mais de uma.
    moreTasks(p) {
      return p.working_on.slice(1).map((w) => w.task.name + ' · ' + w.project.name).join('\n');
    },
  }));

  Alpine.data('orgPeople', () => ({
    ...form(),
    me,
    loading: true,
    people: [],
    invites: [],
    invite: { email: '', role: 'member' },
    lastLink: '',
    // Como o último convite chegou (email, terminal ou link) e para quem, para o modal dizer o que aconteceu.
    lastDelivery: '',
    lastEmail: '',
    editing: null, // a pessoa aberta no modal da jornada semanal e das permissões
    draft: { weekly_hours: '', permissions: [] },
    orgKeys: [], // as permissões da organização do catálogo, para o dono liberar
    async init() {
      try {
        const [people, invites, catalog] = await Promise.all([
          api('GET', '/api/orgs/' + orgId + '/persons'),
          api('GET', '/api/orgs/' + orgId + '/invites'),
          me.is_owner ? api('GET', '/api/permissions') : null,
        ]);
        this.people = people || [];
        this.invites = invites || [];
        if (catalog) this.orgKeys = catalog.organization;
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
      // O atalho "Adicionar colaborador" da página inicial chega com ?add=1: abre o modal do
      // convite e tira o parâmetro, para recarregar ou voltar não abrir o modal de novo.
      if (new URLSearchParams(location.search).has('add')) {
        history.replaceState(null, '', location.pathname);
        this.openInvite();
      }
    },
    setRole(person, role) {
      return this.run('role', async () => {
        const updated = await api('PATCH', '/api/persons/' + person.id + '/role', { role });
        person.role = updated.role;
        toast(WTT.t(updated.role === 'admin' ? 'org.people.now_admin' : 'org.people.now_member', { name: person.name }));
        // Quem tirou o próprio admin perde o acesso a esta página.
        if (person.id === me.id && updated.role !== 'admin') location.href = '/';
      });
    },
    // As permissões da organização só se liberam a quem não é admin, e só o dono as dá.
    canGrant(person) {
      return !!me.is_owner && !!person && person.role !== 'admin' && !person.is_owner;
    },
    // openEdit abre o modal com a jornada e as permissões da pessoa. Ele edita um rascunho:
    // nada vai para o servidor antes de Salvar. O papel muda direto na linha, pelo botão.
    openEdit(person) {
      this.editing = person;
      this.draft = { weekly_hours: person.weekly_hours || '', permissions: [...(person.permissions || [])] };
      this.errors.edit = '';
      Alpine.store('modal').open('person-edit', WTT.t('org.people.edit_title'), () => !this.pending);
    },
    savePerson() {
      return this.run('edit', async () => {
        const person = this.editing;
        const text = String(this.draft.weekly_hours).trim();
        const hours = text === '' ? 0 : Number(text);
        if (!Number.isInteger(hours) || hours < 0 || hours > 168) throw new Error(WTT.t('errors.person.invalid_week_hours'));
        if (WTT.can('people.manage') && hours !== (person.weekly_hours || 0)) {
          const updated = await api('PATCH', '/api/persons/' + person.id + '/weekly-hours', { weekly_hours: hours });
          person.weekly_hours = updated.weekly_hours;
        }
        const before = [...(person.permissions || [])].sort().join();
        if (this.canGrant(person) && [...this.draft.permissions].sort().join() !== before) {
          const updated = await api('PATCH', '/api/persons/' + person.id + '/permissions', { permissions: this.draft.permissions });
          person.permissions = updated.permissions;
        }
        toast(WTT.t('org.people.hours_saved', { name: person.name }));
        Alpine.store('modal').close();
      });
    },
    // openInvite abre o modal de adicionar colaborador, sempre com o formulário zerado.
    openInvite() {
      this.invite = { email: '', role: 'member' };
      this.lastLink = '';
      this.lastDelivery = '';
      this.lastEmail = '';
      this.errors.invite = '';
      Alpine.store('modal').open('invite', WTT.t('org.people.add'), () => !this.pending);
    },
    // O modal fica aberto depois de gerar o convite: o link só aparece nesta hora.
    createInvite() {
      return this.run('invite', async () => {
        const inv = await api('POST', '/api/orgs/' + orgId + '/invites', { email: this.invite.email, role: this.invite.role });
        this.lastLink = location.origin + inv.path;
        this.lastDelivery = inv.delivery;
        this.lastEmail = inv.email || '';
        this.invites = [inv, ...this.invites];
        // O bloco do link entra com x-transition, e o Alpine segura o $nextTick até ele
        // aparecer. Sem a transição o campo ainda estaria escondido e não aceitaria o foco.
        this.$nextTick(() => this.$refs.link && this.$refs.link.focus());
      });
    },
    async copyLink() {
      const ok = await WTT.copyText(this.lastLink);
      toast(ok ? WTT.t('org.people.link_copied') : WTT.t('org.people.copy_failed'), ok ? 'info' : 'error');
    },
    revoke(inv) {
      return this.run('revoke', async () => {
        await api('DELETE', '/api/invites/' + inv.id);
        this.invites = this.invites.filter((i) => i.id !== inv.id);
        toast(WTT.t('org.people.revoked'));
      });
    },
  }));

  Alpine.data('orgSettings', () => ({
    ...form(),
    form: orgForm(org),
    timezones: timezoneOptions(org.timezone),
    thisYear: new Date().getFullYear(),
    confirmDelete: false,
    saveOrg() {
      return this.run('org', async () => {
        // Texto vazio e zero apagam o campo; a API mantém o que não vier no corpo.
        const body = {};
        orgTexts.forEach((k) => { body[k] = this.form[k]; });
        orgNumbers.forEach((k) => { body[k] = Number(this.form[k]) || 0; });
        await api('PATCH', '/api/orgs/' + orgId, body);
        // A edição volta para a aba Sobre, que recarrega com os dados novos e mostra o toast.
        Alpine.store('toast').flash(WTT.t('org.settings.saved'));
        location.href = '/orgs/' + orgId + '/about';
      });
    },
    deleteOrg() {
      return this.run('delete', async () => {
        await api('DELETE', '/api/orgs/' + orgId);
        location.href = '/signup';
      });
    },
  }));

  Alpine.data('orgAbout', () => ({
    org,
    identity() {
      return rows([
        [WTT.t('org.fields.segment'), org.industry],
        [WTT.t('org.fields.size'), WTT.fmt.orgSize(org.size)],
        [WTT.t('org.about.founded'), org.founded_year ? String(org.founded_year) : ''],
        [WTT.t('org.about.location'), [org.city, org.state, org.country].filter(Boolean).join(', ')],
      ]);
    },
    contact() {
      return rows([
        [WTT.t('org.fields.website'), bareURL(org.website), { href: org.website, external: true }],
        [WTT.t('common.email'), org.contact_email, { href: 'mailto:' + org.contact_email }],
        [WTT.t('common.phone'), org.phone, { href: 'tel:' + (org.phone || '').replace(/[^0-9+]/g, '') }],
        [WTT.t('org.fields.linkedin'), bareURL(org.linkedin_url), { href: org.linkedin_url, external: true }],
        [WTT.t('org.fields.instagram'), bareURL(org.instagram_url), { href: org.instagram_url, external: true }],
      ]);
    },
    legal() {
      return rows([
        [WTT.t('org.fields.legal_name'), org.legal_name],
        [WTT.t('org.fields.cnpj'), WTT.fmt.cnpj(org.cnpj), { mono: true }],
        [WTT.t('org.fields.address'), [org.address_line1, org.address_line2, org.postal_code].filter(Boolean).join(' · ')],
      ]);
    },
    defaults() {
      return rows([
        [WTT.t('org.about.work_mode'), WTT.fmt.workMode(org.work_mode)],
        [WTT.t('org.fields.timezone'), org.timezone],
        [WTT.t('org.fields.currency'), WTT.fmt.currency(org.currency)],
      ]);
    },
    // Fuso e moeda sempre têm valor, então não contam como perfil preenchido.
    isEmpty() {
      return !org.description && [...this.identity(), ...this.contact(), ...this.legal()].every((f) => f.empty);
    },
  }));

  const blankCustomer = () => ({ name: '', document: '', contact_name: '', contact_email: '', contact_phone: '' });

  Alpine.data('orgCustomers', () => ({
    ...form(),
    loading: true,
    customers: [],
    editing: null, // id do cliente em edição; null quando o formulário cria um novo
    draft: blankCustomer(),
    confirming: null,
    async init() {
      try {
        this.customers = (await api('GET', '/api/orgs/' + orgId + '/customers')) || [];
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
    },
    // openForm abre o modal para criar (sem argumento) ou para editar o cliente c. O
    // formulário é zerado aqui, e não ao fechar, para não mudar durante a transição de saída.
    openForm(c) {
      this.editing = c ? c.id : null;
      this.draft = c ? {
        name: c.name,
        document: WTT.fmt.cnpj(c.document),
        contact_name: c.contact_name,
        contact_email: c.contact_email,
        contact_phone: c.contact_phone,
      } : blankCustomer();
      this.errors.save = '';
      // Enquanto salva, o modal não fecha: um erro do servidor ficaria sem ter onde aparecer.
      Alpine.store('modal').open('customer', c ? WTT.t('org.customers.edit_title') : WTT.t('org.customers.new'), () => !this.pending);
    },
    save() {
      return this.run('save', async () => {
        if (this.editing) {
          const saved = await api('PATCH', '/api/customers/' + this.editing, this.draft);
          this.customers = this.customers.map((c) => (c.id === saved.id ? saved : c));
          toast(WTT.t('org.customers.saved'));
        } else {
          this.customers = [...this.customers, await api('POST', '/api/orgs/' + orgId + '/customers', this.draft)];
          toast(WTT.t('org.customers.created'));
        }
        this.customers.sort((a, b) => a.name.localeCompare(b.name, WTT.lang));
        Alpine.store('modal').close();
      });
    },
    remove(c) {
      return this.run('remove', async () => {
        this.confirming = null;
        await api('DELETE', '/api/customers/' + c.id);
        this.customers = this.customers.filter((x) => x.id !== c.id);
        toast(WTT.t('org.customers.deleted'));
      });
    },
  }));

  Alpine.data('profileSettings', () => ({
    ...form(),
    profile: { name: me.name, email: me.email },
    password: { current: '', next: '' },
    rates: [],
    ratesLoaded: false,
    weeklyHours: undefined, // a jornada da pessoa; null quando nenhum admin informou
    async init() {
      // A jornada é só para ler: sem ela, o cartão deixa de mostrar a linha.
      api('GET', '/api/persons/' + me.id).then((p) => { this.weeklyHours = p.weekly_hours; }).catch(() => {});
      try {
        this.rates = (await api('GET', '/api/persons/' + me.id + '/allocations')) || [];
      } catch (e) {
        this.errors.rates = e.message;
      } finally {
        this.ratesLoaded = true;
      }
    },
    saveProfile() {
      return this.run('profile', async () => {
        const p = await api('PATCH', '/api/persons/' + me.id, this.profile);
        this.profile = { name: p.name, email: p.email };
        setText('[data-me-name]', p.name);
        toast(WTT.t('profile.personal.saved'));
      });
    },
    changePassword() {
      return this.run('password', async () => {
        await api('POST', '/api/auth/password', { current_password: this.password.current, new_password: this.password.next });
        this.password = { current: '', next: '' };
        toast(WTT.t('profile.password.changed'));
      });
    },
  }));
});
