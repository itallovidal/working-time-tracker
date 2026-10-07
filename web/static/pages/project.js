// Componentes das abas de um projeto.
document.addEventListener('alpine:init', () => {
  const { api, form } = WTT;
  const me = WTT.boot.me;
  const project = WTT.boot.project;
  const toast = (msg, kind) => Alpine.store('toast').show(msg, kind);
  const clock = () => Alpine.store('clock');
  const DAY = 24 * 60 * 60 * 1000;

  // priorityClass e statusClass dão a cor de uma prioridade e de um status (`.tone` e a variante,
  // em app.css), no selo, no chip do filtro e no select. Uma tarefa sem valor conhecido cai em
  // "sem prioridade" e em "backlog", os padrões da API.
  const priorityClass = (p) => 'tone prio-' + (['urgent', 'high', 'medium', 'low'].includes(p) ? p : 'none');
  const statusClass = (s) => 'tone status-' + (['in_progress', 'awaiting_closure', 'closed'].includes(s) ? s : 'backlog');
  const byLabelName = (a, b) => a.name.localeCompare(b.name, WTT.lang);

  // Grupos de permissões que a API tem e as telas não oferecem.
  const HIDDEN_PRESETS = ['finance'];

  // labelTools é o que as telas que escolhem etiquetas têm em comum: a lista do projeto e,
  // para admins, criar uma na hora. O rascunho de quem usa guarda os ids marcados.
  const labelTools = () => ({
    labels: [], // as etiquetas do projeto
    newLabel: '',
    isAdmin: me.role === 'admin',
    async loadLabels() {
      try {
        this.labels = ((await api('GET', '/api/projects/' + project.id + '/labels')) || []).sort(byLabelName);
      } catch (e) {
        this.errors.label = e.message;
      }
    },
    // createLabel cria a etiqueta digitada (só admins) e a marca em ids. Um nome que o
    // projeto já tem só marca a que existe, sem diferenciar maiúsculas.
    createLabel(ids) {
      const name = this.newLabel.trim();
      if (!name) return undefined;
      const existing = this.labels.find((l) => l.name.toLowerCase() === name.toLowerCase());
      if (existing) {
        if (!ids.includes(existing.id)) ids.push(existing.id);
        this.newLabel = '';
        return undefined;
      }
      return this.run('label', async () => {
        const l = await api('POST', '/api/projects/' + project.id + '/labels', { name });
        this.labels = [...this.labels, l].sort(byLabelName);
        ids.push(l.id);
        this.newLabel = '';
      });
    },
    priorityClass,
    statusClass,
  });

  // taskWizard é o passo a passo dos modais Nova tarefa e Editar tarefa: a etapa 1 é o nome e
  // a descrição (em Markdown, com pré-visualização) e a etapa 2, o resto. Quem usa tem um
  // `draft` e labelTools. As duas etapas ficam na página, só escondidas, e o formulário não
  // valida sozinho (novalidate): quem confere cada etapa é submitStep.
  const taskWizard = () => ({
    step: 1,
    hasStatus: false, // o status só se escolhe ao editar; a tarefa nova nasce em backlog
    mdView: 'write', // write ou preview
    resetWizard() {
      this.step = 1;
      this.mdView = 'write';
      this.errors.name = '';
      this.errors.assignee = '';
    },
    previewHTML() {
      return WTT.markdown(this.draft.description);
    },
    // setMd troca entre Escrever e Pré-visualizar; com `focus`, leva o foco para o botão (setas).
    setMd(view, focus) {
      this.mdView = view;
      if (focus) this.$nextTick(() => (view === 'write' ? this.$refs.mdWrite : this.$refs.mdPreview).focus());
    },
    // focusStep põe o foco no primeiro campo da etapa: o store do modal só foca ao abrir.
    focusStep() {
      this.$nextTick(() => {
        const el = document.getElementById(this.step === 1 ? 'task-name' : 'task-deadline');
        if (el) el.focus();
      });
    },
    next() {
      if (!this.draft.name.trim()) {
        this.errors.name = WTT.t('tasks.wizard.name_required');
        this.focusStep();
        return false;
      }
      this.errors.name = '';
      this.step = 2;
      this.focusStep();
      return true;
    },
    back() {
      this.step = 1;
      this.focusStep();
    },
    // submitStep é o envio do formulário (Enter ou o botão): na etapa 1 avança, na 2 confere
    // o responsável e chama o método que grava (create ou save).
    submitStep(finish) {
      if (this.step === 1) {
        this.next();
        return undefined;
      }
      if (this.draft.assign === 'other' && !this.draft.assignee_id) {
        this.errors.assignee = WTT.t('tasks.wizard.pick_person');
        return undefined;
      }
      this.errors.assignee = '';
      return this[finish]();
    },
  });

  // deadlineInfo descreve o prazo de uma tarefa para o badge: atrasada, vencendo
  // nas próximas 48 horas ou só a data.
  function deadlineInfo(iso) {
    if (!iso || new Date(iso).getFullYear() < 1971) return { label: WTT.t('tasks.no_deadline'), cls: '' };
    const diff = new Date(iso).getTime() - Date.now();
    if (diff < 0) return { label: WTT.t('tasks.overdue', { date: WTT.fmt.date(iso) }), cls: 'badge-danger' };
    if (diff < 2 * DAY) return { label: WTT.t('tasks.due_soon', { date: WTT.fmt.date(iso) }), cls: 'badge-warn' };
    return { label: WTT.fmt.date(iso), cls: '' };
  }

  // Atalhos do filtro de prazo da lista de tarefas. 'date' abre o campo de data.
  // "Qualquer prazo" fica fixo no template: uma opção de valor vazio criada por
  // x-for fica sem o atributo value e passa a valer o próprio rótulo.
  const dueOptions = [
    { value: 'overdue', label: WTT.t('tasks.due.overdue') },
    { value: 'today', label: WTT.t('tasks.due.today') },
    { value: 'week', label: WTT.t('tasks.due.week') },
    { value: 'next_week', label: WTT.t('tasks.due.next_week') },
    { value: 'date', label: WTT.t('tasks.due.date') },
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

  // earnedRate é o valor por hora que a sessão rendeu a quem trabalhou nela: o que a
  // pessoa recebe, ou, para o dono da organização, o valor cobrado do cliente.
  function earnedRate(s) {
    return s.owner_hours ? s.bill_rate_cents : s.pay_rate_cents;
  }

  // amountWithin soma quanto a pessoa ganhou depois de "since": o tempo de cada
  // sessão dentro do período vezes o valor por hora daquela sessão. Usa o mesmo
  // arredondamento do servidor, por sessão.
  function amountWithin(sessions, since) {
    return sessions.reduce((sum, s) => {
      const rate = earnedRate(s);
      if (rate === null || rate === undefined) return sum;
      return sum + Math.round(secondsWithin([s], since) * rate / 3600);
    }, 0);
  }

  // Os tipos de integração vêm do servidor (internal/adapter), com o que cada um pede:
  // os campos do metadata e os rótulos. Um tipo novo lá aparece aqui sem mudança.
  const integrationTypes = WTT.boot.integration_types || [];
  const integrationType = (type) => integrationTypes.find((t) => t.type === type);
  // externalLabel descreve o item vinculado: "GitHub #42", "Trello H0TZyzbK".
  function externalLabel(task) {
    const type = task.external_integration ? task.external_integration.type : '';
    const t = integrationType(type);
    if (!t) return (type || WTT.t('tasks.item')) + ' #' + task.external_item_id;
    return t.label + ' ' + (t.item_numeric ? '#' : '') + task.external_item_id;
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

  // sessionList é o que as telas de sessões têm em comum: a lista, os filtros de
  // pessoa, data e tarefa, os totais, que seguem os filtros, e as páginas. Quem usa
  // põe as sessões em this.sessions e chama watchSessionFilters() no init.
  const SESSIONS_PER_PAGE = 10;
  const sessionList = () => ({
    sessions: [],
    filter: { task: '', person: '', date: '' }, // date é um dia, como "2026-10-06"
    page: 1,
    // Um filtro novo volta para a primeira página.
    watchSessionFilters() {
      this.$watch('filter', () => { this.page = 1; });
    },
    people() {
      const seen = new Map();
      this.sessions.forEach((s) => { if (s.person && !seen.has(s.person.id)) seen.set(s.person.id, s.person); });
      return [...seen.values()].sort((a, b) => a.name.localeCompare(b.name, WTT.lang));
    },
    // As tarefas do filtro são as que aparecem nas sessões: uma sessão pode ser de
    // uma tarefa que já não é de quem olha.
    sessionTasks() {
      const seen = new Map();
      this.sessions.forEach((s) => s.tasks.forEach((l) => { if (!seen.has(l.task_id)) seen.set(l.task_id, l.task); }));
      return [...seen.values()].sort((a, b) => a.name.localeCompare(b.name, WTT.lang));
    },
    // As tarefas de uma sessão, uma vez cada (a mesma pode ter voltado), na ordem em que
    // entraram. A tabela mostra as duas primeiras e quantas mais há.
    distinctTasks(s) {
      const seen = new Map();
      s.tasks.forEach((l) => { if (!seen.has(l.task_id)) seen.set(l.task_id, l.task); });
      return [...seen.values()];
    },
    firstTasks(s) {
      return this.distinctTasks(s).slice(0, 2);
    },
    moreTasks(s) {
      return Math.max(0, this.distinctTasks(s).length - 2);
    },
    // openSession abre o modal da sessão da linha.
    openSession(s) {
      Alpine.store('sessionView').open(s);
    },
    // seconds é o tempo da linha: o da sessão ou, com uma tarefa no filtro, o que a tarefa teve nela.
    seconds(s) {
      return this.filter.task ? clock().taskElapsed(s, this.filter.task) : clock().elapsed(s);
    },
    // filtered aplica os filtros da lista. A data pega as sessões iniciadas
    // naquele dia, no fuso de quem está olhando.
    filtered() {
      return this.sessions.filter((s) =>
        (!this.filter.task || s.tasks.some((l) => l.task_id === this.filter.task)) &&
        (!this.filter.person || s.person_id === this.filter.person) &&
        (!this.filter.date || WTT.fmt.dateInput(s.start_at) === this.filter.date));
    },
    hasFilters() {
      return !!(this.filter.task || this.filter.person || this.filter.date);
    },
    clearFilters() {
      this.filter = { task: '', person: '', date: '' };
    },
    sessionPages() {
      return Math.max(1, Math.ceil(this.filtered().length / SESSIONS_PER_PAGE));
    },
    sessionRows() {
      const start = (Math.min(this.page, this.sessionPages()) - 1) * SESSIONS_PER_PAGE;
      return this.filtered().slice(start, start + SESSIONS_PER_PAGE);
    },
    sessionSummary() {
      return WTT.t('time.page_summary', { page: Math.min(this.page, this.sessionPages()), pages: this.sessionPages(), count: this.filtered().length });
    },
    filteredTotal() {
      return this.filtered().reduce((sum, s) => sum + this.seconds(s), 0);
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
    // aberta acompanha o cronômetro. null quando não há valor ou ele não é visível. Com uma
    // tarefa no filtro, é o valor do tempo dela na sessão: a soma dos intervalos, cada um
    // arredondado como o servidor faz.
    amount(s, kind) {
      const rate = s[kind + '_rate_cents'];
      if (rate === null || rate === undefined) return null;
      if (this.filter.task) {
        return s.tasks.filter((l) => l.task_id === this.filter.task).reduce((sum, l) =>
          sum + (s.end_at ? l[kind + '_amount_cents'] : Math.round(clock().linkElapsed(s, l) * rate / 3600)), 0);
      }
      return s.end_at ? s[kind + '_amount_cents'] : Math.round(clock().elapsed(s) * rate / 3600);
    },
    filteredAmount(kind) {
      const values = this.filtered().map((s) => this.amount(s, kind)).filter((v) => v !== null && v !== undefined);
      return values.length === 0 ? null : values.reduce((sum, v) => sum + v, 0);
    },
    // earnedAmount é o que a sessão rendeu a quem trabalhou nela: o valor pago, ou, para o
    // dono da organização, o valor cobrado. filteredEarned soma o das sessões da tela.
    earnedAmount(s) {
      return this.amount(s, s.owner_hours ? 'bill' : 'pay');
    },
    filteredEarned() {
      const values = this.filtered().map((s) => this.earnedAmount(s)).filter((v) => v !== null && v !== undefined);
      return values.length === 0 ? null : values.reduce((sum, v) => sum + v, 0);
    },
    filteredMargin() {
      const bill = this.filteredAmount('bill');
      return bill === null ? null : bill - (this.filteredAmount('pay') || 0);
    },
  });

  // projectOverview é a Visão geral da Gestão, só de admins: os números do projeto lidos de
  // uma vez em /overview, e as sessões de todos. É uma fotografia do instante em generated_at, sem
  // relógio correndo; o botão Atualizar e um ponto batido nesta aba releem tudo.
  Alpine.data('projectOverview', () => ({
    ...form(),
    ...sessionList(),
    loading: true,
    data: null,
    async init() {
      this.watchSessionFilters();
      await this.load();
      this.loading = false;
      window.addEventListener('wtt:sessions-changed', () => this.load());
    },
    // Os números do projeto e as sessões de todos vêm juntos, para a lista e os
    // totais da tela serem da mesma hora.
    load() {
      return this.run('load', async () => {
        const [data, sessions] = await Promise.all([
          api('GET', '/api/projects/' + project.id + '/overview'),
          api('GET', '/api/projects/' + project.id + '/work-sessions'),
        ]);
        this.data = data;
        this.sessions = sessions || [];
      });
    },
    // filteredMarginShare é a margem dos totais da tela como parte da receita, em por
    // cento inteiro; null quando não há receita.
    filteredMarginShare() {
      const margin = this.filteredMargin();
      const bill = this.filteredAmount('bill');
      return margin === null || !bill ? null : Math.round(margin / bill * 100);
    },
    workingNames() {
      return this.data.by_person.filter((p) => p.working_now).map((p) => p.person.name).join(', ');
    },
    integrationsSummary() {
      const { total, enabled } = this.data.integrations;
      return WTT.t('overview.configured', { count: total }) + ' · ' + WTT.t('overview.enabled', { count: enabled });
    },
    typeLabel(type) {
      const t = integrationType(type);
      return t ? t.label : type;
    },
  }));

  // As duas listas da página de tarefas: as que ninguém pegou e as que já têm responsável.
  const taskLists = ['free', 'taken'];
  const emptyList = () => ({ tasks: [], total: 0, page: 1, perPage: 10 });

  Alpine.data('projectTasks', () => ({
    ...form(),
    ...taskWizard(),
    loading: true,
    // Cada lista guarda só a página em uso; os filtros e a paginação rodam no servidor.
    lists: { free: emptyList(), taken: emptyList() },
    taskLists,
    ...labelTools(),
    members: [], // quem está no projeto: pode ser responsável por tarefa nova
    assignees: [], // quem já é responsável por alguma tarefa, mesmo fora dos times
    // priority, status e label são listas: a tarefa passa se tem qualquer uma das marcadas.
    // assignee só vale para a lista Com responsável: a das sem responsável não tem dono.
    filters: { q: '', assignee: '', due: '', date: '', priority: [], status: [], label: [] },
    dueOptions,
    seq: 0,
    draft: { name: '', description: '', assign: 'none', assignee_id: '', deadline: '', priority: 'none', label_ids: [] }, // assign: me, none ou other
    async init() {
      this.readURL();
      const members = api('GET', '/api/projects/' + project.id + '/members')
        .then((list) => { this.members = list || []; })
        .catch((e) => { this.errors.members = e.message; });
      // As etiquetas vêm antes da primeira busca: uma etiqueta excluída que ainda está na
      // URL sai do filtro, em vez de zerar a lista.
      await Promise.all([members, this.loadLabels()]);
      this.filters.label = this.filters.label.filter((id) => this.labels.some((l) => l.id === id));
      await this.load();
      this.loading = false;
      // Bater o ponto numa tarefa sem responsável a passa para quem bateu.
      window.addEventListener('wtt:sessions-changed', () => this.load());
    },
    // A URL guarda os filtros e a página de cada lista, para recarregar ou voltar do detalhe
    // de uma tarefa sem perder o lugar. O prazo vai como o nome do atalho ou a
    // data escolhida, nunca como instante.
    readURL() {
      const p = new URLSearchParams(location.search);
      const due = p.get('due') || '';
      const isDate = /^\d{4}-\d{2}-\d{2}$/.test(due);
      // As tarefas de quem está logado ficam em Minhas tarefas: a lista não as mostra.
      const assignee = /^[0-9a-f-]{36}$/i.test(p.get('assignee') || '') && p.get('assignee') !== me.id ? p.get('assignee') : '';
      this.filters = {
        q: p.get('q') || '',
        assignee,
        due: isDate ? 'date' : (dueOptions.some((o) => o.value === due) ? due : ''),
        date: isDate ? due : '',
        priority: (p.get('priority') || '').split(',').filter((v) => WTT.priorities.some((o) => o.value === v)),
        status: (p.get('status') || '').split(',').filter((v) => WTT.taskStatuses.some((o) => o.value === v)),
        label: (p.get('label') || '').split(',').filter((v) => /^[0-9a-f-]{36}$/i.test(v)),
      };
      taskLists.forEach((key) => {
        this.lists[key].page = Math.max(1, parseInt(p.get(key + '_page'), 10) || 1);
      });
    },
    writeURL() {
      const f = this.filters;
      const p = new URLSearchParams();
      if (f.q.trim()) p.set('q', f.q.trim());
      if (f.assignee) p.set('assignee', f.assignee);
      const due = f.due === 'date' ? f.date : f.due;
      if (due) p.set('due', due);
      if (f.priority.length) p.set('priority', f.priority.join(','));
      if (f.status.length) p.set('status', f.status.join(','));
      if (f.label.length) p.set('label', f.label.join(','));
      taskLists.forEach((key) => {
        if (this.lists[key].page > 1) p.set(key + '_page', this.lists[key].page);
      });
      const query = p.toString();
      history.replaceState(null, '', location.pathname + (query ? '?' + query : ''));
      try {
        sessionStorage.setItem(tasksQueryKey, query);
      } catch (e) {
        // Sem sessionStorage o Voltar da tarefa só deixa de lembrar os filtros.
      }
    },
    // loadList busca a página em uso de uma lista: a das sem responsável pede assignee_id=none,
    // e a das com responsável, a pessoa escolhida ou, sem escolha, as de outras pessoas (others):
    // as de quem está logado ficam na aba Minhas tarefas.
    async loadList(key) {
      const f = this.filters;
      const list = this.lists[key];
      const p = new URLSearchParams({ page: list.page });
      if (f.q.trim()) p.set('q', f.q.trim());
      p.set('assignee_id', key === 'free' ? 'none' : (f.assignee || 'others'));
      const limit = dueLimit(f.due, f.date);
      if (limit) p.set('deadline_to', limit);
      if (f.priority.length) p.set('priority', f.priority.join(','));
      if (f.status.length) p.set('status', f.status.join(','));
      if (f.label.length) p.set('label_id', f.label.join(','));
      return api('GET', '/api/projects/' + project.id + '/tasks?' + p);
    },
    // load busca a página em uso das duas listas. Não passa por run(), que descartaria uma
    // troca de filtro feita durante outra ação, e ignora a resposta de um
    // pedido mais antigo que o último.
    async load() {
      const seq = ++this.seq;
      try {
        const results = await Promise.all(taskLists.map((key) => this.loadList(key)));
        if (seq !== this.seq) return;
        taskLists.forEach((key, i) => {
          const res = results[i];
          this.lists[key] = {
            tasks: res.items || [],
            total: res.total,
            page: res.page, // o servidor devolve a última quando a pedida não existe mais
            perPage: res.per_page,
          };
          this.assignees = res.assignees || [];
        });
        this.errors.load = '';
        this.writeURL();
      } catch (e) {
        if (seq === this.seq) this.errors.load = e.message;
      }
    },
    // apply é o que os filtros chamam ao mudar: as duas listas voltam para a primeira página.
    apply() {
      taskLists.forEach((key) => { this.lists[key].page = 1; });
      return this.load();
    },
    go(key, page) {
      this.lists[key].page = page;
      return this.load();
    },
    clear() {
      this.filters = { q: '', assignee: '', due: '', date: '', priority: [], status: [], label: [] };
      return this.apply();
    },
    // toggleFilter marca ou desmarca uma prioridade, um status ou uma etiqueta do filtro.
    toggleFilter(kind, value) {
      const list = this.filters[kind];
      this.filters[kind] = list.includes(value) ? list.filter((v) => v !== value) : [...list, value];
      return this.apply();
    },
    hasFilters() {
      const f = this.filters;
      return !!(f.q.trim() || f.assignee || dueLimit(f.due, f.date) || f.priority.length || f.status.length || f.label.length);
    },
    // listFiltered diz se algum filtro vale para a lista: o responsável só vale para a dos que têm um.
    listFiltered(key) {
      const f = this.filters;
      return key === 'taken' ? this.hasFilters() : !!(f.q.trim() || dueLimit(f.due, f.date) || f.priority.length || f.status.length || f.label.length);
    },
    pages(key) {
      const l = this.lists[key];
      return Math.max(1, Math.ceil(l.total / l.perPage));
    },
    summary(key) {
      return WTT.t('tasks.summary', { page: this.lists[key].page, pages: this.pages(key), count: this.lists[key].total });
    },
    // Quem aparece no filtro de responsável: os times e quem tem tarefa aqui, menos a própria
    // pessoa, cujas tarefas ficam em Minhas tarefas.
    assigneeOptions() {
      const byId = new Map();
      [...this.assignees, ...this.members].forEach((p) => { if (p.id !== me.id) byId.set(p.id, p); });
      return [...byId.values()].sort((a, b) => a.name.localeCompare(b.name, WTT.lang));
    },
    // Outra pessoa só pode ser responsável se estiver num time do projeto; a própria, não precisa.
    otherMembers() {
      return this.members.filter((m) => m.id !== me.id);
    },
    openCreate() {
      this.draft = { name: '', description: '', assign: 'me', assignee_id: '', deadline: '', priority: 'none', label_ids: [] };
      this.newLabel = '';
      this.errors.create = '';
      this.errors.label = '';
      this.resetWizard();
      Alpine.store('modal').open('task-new', WTT.t('tasks.new'), () => !this.pending);
    },
    create() {
      return this.run('create', async () => {
        const t = await api('POST', '/api/projects/' + project.id + '/tasks', {
          name: this.draft.name,
          description: this.draft.description,
          assignee_id: { me: me.id, none: '', other: this.draft.assignee_id }[this.draft.assign],
          deadline: WTT.fmt.fromDateInput(this.draft.deadline),
          priority: this.draft.priority,
          label_ids: this.draft.label_ids,
        });
        Alpine.store('modal').close();
        // A tarefa nova é a primeira da lista dela, se os filtros em uso a mostrarem.
        await this.apply();
        const shown = taskLists.some((key) => this.lists[key].tasks.some((x) => x.id === t.id));
        // Uma tarefa sua não aparece nesta página, que mostra as de outras pessoas: está em Minhas tarefas.
        toast(WTT.t(shown ? 'tasks.created' : (t.assignee_id === me.id ? 'tasks.created_mine' : 'tasks.created_hidden')));
      });
    },
    isRunning(t) {
      return clock().isRunning(t.id);
    },
    deadlineClass: (t) => deadlineInfo(t.deadline).cls,
    deadlineLabel: (t) => deadlineInfo(t.deadline).label,
  }));

  // Minhas tarefas: as tarefas de que a pessoa é responsável, numa lista por status, na ordem
  // em que o trabalho costuma andar. O que já fechou começa recolhido, porque só cresce.
  Alpine.data('projectMyTasks', () => ({
    ...form(),
    loading: true,
    tasks: [],
    // A ordem em que o trabalho anda: registrada e ainda não começada (backlog), em progresso,
    // aguardando fechamento e fechada. É a mesma ordem dos filtros de status da lista.
    statuses: WTT.taskStatuses,
    open: { backlog: true, in_progress: true, awaiting_closure: true, closed: false },
    async init() {
      await this.load();
      this.loading = false;
      // Bater o ponto ou parar mexe no status das tarefas daqui.
      window.addEventListener('wtt:sessions-changed', () => this.load());
    },
    async load() {
      try {
        this.tasks = (await api('GET', '/api/projects/' + project.id + '/tasks?assignee_id=' + me.id)) || [];
        this.errors.load = '';
      } catch (e) {
        this.errors.load = e.message;
      }
    },
    // inStatus devolve as tarefas de um status, com o prazo mais perto primeiro.
    inStatus(status) {
      const time = (t) => (new Date(t.deadline).getFullYear() < 1971 ? Infinity : new Date(t.deadline).getTime());
      return this.tasks.filter((t) => t.status === status).sort((a, b) => time(a) - time(b));
    },
    toggle(status) {
      this.open[status] = !this.open[status];
    },
    // Com o ponto aberto neste projeto, a tarefa entra na sessão em vez de abrir outra.
    start(t) {
      return this.run('clock', async () => {
        if (this.sessionHere()) {
          await clock().addTask(t.id);
          toast(WTT.t('session.modal.added', { name: t.name }));
          return;
        }
        await clock().clockIn(project.id, t.id);
        toast(WTT.t('tasks.started', { name: t.name }));
      });
    },
    isRunning(t) {
      return clock().isRunning(t.id);
    },
    sessionHere() {
      const s = clock().session;
      return !!s && s.project_id === project.id;
    },
    // Com o ponto aberto em outro projeto, a tarefa daqui não entra.
    otherProject() {
      const s = clock().session;
      return !!s && s.project_id !== project.id;
    },
    priorityClass,
    statusClass,
    deadlineClass: (t) => deadlineInfo(t.deadline).cls,
    deadlineLabel: (t) => deadlineInfo(t.deadline).label,
  }));

  Alpine.data('taskDetail', () => ({
    ...form(),
    ...taskWizard(),
    taskId: WTT.boot.task_id,
    hasStatus: true,
    loading: true,
    task: null,
    members: [],
    integrations: [],
    sessions: [],
    ...labelTools(),
    draft: { name: '', description: '', assign: 'none', assignee_id: '', deadline: '', priority: 'none', status: 'backlog', label_ids: [] }, // assign: me, none ou other
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
          this.loadLabels(),
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
      this.draft = {
        name: t.name,
        description: t.description || '',
        // Sem responsável, a própria pessoa, ou outra: é o que o modal pergunta.
        assign: !t.assignee_id ? 'none' : (t.assignee_id === me.id ? 'me' : 'other'),
        assignee_id: t.assignee_id && t.assignee_id !== me.id ? t.assignee_id : '',
        deadline: WTT.fmt.dateInput(t.deadline),
        priority: t.priority || 'none',
        status: t.status || 'backlog',
        label_ids: (t.labels || []).map((l) => l.id),
      };
      // Quem saiu dos times continua aparecendo como responsável atual.
      if (t.assignee && !this.members.some((m) => m.id === t.assignee_id)) {
        this.members = [t.assignee, ...this.members];
      }
      if (t.external_item_id) this.loadExternal();
      else this.external = { loading: false, details: null, error: '' };
    },
    // O ponto mudou: recarrega as sessões e a tarefa, que bater o ponto põe em progresso (e,
    // sem responsável, passa para quem bateu). Só os dados mostrados: o rascunho do modal fica.
    async reloadSessions() {
      try {
        const [sessions, task] = await Promise.all([
          api('GET', '/api/projects/' + project.id + '/work-sessions?task_id=' + this.taskId),
          api('GET', '/api/tasks/' + this.taskId),
        ]);
        this.sessions = sessions || [];
        this.task = task;
      } catch (e) {
        // Mantém a lista anterior; a próxima ação mostra o erro.
      }
    },
    async loadExternal() {
      this.external = { loading: true, details: null, error: '' };
      try {
        const res = await api('GET', '/api/tasks/' + this.taskId + '/external-details');
        this.external = { loading: false, details: res.details, error: res.details ? '' : (res.error ? WTT.errorText(res.error) : WTT.t('task_detail.no_response')) };
      } catch (e) {
        this.external = { loading: false, details: null, error: e.message };
      }
    },
    // O lápis abre o modal com um rascunho da tarefa; nada vai ao servidor antes de Salvar.
    openEdit() {
      this.setTask(this.task);
      this.errors.save = '';
      this.errors.delete = '';
      this.errors.label = '';
      this.newLabel = '';
      this.confirmDelete = false;
      this.resetWizard();
      Alpine.store('modal').open('task-edit', WTT.t('task_detail.edit_title'), () => !this.pending);
    },
    // Outra pessoa só pode ser responsável se estiver no projeto; quem saiu dele
    // continua aparecendo enquanto for o responsável atual (ver setTask).
    otherMembers() {
      return this.members.filter((m) => m.id !== me.id);
    },
    start() {
      return this.run('clock', async () => {
        await clock().clockIn(project.id, this.taskId);
        toast(WTT.t('tasks.started', { name: this.task.name }));
      });
    },
    // Com o ponto aberto neste projeto, a tarefa entra na sessão em vez de abrir outra.
    addToSession() {
      return this.run('clock', async () => {
        await clock().addTask(this.taskId);
        toast(WTT.t('session.modal.added', { name: this.task.name }));
      });
    },
    isRunning() {
      return clock().isRunning(this.taskId);
    },
    // sessionHere diz se o ponto aberto é deste projeto; num de outro, a tarefa não entra.
    sessionHere() {
      const s = clock().session;
      return !!s && s.project_id === project.id;
    },
    otherProject() {
      const s = clock().session;
      return !!s && s.project_id !== project.id;
    },
    openSession(s) {
      Alpine.store('sessionView').open(s || clock().session);
    },
    // O tempo da tarefa numa sessão e os nomes das outras tarefas dela.
    taskTime(s) {
      return clock().taskElapsed(s, this.taskId);
    },
    otherTasks(s) {
      const seen = new Map();
      s.tasks.forEach((l) => { if (l.task_id !== this.taskId && !seen.has(l.task_id)) seen.set(l.task_id, l.task.name); });
      return [...seen.values()].join(', ');
    },
    save() {
      return this.run('save', async () => {
        const t = await api('PATCH', '/api/tasks/' + this.taskId, {
          name: this.draft.name,
          description: this.draft.description,
          assignee_id: { me: me.id, none: '', other: this.draft.assignee_id }[this.draft.assign],
          deadline: WTT.fmt.fromDateInput(this.draft.deadline),
          priority: this.draft.priority,
          status: this.draft.status,
          label_ids: this.draft.label_ids,
        });
        this.setTask(t);
        Alpine.store('modal').close();
        toast(WTT.t('task_detail.saved'));
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
    // O tempo da tarefa: o que ela teve em cada sessão em que esteve.
    totalSeconds() {
      return this.sessions.reduce((sum, s) => sum + this.taskTime(s), 0);
    },
    deadlineClass() { return this.task ? deadlineInfo(this.task.deadline).cls : ''; },
    deadlineLabel() { return this.task ? deadlineInfo(this.task.deadline).label : ''; },
    externalLabel() { return this.task ? externalLabel(this.task) : ''; },
    // linkType é o tipo da integração escolhida no vínculo: dele vêm o rótulo e o
    // exemplo do campo do item (o número da issue, o cartão).
    linkType() {
      const chosen = this.integrations.find((i) => i.id === this.linkForm.integration_id);
      return (chosen && integrationType(chosen.type)) || {};
    },
    typeLabel(type) {
      const t = integrationType(type);
      return t ? t.label : type;
    },
  }));

  Alpine.data('projectIntegrations', () => ({
    ...form(),
    types: integrationTypes,
    loading: true,
    items: [],
    // O rascunho do modal, o mesmo para criar e editar (id nulo é criação). O corpo é
    // igual para todos os tipos: o que é da plataforma vai em metadata. Nada vai para
    // o servidor antes de Salvar.
    draft: { id: null, type: '', display_name: '', token: '', metadata: {}, enabled: true, has_token: false },
    confirming: false,
    async init() {
      try {
        this.items = (await api('GET', '/api/projects/' + project.id + '/integrations')) || [];
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
    },
    typeOf(type) {
      return integrationType(type) || { type, label: type, metadata: [] };
    },
    // facts são os campos que identificam a conexão no cartão: o repositório, o quadro.
    facts(it) {
      return this.typeOf(it.type).metadata.filter((f) => f.summary)
        .map((f) => ({ key: f.key, label: f.label, value: (it.metadata || {})[f.key] || '' }));
    },
    // Uma integração de antes do metadata não tem os campos da plataforma guardados.
    incomplete(it) {
      return this.typeOf(it.type).metadata.some((f) => f.required && !(it.metadata || {})[f.key]);
    },
    openCreate() {
      this.draft = { id: null, type: this.types.length ? this.types[0].type : '', display_name: '', token: '', metadata: {}, enabled: true, has_token: false };
      this.openForm(WTT.t('integrations.new'));
    },
    openEdit(it) {
      this.draft = {
        id: it.id, type: it.type, display_name: it.display_name, token: '',
        metadata: { ...(it.metadata || {}) }, enabled: it.enabled, has_token: it.has_token,
      };
      this.openForm(WTT.t('integrations.edit_title'));
    },
    openForm(title) {
      this.confirming = false;
      this.errors.save = '';
      this.errors.remove = '';
      Alpine.store('modal').open('integration-form', title, () => !this.pending);
    },
    // Cada plataforma tem os seus campos: trocar de uma para outra começa do zero.
    pickType() {
      this.draft.metadata = {};
    },
    save() {
      this.errors.remove = '';
      return this.run('save', async () => {
        const d = this.draft;
        const body = { display_name: d.display_name.trim(), enabled: d.enabled, token: d.token.trim(), metadata: { ...d.metadata } };
        if (!body.display_name) throw new Error(WTT.t('integrations.name_required'));
        if (d.id) {
          const saved = await api('PATCH', '/api/integrations/' + d.id, body);
          this.items = this.items.map((x) => (x.id === saved.id ? saved : x));
          toast(WTT.t('integrations.saved'));
        } else {
          const created = await api('POST', '/api/projects/' + project.id + '/integrations', { type: d.type, ...body });
          this.items = [created, ...this.items];
          toast(WTT.t('integrations.created'));
        }
        Alpine.store('modal').close();
      });
    },
    remove() {
      this.errors.save = '';
      return this.run('remove', async () => {
        const id = this.draft.id;
        await api('DELETE', '/api/integrations/' + id);
        this.items = this.items.filter((x) => x.id !== id);
        Alpine.store('modal').close();
        toast(WTT.t('integrations.deleted'));
      });
    },
  }));

  // O Início do projeto é o de quem está olhando, admin ou não: o relógio, as tarefas em
  // que a pessoa está mexendo (as que têm o nome dela; pegar uma disponível é pelo quadro
  // de tarefas), o seu tempo neste projeto e as suas sessões, que o servidor já limita a
  // quem pede.
  Alpine.data('projectMyOverview', () => ({
    ...form(),
    ...sessionList(),
    project,
    loading: true,
    tasks: [], // só as tarefas da pessoa logada
    taskId: '',
    myRate: null, // quanto a pessoa logada recebe por hora aqui; null se ainda não tem valor
    async init() {
      this.watchSessionFilters();
      try {
        const [sessions, allocations, tasks, billing] = await Promise.all([
          api('GET', '/api/projects/' + project.id + '/work-sessions?person_id=' + me.id),
          api('GET', '/api/projects/' + project.id + '/allocations'),
          api('GET', '/api/projects/' + project.id + '/tasks?assignee_id=' + me.id),
          // O dono ganha o valor cobrado, que só os admins leem.
          me.is_owner ? api('GET', '/api/projects/' + project.id + '/billing') : null,
        ]);
        this.sessions = sessions || [];
        this.tasks = tasks || [];
        this.taskId = this.tasks.length ? this.tasks[0].id : '';
        const own = (allocations || []).find((a) => a.person_id === me.id);
        this.myRate = me.is_owner ? billing.bill_rate_cents : (own ? own.pay_rate_cents : null);
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
      window.addEventListener('wtt:sessions-changed', () => this.reloadSessions());
    },
    // noRate é quem não pode bater ponto por falta de valor por hora. O dono bate sem
    // valor pago: num projeto sem valor cobrado, só conta o tempo.
    noRate() {
      return !me.is_owner && this.myRate === null;
    },
    async reloadSessions() {
      try {
        this.sessions = (await api('GET', '/api/projects/' + project.id + '/work-sessions?person_id=' + me.id)) || [];
      } catch (e) {
        this.errors.load = e.message;
      }
    },
    start() {
      return this.run('clock', () => clock().clockIn(project.id, this.taskId));
    },
    // Com o ponto aberto neste projeto, a tarefa entra na sessão em vez de abrir outra.
    startTask(t) {
      if (this.sessionHere()) return this.run('clock', () => clock().addTask(t.id));
      this.taskId = t.id;
      return this.start();
    },
    stop() {
      return this.run('clock', () => clock().clockOut());
    },
    isRunning(t) {
      return clock().isRunning(t.id);
    },
    sessionHere() {
      const s = clock().session;
      return !!s && s.project_id === project.id;
    },
    openSession(s) {
      Alpine.store('sessionView').open(s || clock().session);
    },
    // sessionValue é quanto a sessão aberta já rendeu: o tempo corrido vezes o valor por
    // hora travado no clock-in, com o arredondamento do servidor. null sem valor.
    sessionValue() {
      const s = clock().session;
      const rate = s ? earnedRate(s) : null;
      if (rate === null || rate === undefined) return null;
      return Math.round(clock().elapsed(s) * rate / 3600);
    },
  }));

  // A aba Configurações só mostra o projeto. Quem altera é o modal Editar projeto,
  // só de admins: ele edita um rascunho, e nada vai para o servidor antes de Salvar.
  Alpine.data('projectSettings', () => ({
    ...form(),
    loading: true,
    current: { name: '', description: '', sprint_duration_days: 14, customer: null }, // o projeto como está no servidor
    billRateCents: null, // o valor cobrado; só admins recebem
    customers: [],
    draft: { name: '', description: '', sprint_duration_days: 14, ...WTT.routine.blank(), customer_id: '', rate: '' },
    confirmDelete: false,
    async init() {
      try {
        this.current = await api('GET', '/api/projects/' + project.id);
        // O valor cobrado é de quem vê o faturamento, e a lista de clientes, de quem cuida deles.
        const [billing, customers] = await Promise.all([
          WTT.can('billing.view') ? api('GET', '/api/projects/' + project.id + '/billing') : null,
          WTT.can('customers.manage') ? api('GET', '/api/orgs/' + me.organization_id + '/customers') : null,
        ]);
        this.customers = customers || [];
        if (billing) this.billRateCents = billing.bill_rate_cents;
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
    },
    // As linhas do cartão do projeto. Sem daily ou sem weekly é uma resposta, e
    // não um campo por preencher: o time pode não fazer.
    routine() {
      const p = this.current;
      return [
        { label: WTT.t('project.fields.sprint'), value: WTT.fmt.sprint(p.sprint_duration_days) },
        { label: WTT.t('project.fields.daily'), value: p.daily_time || WTT.t('project.fields.no_daily'), empty: !p.daily_time },
        { label: WTT.t('project.fields.weekly'), value: p.weekly_sync_day ? WTT.fmt.weeklySlot(p.weekly_sync_day, p.weekly_sync_time) : WTT.t('project.fields.no_weekly'), empty: !p.weekly_sync_day },
      ];
    },
    // A reunião com o cliente aparece no cartão do cliente, e só com um cliente.
    meeting() {
      const p = this.current;
      return p.customer_meeting_day ? WTT.fmt.weeklySlot(p.customer_meeting_day, p.customer_meeting_time) : WTT.t('project.fields.no_meeting');
    },
    sprintChoices() {
      return WTT.sprintChoices(this.current.sprint_duration_days);
    },
    openEdit() {
      const p = this.current;
      this.draft = {
        name: p.name,
        description: p.description || '',
        sprint_duration_days: p.sprint_duration_days,
        ...WTT.routine.fromProject(p),
        customer_id: p.customer ? p.customer.id : '',
        rate: WTT.fmt.moneyInput(this.billRateCents),
      };
      this.errors.save = '';
      this.errors.delete = '';
      this.confirmDelete = false;
      // Enquanto salva, o modal não fecha: um erro do servidor ficaria sem ter onde aparecer.
      Alpine.store('modal').open('project-edit', WTT.t('project_settings.edit_title'), () => !this.pending);
    },
    // save manda o projeto e, se o cliente ou o valor mudou, a cobrança, que é
    // outra rota. Se a segunda falhar, a primeira já valeu: a aba atrás do modal
    // mostra o que foi salvo, e Salvar de novo só repete o que faltou.
    save() {
      return this.run('save', async () => {
        const d = this.draft;
        const editProject = WTT.can('project.edit');
        const editBilling = WTT.can('billing.manage');
        // O valor é conferido antes de qualquer chamada. Vazio apaga.
        const cents = editBilling ? WTT.toCents(d.rate) : this.billRateCents;
        if (editBilling) {
          if (cents === null && String(d.rate).trim() !== '') throw new Error(WTT.t('org.projects.rate_invalid'));
          if (cents !== null && cents > 100000000) throw new Error(WTT.t('org.projects.rate_too_high'));
        }

        // Cada parte vai pela rota da permissão dela: o projeto e a reunião, em project.edit,
        // e o cliente e o valor cobrado, em billing.manage. Sem daily ou sem weekly vai texto
        // vazio, que apaga; a API mantém o que não vier no corpo.
        if (editProject) {
          this.current = await api('PATCH', '/api/projects/' + project.id, {
            name: d.name,
            description: d.description,
            sprint_duration_days: Number(d.sprint_duration_days) || 0,
            ...WTT.routine.payload(d),
          });
          document.querySelectorAll('[data-project-name]').forEach((el) => { el.textContent = this.current.name; });
        }

        const customerId = this.current.customer ? this.current.customer.id : '';
        if (editBilling && (d.customer_id !== customerId || cents !== this.billRateCents)) {
          const billing = await api('PUT', '/api/projects/' + project.id + '/billing', {
            customer_id: d.customer_id || null,
            bill_rate_cents: cents,
          });
          this.current.customer = billing.customer;
          this.billRateCents = billing.bill_rate_cents;
          // Tirar o cliente apaga a reunião com ele, e o servidor devolve o projeto como ficou.
          if (!billing.customer) {
            this.current.customer_meeting_day = undefined;
            this.current.customer_meeting_time = undefined;
          }
        }
        toast(WTT.t('project_settings.saved'));
        Alpine.store('modal').close();
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

  // byName ordena colaboradores pelo nome como se lê no idioma da tela. A API ordena
  // pelos bytes, o que joga "Íris" para depois de "Nuno"; com a lista em
  // páginas, a pessoa iria parar na página errada.
  const byName = (a, b) => a.person.name.localeCompare(b.person.name, WTT.lang);

  // As listas de pessoas da aba Colaboradores mostram no máximo cinco por vez:
  // a tabela e os integrantes de cada cartão de time.
  const PEOPLE_PER_PAGE = 5;

  // paginate corta a lista na página pedida. Quando a lista encolheu (uma
  // busca, alguém que saiu), a página pedida pode não existir mais, e vale a última.
  function paginate(list, page) {
    const pages = Math.max(1, Math.ceil(list.length / PEOPLE_PER_PAGE));
    const current = Math.min(Math.max(1, page || 1), pages);
    const start = (current - 1) * PEOPLE_PER_PAGE;
    return {
      rows: list.slice(start, start + PEOPLE_PER_PAGE),
      page: current,
      pages,
      total: list.length,
      from: list.length ? start + 1 : 0,
      to: Math.min(start + PEOPLE_PER_PAGE, list.length),
    };
  }

  // A aba Colaboradores: quem está no projeto, com o valor por hora de cada
  // pessoa, e os times. A pessoa entra no projeto com o valor dela e só depois
  // pode entrar num time. As mesmas pessoas aparecem em duas visões, a lista de
  // todas e os times, e tudo de uma pessoa muda no modal do colaborador.
  Alpine.data('projectTeams', () => ({
    ...form(),
    meId: me.id,
    loading: true,
    view: new URLSearchParams(location.search).get('view') === 'teams' ? 'teams' : 'people',
    collaborators: [], // { person, teams, pay_rate_cents }
    teams: [],
    billing: { customer: null, bill_rate_cents: null }, // o que o cliente paga; só admins recebem
    people: [], // todas as pessoas da organização, para os admins adicionarem
    sessions: [], // as sessões de ponto do projeto, para o admin ver quanto as horas valem
    search: '',
    page: 1, // a página da tabela de pessoas
    teamPages: {}, // por time, a página dos integrantes no cartão
    confirming: null, // 'person-<id>' ou 'team-<id>'
    add: { search: '', person_id: '', rate: '', team_id: '' },
    addStep: 1, // 1: quem entra, o valor e o time; 2: o grupo de permissões
    presets: [], // os grupos do catálogo da API
    preset: 'member', // o grupo marcado no modal aberto (Adicionar pessoa ou Editar colaborador)
    newTeam: '',
    // O rascunho do modal Editar time: o nome, quem está marcado e, em people,
    // quem está no projeto, na ordem em que a lista aparece.
    edit: { id: '', name: '', search: '', member_ids: [], people: [] },
    // O rascunho do modal do colaborador: o valor por hora como texto e os times marcados.
    person: { id: '', name: '', email: '', is_owner: false, is_admin: false, preset: 'member', rate: '', team_ids: [] },
    async init() {
      this.$watch('search', () => { this.page = 1; }); // uma busca nova começa da primeira página
      try {
        // Fora da Gestão a aba é só de leitura, para admin também.
        const manage = !WTT.boot.readonly;
        const [collaborators, teams, billing, people, sessions] = await Promise.all([
          api('GET', '/api/projects/' + project.id + '/collaborators'),
          api('GET', '/api/projects/' + project.id + '/teams'),
          // O valor cobrado, a lista de quem pode entrar e as sessões só servem a quem cuida do projeto.
          manage && WTT.can('billing.view') ? api('GET', '/api/projects/' + project.id + '/billing') : null,
          manage && WTT.can('collaborators.manage') ? api('GET', '/api/orgs/' + me.organization_id + '/persons') : null,
          manage && WTT.can('rates.view') ? api('GET', '/api/projects/' + project.id + '/work-sessions') : null,
        ]);
        this.collaborators = (collaborators || []).sort(byName);
        this.teams = teams || [];
        if (billing) this.billing = billing;
        this.people = people || [];
        this.sessions = sessions || [];
        if (WTT.can('collaborators.manage') && manage) this.presets = (await api('GET', '/api/permissions')).presets;
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
    },
    // reload busca os colaboradores de novo: quem entra ou sai de um time, ou do
    // projeto, muda a tabela de pessoas e os cartões dos times de uma vez.
    async reload() {
      this.collaborators = ((await api('GET', '/api/projects/' + project.id + '/collaborators')) || []).sort(byName);
    },
    // setView troca entre a lista de pessoas e os times, e guarda a escolha na
    // URL para um recarregamento cair na mesma visão.
    setView(view) {
      this.view = view;
      history.replaceState(null, '', location.pathname + (view === 'teams' ? '?view=teams' : ''));
    },
    // focusView é a troca pelas setas do teclado: o foco acompanha a aba escolhida.
    focusView(view) {
      this.setView(view);
      this.$nextTick(() => this.$refs[view === 'teams' ? 'tabTeams' : 'tabPeople'].focus());
    },
    rows() {
      const query = fold(this.search.trim());
      return this.collaborators.filter((c) => matches(c.person, query));
    },
    peoplePage() {
      return paginate(this.rows(), this.page);
    },
    peopleSummary() {
      const p = this.peoplePage();
      return WTT.t('collab.people_page', { page: p.page, pages: p.pages, count: p.total });
    },
    teamPage(team) {
      return paginate(this.membersOf(team), this.teamPages[team.id]);
    },
    // margin é a soma das margens por hora de quem tem valor, com os valores
    // de hoje: o que o projeto ganha numa hora em que todos trabalham. null sem
    // valor cobrado do cliente.
    summary() {
      const bill = this.billing.bill_rate_cents;
      return {
        people: this.collaborators.length,
        noTeam: this.collaborators.filter((c) => c.teams.length === 0).length,
        teams: this.teams.length,
        margin: bill === null ? null : this.collaborators
          .filter((c) => c.pay_rate_cents !== null)
          .reduce((sum, c) => sum + bill - c.pay_rate_cents, 0),
      };
    },
    // recorded soma as sessões de ponto já fechadas do projeto e quanto elas
    // valem: o tempo de cada uma vezes o valor por hora que a pessoa tinha
    // quando o ponto abriu, na conta que o servidor fez ao fechar. A sessão em
    // andamento fica de fora até o clock-out.
    recorded() {
      return this.sessions.reduce((total, s) => {
        if (!s.end_at) return total;
        total.seconds += clock().elapsed(s);
        if (s.pay_amount_cents !== null && s.pay_amount_cents !== undefined) total.cents += s.pay_amount_cents;
        return total;
      }, { seconds: 0, cents: 0 });
    },
    // Quem entrou num time antes de o valor por hora ser obrigatório e ficou
    // sem valor: não bate ponto até um admin definir. Hoje ninguém entra assim.
    missing() {
      return this.collaborators.filter((c) => c.pay_rate_cents === null);
    },
    membersOf(team) {
      return this.collaborators.filter((c) => c.teams.some((t) => t.id === team.id));
    },
    inProject(person) {
      return this.collaborators.some((c) => c.person.id === person.id);
    },
    margin(c) {
      if (this.billing.bill_rate_cents === null || c.pay_rate_cents === null) return null;
      return this.billing.bill_rate_cents - c.pay_rate_cents;
    },
    // Os grupos que quem olha pode dar: só os que têm tudo o que ele mesmo pode, porque ninguém
    // concede o que não tem (o servidor confere de novo).
    // O grupo financeiro existe na API, mas as telas não o oferecem por enquanto.
    presetChoices() {
      return this.presets
        .filter((p) => !HIDDEN_PRESETS.includes(p.id))
        .filter((p) => p.permissions.every((k) => WTT.can(k)));
    },
    // presetParts diz como mostrar as permissões de um grupo: quando ele traz tudo o que um
    // grupo menor já traz, aparece só "tudo o que ele faz, e mais:" e o que o grupo acrescenta,
    // para a diferença entre os dois ficar à vista.
    presetParts(p) {
      const shown = this.presets.filter((q) => !HIDDEN_PRESETS.includes(q.id) && q.id !== p.id);
      const base = shown
        .filter((q) => q.permissions.length > 0 && q.permissions.every((k) => p.permissions.includes(k)))
        .sort((a, b) => b.permissions.length - a.permissions.length)[0];
      if (!base) return { from: '', keys: p.permissions };
      return { from: base.id, keys: p.permissions.filter((k) => !base.permissions.includes(k)) };
    },
    // presetText é o nome curto de uma permissão do catálogo, para os selos dos grupos.
    presetText(key) {
      return WTT.t('permissions.keys.' + key.replace('.', '_') + '.name');
    },
    // O dono e os admins já têm todas as permissões: para eles não há grupo a escolher.
    hasAll(person) {
      return !!person && (!!person.is_owner || person.role === 'admin');
    },
    // openPerson abre o modal do colaborador com um rascunho do valor, do grupo e dos
    // times. Abre da linha da tabela e da pessoa no cartão de um time.
    openPerson(c) {
      const full = this.people.find((p) => p.id === c.person.id);
      this.person = {
        id: c.person.id,
        name: c.person.name,
        email: c.person.email,
        is_owner: !!c.person.is_owner,
        is_admin: this.hasAll(full),
        preset: c.preset || 'member',
        rate: WTT.fmt.moneyInput(c.pay_rate_cents),
        team_ids: c.teams.map((t) => t.id),
      };
      this.preset = this.person.preset;
      this.confirming = null;
      this.errors.person = '';
      Alpine.store('modal').open('collab-edit', WTT.t('collab.edit_person'), () => !this.pending);
    },
    // A margem com o valor que está digitado no modal, ou null sem valor cobrado
    // do cliente ou sem um valor válido no campo.
    personMargin() {
      const cents = WTT.toCents(this.person.rate);
      if (this.billing.bill_rate_cents === null || cents === null) return null;
      return this.billing.bill_rate_cents - cents;
    },
    // savePerson aplica o rascunho com as rotas que já existiam: o valor e,
    // time a time, de onde a pessoa saiu e onde entrou. O valor vai primeiro,
    // porque um time só aceita quem tem valor no projeto. Se uma chamada
    // falhar, o que já foi aplicado continua valendo e o modal fica aberto com
    // o erro; salvar de novo só repete o que faltou.
    savePerson() {
      return this.run('person', async () => {
        const c = this.collaborators.find((x) => x.person.id === this.person.id);
        if (!c) throw new Error(WTT.t('collab.person_gone'));
        // O dono não tem valor pago: as horas dele valem o valor cobrado.
        // Quem não define o valor deixa o campo como está: ele nem aparece.
        const cents = c.person.is_owner || !WTT.can('rates.manage') ? c.pay_rate_cents : WTT.toCents(this.person.rate);
        if (cents === null && WTT.can('rates.manage')) throw new Error(WTT.t('collab.rate_required'));
        const current = c.teams.map((t) => t.id);
        const wanted = this.person.team_ids;
        const leaving = current.filter((id) => !wanted.includes(id));
        const joining = wanted.filter((id) => !current.includes(id));
        const body = { person_id: c.person.id };
        try {
          // O valor e o grupo vão pela mesma rota, só com o que mudou: cada um pede a sua permissão.
          const allocation = {};
          if (WTT.can('rates.manage') && !c.person.is_owner && cents !== c.pay_rate_cents) allocation.pay_rate_cents = cents;
          if (WTT.can('collaborators.manage') && !this.person.is_admin && this.preset !== this.person.preset && this.preset !== 'custom') allocation.preset = this.preset;
          if (Object.keys(allocation).length) {
            await api('PUT', '/api/projects/' + project.id + '/allocations/' + c.person.id, allocation);
          }
          if (WTT.can('teams.manage')) {
            for (const id of leaving) await api('DELETE', '/api/teams/' + id + '/members', body);
            for (const id of joining) await api('POST', '/api/teams/' + id + '/members', body);
          }
        } catch (e) {
          await this.reload().catch(() => {});
          throw e;
        }
        await this.reload();
        Alpine.store('modal').close();
        toast(WTT.t('collab.person_saved', { name: c.person.name }));
      });
    },
    removePerson() {
      return this.run('person', async () => {
        const { id, name } = this.person;
        await api('DELETE', '/api/projects/' + project.id + '/collaborators/' + id);
        Alpine.store('modal').close();
        toast(WTT.t('collab.left', { name }));
        await this.reload();
      });
    },
    // Quem pode entrar no projeto: as pessoas da organização que ainda não estão nele.
    addCandidates() {
      const query = fold(this.add.search.trim());
      return this.people.filter((p) => !this.inProject(p) && matches(p, query));
    },
    // addIsOwner diz se a pessoa escolhida é o dono, que entra sem valor por hora.
    addIsOwner() {
      const p = this.people.find((x) => x.id === this.add.person_id);
      return !!p && !!p.is_owner;
    },
    // pruneAdd desfaz a escolha quando a busca tira da lista a pessoa escolhida.
    pruneAdd() {
      if (!this.addCandidates().some((p) => p.id === this.add.person_id)) this.add.person_id = '';
    },
    openAdd() {
      this.add = { search: '', person_id: '', rate: '', team_id: '' };
      this.addStep = 1;
      this.preset = 'member';
      this.errors.add = '';
      Alpine.store('modal').open('collab-add', WTT.t('collab.add_title'), () => !this.pending);
    },
    // A pessoa escolhida na etapa 1, com o papel e o dono, para saber se ela precisa de um grupo.
    addTarget() {
      return this.addCandidates().find((p) => p.id === this.add.person_id);
    },
    addNeedsGroup() {
      const p = this.addTarget();
      return !!p && !this.hasAll(p) && this.presetChoices().length > 0;
    },
    // addNext confere a etapa 1 e, quando a pessoa precisa de um grupo, vai para a 2; senão grava direto.
    addNext() {
      const person = this.addTarget();
      if (!person) {
        this.errors.add = WTT.t('collab.choose_person');
        return undefined;
      }
      if (!person.is_owner && WTT.toCents(this.add.rate) === null) {
        this.errors.add = WTT.t('collab.rate_required');
        return undefined;
      }
      this.errors.add = '';
      if (!this.addNeedsGroup()) return this.addPerson();
      this.addStep = 2;
      return undefined;
    },
    addBack() {
      this.addStep = 1;
      this.errors.add = '';
    },
    // addSubmit é o envio do formulário: na etapa 1 avança, na 2 grava.
    addSubmit() {
      return this.addStep === 1 ? this.addNext() : this.addPerson();
    },
    addPerson() {
      return this.run('add', async () => {
        const person = this.addTarget();
        if (!person) throw new Error(WTT.t('collab.choose_person'));
        const cents = person.is_owner ? 0 : WTT.toCents(this.add.rate);
        if (cents === null) throw new Error(WTT.t('collab.rate_required'));
        const body = { pay_rate_cents: cents };
        if (this.addNeedsGroup() && this.preset !== 'member') body.preset = this.preset;
        await api('PUT', '/api/projects/' + project.id + '/allocations/' + person.id, body);
        // Daqui em diante a pessoa já está no projeto: se o time falhar, ela
        // fica sem time e a tela avisa, em vez de parecer que nada aconteceu.
        let teamError = '';
        if (this.add.team_id && WTT.can('teams.manage')) {
          try {
            await api('POST', '/api/teams/' + this.add.team_id + '/members', { person_id: person.id });
          } catch (e) {
            teamError = e.message;
          }
        }
        await this.reload();
        // A tabela vai para a página em que a pessoa ficou, pela ordem dos nomes.
        const at = this.rows().findIndex((c) => c.person.id === person.id);
        if (at >= 0) this.page = Math.floor(at / PEOPLE_PER_PAGE) + 1;
        Alpine.store('modal').close();
        if (teamError) Alpine.store('toast').error(WTT.t('collab.joined_no_team', { name: person.name, error: teamError }));
        else toast(WTT.t('collab.joined', { name: person.name }));
      });
    },
    openTeam() {
      this.newTeam = '';
      this.errors.create = '';
      Alpine.store('modal').open('team-new', WTT.t('collab.new_team'), () => !this.pending);
    },
    createTeam() {
      return this.run('create', async () => {
        const t = await api('POST', '/api/projects/' + project.id + '/teams', { name: this.newTeam });
        this.teams = [t, ...this.teams];
        Alpine.store('modal').close();
        toast(WTT.t('collab.team_created'));
      });
    },
    // openEdit abre o modal Editar time com um rascunho do nome e dos integrantes.
    // A lista traz quem já está no time e, depois, o resto de quem está no
    // projeto: um time só aceita quem já entrou no projeto, com valor por hora.
    // A ordem é fixada aqui, para as linhas não trocarem de lugar a cada caixa
    // marcada.
    openEdit(team) {
      const members = this.membersOf(team).map((c) => c.person);
      const inTeam = new Set(members.map((p) => p.id));
      const others = this.collaborators
        .filter((c) => c.pay_rate_cents !== null && !inTeam.has(c.person.id))
        .map((c) => c.person);
      this.edit = { id: team.id, name: team.name, search: '', member_ids: [...inTeam], people: [...members, ...others] };
      this.confirming = null;
      this.errors.team = '';
      Alpine.store('modal').open('team-edit', WTT.t('collab.edit_team'), () => !this.pending);
      // A lista guarda a rolagem da última abertura; cada uma começa do topo, onde estão os integrantes.
      this.$nextTick(() => { this.$refs.editList.scrollTop = 0; });
    },
    editCandidates() {
      const query = fold(this.edit.search.trim());
      return this.edit.people.filter((p) => matches(p, query));
    },
    // saveTeam aplica o rascunho com as rotas de time que já existiam: o nome e,
    // pessoa a pessoa, quem saiu e quem entrou. Se uma chamada falhar, o que já
    // foi aplicado continua valendo e o modal fica aberto com o erro; salvar de
    // novo só repete o que faltou, porque a diferença é calculada contra o que
    // o servidor tem.
    saveTeam() {
      return this.run('team', async () => {
        const team = this.teams.find((t) => t.id === this.edit.id);
        const name = this.edit.name.trim();
        if (!name) throw new Error(WTT.t('collab.team_name_required'));
        const current = this.membersOf(team).map((c) => c.person.id);
        const wanted = this.edit.member_ids;
        const leaving = current.filter((id) => !wanted.includes(id));
        const joining = wanted.filter((id) => !current.includes(id));
        const members = '/api/teams/' + team.id + '/members';
        try {
          if (name !== team.name) team.name = (await api('PATCH', '/api/teams/' + team.id, { name })).name;
          for (const id of leaving) await api('DELETE', members, { person_id: id });
          for (const id of joining) await api('POST', members, { person_id: id });
        } catch (e) {
          await this.reload().catch(() => {});
          throw e;
        }
        // O nome do time e quem está nele também aparecem na tabela de pessoas.
        await this.reload();
        Alpine.store('modal').close();
        toast(WTT.t('collab.team_saved'));
      });
    },
    removeTeam() {
      return this.run('team', async () => {
        const id = this.edit.id;
        await api('DELETE', '/api/teams/' + id);
        this.teams = this.teams.filter((t) => t.id !== id);
        Alpine.store('modal').close();
        toast(WTT.t('collab.team_deleted'));
        await this.reload(); // o time sai da linha de cada pessoa que estava nele
      });
    },
  }));
});
