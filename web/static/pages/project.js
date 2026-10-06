// Componentes das abas de um projeto.
document.addEventListener('alpine:init', () => {
  const { api, form } = WTT;
  const me = WTT.boot.me;
  const project = WTT.boot.project;
  const toast = (msg, kind) => Alpine.store('toast').show(msg, kind);
  const clock = () => Alpine.store('clock');
  const DAY = 24 * 60 * 60 * 1000;

  // deadlineInfo descreve o prazo de uma tarefa para o badge: atrasada, vencendo
  // nas próximas 48 horas ou só a data.
  function deadlineInfo(iso) {
    if (!iso || new Date(iso).getFullYear() < 1971) return { label: 'Sem prazo', cls: '' };
    const diff = new Date(iso).getTime() - Date.now();
    if (diff < 0) return { label: 'Atrasada · ' + WTT.fmt.date(iso), cls: 'badge-danger' };
    if (diff < 2 * DAY) return { label: 'Vence ' + WTT.fmt.date(iso), cls: 'badge-warn' };
    return { label: WTT.fmt.date(iso), cls: '' };
  }

  // Atalhos do filtro de prazo da lista de tarefas. 'date' abre o campo de data.
  // "Qualquer prazo" fica fixo no template: uma opção de valor vazio criada por
  // x-for fica sem o atributo value e passa a valer o próprio rótulo.
  const dueOptions = [
    { value: 'overdue', label: 'Atrasadas' },
    { value: 'today', label: 'Até hoje' },
    { value: 'week', label: 'Até o fim desta semana' },
    { value: 'next_week', label: 'Até o fim da semana que vem' },
    { value: 'date', label: 'Até uma data…' },
  ];

  // dueLimit devolve, em ISO, o instante-limite de um atalho de prazo, ou null
  // quando ele não filtra nada. É calculado no fuso de quem está olhando, e a
  // semana termina no domingo, como em periodStart.
  function dueLimit(due, date) {
    const endOfDay = (d) => {
      d.setHours(23, 59, 59, 999);
      return d.toISOString();
    };
    const sunday = (weeksAhead) => {
      const d = new Date();
      d.setDate(d.getDate() + ((7 - d.getDay()) % 7) + 7 * weeksAhead);
      return endOfDay(d);
    };
    switch (due) {
      case 'overdue': return new Date().toISOString();
      case 'today': return endOfDay(new Date());
      case 'week': return sunday(0);
      case 'next_week': return sunday(1);
      case 'date': return date ? endOfDay(new Date(date + 'T00:00:00')) : null;
      default: return null;
    }
  }

  // Onde a lista de tarefas guarda os filtros e a página em uso, para o Voltar
  // do detalhe de uma tarefa cair no mesmo lugar.
  const tasksQueryKey = 'wtt:tasks:' + project.id;
  function tasksHref() {
    let query = '';
    try {
      query = sessionStorage.getItem(tasksQueryKey) || '';
    } catch (e) {
      // Sem sessionStorage o Voltar leva à primeira página, sem filtros.
    }
    return '/projects/' + project.id + '/tasks' + (query ? '?' + query : '');
  }

  // periodStart devolve o início de hoje ou desta semana (a partir de segunda).
  function periodStart(period) {
    const start = new Date(clock().now);
    start.setHours(0, 0, 0, 0);
    if (period === 'week') start.setDate(start.getDate() - ((start.getDay() + 6) % 7));
    return start.getTime();
  }

  // amountWithin soma quanto a pessoa ganhou depois de "since": o tempo de cada
  // sessão dentro do período vezes o valor por hora daquela sessão. Usa o mesmo
  // arredondamento do servidor, por sessão.
  function amountWithin(sessions, since) {
    return sessions.reduce((sum, s) => {
      if (s.pay_rate_cents === null || s.pay_rate_cents === undefined) return sum;
      return sum + Math.round(secondsWithin([s], since) * s.pay_rate_cents / 3600);
    }, 0);
  }

  const integrationNames = { github: 'GitHub', gitlab: 'GitLab' };
  function externalLabel(task) {
    const type = task.external_integration ? task.external_integration.type : '';
    return (integrationNames[type] || type || 'Item') + ' #' + task.external_item_id;
  }

  // secondsWithin soma quanto de cada sessão caiu depois de "since", contando
  // a sessão aberta até agora.
  function secondsWithin(sessions, since) {
    return sessions.reduce((sum, s) => {
      const start = Math.max(new Date(s.start_at).getTime(), since);
      const end = s.end_at ? new Date(s.end_at).getTime() : clock().now;
      return sum + Math.max(0, end - start) / 1000;
    }, 0);
  }

  Alpine.data('projectTasks', () => ({
    ...form(),
    loading: true,
    tasks: [], // só a página em uso; os filtros e a paginação rodam no servidor
    total: 0,
    page: 1,
    perPage: 10,
    members: [], // quem está nos times: pode ser responsável por tarefa nova
    assignees: [], // quem já é responsável por alguma tarefa, mesmo fora dos times
    // mine é a caixa "Só as minhas tarefas": ligada, prende o responsável em
    // quem está logado e desliga a busca e o filtro de responsável.
    filters: { q: '', assignee: '', due: '', date: '', mine: false },
    dueOptions,
    seq: 0,
    draft: { name: '', description: '', assignee_id: '', deadline: '' },
    async init() {
      this.readURL();
      const members = api('GET', '/api/projects/' + project.id + '/members')
        .then((list) => { this.members = list || []; })
        .catch((e) => { this.errors.members = e.message; });
      await Promise.all([members, this.load()]);
      this.loading = false;
    },
    // A URL guarda os filtros e a página, para recarregar ou voltar do detalhe
    // de uma tarefa sem perder o lugar. O prazo vai como o nome do atalho ou a
    // data escolhida, nunca como instante.
    readURL() {
      const p = new URLSearchParams(location.search);
      const due = p.get('due') || '';
      const isDate = /^\d{4}-\d{2}-\d{2}$/.test(due);
      const assignee = /^[0-9a-f-]{36}$/i.test(p.get('assignee') || '') ? p.get('assignee') : '';
      const mine = p.get('mine') === '1';
      this.filters = {
        q: mine ? '' : (p.get('q') || ''),
        assignee: mine ? me.id : assignee,
        due: isDate ? 'date' : (dueOptions.some((o) => o.value === due) ? due : ''),
        date: isDate ? due : '',
        mine,
      };
      this.page = Math.max(1, parseInt(p.get('page'), 10) || 1);
    },
    writeURL() {
      const f = this.filters;
      const p = new URLSearchParams();
      if (f.mine) {
        p.set('mine', '1');
      } else {
        if (f.q.trim()) p.set('q', f.q.trim());
        if (f.assignee) p.set('assignee', f.assignee);
      }
      const due = f.due === 'date' ? f.date : f.due;
      if (due) p.set('due', due);
      if (this.page > 1) p.set('page', this.page);
      const query = p.toString();
      history.replaceState(null, '', location.pathname + (query ? '?' + query : ''));
      try {
        sessionStorage.setItem(tasksQueryKey, query);
      } catch (e) {
        // Sem sessionStorage o Voltar da tarefa só deixa de lembrar os filtros.
      }
    },
    // load busca a página em uso. Não passa por run(), que descartaria uma
    // troca de filtro feita durante outra ação, e ignora a resposta de um
    // pedido mais antigo que o último.
    async load() {
      const seq = ++this.seq;
      const f = this.filters;
      const p = new URLSearchParams({ page: this.page });
      if (f.q.trim()) p.set('q', f.q.trim());
      if (f.assignee) p.set('assignee_id', f.assignee);
      const limit = dueLimit(f.due, f.date);
      if (limit) p.set('deadline_to', limit);
      try {
        const res = await api('GET', '/api/projects/' + project.id + '/tasks?' + p);
        if (seq !== this.seq) return;
        this.tasks = res.items || [];
        this.total = res.total;
        this.page = res.page; // o servidor devolve a última quando a pedida não existe mais
        this.perPage = res.per_page;
        this.assignees = res.assignees || [];
        this.errors.load = '';
        this.writeURL();
      } catch (e) {
        if (seq === this.seq) this.errors.load = e.message;
      }
    },
    // apply é o que os filtros chamam ao mudar: volta para a primeira página.
    apply() {
      this.page = 1;
      return this.load();
    },
    go(page) {
      this.page = page;
      return this.load();
    },
    clear() {
      this.filters = { q: '', assignee: '', due: '', date: '', mine: false };
      return this.apply();
    },
    // toggleMine liga ou desliga "Só as minhas tarefas". Ligada, a busca é
    // apagada e o responsável passa a ser quem está logado; os dois campos ficam
    // desligados na tela e só o prazo continua valendo. Desligada, tudo volta a
    // "Todos os responsáveis".
    toggleMine(on) {
      this.filters.mine = on;
      this.filters.q = '';
      this.filters.assignee = on ? me.id : '';
      return this.apply();
    },
    hasFilters() {
      const f = this.filters;
      return !!(f.q.trim() || f.assignee || dueLimit(f.due, f.date));
    },
    pages() {
      return Math.max(1, Math.ceil(this.total / this.perPage));
    },
    summary() {
      return 'Página ' + this.page + ' de ' + this.pages() + ' · ' + this.total + (this.total === 1 ? ' tarefa' : ' tarefas');
    },
    // Quem aparece no filtro de responsável: os times, quem tem tarefa aqui e a
    // própria pessoa, para o campo mostrar o nome dela com "Só as minhas" ligada.
    assigneeOptions() {
      const byId = new Map([[me.id, { id: me.id, name: me.name }]]);
      [...this.assignees, ...this.members].forEach((p) => byId.set(p.id, p));
      return [...byId.values()].sort((a, b) => a.name.localeCompare(b.name));
    },
    openCreate() {
      const self = this.members.find((m) => m.id === me.id) || this.members[0];
      this.draft = { name: '', description: '', assignee_id: self ? self.id : '', deadline: '' };
      this.errors.create = '';
      Alpine.store('modal').open('task-new', 'Nova tarefa', () => !this.pending);
    },
    create() {
      return this.run('create', async () => {
        const t = await api('POST', '/api/projects/' + project.id + '/tasks', {
          name: this.draft.name,
          description: this.draft.description,
          assignee_id: this.draft.assignee_id,
          deadline: WTT.fmt.fromDateInput(this.draft.deadline),
        });
        Alpine.store('modal').close();
        // A tarefa nova é a primeira da lista, se os filtros em uso a mostrarem.
        await this.apply();
        const shown = this.tasks.some((x) => x.id === t.id);
        toast(shown ? 'Tarefa criada.' : 'Tarefa criada. Os filtros em uso não a mostram.');
      });
    },
    isRunning(t) {
      const s = clock().session;
      return !!s && s.task_id === t.id;
    },
    start(t) {
      return this.run('clock', async () => {
        await clock().clockIn(project.id, t.id);
        toast('Ponto iniciado em "' + t.name + '".');
      });
    },
    deadlineClass: (t) => deadlineInfo(t.deadline).cls,
    deadlineLabel: (t) => deadlineInfo(t.deadline).label,
    externalLabel,
  }));

  Alpine.data('taskDetail', () => ({
    ...form(),
    taskId: WTT.boot.task_id,
    loading: true,
    task: null,
    members: [],
    integrations: [],
    sessions: [],
    form: { name: '', description: '', assignee_id: '', deadline: '' },
    linkForm: { integration_id: '', external_item_id: '', external_item_url: '' },
    external: { loading: false, details: null, error: '' },
    confirmDelete: false,
    async init() {
      try {
        const [task, members, integrations, sessions] = await Promise.all([
          api('GET', '/api/tasks/' + this.taskId),
          api('GET', '/api/projects/' + project.id + '/members'),
          api('GET', '/api/projects/' + project.id + '/integrations'),
          api('GET', '/api/projects/' + project.id + '/work-sessions?task_id=' + this.taskId),
        ]);
        this.members = members || [];
        this.integrations = (integrations || []).filter((i) => i.enabled);
        this.sessions = sessions || [];
        this.setTask(task);
        if (this.integrations.length > 0) this.linkForm.integration_id = this.integrations[0].id;
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
      window.addEventListener('wtt:sessions-changed', () => this.reloadSessions());
    },
    setTask(t) {
      this.task = t;
      this.form = {
        name: t.name,
        description: t.description || '',
        assignee_id: t.assignee_id,
        deadline: WTT.fmt.dateInput(t.deadline),
      };
      // Quem saiu dos times continua aparecendo como responsável atual.
      if (t.assignee && !this.members.some((m) => m.id === t.assignee_id)) {
        this.members = [t.assignee, ...this.members];
      }
      if (t.external_item_id) this.loadExternal();
      else this.external = { loading: false, details: null, error: '' };
    },
    async reloadSessions() {
      try {
        this.sessions = (await api('GET', '/api/projects/' + project.id + '/work-sessions?task_id=' + this.taskId)) || [];
      } catch (e) {
        // Mantém a lista anterior; a próxima ação mostra o erro.
      }
    },
    async loadExternal() {
      this.external = { loading: true, details: null, error: '' };
      try {
        const res = await api('GET', '/api/tasks/' + this.taskId + '/external-details');
        this.external = { loading: false, details: res.details, error: res.details ? '' : (res.error || 'sem resposta da integração') };
      } catch (e) {
        this.external = { loading: false, details: null, error: e.message };
      }
    },
    save() {
      return this.run('save', async () => {
        const t = await api('PATCH', '/api/tasks/' + this.taskId, {
          name: this.form.name,
          description: this.form.description,
          assignee_id: this.form.assignee_id,
          deadline: WTT.fmt.fromDateInput(this.form.deadline),
        });
        this.setTask(t);
        toast('Tarefa salva.');
      });
    },
    link() {
      return this.run('link', async () => {
        const t = await api('POST', '/api/tasks/' + this.taskId + '/link-external-item', this.linkForm);
        this.setTask(t);
        this.linkForm.external_item_id = '';
        this.linkForm.external_item_url = '';
      });
    },
    unlink() {
      return this.run('unlink', async () => {
        this.setTask(await api('DELETE', '/api/tasks/' + this.taskId + '/link-external-item'));
      });
    },
    remove() {
      return this.run('delete', async () => {
        await api('DELETE', '/api/tasks/' + this.taskId);
        location.href = tasksHref();
      });
    },
    backHref: tasksHref,
    totalSeconds() {
      return this.sessions.reduce((sum, s) => sum + clock().elapsed(s), 0);
    },
    deadlineClass() { return this.task ? deadlineInfo(this.task.deadline).cls : ''; },
    deadlineLabel() { return this.task ? deadlineInfo(this.task.deadline).label : ''; },
    externalLabel() { return this.task ? externalLabel(this.task) : ''; },
  }));

  Alpine.data('projectIntegrations', () => ({
    ...form(),
    types: WTT.integrationTypes,
    loading: true,
    items: [],
    creating: false,
    draft: { type: 'github', display_name: '', config: {} },
    editing: null,
    editDraft: { display_name: '', config: {} },
    confirming: null,
    async init() {
      try {
        this.items = (await api('GET', '/api/projects/' + project.id + '/integrations')) || [];
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
    },
    fieldsFor(type) {
      const t = this.types.find((x) => x.value === type);
      return t ? t.fields : [];
    },
    typeLabel(type) {
      const t = this.types.find((x) => x.value === type);
      return t ? t.label : type;
    },
    openCreate() {
      this.draft = { type: this.types[0].value, display_name: '', config: {} };
      this.creating = true;
      this.$nextTick(() => this.$refs.name && this.$refs.name.focus());
    },
    create() {
      return this.run('create', async () => {
        const it = await api('POST', '/api/projects/' + project.id + '/integrations', {
          type: this.draft.type,
          display_name: this.draft.display_name,
          config: { ...this.draft.config },
          enabled: true,
        });
        this.items = [it, ...this.items];
        this.creating = false;
        toast('Integração criada. As credenciais foram validadas na plataforma.');
      });
    },
    toggle(it) {
      return this.run('item-' + it.id, async () => {
        Object.assign(it, await api('PATCH', '/api/integrations/' + it.id, { enabled: !it.enabled }));
      });
    },
    startEdit(it) {
      this.editing = it.id;
      this.editDraft = { display_name: it.display_name, config: {} };
    },
    saveEdit(it) {
      return this.run('item-' + it.id, async () => {
        const body = { display_name: this.editDraft.display_name };
        const filled = Object.values(this.editDraft.config).some((v) => v);
        if (filled) body.config = { ...this.editDraft.config };
        Object.assign(it, await api('PATCH', '/api/integrations/' + it.id, body));
        this.editing = null;
        toast(filled ? 'Integração salva com a credencial nova.' : 'Integração salva.');
      });
    },
    remove(it) {
      return this.run('item-' + it.id, async () => {
        await api('DELETE', '/api/integrations/' + it.id);
        this.items = this.items.filter((x) => x.id !== it.id);
        this.confirming = null;
        toast('Integração excluída.');
      });
    },
  }));

  Alpine.data('timeTracking', () => ({
    ...form(),
    project,
    loading: true,
    tasks: [],
    sessions: [],
    taskId: '',
    myRate: null, // quanto a pessoa logada recebe por hora aqui; null se ainda não tem valor
    filter: { task: '', person: '' },
    async init() {
      try {
        const [tasks, sessions, allocations] = await Promise.all([
          api('GET', '/api/projects/' + project.id + '/tasks'),
          api('GET', '/api/projects/' + project.id + '/work-sessions'),
          api('GET', '/api/projects/' + project.id + '/allocations'),
        ]);
        this.tasks = tasks || [];
        this.sessions = sessions || [];
        const own = (allocations || []).find((a) => a.person_id === me.id);
        this.myRate = own ? own.pay_rate_cents : null;
        // Sugere uma tarefa da própria pessoa.
        const mine = this.tasks.find((t) => t.assignee_id === me.id) || this.tasks[0];
        this.taskId = mine ? mine.id : '';
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
      window.addEventListener('wtt:sessions-changed', () => this.reloadSessions());
    },
    async reloadSessions() {
      try {
        this.sessions = (await api('GET', '/api/projects/' + project.id + '/work-sessions')) || [];
      } catch (e) {
        this.errors.load = e.message;
      }
    },
    start() {
      return this.run('clock', () => clock().clockIn(project.id, this.taskId));
    },
    stop() {
      return this.run('clock', () => clock().clockOut());
    },
    people() {
      const seen = new Map();
      this.sessions.forEach((s) => { if (s.person && !seen.has(s.person.id)) seen.set(s.person.id, s.person); });
      return [...seen.values()].sort((a, b) => a.name.localeCompare(b.name));
    },
    filtered() {
      return this.sessions.filter((s) =>
        (!this.filter.task || s.task_id === this.filter.task) &&
        (!this.filter.person || s.person_id === this.filter.person));
    },
    filteredTotal() {
      return this.filtered().reduce((sum, s) => sum + clock().elapsed(s), 0);
    },
    // mine soma o tempo da pessoa logada hoje ou nesta semana, e earned, quanto
    // ela ganhou nesse tempo.
    mine(period) {
      return secondsWithin(this.sessions.filter((s) => s.person_id === me.id), periodStart(period));
    },
    earned(period) {
      return amountWithin(this.sessions.filter((s) => s.person_id === me.id), periodStart(period));
    },
    // amount é o valor de uma sessão: 'pay' é o que a pessoa recebe e 'bill', o
    // que o cliente paga. A sessão fechada usa o valor que o servidor calculou; a
    // aberta acompanha o cronômetro. null quando não há valor ou ele não é visível.
    amount(s, kind) {
      const rate = s[kind + '_rate_cents'];
      if (rate === null || rate === undefined) return null;
      return s.end_at ? s[kind + '_amount_cents'] : Math.round(clock().elapsed(s) * rate / 3600);
    },
    filteredAmount(kind) {
      const values = this.filtered().map((s) => this.amount(s, kind)).filter((v) => v !== null && v !== undefined);
      return values.length === 0 ? null : values.reduce((sum, v) => sum + v, 0);
    },
    filteredMargin() {
      const bill = this.filteredAmount('bill');
      return bill === null ? null : bill - (this.filteredAmount('pay') || 0);
    },
  }));

  Alpine.data('projectSettings', () => ({
    ...form(),
    loading: true,
    form: { name: '', description: '', sprint_duration_days: 14, weekly_hours: '', daily_time: '', weekly_sync_day: '' },
    customerName: '',
    customers: [],
    billing: { customer_id: '', rate: '' },
    confirmDelete: false,
    async init() {
      try {
        const p = await api('GET', '/api/projects/' + project.id);
        this.form = {
          name: p.name,
          description: p.description || '',
          sprint_duration_days: p.sprint_duration_days,
          weekly_hours: p.weekly_hours || '',
          daily_time: p.daily_time || '',
          weekly_sync_day: p.weekly_sync_day || '',
        };
        this.customerName = p.customer ? p.customer.name : '';
        // O valor cobrado e a lista de clientes são rotas de admin.
        if (me.role === 'admin') {
          const [billing, customers] = await Promise.all([
            api('GET', '/api/projects/' + project.id + '/billing'),
            api('GET', '/api/orgs/' + me.organization_id + '/customers'),
          ]);
          this.customers = customers || [];
          this.setBilling(billing);
        }
      } catch (e) {
        this.errors.save = e.message;
      } finally {
        this.loading = false;
      }
    },
    setBilling(b) {
      this.billing = { customer_id: b.customer ? b.customer.id : '', rate: WTT.fmt.moneyInput(b.bill_rate_cents) };
      this.customerName = b.customer ? b.customer.name : '';
    },
    saveBilling() {
      return this.run('billing', async () => {
        const cents = WTT.toCents(this.billing.rate);
        if (cents === null && String(this.billing.rate).trim() !== '') throw new Error('Informe um valor, por exemplo 100,00.');
        this.setBilling(await api('PUT', '/api/projects/' + project.id + '/billing', {
          customer_id: this.billing.customer_id || null,
          bill_rate_cents: cents,
        }));
        toast('Cobrança salva.');
      });
    },
    save() {
      return this.run('save', async () => {
        // Texto vazio apaga daily e weekly, e zero apaga a jornada; a API mantém
        // o que não vier no corpo.
        const p = await api('PATCH', '/api/projects/' + project.id, {
          name: this.form.name,
          description: this.form.description,
          sprint_duration_days: Number(this.form.sprint_duration_days) || 0,
          weekly_hours: Number(this.form.weekly_hours) || 0,
          daily_time: this.form.daily_time || '',
          weekly_sync_day: this.form.weekly_sync_day || '',
        });
        document.querySelectorAll('[data-project-name]').forEach((el) => { el.textContent = p.name; });
        toast('Projeto salvo.');
      });
    },
    remove() {
      return this.run('delete', async () => {
        await api('DELETE', '/api/projects/' + project.id);
        location.href = '/orgs/' + me.organization_id;
      });
    },
  }));

  // fold tira acentos e maiúsculas, para a busca achar "Mônica" com "monica".
  const fold = (s) => (s || '').normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();
  const matches = (person, query) => !query || fold(person.name).includes(query) || fold(person.email).includes(query);

  // A aba Colaboradores: quem está no projeto, com o valor por hora de cada
  // pessoa, e os times. Colaborador é quem tem valor aqui ou está em algum time.
  Alpine.data('projectTeams', () => ({
    ...form(),
    meId: me.id,
    loading: true,
    collaborators: [], // { person, teams, pay_rate_cents, draft }; draft é o texto do campo de valor
    teams: [],
    billing: { customer: null, bill_rate_cents: null }, // o que o cliente paga; só admins recebem
    people: [], // todas as pessoas da organização, para os admins adicionarem
    search: '',
    confirming: null, // 'person-<id>' ou 'team-<id>'
    editing: null,
    editName: '',
    adding: {}, // por time, a pessoa escolhida no seletor
    add: { search: '', person_id: '', rate: '', team_id: '' },
    newTeam: '',
    async init() {
      try {
        const admin = me.role === 'admin';
        const [collaborators, teams, billing, people] = await Promise.all([
          api('GET', '/api/projects/' + project.id + '/collaborators'),
          api('GET', '/api/projects/' + project.id + '/teams'),
          // O valor cobrado e a lista de quem pode entrar só servem às ações de admin.
          admin ? api('GET', '/api/projects/' + project.id + '/billing') : null,
          admin ? api('GET', '/api/orgs/' + me.organization_id + '/persons') : null,
        ]);
        this.setCollaborators(collaborators);
        this.teams = teams || [];
        if (billing) this.billing = billing;
        this.people = people || [];
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
    },
    // setCollaborators troca a lista e mantém o que foi digitado, e ainda não
    // salvo, no campo de valor de cada pessoa.
    setCollaborators(list) {
      const typed = new Map(this.collaborators.filter((c) => this.dirty(c)).map((c) => [c.person.id, c.draft]));
      this.collaborators = (list || []).map((c) => ({
        ...c,
        draft: typed.has(c.person.id) ? typed.get(c.person.id) : WTT.fmt.moneyInput(c.pay_rate_cents),
      }));
    },
    // reload busca os colaboradores de novo: quem entra ou sai de um time, ou do
    // projeto, muda a tabela de pessoas e os cartões dos times de uma vez.
    async reload() {
      this.setCollaborators(await api('GET', '/api/projects/' + project.id + '/collaborators'));
    },
    rows() {
      const query = fold(this.search.trim());
      return this.collaborators.filter((c) => matches(c.person, query));
    },
    summary() {
      return {
        people: this.collaborators.length,
        noTeam: this.collaborators.filter((c) => c.teams.length === 0).length,
        teams: this.teams.length,
        noRate: this.missing().length,
      };
    },
    // Quem está no projeto sem valor por hora: não bate ponto até um admin definir.
    missing() {
      return this.collaborators.filter((c) => c.pay_rate_cents === null);
    },
    membersOf(team) {
      return this.collaborators.filter((c) => c.teams.some((t) => t.id === team.id));
    },
    inProject(person) {
      return this.collaborators.some((c) => c.person.id === person.id);
    },
    dirty(c) {
      return WTT.toCents(c.draft) !== c.pay_rate_cents;
    },
    margin(c) {
      if (this.billing.bill_rate_cents === null || c.pay_rate_cents === null) return null;
      return this.billing.bill_rate_cents - c.pay_rate_cents;
    },
    saveRate(c) {
      return this.run('person-' + c.person.id, async () => {
        const cents = WTT.toCents(c.draft);
        if (cents === null) throw new Error('Informe um valor, por exemplo 20,00.');
        const a = await api('PUT', '/api/projects/' + project.id + '/allocations/' + c.person.id, { pay_rate_cents: cents });
        c.pay_rate_cents = a.pay_rate_cents;
        c.draft = WTT.fmt.moneyInput(a.pay_rate_cents);
        toast('Valor de ' + c.person.name + ' salvo.');
      });
    },
    removePerson(c) {
      return this.run('person-' + c.person.id, async () => {
        await api('DELETE', '/api/projects/' + project.id + '/collaborators/' + c.person.id);
        this.confirming = null;
        await this.reload();
        toast(c.person.name + ' saiu do projeto.');
      });
    },
    // Quem pode entrar no projeto: as pessoas da organização que ainda não estão nele.
    addCandidates() {
      const query = fold(this.add.search.trim());
      return this.people.filter((p) => !this.inProject(p) && matches(p, query));
    },
    // pruneAdd desfaz a escolha quando a busca tira da lista a pessoa escolhida.
    pruneAdd() {
      if (!this.addCandidates().some((p) => p.id === this.add.person_id)) this.add.person_id = '';
    },
    openAdd() {
      this.add = { search: '', person_id: '', rate: '', team_id: '' };
      this.errors.add = '';
      Alpine.store('modal').open('collab-add', 'Adicionar pessoa ao projeto', () => !this.pending);
    },
    addPerson() {
      return this.run('add', async () => {
        const person = this.addCandidates().find((p) => p.id === this.add.person_id);
        if (!person) throw new Error('Escolha uma pessoa.');
        const cents = WTT.toCents(this.add.rate);
        if (cents === null) throw new Error('Informe o valor por hora, por exemplo 20,00.');
        await api('PUT', '/api/projects/' + project.id + '/allocations/' + person.id, { pay_rate_cents: cents });
        // Daqui em diante a pessoa já está no projeto: se o time falhar, ela
        // fica sem time e a tela avisa, em vez de parecer que nada aconteceu.
        let teamError = '';
        if (this.add.team_id) {
          try {
            await api('POST', '/api/teams/' + this.add.team_id + '/members', { person_id: person.id });
          } catch (e) {
            teamError = e.message;
          }
        }
        await this.reload();
        Alpine.store('modal').close();
        if (teamError) Alpine.store('toast').error(person.name + ' entrou no projeto, mas não no time: ' + teamError);
        else toast(person.name + ' entrou no projeto.');
      });
    },
    openTeam() {
      this.newTeam = '';
      this.errors.create = '';
      Alpine.store('modal').open('team-new', 'Novo time', () => !this.pending);
    },
    createTeam() {
      return this.run('create', async () => {
        const t = await api('POST', '/api/projects/' + project.id + '/teams', { name: this.newTeam });
        this.teams = [t, ...this.teams];
        Alpine.store('modal').close();
        toast('Time criado.');
      });
    },
    startRename(team) {
      this.editing = team.id;
      this.editName = team.name;
    },
    rename(team) {
      return this.run('team-' + team.id, async () => {
        const t = await api('PATCH', '/api/teams/' + team.id, { name: this.editName });
        team.name = t.name;
        this.editing = null;
        await this.reload(); // o nome do time também aparece na tabela de pessoas
      });
    },
    removeTeam(team) {
      return this.run('team-' + team.id, async () => {
        await api('DELETE', '/api/teams/' + team.id);
        this.teams = this.teams.filter((t) => t.id !== team.id);
        this.confirming = null;
        await this.reload(); // quem só estava neste time, sem valor, deixa de ser do projeto
        toast('Time excluído.');
      });
    },
    // Quem pode entrar no time: qualquer pessoa da organização que não esteja
    // nele, com quem já é do projeto primeiro.
    teamCandidates(team) {
      const inTeam = new Set(this.membersOf(team).map((c) => c.person.id));
      return this.people.filter((p) => !inTeam.has(p.id))
        .sort((a, b) => (this.inProject(b) - this.inProject(a)) || a.name.localeCompare(b.name));
    },
    addMember(team) {
      const personId = this.adding[team.id];
      if (!personId) return undefined;
      return this.run('team-' + team.id, async () => {
        await api('POST', '/api/teams/' + team.id + '/members', { person_id: personId });
        this.adding[team.id] = '';
        await this.reload();
      });
    },
    removeMember(team, member) {
      return this.run('team-' + team.id, async () => {
        await api('DELETE', '/api/teams/' + team.id + '/members', { person_id: member.person.id });
        await this.reload();
      });
    },
  }));
});
