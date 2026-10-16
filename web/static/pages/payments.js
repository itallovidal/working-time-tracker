// A tela de pagamentos, só do dono: os próximos pagamentos (os períodos sem baixa, atrasados incluídos), a baixa de
// cada um e os pagos recentemente, com o desfazer. O que cada pessoa recebe, com o histórico e o detalhe por projeto,
// fica no perfil dela.
document.addEventListener('alpine:init', () => {
  const { api, form, t, fmt, toCents, errorText } = WTT;
  const orgId = WTT.boot.me.organization_id;

  Alpine.data('orgPayments', () => ({
    ...form(),
    loading: true,
    team: null,
    paying: null, // a linha de Próximos pagamentos que o modal Dar baixa mostra
    amount: '', // o texto do campo Valor pago
    confirming: null, // a baixa de Pagos recentemente que pede confirmação para desfazer
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
    // openPayout abre o modal Dar baixa com o valor calculado no campo, que o dono pode mudar.
    openPayout(row) {
      this.paying = row;
      this.amount = fmt.moneyInput(row.amount_cents);
      this.errors.pay = '';
      this.clearFields();
      Alpine.store('modal').open('payout', t('payments.modal.title', { name: row.person.name }), () => !this.pending);
    },
    // markPaid grava a baixa do período com o valor do campo e relê a tela: a linha passa para Pagos recentemente.
    markPaid() {
      const row = this.paying;
      const amount = (v, field) => (toCents(v, { strict: true }) === null ? errorText({ code: 'payment.invalid_amount', params: { field } }) : '');
      if (!row || !this.check({ amount_cents: [this.amount, amount] })) return undefined;
      return this.run('pay', async () => {
        await api('POST', '/api/orgs/' + orgId + '/payouts', {
          person_id: row.person.id, start: row.start, pay_date: row.pay_date, amount_cents: toCents(this.amount, { strict: true }),
        });
        Alpine.store('modal').close();
        Alpine.store('toast').show(t('payments.marked', { name: row.person.name }));
        await this.reload();
      });
    },
    // undo desfaz a baixa: o período volta para Próximos pagamentos.
    undo(row) {
      return this.run('undo', async () => {
        await api('DELETE', '/api/orgs/' + orgId + '/payouts/' + row.id);
        this.confirming = null;
        Alpine.store('toast').show(t('payments.unmarked'));
        await this.reload();
      });
    },
  }));
});
