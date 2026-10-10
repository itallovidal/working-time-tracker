// Componentes das páginas públicas: login, signup, aceite de convite e a volta do Clerk.
document.addEventListener('alpine:init', () => {
  const { api, form, t, rules } = WTT;

  // passwordRule confere uma senha nova como o servidor: pelo menos 8 caracteres e, no máximo, 72 bytes (o limite
  // do bcrypt). Não apara: o espaço faz parte da senha. Vazio também erra, com o mesmo texto de "curta demais".
  const passwordRule = (v) => {
    const s = String(v === null || v === undefined ? '' : v);
    if (Array.from(s).length < 8) return t('errors.auth.weak_password');
    if (new TextEncoder().encode(s).length > 72) return t('errors.auth.long_password');
    return '';
  };

  // ---------- Clerk no navegador ----------
  // Com as chaves do Clerk no servidor (WTT.boot.clerk), o login, o cadastro e o convite oferecem também o Google,
  // que entra pelo Clerk, logo abaixo do email e da senha do sistema (que são o jeito principal de entrar). O Clerk
  // só diz quem a pessoa é: depois de entrar lá, /auth/clerk/continue troca o token dele por uma sessão daqui (o
  // cookie de sempre), e o resto do sistema não sabe que o Clerk existe. O email e a senha do próprio Clerk não
  // são oferecidos: o que ele mostra além do Google vem da configuração do app dele (ver o README). O clerk-js vem do Frontend API do app
  // no Clerk, não do nosso servidor; a versão fica nestas constantes porque uma versão principal nova pode mudar
  // a forma de carregar.
  const CLERK_JS = '@clerk/clerk-js@6';
  const CLERK_UI = '@clerk/ui@1';
  // Marca que a página de retorno acabou de tentar entrar. Se o cookie for recusado, o login não pode mandar a
  // pessoa de volta para ela em círculo.
  const TRIED = 'wtt_clerk_continue';
  const TRIED_WINDOW_MS = 30 * 1000;

  const loadScript = (src, attrs = {}) => new Promise((resolve, reject) => {
    const el = document.createElement('script');
    el.src = src;
    el.async = true;
    el.crossOrigin = 'anonymous';
    Object.entries(attrs).forEach(([k, v]) => el.setAttribute(k, v));
    el.onload = resolve;
    el.onerror = () => reject(new Error('could not load ' + src));
    document.head.appendChild(el);
  });

  // appearance pinta o Clerk com as cores do sistema (claro e escuro) e tira o cartão, o título e o link de
  // cadastro dele: o cartão, o título e os links são os desta página.
  function appearance() {
    const css = getComputedStyle(document.documentElement);
    const v = (name) => css.getPropertyValue(name).trim();
    const flat = { boxShadow: 'none', border: 'none', background: 'transparent', padding: '2px', width: '100%' };
    return {
      variables: {
        colorPrimary: v('--accent'),
        colorTextOnPrimaryBackground: v('--accent-ink'),
        colorBackground: v('--surface'),
        colorInputBackground: v('--surface'),
        colorText: v('--ink'),
        colorInputText: v('--ink'),
        colorTextSecondary: v('--muted'),
        fontFamily: v('--font'),
        borderRadius: v('--radius'),
      },
      elements: {
        rootBox: { width: '100%' },
        cardBox: flat,
        card: flat,
        header: { display: 'none' },
        footerAction: { display: 'none' },
        footer: { background: 'transparent' },
      },
    };
  }

  let clerkLoading = null;
  // loadClerk carrega o clerk-js uma vez e devolve o Clerk pronto.
  function loadClerk() {
    if (!clerkLoading) {
      clerkLoading = (async () => {
        const cfg = WTT.boot.clerk;
        if (!cfg || !cfg.frontendApi) throw new Error('the Clerk is not configured');
        const base = 'https://' + cfg.frontendApi + '/npm/';
        await Promise.all([
          loadScript(base + CLERK_UI + '/dist/ui.browser.js'),
          loadScript(base + CLERK_JS + '/dist/clerk.browser.js', { 'data-clerk-publishable-key': cfg.publishableKey }),
        ]);
        const options = {
          ui: { ClerkUI: window.__internal_ClerkUICtor },
          signInUrl: '/login',
          signUpUrl: '/signup',
          appearance: appearance(),
        };
        // O Clerk fala inglês; em português, carrega os textos dele (o arquivo vai junto do sistema).
        if (WTT.lang === 'pt-BR') options.localization = (await import('/static/clerk-pt-BR.js')).ptBR;
        await window.Clerk.load(options);
        return window.Clerk;
      })();
      clerkLoading.catch(() => { clerkLoading = null; });
    }
    return clerkLoading;
  }

  // sessionReady espera a sessão do Clerk aparecer: na volta de um login por Google, por exemplo, ela chega um
  // instante depois do load.
  function sessionReady(clerk, ms) {
    if (clerk.session) return Promise.resolve(clerk.session);
    return new Promise((resolve) => {
      let off = () => {};
      const timer = setTimeout(() => { off(); resolve(null); }, ms);
      off = clerk.addListener(({ session }) => {
        if (session) { clearTimeout(timer); off(); resolve(session); }
      });
    });
  }

  // continueURL é para onde o Clerk leva a pessoa depois de entrar. O ?next= já veio saneado do servidor.
  function continueURL(invite) {
    const q = new URLSearchParams();
    if (WTT.boot.next && WTT.boot.next !== '/') q.set('next', WTT.boot.next);
    if (invite) q.set('invite', invite);
    const qs = q.toString();
    return '/auth/clerk/continue' + (qs ? '?' + qs : '');
  }

  const markTried = () => { try { sessionStorage.setItem(TRIED, String(Date.now())); } catch (e) { /* sem sessionStorage, sem a guarda */ } };
  const clearTried = () => { try { sessionStorage.removeItem(TRIED); } catch (e) { /* idem */ } };
  function triedRecently() {
    try { return Date.now() - Number(sessionStorage.getItem(TRIED)) < TRIED_WINDOW_MS; } catch (e) { return false; }
  }

  // mountGoogle desenha o login do Clerk (o Google) no elemento. O Clerk volta para esta mesma página depois do
  // Google (em #/sso-callback) e termina a entrada sozinho, levando a pessoa a `to`.
  function mountGoogle(clerk, el, to) {
    clerk.mountSignIn(el, { forceRedirectUrl: to, signUpForceRedirectUrl: to });
  }

  // leaveClerk desconecta do Clerk e volta ao login. Sem isso, quem saiu do sistema entraria de novo sozinho,
  // porque o Clerk continua logado.
  async function leaveClerk() {
    clearTried();
    const clerk = await loadClerk();
    await clerk.signOut({ redirectUrl: '/login' });
  }

  // ---------- Login ----------
  Alpine.data('loginForm', () => ({
    ...form(),
    email: '',
    password: '',
    clerk: !!WTT.boot.clerk,
    loading: !!WTT.boot.clerk,
    stuck: false,
    async init() {
      if (!this.clerk) return;
      try {
        const clerk = await loadClerk();
        if (clerk.session) {
          // Saiu do sistema (out=1): sai do Clerk também e mostra o login.
          if (WTT.boot.signedOut) { await leaveClerk(); return; }
          // Já está no Clerk e só falta a sessão daqui. Se acabou de tentar e voltou, algo barrou o cookie.
          if (triedRecently()) { this.stuck = true; return; }
          markTried();
          location.replace(continueURL());
          return;
        }
        mountGoogle(clerk, this.$refs.clerk, continueURL());
      } catch (e) {
        console.error(e);
        this.errors.clerk = t('auth.clerk.load_failed');
      } finally {
        this.loading = false;
      }
    },
    retry() {
      clearTried();
      location.replace(continueURL());
    },
    otherAccount() { return leaveClerk(); },
    submit() {
      // O login só exige preenchido: o formato do email e o tamanho da senha são do servidor, que responde sempre
      // o mesmo erro para não dizer o que existe.
      if (!this.check({ email: [this.email, rules.required], password: [this.password, rules.required] })) return undefined;
      return this.run('login', async () => {
        await api('POST', '/api/auth/login', { email: this.email.trim(), password: this.password });
        location.href = WTT.boot.next || '/';
      });
    },
  }));

  // ---------- Cadastro de uma organização ----------
  Alpine.data('signupForm', () => ({
    ...form(),
    organization_name: '',
    name: '',
    email: '',
    password: '',
    confirmation: '',
    // step: 1 é a organização (o nome dela e o da pessoa), 2 é o acesso (email e senha). Os valores ficam aqui, não
    // nos campos, então voltar à etapa 1 e seguir de novo não perde nada.
    step: 1,
    // O país da organização: o do idioma da página, até a pessoa trocar. Define a moeda, o fuso e os dados fiscais.
    country: WTT.countries.forLang(WTT.lang),
    countries: WTT.countries.list(),
    // confirmTouched: a pessoa já saiu do campo de confirmação. Até lá, "não confere" só aparece quando o que ela
    // digitou já tem o tamanho da senha, para não reclamar a cada letra.
    confirmTouched: false,
    reveal: false,
    clerk: !!WTT.boot.clerk,
    loading: !!WTT.boot.clerk,
    get longEnough() { return [...this.password].length >= 8; },
    get matches() { return this.confirmation !== '' && this.confirmation === this.password; },
    get showMismatch() {
      return this.confirmation !== '' && this.confirmation !== this.password
        && (this.confirmTouched || this.confirmation.length >= this.password.length);
    },
    async next() {
      // Apara os dois campos e confere como o servidor, para um nome só de espaços ou comprido demais não passar
      // para a etapa 2 e só falhar lá, onde o campo não está à vista.
      this.organization_name = this.organization_name.trim();
      this.name = this.name.trim();
      const ok = this.check({
        organization_name: [this.organization_name, rules.required, rules.max('name')],
        country: [this.country, rules.required],
        name: [this.name, rules.required, rules.max('name')],
      });
      if (!ok) return;
      // O primeiro campo de cada etapa se foca sozinho ao entrar (x-init no template): o autofocus não vale para
      // campo que entra depois do carregamento.
      this.step = 2;
    },
    back() {
      this.errors.signup = '';
      this.clearFields();
      this.step = 1;
    },
    async init() {
      if (!this.clerk) return;
      try {
        const clerk = await loadClerk();
        const to = continueURL();
        // Já está no Clerk: a página de retorno cuida do resto (entrar, ou pedir o nome da organização).
        if (clerk.session) { location.replace(to); return; }
        mountGoogle(clerk, this.$refs.clerk, to);
      } catch (e) {
        console.error(e);
        this.errors.clerk = t('auth.clerk.load_failed');
      } finally {
        this.loading = false;
      }
    },
    submit() {
      // Enter ou o botão: na etapa 1 avança, na 2 cria a conta.
      if (this.step === 1) return this.next();
      this.email = this.email.trim();
      const ok = this.check({
        email: [this.email, rules.required, rules.email, rules.max('email')],
        password: [this.password, passwordRule],
        // A confirmação é só da tela (o servidor recebe uma senha): vazia ou diferente erra embaixo do campo, como os
        // outros, e o check leva o foco ao primeiro erro.
        confirm_password: [this.confirmation, rules.confirmation(this.password)],
      });
      if (!ok) return undefined;
      return this.run('signup', async () => {
        await api('POST', '/api/auth/signup', {
          organization_name: this.organization_name,
          country: this.country,
          name: this.name,
          email: this.email,
          password: this.password,
        });
        location.href = '/';
      });
    },
  }));

  // ---------- Aceite de convite ----------
  Alpine.data('inviteForm', () => ({
    ...form(),
    loading: true,
    loadError: '',
    info: null,
    name: '',
    email: '',
    password: '',
    clerk: !!WTT.boot.clerk,
    clerkLoading: !!WTT.boot.clerk,
    async init() {
      // O link do email do Clerk volta com um __clerk_ticket. O convite é aceito aqui, com email e senha do sistema
      // ou com o Google; o ticket abriria o cadastro com email do próprio Clerk, que não é o que se quer. Sai da
      // URL antes de o Clerk carregar.
      const query = new URLSearchParams(location.search);
      if (query.has('__clerk_ticket') || query.has('__clerk_status')) {
        query.delete('__clerk_ticket');
        query.delete('__clerk_status');
        const rest = query.toString();
        history.replaceState(null, '', location.pathname + (rest ? '?' + rest : '') + location.hash);
      }
      try {
        this.info = await api('GET', '/api/auth/invites/' + encodeURIComponent(WTT.boot.token));
        this.email = this.info.email || '';
      } catch (e) {
        this.loadError = e.message;
      } finally {
        this.loading = false;
      }
      if (this.info && this.clerk) {
        await this.$nextTick();
        await this.drawClerk();
      }
    },
    async drawClerk() {
      try {
        const clerk = await loadClerk();
        const to = continueURL(WTT.boot.token);
        if (clerk.session) { location.replace(to); return; }
        mountGoogle(clerk, this.$refs.clerk, to);
      } catch (e) {
        console.error(e);
        this.errors.clerk = t('auth.clerk.load_failed');
      } finally {
        this.clerkLoading = false;
      }
    },
    submit() {
      this.name = this.name.trim();
      this.email = this.email.trim();
      const ok = this.check({
        name: [this.name, rules.required, rules.max('name')],
        email: [this.email, rules.required, rules.email, rules.max('email')],
        password: [this.password, passwordRule],
      });
      if (!ok) return undefined;
      return this.run('accept', async () => {
        await api('POST', '/api/auth/invites/' + encodeURIComponent(WTT.boot.token) + '/accept', {
          name: this.name,
          email: this.email,
          password: this.password,
        });
        location.href = '/';
      });
    },
  }));

  // ---------- Volta do Clerk ----------
  // A pessoa entrou no Clerk; aqui o token dele vira uma sessão do sistema. Estados: working (falando com o
  // servidor), password (a conta antiga com senha pede a senha uma vez), choose (não há conta: entrar num
  // convite ou criar a organização) e error. Esta página nunca manda sozinha de volta ao login: em erro ela
  // mostra o motivo e deixa a pessoa tentar de novo ou trocar de conta.
  Alpine.data('clerkContinue', () => ({
    ...form(),
    state: 'working',
    message: '',
    email: '',
    name: '',
    invites: [],
    password: '',
    organization_name: '',
    country: WTT.countries.forLang(WTT.lang),
    countries: WTT.countries.list(),
    async init() {
      markTried();
      try {
        await this.finish(await this.call('/api/auth/clerk/login', { invite_token: WTT.boot.invite || '' }));
      } catch (e) {
        this.fail(e);
      }
    },
    // call fala com o servidor levando o token de sessão do Clerk, que vale 60 segundos: pede um novo a cada
    // chamada e, se o servidor o achar inválido, tenta uma vez mais com outro.
    async call(path, body) {
      for (let attempt = 0; ; attempt++) {
        const clerk = await loadClerk();
        const session = await sessionReady(clerk, 5000);
        if (!session) throw new Error(t('auth.clerk.no_session'));
        const token = await session.getToken({ skipCache: true });
        try {
          return await api('POST', path, body, { Authorization: 'Bearer ' + token });
        } catch (e) {
          if (e.code === 'auth.clerk_token_invalid' && attempt === 0) continue;
          throw e;
        }
      }
    },
    fail(e) {
      console.error(e);
      this.message = e && e.message ? e.message : String(e);
      this.state = 'error';
    },
    async finish(res) {
      if (res.status === 'ok') return this.done();
      this.email = res.email || '';
      if (res.status === 'needs_password') {
        this.state = 'password';
      } else {
        this.name = res.name || '';
        this.invites = res.invites || [];
        this.state = 'choose';
      }
    },
    // done confere que o cookie pegou antes de sair: um navegador que o recusa (Secure em http, por exemplo)
    // mandaria a pessoa ao login, e o login de volta para cá.
    async done() {
      try {
        await api('GET', '/api/auth/me');
      } catch (e) {
        this.message = t('auth.clerk.cookie_blocked');
        this.state = 'error';
        return;
      }
      clearTried();
      location.replace(WTT.boot.next || '/');
    },
    submitPassword() {
      if (!this.check({ password: [this.password, rules.required] })) return undefined;
      return this.run('password', async () => {
        await this.finish(await this.call('/api/auth/clerk/login', { invite_token: WTT.boot.invite || '', password: this.password }));
      });
    },
    join(inv) {
      // O nome é opcional: sem ele o servidor usa o do Clerk.
      this.name = this.name.trim();
      if (!this.check({ name: [this.name, rules.max('name')] })) return undefined;
      return this.run('choose', async () => {
        await this.finish(await this.call('/api/auth/clerk/join', { invite_id: inv.id, name: this.name }));
      });
    },
    createOrg() {
      this.organization_name = this.organization_name.trim();
      this.name = this.name.trim();
      const ok = this.check({
        organization_name: [this.organization_name, rules.required, rules.max('name')],
        country: [this.country, rules.required],
        name: [this.name, rules.max('name')],
      });
      if (!ok) return undefined;
      return this.run('choose', async () => {
        await this.finish(await this.call('/api/auth/clerk/signup', { organization_name: this.organization_name, country: this.country, name: this.name }));
      });
    },
    retry() {
      this.state = 'working';
      this.init();
    },
    otherAccount() { return leaveClerk(); },
  }));
});
