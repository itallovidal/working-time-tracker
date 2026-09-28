// Componentes das abas de um projeto.
document.addEventListener('alpine:init', () => {
  const { api, form } = WTT;
  const me = WTT.boot.me;
  const project = WTT.boot.project;
  const toast = (msg, kind) => Alpine.store('toast').show(msg, kind);

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
