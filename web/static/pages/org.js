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

  // Os documentos fiscais vêm do cadastro de países ('cnpj', 'ein'): um país novo com documento novo entra sozinho.
  const legalFields = WTT.countries.list().map((c) => c.legal_id.field);
  const orgTexts = [
    'name', 'summary', 'description',
    'website', 'contact_email', 'linkedin_url',
    'legal_name', ...legalFields, 'address_line1', 'address_line2', 'country',
    'work_mode', 'timezone', 'currency',
  ];

  // orgForm copia a organização para o formulário: campo sem valor vira texto vazio. Cada documento fiscal aparece com a
  // máscara do país dono dele; o servidor guarda sem máscara e o confere de novo.
  function orgForm(o) {
    const f = {};
    orgTexts.forEach((k) => { f[k] = o[k] || ''; });
    WTT.countries.list().forEach((c) => { f[c.legal_id.field] = WTT.fmt.mask(f[c.legal_id.field], c.legal_id.mask); });
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

  // projectCreation é o modal de novo projeto da página Projetos (o formulário é o parcial project_form) e a criação.
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

  const PROJECTS_PER_PAGE = 6;
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

  // O painel de cada pessoa na página inicial, para todos os papéis: as horas de hoje e da semana e as
  // tarefas dela em todos os projetos (os números vêm de /me/overview, e a lista, cinco por página, de
  // /me/tasks). Relê sozinho de minuto em minuto, enquanto a aba está à vista, e ao voltar a ela; um
  // refresh que falha deixa os últimos números. Quem usa tem `...form()` (errors) e chama `initMine()`.
  const HOME_TASKS_PER_PAGE = 5;
  const MINE_REFRESH_MS = 60 * 1000;
  const homeMine = () => ({
    mine: null, // o painel; null até a primeira resposta
    myState: 'open', // a lista mostra as tarefas abertas ou as concluídas
    myTasks: [],
    myTotal: 0,
    myPage: 1,
    myLoading: true,
    initMine() {
      this.loadMine();
      this.loadMyTasks();
      const refresh = () => {
        if (document.visibilityState !== 'visible') return;
        this.loadMine();
        this.loadMyTasks();
      };
      setInterval(refresh, MINE_REFRESH_MS);
      document.addEventListener('visibilitychange', refresh);
    },
    async loadMine() {
      // O fuso de quem olha decide onde o dia e a semana começam.
      const tz = encodeURIComponent(Intl.DateTimeFormat().resolvedOptions().timeZone || '');
      try {
        this.mine = await api('GET', '/api/orgs/' + orgId + '/me/overview?tz=' + tz);
        this.errors.mine = '';
      } catch (e) {
        if (!this.mine) this.errors.mine = e.message;
      }
    },
    async loadMyTasks() {
      try {
        const res = await api('GET', '/api/orgs/' + orgId + '/me/tasks?state=' + this.myState + '&page=' + this.myPage + '&per_page=' + HOME_TASKS_PER_PAGE);
        this.myTasks = res.items || [];
        this.myTotal = res.total;
        this.myPage = res.page;
        this.errors.myTasks = '';
      } catch (e) {
        if (this.myLoading) this.errors.myTasks = e.message;
      } finally {
        this.myLoading = false;
      }
    },
    setMyState(state) {
      if (this.myState === state) return;
      this.myState = state;
      this.myPage = 1;
      this.myTasks = [];
      this.myLoading = true;
      this.loadMyTasks();
    },
    goMyPage(page) {
      this.myPage = page;
      return this.loadMyTasks();
    },
    myPages() {
      return Math.max(1, Math.ceil(this.myTotal / HOME_TASKS_PER_PAGE));
    },
    myRange() {
      const from = this.myTotal === 0 ? 0 : (this.myPage - 1) * HOME_TASKS_PER_PAGE + 1;
      return WTT.t('home.team.range', { from, to: Math.min(this.myTotal, this.myPage * HOME_TASKS_PER_PAGE), total: this.myTotal });
    },

    // Os quatro números. Cada um tem o valor e a dica que o acompanha. Menos de um minuto é "0min": segundos
    // soltos num painel de horas só atrapalham.
    hoursText: (seconds) => (seconds < 60 ? '0min' : WTT.fmt.hours(seconds)),
    todayHint() {
      const on = this.mine.working_on;
      if (!this.mine.working_now) return WTT.t('home.team.idle');
      if (on.length === 0) return WTT.t('home.team.no_task');
      return on[0].task.name + ' · ' + on[0].project.name;
    },
    weekHint() {
      const goal = this.mine.weekly_hours;
      if (!goal) return WTT.t('home.me.week_hint');
      return WTT.t('home.me.week_goal', { percent: Math.round(this.mine.hours.week_seconds / (goal * 3600) * 100), hours: goal });
    },
    openHint() {
      const t = this.mine.tasks;
      if (t.overdue > 0) return WTT.t('home.me.open_overdue', { count: t.overdue });
      if (t.due_this_week > 0) return WTT.t('home.me.open_week', { count: t.due_this_week });
      return t.open > 0 ? WTT.t('home.me.open_none_late') : WTT.t('home.me.open_empty');
    },
    doneHint() {
      const t = this.mine.tasks;
      return t.total > 0 ? WTT.t('home.me.done_share', { percent: Math.round(t.closed / t.total * 100) }) : WTT.t('home.me.done_empty');
    },

    // O que a lista diz quando está vazia: a pessoa nunca teve tarefa, ou não tem nenhuma neste estado.
    myEmpty() {
      if (this.mine && this.mine.tasks.total === 0) return { title: WTT.t('home.me.empty_none'), text: WTT.t('home.me.empty_none_text') };
      return this.myState === 'closed'
        ? { title: WTT.t('home.me.empty_closed'), text: WTT.t('home.me.empty_closed_text') }
        : { title: WTT.t('home.me.empty_open'), text: WTT.t('home.me.empty_open_text') };
    },

    // A semana em sete colunas, de segunda a domingo. A escala é a do maior dia, nunca menos de quatro
    // horas, para um dia curto não parecer cheio. Os dias que ainda não chegaram ficam sem barra.
    bars() {
      const days = this.mine.hours.days;
      const today = new Date(this.mine.generated_at).toLocaleDateString('sv-SE', { timeZone: this.mine.timezone }); // AAAA-MM-DD
      const scale = Math.max(4 * 3600, ...days.map((d) => d.seconds));
      return days.map((d) => {
        const at = new Date(d.date + 'T12:00:00');
        const name = at.toLocaleDateString(WTT.lang, { weekday: 'long', day: 'numeric', month: 'short' });
        return {
          date: d.date,
          short: at.toLocaleDateString(WTT.lang, { weekday: 'short' }).replace('.', ''),
          value: WTT.fmt.hoursShort(d.seconds),
          height: d.seconds > 0 ? Math.max(4, Math.round(d.seconds / scale * 100)) : 0,
          today: d.date === today,
          label: WTT.t('home.me.day_label', { day: name, time: d.seconds > 0 ? WTT.fmt.hours(d.seconds) : WTT.t('home.me.day_none') }),
        };
      });
    },
    // A tarefa fechada mostra só a data do prazo: "atrasada" não se aplica ao que já terminou.
    taskDeadlineClass: (t) => (t.status === 'closed' || !t.deadline ? '' : WTT.deadlineInfo(t.deadline).cls),
    taskDeadlineLabel: (t) => (t.status === 'closed' && WTT.hasDeadlineDate(t.deadline) ? WTT.fmt.date(t.deadline) : WTT.deadlineInfo(t.deadline).label),
    priorityClass: WTT.priorityClass,
    statusClass: WTT.statusClass,
  });

  // A página Projetos: os projetos de quem olha (todos, para os admins) em cartões, seis por página.
  Alpine.data('orgProjects', () => ({
    ...projectCreation(),
    loading: true,
    projects: [],
    total: 0,
    page: 1,
    hueOf,
    markOf,
    init() {
      const wanted = parseInt(new URLSearchParams(location.search).get('page'), 10);
      this.page = wanted > 0 ? wanted : 1;
      this.loadProjects();
    },
    // Uma página por vez, vinda do servidor, que corrige uma página que não existe mais.
    loadProjects() {
      return this.run('page', async () => {
        this.errors.load = '';
        try {
          const res = await api('GET', '/api/orgs/' + orgId + '/projects?page=' + this.page + '&per_page=' + PROJECTS_PER_PAGE);
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
      return Math.max(1, Math.ceil(this.total / PROJECTS_PER_PAGE));
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
  }));

  // A página inicial: para admins, a visão geral da organização (tempo e dinheiro de todos os
  // projetos) e a equipe; para todos, o painel da própria pessoa.
  Alpine.data('orgHome', () => ({
    ...form(),
    ...homeMine(),
    stats: null,
    statsLoading: false,
    period: 'last_30_days',
    teamAt: 1,
    periods: [
      { key: 'last_7_days', label: WTT.t('home.stats.last_7') },
      { key: 'last_30_days', label: WTT.t('home.stats.last_30') },
      { key: 'all_time', label: WTT.t('home.stats.all_time') },
    ],
    initialsOf,
    init() {
      this.initMine();
      if (me.role === 'admin') this.loadStats();
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

    // Quem está com o ponto aberto agora, por nome, sem quem abriu a página (o painel dela é o de baixo).
    // Não há ordem por horas de propósito (veja teamHours).
    teamRows() {
      const me = window.BOOT && window.BOOT.me && window.BOOT.me.id;
      return this.stats.by_person.filter((p) => p.working_now && p.person.id !== me).sort((a, b) => a.person.name.localeCompare(b.person.name));
    },
    teamView() {
      return pageOf(this.teamRows(), this.teamAt, HOME_TEAM_PER_PAGE);
    },
    // As outras tarefas da sessão, além da que aparece na linha, para a dica de quem tem mais de uma.
    moreTasks(p) {
      return p.working_on.slice(1).map((w) => w.task.name + ' · ' + w.project.name).join('\n');
    },
  }));

  // personEditor é o modal Editar colaborador (o parcial person_edit_modal), que a lista de colaboradores e o perfil de
  // um deles têm em comum: o estado do rascunho e o Salvar. Cada componente o espalha no seu objeto, junto do form().
  const personEditor = () => ({
    me,
    editing: null, // a pessoa aberta no modal do colaborador
    tab: 'payment', // a aba do modal: payment, permissions ou projects
    tabs: [], // as abas que quem olha pode usar
    // O rascunho do modal: nada vai para o servidor antes de Salvar. projects são as linhas da aba de projetos:
    // { project_id, name, cents (o valor gravado, null na linha nova), rate (o texto do campo), isNew, removed }.
    draft: { weekly_hours: '', payment: { frequency: '', day: '', start: '' }, permissions: [], projects: [] },
    addForm: { project_id: '', rate: '' }, // o projeto e o valor escolhidos para a pessoa entrar
    allProjects: null, // os projetos da organização, buscados na primeira vez que o modal abre para um admin
    projectsState: 'idle', // idle, loading, ready ou error
    orgKeys: [], // as permissões da organização do catálogo, para o dono liberar
    // As permissões da organização só se liberam a quem não é admin, e só o dono as dá.
    canGrant(person) {
      return !!me.is_owner && !!person && person.role !== 'admin' && !person.is_owner;
    },
    // availableTabs são as abas que quem olha pode usar: a jornada (people.manage), as permissões da organização
    // (só o dono) e os projetos com o valor por hora (só admins, que têm todas as permissões de projeto). Com uma só,
    // o modal não mostra a faixa de abas.
    availableTabs() {
      return [
        WTT.can('people.manage') && 'payment',
        me.is_owner && 'permissions',
        me.role === 'admin' && 'projects',
      ].filter(Boolean);
    },
    // moveTab é a troca pelas setas, Home e End: o foco acompanha a aba escolhida.
    moveTab(to) {
      const last = this.tabs.length - 1;
      const at = this.tabs.indexOf(this.tab);
      const next = { first: 0, last, next: at >= last ? 0 : at + 1, prev: at <= 0 ? last : at - 1 }[to];
      this.tab = this.tabs[next];
      this.$nextTick(() => this.$refs['tab-' + this.tab].focus());
    },
    // openEdit abre o modal do colaborador: a jornada, as permissões e os projetos da pessoa. Ele edita um rascunho:
    // nada vai para o servidor antes de Salvar. O papel muda direto na linha, pelo botão.
    openEdit(person) {
      this.editing = person;
      this.tabs = this.availableTabs();
      this.tab = this.tabs[0] || 'payment';
      const rule = person.payment || {};
      this.draft = { weekly_hours: person.weekly_hours || '', payment: { frequency: rule.frequency || '', day: rule.day || '', start: rule.start || '' }, permissions: [...(person.permissions || [])], projects: [] };
      this.addForm = { project_id: '', rate: '' };
      this.projectsState = 'idle';
      this.errors.edit = '';
      this.errors.projects = '';
      Alpine.store('modal').open('person-edit', WTT.t('org.people.edit_title'), () => !this.pending);
      if (this.tabs.includes('projects')) this.loadProjects(person);
    },
    // loadProjects traz os projetos da pessoa com o valor dela em cada um e, na primeira vez, os projetos da organização,
    // para o seletor de entrar em outro. A aba mostra o carregando e o erro; as outras seguem usáveis.
    async loadProjects(person) {
      this.projectsState = 'loading';
      this.errors.projects = '';
      try {
        const [allocations, projects] = await Promise.all([
          api('GET', '/api/persons/' + person.id + '/allocations'),
          this.allProjects || api('GET', '/api/orgs/' + orgId + '/projects'),
        ]);
        if (this.editing !== person) return; // o modal fechou, ou abriu outra pessoa, enquanto carregava
        this.allProjects = projects || [];
        this.draft.projects = (allocations || []).map((a) => ({
          project_id: a.project_id,
          name: a.project ? a.project.name : a.project_id,
          cents: a.pay_rate_cents,
          rate: WTT.fmt.moneyInput(a.pay_rate_cents),
          isNew: false,
          removed: false,
        }));
        this.projectsState = 'ready';
      } catch (e) {
        if (this.editing !== person) return;
        this.errors.projects = e.message;
        this.projectsState = 'error';
      }
    },
    // addableProjects são os projetos da organização em que a pessoa ainda não está. O que está marcado para sair
    // continua na lista até o Salvar, e o botão Desfazer o devolve.
    addableProjects() {
      const taken = new Set(this.draft.projects.map((r) => r.project_id));
      return (this.allProjects || []).filter((p) => !taken.has(p.id));
    },
    // addProject põe a linha do projeto escolhido no rascunho. O dono entra sem valor: as horas dele valem o valor cobrado.
    addProject() {
      const project = (this.allProjects || []).find((p) => p.id === this.addForm.project_id);
      if (!project) {
        this.errors.edit = WTT.t('org.people.choose_project');
        return;
      }
      const owner = !!this.editing.is_owner;
      const cents = owner ? 0 : WTT.toCents(this.addForm.rate);
      if (cents === null) {
        this.errors.edit = WTT.t('collab.rate_required');
        return;
      }
      this.errors.edit = '';
      this.draft.projects.push({
        project_id: project.id, name: project.name, cents: null,
        rate: owner ? '' : WTT.fmt.moneyInput(cents), isNew: true, removed: false,
      });
      this.addForm = { project_id: '', rate: '' };
    },
    // toggleRemove marca o projeto para a pessoa sair ao salvar, ou desfaz a marca. Uma linha que ainda não foi gravada
    // simplesmente some do rascunho.
    toggleRemove(row) {
      if (row.isNew) this.draft.projects = this.draft.projects.filter((r) => r.project_id !== row.project_id);
      else row.removed = !row.removed;
    },
    // projectChanges separa o que o rascunho mudou nos projetos e confere os valores antes de qualquer envio: um valor
    // que não é número para tudo, e leva para a aba dos projetos.
    projectChanges() {
      if (this.projectsState !== 'ready') return [];
      const owner = !!this.editing.is_owner;
      const changes = [];
      for (const row of this.draft.projects) {
        if (row.removed) {
          changes.push({ row, remove: true });
        } else if (owner) {
          if (row.isNew) changes.push({ row, cents: 0 });
        } else {
          const cents = WTT.toCents(row.rate);
          if (cents === null) {
            this.tab = 'projects';
            throw new Error(WTT.t('org.people.rate_invalid', { project: row.name }));
          }
          if (row.isNew || cents !== row.cents) changes.push({ row, cents });
        }
      }
      return changes;
    },
    // paymentBody valida a regra do rascunho (ou normaliza a que a pessoa já tem) e devolve o corpo do PATCH. Um erro
    // troca para a aba e nomeia o campo.
    paymentBody(rule) {
      const d = rule ? { frequency: rule.frequency || '', day: rule.day || '', start: rule.start || '' } : this.draft.payment;
      if (d.frequency === 'monthly') {
        const day = Number(String(d.day).trim());
        if (String(d.day).trim() === '' || !Number.isInteger(day) || day < 1 || day > 31) {
          if (!rule) { this.tab = 'payment'; throw new Error(WTT.t('errors.person.invalid_payment_day')); }
        }
        return { frequency: 'monthly', day };
      }
      if (d.frequency === 'biweekly') {
        if (!/^\d{4}-\d{2}-\d{2}$/.test(d.start || '')) {
          if (!rule) { this.tab = 'payment'; throw new Error(WTT.t('errors.person.invalid_payment_start')); }
        }
        return { frequency: 'biweekly', start: d.start };
      }
      return { frequency: '' };
    },
    // savePerson aplica o rascunho com as rotas que já existiam: a jornada, as permissões e, projeto a projeto, o valor
    // ou a saída. Cada chamada que dá certo vira o novo ponto de partida; se uma falhar, o que já foi aplicado continua
    // valendo e o modal fica aberto com o erro. Salvar de novo só repete o que faltou.
    savePerson() {
      return this.run('edit', async () => {
        const person = this.editing;
        const text = String(this.draft.weekly_hours).trim();
        const hours = text === '' ? 0 : Number(text);
        if (!Number.isInteger(hours) || hours < 0 || hours > 168) {
          this.tab = 'payment';
          throw new Error(WTT.t('errors.person.invalid_week_hours'));
        }
        const pay = this.paymentBody();
        const projects = this.projectChanges();
        if (WTT.can('people.manage') && hours !== (person.weekly_hours || 0)) {
          const updated = await api('PATCH', '/api/persons/' + person.id + '/weekly-hours', { weekly_hours: hours });
          person.weekly_hours = updated.weekly_hours;
        }
        if (WTT.can('people.manage') && JSON.stringify(pay) !== JSON.stringify(this.paymentBody(person.payment || {}))) {
          const updated = await api('PATCH', '/api/persons/' + person.id + '/payment', pay);
          person.payment = updated.payment;
        }
        const before = [...(person.permissions || [])].sort().join();
        if (this.canGrant(person) && [...this.draft.permissions].sort().join() !== before) {
          const updated = await api('PATCH', '/api/persons/' + person.id + '/permissions', { permissions: this.draft.permissions });
          person.permissions = updated.permissions;
        }
        for (const { row, remove, cents } of projects) {
          const base = '/api/projects/' + row.project_id;
          try {
            if (remove) {
              await api('DELETE', base + '/collaborators/' + person.id);
              this.draft.projects = this.draft.projects.filter((r) => r.project_id !== row.project_id);
            } else {
              const saved = await api('PUT', base + '/allocations/' + person.id, { pay_rate_cents: cents });
              row.cents = saved.pay_rate_cents;
              row.isNew = false;
            }
          } catch (e) {
            this.tab = 'projects';
            throw new Error(row.name + ': ' + e.message);
          }
        }
        toast(WTT.t('org.people.saved', { name: person.name }));
        Alpine.store('modal').close();
        // A página que abriu o modal diz o que fazer depois (o perfil relê os números).
        if (this.afterPersonSaved) await this.afterPersonSaved(person);
      });
    },
  });

  Alpine.data('orgPeople', () => ({
    ...form(),
    ...personEditor(),
    loading: true,
    people: [],
    invites: [],
    invite: { email: '', role: 'member' },
    lastLink: '',
    // Como o último convite chegou (email, terminal ou link) e para quem, para o modal dizer o que aconteceu.
    lastDelivery: '',
    lastEmail: '',
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
    // profileHref é o perfil da pessoa: para os admins, a linha da lista leva até ele.
    profileHref(person) {
      return '/orgs/' + orgId + '/people/' + person.id;
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

  // personProfile é o que o perfil de uma pessoa mostra (o parcial person_profile), igual no meu perfil e no de um
  // colaborador aberto por um admin: quem ela é, as horas e o valor do período e desde o início, por projeto, e os
  // pagamentos calculados da regra dela. self diz se é o perfil de quem olha, para os textos.
  const personProfile = (id, self) => ({
    self,
    person: null,
    detail: null, // os pagamentos da pessoa: o período corrente, os próximos, o histórico e o "desde o início"
    rates: [], // o valor por hora dela em cada projeto
    profileLoading: true,
    // loadProfile lê tudo de uma vez; os números valem para a hora em que chegam.
    async loadProfile() {
      this.profileLoading = true;
      this.errors.load = '';
      try {
        const [person, detail, rates] = await Promise.all([
          api('GET', '/api/persons/' + id),
          api('GET', '/api/persons/' + id + '/payments'),
          api('GET', '/api/persons/' + id + '/allocations'),
        ]);
        this.person = person;
        this.detail = detail;
        this.rates = rates || [];
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.profileLoading = false;
      }
    },
    // "40h por semana · Mensal · dia 5": a jornada e a regra de pagamento. O dono não é pago, então não tem regra a mostrar.
    summaryLine() {
      const p = this.person;
      if (!p) return '';
      const parts = [WTT.fmt.weeklyHours(p.weekly_hours) || WTT.t('profile.no_weekly_hours')];
      if (!p.is_owner) parts.push(WTT.fmt.paymentRule(p.payment) || WTT.t('profile.no_payment_rule'));
      return parts.join(' · ');
    },
    // projectRows junta, por projeto, o valor por hora (a alocação), o que a pessoa fez no período corrente e desde o
    // início, em ordem de nome. Um projeto com horas e sem alocação é um de que ela saiu.
    projectRows() {
      if (!this.detail) return [];
      const rows = new Map();
      const row = (projectId, name) => {
        if (!rows.has(projectId)) rows.set(projectId, { id: projectId, name, allocated: false, rate: null, period: null, lifetime: null });
        return rows.get(projectId);
      };
      this.rates.forEach((a) => {
        const r = row(a.project_id, a.project ? a.project.name : a.project_id);
        r.allocated = true;
        r.rate = a.pay_rate_cents;
      });
      ((this.detail.current && this.detail.current.projects) || []).forEach((p) => { row(p.project.id, p.project.name).period = p; });
      this.detail.lifetime.projects.forEach((p) => { row(p.project.id, p.project.name).lifetime = p; });
      return [...rows.values()].sort((a, b) => a.name.localeCompare(b.name));
    },
    // historyRows são os períodos fechados que a tela lista: os que terminam do dia da primeira sessão em diante. Os de
    // antes, em que a pessoa ainda não registrava horas, vêm zerados do servidor e só encheriam a tabela.
    historyRows() {
      const first = this.detail.lifetime.first_day;
      return first ? this.detail.history.filter((h) => h.pay_date >= first) : [];
    },
    // "8h 30min · R$ 400,00": as horas e, para quem é pago, o valor. Sem nada no projeto, um traço.
    shareText(share) {
      if (!share) return '—';
      const hours = WTT.fmt.hours(share.seconds);
      return this.detail.owner || share.amount_cents === null || share.amount_cents === undefined ? hours : hours + ' · ' + WTT.fmt.money(share.amount_cents);
    },
    // A dica do cartão "Desde o início": o valor (de quem é pago) e o dia da primeira sessão.
    // O período que o card Próximo pagamento mostra: o corrente ou, se a contagem da regra ainda não começou, o primeiro
    // que vem, zerado (as horas, o valor e, para os admins, a receita).
    nx() {
      const d = this.detail;
      if (!d || !d.rule) return null;
      if (d.current) return d.current;
      const up = d.upcoming && d.upcoming[0];
      if (!up) return null;
      const hours = this.person && this.person.weekly_hours;
      const v = { ...up, seconds: 0, amount_cents: 0, projects: [], days_left: undefined };
      if (hours) v.goal_seconds = hours * 3600 * up.days / 7;
      if (WTT.boot.me.role === 'admin') { v.revenue_cents = 0; v.margin_cents = 0; }
      return v;
    },
    lifetimeHint() {
      const life = this.detail.lifetime;
      const parts = [];
      if (!this.detail.owner && life.amount_cents !== null) parts.push(WTT.fmt.money(life.amount_cents));
      parts.push(life.first_day ? WTT.t('profile.lifetime_since', { date: WTT.fmt.day(life.first_day) }) : WTT.t('profile.lifetime_empty'));
      return parts.join(' · ');
    },
  });

  // O perfil de um colaborador, aberto por um admin: o que a pessoa vê no dela, com o modal Editar colaborador.
  Alpine.data('orgPerson', () => ({
    ...form(),
    ...personEditor(),
    ...personProfile((WTT.boot.person || {}).id, false),
    init() {
      if (me.is_owner) api('GET', '/api/permissions').then((catalog) => { this.orgKeys = catalog.organization; }).catch(() => {});
      return this.loadProfile();
    },
    // A jornada e a regra mudam os períodos, e os projetos mudam a tabela: depois de salvar, tudo é relido.
    afterPersonSaved() {
      return this.loadProfile();
    },
  }));

  Alpine.data('orgSettings', () => ({
    ...form(),
    form: orgForm(org),
    timezones: timezoneOptions(org.timezone),
    confirmDelete: false,
    countryOptions: WTT.countries.list().map((c) => ({ code: c.code, name: WTT.countries.name(c.code) })),
    // profile são as regras do país escolhido no formulário: o documento fiscal e os textos do endereço.
    get profile() {
      return WTT.countries.get(this.form.country) || WTT.countries.list()[0];
    },
    maskLegal() {
      const id = this.profile.legal_id;
      this.form[id.field] = WTT.fmt.mask(this.form[id.field], id.mask);
    },
    saveOrg() {
      return this.run('org', async () => {
        // Texto vazio apaga o campo; a API mantém o que não vier no corpo.
        const body = {};
        orgTexts.forEach((k) => { body[k] = this.form[k]; });
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
        [WTT.t('org.settings.country'), org.country && WTT.countries.name(org.country)],
      ]);
    },
    contact() {
      return rows([
        [WTT.t('org.fields.website'), bareURL(org.website), { href: org.website, external: true }],
        [WTT.t('common.email'), org.contact_email, { href: 'mailto:' + org.contact_email }],
        [WTT.t('org.fields.linkedin'), bareURL(org.linkedin_url), { href: org.linkedin_url, external: true }],
      ]);
    },
    legal() {
      // O documento fiscal é o do país da organização; o de outro país, se há, fica guardado e escondido.
      const id = WTT.countries.legalId(org.country);
      return rows([
        [WTT.t('org.fields.legal_name'), org.legal_name],
        [id.label, WTT.fmt.mask(org[id.field], id.mask), { mono: true }],
        [WTT.t('org.fields.address'), [org.address_line1, org.address_line2].filter(Boolean).join(' · ')],
      ]);
    },
    defaults() {
      return rows([
        [WTT.t('org.about.work_mode'), WTT.fmt.workMode(org.work_mode)],
        [WTT.t('org.fields.timezone'), org.timezone],
        [WTT.t('org.fields.currency'), WTT.fmt.currency(org.currency)],
      ]);
    },
    // O país, o fuso e a moeda sempre têm valor, então não contam como perfil preenchido.
    isEmpty() {
      return !org.description && [...this.contact(), ...this.legal()].every((f) => f.empty);
    },
  }));

  // O cliente novo começa no país da organização.
  const blankCustomer = () => ({ name: '', country: org.country || 'BR', document: '', contact_name: '', contact_email: '', contact_phone: '' });

  // customerDocument é o que o modal de cliente e a etapa dos clientes das boas-vindas têm em comum (o parcial
  // customer_document): a lista de países, o documento fiscal do país escolhido (rótulo, máscara, placeholder) e a
  // máscara aplicada enquanto se digita. model é o nome do objeto do cliente no componente ('draft' ou 'customer').
  const customerDocument = () => ({
    // Os países do cadastro primeiro, e depois todos os outros por nome: o cliente pode ser de qualquer lugar.
    customerCountries: WTT.countries.list().map((c) => ({ code: c.code, name: WTT.countries.name(c.code) })).concat(WTT.countries.others()),
    docId(code) { return WTT.countries.legalId(code); },
    maskDoc(model) {
      const id = WTT.countries.legalId(this[model].country);
      if (id.mask) this[model].document = WTT.fmt.mask(this[model].document, id.mask);
    },
    // O documento de um país não vale em outro: trocar o país esvazia o campo.
    docCountryChanged(model) { this[model].document = ''; },
  });

  Alpine.data('orgCustomers', () => ({
    ...form(),
    ...customerDocument(),
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
        country: c.country,
        document: WTT.fmt.taxId(c.document, c.country),
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

  // As boas-vindas do primeiro acesso (partials/onboarding.gohtml): quatro etapas num modal que abre sozinho na
  // página inicial do dono que acabou de criar a organização. Cada etapa guarda o que a pessoa fez, para a última
  // dizer o que ficou para depois. Fechar de qualquer jeito (Concluir, Esc, o X ou o fundo) dá baixa nas boas-vindas
  // no servidor, e elas não voltam; recarregar no meio, sem fechar, recomeça da etapa 1, já com o que foi salvo.
  Alpine.data('onboarding', () => ({
    ...form(),
    ...customerDocument(),
    step: 1,
    orgName: me.organization_name,
    timezones: timezoneOptions(org.timezone),
    orgDraft: {
      summary: org.summary || '',
      work_mode: org.work_mode || '',
      timezone: org.timezone,
      currency: org.currency,
    },
    customer: blankCustomer(),
    invite: { email: '', role: 'member' },
    lastLink: '',
    lastDelivery: '',
    lastEmail: '',
    saved: { org: false, customer: false },
    invited: 0,
    shown: false,
    completing: null,
    init() {
      // Depois de o Alpine montar o modal no #modal-root.
      this.$nextTick(() => this.show());
      this.$watch('$store.modal.isOpen', (open) => {
        if (!open && this.shown) this.closed();
      });
    },
    titles() {
      return [WTT.t('onboarding.org.title'), WTT.t('onboarding.customers.title'), WTT.t('onboarding.people.title'), WTT.t('onboarding.done.title')];
    },
    show() {
      this.shown = true;
      // Enquanto grava, o modal não fecha: um erro do servidor ficaria sem ter onde aparecer.
      Alpine.store('modal').open('onboarding', this.titles()[0], () => !this.pending);
      this.focusField();
    },
    // focusField põe o foco no primeiro campo da etapa atual (o data-step dele). O modal abre durante o carregamento
    // da página, com o painel ainda em transição, e na troca de etapa a anterior só some depois: o campo só aceita
    // foco quando está à vista, por isso a tentativa se repete por um instante. O campo é o da etapa, e não "o primeiro
    // visível", senão o da etapa que está saindo contaria como pronto.
    focusField(tries = 0) {
      const field = document.querySelector('.modal-panel [data-autofocus][data-step="' + this.step + '"]');
      const ready = !!field && !!field.offsetParent;
      if (ready) field.focus();
      if ((!ready || document.activeElement !== field) && tries < 60) setTimeout(() => this.focusField(tries + 1), 50);
    },
    // go muda de etapa, troca o título do modal e põe o foco no primeiro campo dela.
    go(step) {
      this.step = step;
      Alpine.store('modal').title = this.titles()[step - 1];
      // Quem chegou à última etapa pode sair pelos links dela: a baixa vai antes, não ao fechar.
      if (step === 4) this.complete();
      this.$nextTick(() => this.focusField());
    },
    next() { this.go(this.step + 1); },
    saveOrg() {
      return this.run('org', async () => {
        // A API mantém o que não vier no corpo, então o resto do perfil fica como está.
        await api('PATCH', '/api/orgs/' + orgId, { ...this.orgDraft });
        this.saved.org = true;
        this.next();
      });
    },
    saveCustomer() {
      return this.run('customer', async () => {
        await api('POST', '/api/orgs/' + orgId + '/customers', this.customer);
        this.saved.customer = true;
        toast(WTT.t('org.customers.created'));
        this.next();
      });
    },
    // O convite fica na etapa depois de criado: o link só aparece agora, e quem convidou pode gerar outro.
    createInvite() {
      return this.run('invite', async () => {
        const inv = await api('POST', '/api/orgs/' + orgId + '/invites', { email: this.invite.email, role: this.invite.role });
        this.lastLink = location.origin + inv.path;
        this.lastDelivery = inv.delivery;
        this.lastEmail = inv.email || '';
        this.invited += 1;
        this.$nextTick(() => this.$refs.link && this.$refs.link.focus());
      });
    },
    anotherInvite() {
      this.invite = { email: '', role: 'member' };
      this.lastLink = '';
      this.lastDelivery = '';
      this.lastEmail = '';
      this.errors.invite = '';
      this.$nextTick(() => document.getElementById('ob-invite-email')?.focus());
    },
    async copyLink() {
      const ok = await WTT.copyText(this.lastLink);
      toast(ok ? WTT.t('org.people.link_copied') : WTT.t('org.people.copy_failed'), ok ? 'info' : 'error');
    },
    // complete dá baixa nas boas-vindas uma vez só. Se falhar, elas voltam no próximo acesso: não vale travar a tela.
    complete() {
      if (!this.completing) this.completing = api('POST', '/api/auth/onboarding/complete').catch(() => {});
      return this.completing;
    },
    finish() { Alpine.store('modal').close(); },
    async closed() {
      this.shown = false;
      await this.complete();
      // A moeda, o fuso e o resumo mudaram: a página inicial os mostra, então recarrega com os novos.
      if (this.saved.org) location.reload();
    },
  }));

  // O meu perfil: o que fiz e quando sou pago, para ler, e o modal Editar perfil, com os dados pessoais e a senha.
  Alpine.data('profileSettings', () => ({
    ...form(),
    ...personProfile(me.id, true),
    profile: { name: me.name, email: me.email },
    password: { current: '', next: '' },
    tab: 'personal', // a aba do modal: personal ou password
    init() {
      return this.loadProfile();
    },
    // openProfileEdit abre o modal com os dados de agora e a senha em branco.
    openProfileEdit() {
      const p = this.person || me;
      this.profile = { name: p.name, email: p.email };
      this.password = { current: '', next: '' };
      this.tab = 'personal';
      this.errors.profile = '';
      this.errors.password = '';
      Alpine.store('modal').open('profile-edit', WTT.t('profile.edit'), () => !this.pending);
    },
    focusTab(tab) {
      this.tab = tab;
      this.$nextTick(() => this.$refs['tab-' + tab].focus());
    },
    saveProfile() {
      return this.run('profile', async () => {
        const p = await api('PATCH', '/api/persons/' + me.id, this.profile);
        if (this.person) Object.assign(this.person, { name: p.name, email: p.email });
        setText('[data-me-name]', p.name);
        toast(WTT.t('profile.personal.saved'));
        Alpine.store('modal').close();
      });
    },
    changePassword() {
      return this.run('password', async () => {
        await api('POST', '/api/auth/password', { current_password: this.password.current, new_password: this.password.next });
        toast(WTT.t('profile.password.changed'));
        Alpine.store('modal').close();
      });
    },
  }));
});
