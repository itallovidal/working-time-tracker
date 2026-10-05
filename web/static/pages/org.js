// Componentes das páginas da organização (a lista de projetos e as abas Geral, Pessoas e Projetos)
// e do perfil de quem está logado.
document.addEventListener('alpine:init', () => {
  const { api, form } = WTT;
  const me = WTT.boot.me;
  const orgId = me.organization_id;
  const toast = (msg, kind) => Alpine.store('toast').show(msg, kind);
  const setText = (selector, text) => document.querySelectorAll(selector).forEach((el) => { el.textContent = text; });

  const blankProject = () => ({ name: '', description: '', sprint_duration_days: 14, daily_time: '', weekly_sync_day: '' });

  Alpine.data('orgProjects', () => ({
    ...form(),
    loading: true,
    projects: [],
    creating: false,
    draft: blankProject(),
    async init() {
      try {
        this.projects = (await api('GET', '/api/orgs/' + orgId + '/projects')) || [];
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
    },
    openCreate() {
      this.draft = blankProject();
      this.creating = true;
      this.$nextTick(() => this.$refs.name && this.$refs.name.focus());
    },
    create() {
      return this.run('create', async () => {
        const p = await api('POST', '/api/orgs/' + orgId + '/projects', {
          name: this.draft.name,
          description: this.draft.description,
          sprint_duration_days: Number(this.draft.sprint_duration_days) || 0,
          daily_time: this.draft.daily_time || null,
          weekly_sync_day: this.draft.weekly_sync_day || null,
        });
        location.href = '/projects/' + p.id;
      });
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
    async init() {
      try {
        const [people, invites] = await Promise.all([
          api('GET', '/api/orgs/' + orgId + '/persons'),
          api('GET', '/api/orgs/' + orgId + '/invites'),
        ]);
        this.people = people || [];
        this.invites = invites || [];
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
    },
    setRole(person, role) {
      return this.run('role', async () => {
        const updated = await api('PATCH', '/api/persons/' + person.id + '/role', { role });
        person.role = updated.role;
        toast(person.name + ' agora é ' + WTT.fmt.role(updated.role).toLowerCase() + '.');
        // Quem tirou o próprio admin perde o acesso a esta página.
        if (person.id === me.id && updated.role !== 'admin') location.href = '/';
      });
    },
    createInvite() {
      return this.run('invite', async () => {
        const inv = await api('POST', '/api/orgs/' + orgId + '/invites', { email: this.invite.email, role: this.invite.role });
        this.lastLink = location.origin + inv.path;
        this.invite.email = '';
        this.invites = [inv, ...this.invites];
      });
    },
    async copyLink() {
      const ok = await WTT.copyText(this.lastLink);
      toast(ok ? 'Link copiado.' : 'Não deu para copiar. Selecione o link e copie manualmente.', ok ? 'info' : 'error');
    },
    revoke(inv) {
      return this.run('invite', async () => {
        await api('DELETE', '/api/invites/' + inv.id);
        this.invites = this.invites.filter((i) => i.id !== inv.id);
        toast('Convite revogado.');
      });
    },
  }));

  Alpine.data('orgSettings', () => ({
    ...form(),
    orgName: me.organization_name,
    confirmDelete: false,
    saveOrg() {
      return this.run('org', async () => {
        const org = await api('PATCH', '/api/orgs/' + orgId, { name: this.orgName });
        setText('[data-org-name]', org.name);
        toast('Organização salva.');
      });
    },
    deleteOrg() {
      return this.run('delete', async () => {
        await api('DELETE', '/api/orgs/' + orgId);
        location.href = '/signup';
      });
    },
  }));

  Alpine.data('profileSettings', () => ({
    ...form(),
    profile: { name: me.name, email: me.email },
    password: { current: '', next: '' },
    saveProfile() {
      return this.run('profile', async () => {
        const p = await api('PATCH', '/api/persons/' + me.id, this.profile);
        this.profile = { name: p.name, email: p.email };
        setText('[data-me-name]', p.name);
        toast('Perfil salvo.');
      });
    },
    changePassword() {
      return this.run('password', async () => {
        await api('POST', '/api/auth/password', { current_password: this.password.current, new_password: this.password.next });
        this.password = { current: '', next: '' };
        toast('Senha trocada. As outras sessões foram encerradas.');
      });
    },
  }));
});
