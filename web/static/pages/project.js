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
    tasks: [],
    members: [],
    creating: false,
    onlyMine: false,
    draft: { name: '', description: '', assignee_id: '', deadline: '' },
    async init() {
      try {
        const [tasks, members] = await Promise.all([
          api('GET', '/api/projects/' + project.id + '/tasks'),
          api('GET', '/api/projects/' + project.id + '/members'),
        ]);
        this.tasks = tasks || [];
        this.members = members || [];
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
    },
    visible() {
      return this.onlyMine ? this.tasks.filter((t) => t.assignee_id === me.id) : this.tasks;
    },
    openCreate() {
      const self = this.members.find((m) => m.id === me.id) || this.members[0];
      this.draft = { name: '', description: '', assignee_id: self ? self.id : '', deadline: '' };
      this.creating = true;
      this.$nextTick(() => this.$refs.name && this.$refs.name.focus());
    },
    create() {
      return this.run('create', async () => {
        const t = await api('POST', '/api/projects/' + project.id + '/tasks', {
          name: this.draft.name,
          description: this.draft.description,
          assignee_id: this.draft.assignee_id,
          deadline: WTT.fmt.fromDateInput(this.draft.deadline),
        });
        this.tasks = [t, ...this.tasks];
        this.creating = false;
        toast('Tarefa criada.');
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
        location.href = '/projects/' + project.id + '/tasks';
      });
    },
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
    filter: { task: '', person: '' },
    async init() {
      try {
        const [tasks, sessions] = await Promise.all([
          api('GET', '/api/projects/' + project.id + '/tasks'),
          api('GET', '/api/projects/' + project.id + '/work-sessions'),
        ]);
        this.tasks = tasks || [];
        this.sessions = sessions || [];
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
    // mine soma o tempo da pessoa logada hoje ou nesta semana (a partir de segunda).
    mine(period) {
      const start = new Date(clock().now);
      start.setHours(0, 0, 0, 0);
      if (period === 'week') start.setDate(start.getDate() - ((start.getDay() + 6) % 7));
      return secondsWithin(this.sessions.filter((s) => s.person_id === me.id), start.getTime());
    },
  }));

  Alpine.data('projectSettings', () => ({
    ...form(),
    loading: true,
    form: { name: '', description: '', sprint_duration_days: 14, daily_time: '', weekly_sync_day: '' },
    confirmDelete: false,
    async init() {
      try {
        const p = await api('GET', '/api/projects/' + project.id);
        this.form = {
          name: p.name,
          description: p.description || '',
          sprint_duration_days: p.sprint_duration_days,
          daily_time: p.daily_time || '',
          weekly_sync_day: p.weekly_sync_day || '',
        };
      } catch (e) {
        this.errors.save = e.message;
      } finally {
        this.loading = false;
      }
    },
    save() {
      return this.run('save', async () => {
        // Texto vazio apaga daily e weekly; a API mantém o que não vier no corpo.
        const p = await api('PATCH', '/api/projects/' + project.id, {
          name: this.form.name,
          description: this.form.description,
          sprint_duration_days: Number(this.form.sprint_duration_days) || 0,
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

  Alpine.data('projectTeams', () => ({
    ...form(),
    loading: true,
    teams: [],
    people: [],
    newTeam: '',
    editing: null,
    editName: '',
    confirming: null,
    adding: {},
    async init() {
      try {
        const [teams, people] = await Promise.all([
          api('GET', '/api/projects/' + project.id + '/teams'),
          api('GET', '/api/orgs/' + me.organization_id + '/persons'),
        ]);
        this.people = people || [];
        this.teams = await Promise.all((teams || []).map(async (t) => ({
          ...t,
          members: (await api('GET', '/api/teams/' + t.id + '/members')) || [],
        })));
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
    },
    candidates(team) {
      const inTeam = new Set(team.members.map((m) => m.person_id));
      return this.people.filter((p) => !inTeam.has(p.id));
    },
    createTeam() {
      return this.run('create', async () => {
        const t = await api('POST', '/api/projects/' + project.id + '/teams', { name: this.newTeam });
        this.teams = [{ ...t, members: [] }, ...this.teams];
        this.newTeam = '';
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
      });
    },
    removeTeam(team) {
      return this.run('team-' + team.id, async () => {
        await api('DELETE', '/api/teams/' + team.id);
        this.teams = this.teams.filter((t) => t.id !== team.id);
        this.confirming = null;
        toast('Time excluído.');
      });
    },
    addMember(team) {
      const personId = this.adding[team.id];
      if (!personId) return undefined;
      return this.run('team-' + team.id, async () => {
        await api('POST', '/api/teams/' + team.id + '/members', { person_id: personId });
        team.members = (await api('GET', '/api/teams/' + team.id + '/members')) || [];
        this.adding[team.id] = '';
      });
    },
    removeMember(team, member) {
      return this.run('team-' + team.id, async () => {
        await api('DELETE', '/api/teams/' + team.id + '/members', { person_id: member.person_id });
        team.members = team.members.filter((m) => m.person_id !== member.person_id);
      });
    },
  }));
});
