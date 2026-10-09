// A tela de pagamentos, só de admins: a visão da equipe (quanto pagar, a quem e quando, e as horas de cada pessoa).
// O que cada pessoa recebe, com o histórico e o detalhe por projeto, fica no perfil dela.
document.addEventListener('alpine:init', () => {
  const { api, form } = WTT;
  const orgId = WTT.boot.me.organization_id;

  Alpine.data('orgPayments', () => ({
    ...form(),
    loading: true,
    team: null,
    init() {
      return this.reload();
    },
    // reload lê de novo a equipe; os números valem para a hora em que chegam.
    async reload() {
      this.loading = true;
      this.errors.load = '';
      try {
        this.team = await api('GET', '/api/orgs/' + orgId + '/payments');
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
    },
    // personHref é o perfil da pessoa: a linha da tabela leva até ele.
    personHref(person) {
      return '/orgs/' + orgId + '/people/' + person.id;
    },
  }));
});
