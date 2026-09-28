// Componentes das páginas públicas: login, signup e aceite de convite.
document.addEventListener('alpine:init', () => {
  const { api, form } = WTT;

  Alpine.data('loginForm', () => ({
    ...form(),
    email: '',
    password: '',
    submit() {
      return this.run('login', async () => {
        await api('POST', '/api/auth/login', { email: this.email, password: this.password });
        location.href = WTT.boot.next || '/';
      });
    },
  }));

  Alpine.data('signupForm', () => ({
    ...form(),
    organization_name: '',
    name: '',
    email: '',
    password: '',
    submit() {
      return this.run('signup', async () => {
        await api('POST', '/api/auth/signup', {
          organization_name: this.organization_name,
          name: this.name,
          email: this.email,
          password: this.password,
        });
        location.href = '/';
      });
    },
  }));

  Alpine.data('inviteForm', () => ({
    ...form(),
    loading: true,
    loadError: '',
    info: null,
    name: '',
    email: '',
    password: '',
    async init() {
      try {
        this.info = await api('GET', '/api/auth/invites/' + encodeURIComponent(WTT.boot.token));
        this.email = this.info.email || '';
      } catch (e) {
        this.loadError = e.message;
      } finally {
        this.loading = false;
      }
    },
    submit() {
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
});
