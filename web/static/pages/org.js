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
    name: '', description: '', sprint_duration_days: 14, weekly_hours: '', daily_time: '', weekly_sync_day: '',
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

  Alpine.data('orgProjects', () => ({
    ...form(),
    loading: true,
    projects: [],
    customers: null, // só carregados quando o modal de novo projeto abre pela primeira vez
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
    // openCreate abre o modal de novo projeto. Só admins criam projeto, e só eles podem
    // listar os clientes; por isso a lista vem aqui, e não no init.
    async openCreate() {
      this.draft = blankProject();
      this.errors.create = '';
      Alpine.store('modal').open('project', 'Novo projeto', () => !this.pending);
      if (this.customers !== null) return;
      try {
        this.customers = (await api('GET', '/api/orgs/' + orgId + '/customers')) || [];
      } catch (e) {
        this.errors.create = 'Não deu para carregar os clientes: ' + e.message;
      }
    },
    // create cria o projeto e, se houver cliente ou valor, grava a cobrança logo em seguida.
    create() {
      return this.run('create', async () => {
        // O valor só vale com cliente (o campo some no projeto interno) e é conferido antes:
        // depois de criado, um erro na cobrança deixaria o projeto sem ela.
        const cents = this.draft.customer_id ? WTT.toCents(this.draft.rate) : null;
        if (this.draft.customer_id && cents === null && String(this.draft.rate).trim() !== '') throw new Error('Informe o valor cobrado por hora, por exemplo 150,00.');
        if (cents !== null && cents > 100000000) throw new Error('O valor cobrado por hora deve ficar entre 0 e 1.000.000,00.');
        const p = await api('POST', '/api/orgs/' + orgId + '/projects', {
          name: this.draft.name,
          description: this.draft.description,
          sprint_duration_days: Number(this.draft.sprint_duration_days) || 0,
          weekly_hours: Number(this.draft.weekly_hours) || 0,
          daily_time: this.draft.daily_time || null,
          weekly_sync_day: this.draft.weekly_sync_day || null,
        });
        if (this.draft.customer_id) {
          try {
            await api('PUT', '/api/projects/' + p.id + '/billing', { customer_id: this.draft.customer_id || null, bill_rate_cents: cents });
          } catch (e) {
            // O projeto já existe: as configurações dele são o lugar de definir a cobrança de novo.
            Alpine.store('toast').flash('Projeto criado, mas o cliente e o valor não foram salvos: ' + e.message, 'error');
            location.href = '/projects/' + p.id + '/settings';
            return;
          }
        }
        Alpine.store('toast').flash('Projeto criado.');
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
    // openInvite abre o modal de adicionar colaborador, sempre com o formulário zerado.
    openInvite() {
      this.invite = { email: '', role: 'member' };
      this.lastLink = '';
      this.errors.invite = '';
      Alpine.store('modal').open('invite', 'Adicionar colaborador', () => !this.pending);
    },
    // O modal fica aberto depois de gerar o convite: o link só aparece nesta hora.
    createInvite() {
      return this.run('invite', async () => {
        const inv = await api('POST', '/api/orgs/' + orgId + '/invites', { email: this.invite.email, role: this.invite.role });
        this.lastLink = location.origin + inv.path;
        this.invites = [inv, ...this.invites];
        // O bloco do link entra com x-transition, e o Alpine segura o $nextTick até ele
        // aparecer. Sem a transição o campo ainda estaria escondido e não aceitaria o foco.
        this.$nextTick(() => this.$refs.link && this.$refs.link.focus());
      });
    },
    async copyLink() {
      const ok = await WTT.copyText(this.lastLink);
      toast(ok ? 'Link copiado.' : 'Não deu para copiar. Selecione o link e copie manualmente.', ok ? 'info' : 'error');
    },
    revoke(inv) {
      return this.run('revoke', async () => {
        await api('DELETE', '/api/invites/' + inv.id);
        this.invites = this.invites.filter((i) => i.id !== inv.id);
        toast('Convite revogado.');
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
        Alpine.store('toast').flash('Organização salva.');
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
        ['Segmento', org.industry],
        ['Porte', WTT.fmt.orgSize(org.size)],
        ['Fundação', org.founded_year ? String(org.founded_year) : ''],
        ['Localização', [org.city, org.state, org.country].filter(Boolean).join(', ')],
      ]);
    },
    contact() {
      return rows([
        ['Site', bareURL(org.website), { href: org.website, external: true }],
        ['Email', org.contact_email, { href: 'mailto:' + org.contact_email }],
        ['Telefone', org.phone, { href: 'tel:' + (org.phone || '').replace(/[^0-9+]/g, '') }],
        ['LinkedIn', bareURL(org.linkedin_url), { href: org.linkedin_url, external: true }],
        ['Instagram', bareURL(org.instagram_url), { href: org.instagram_url, external: true }],
      ]);
    },
    legal() {
      return rows([
        ['Razão social', org.legal_name],
        ['CNPJ', WTT.fmt.cnpj(org.cnpj), { mono: true }],
        ['Endereço', [org.address_line1, org.address_line2, org.postal_code].filter(Boolean).join(' · ')],
      ]);
    },
    defaults() {
      return rows([
        ['Regime', WTT.fmt.workMode(org.work_mode)],
        ['Fuso horário', org.timezone],
        ['Moeda', WTT.fmt.currency(org.currency)],
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
      Alpine.store('modal').open('customer', c ? 'Editar cliente' : 'Novo cliente', () => !this.pending);
    },
    save() {
      return this.run('save', async () => {
        if (this.editing) {
          const saved = await api('PATCH', '/api/customers/' + this.editing, this.draft);
          this.customers = this.customers.map((c) => (c.id === saved.id ? saved : c));
          toast('Cliente salvo.');
        } else {
          this.customers = [...this.customers, await api('POST', '/api/orgs/' + orgId + '/customers', this.draft)];
          toast('Cliente criado.');
        }
        this.customers.sort((a, b) => a.name.localeCompare(b.name));
        Alpine.store('modal').close();
      });
    },
    remove(c) {
      return this.run('remove', async () => {
        this.confirming = null;
        await api('DELETE', '/api/customers/' + c.id);
        this.customers = this.customers.filter((x) => x.id !== c.id);
        toast('Cliente excluído.');
      });
    },
  }));

  Alpine.data('profileSettings', () => ({
    ...form(),
    profile: { name: me.name, email: me.email },
    password: { current: '', next: '' },
    rates: [],
    ratesLoaded: false,
    async init() {
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
