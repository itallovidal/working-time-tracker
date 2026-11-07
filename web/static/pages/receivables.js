// A tela A receber do modo solo, só do dono: as propostas (A confirmar), o que foi confirmado e não entrou (Em aberto)
// e o que entrou (Recebidos). Confirmar grava o valor e o vencimento de um período; Receber registra a data e o valor
// que entrou (no mensal, com a multa e os juros que o atraso sugere); cada passo se desfaz. As contas do período e do
// atraso são do servidor: a tela só confere o que ele também confere, com as mesmas regras e o erro embaixo do campo.
document.addEventListener('alpine:init', () => {
  const { api, form, t, fmt, toCents, errorText, rules, limits } = WTT;
  const orgId = WTT.boot.me.organization_id;
  const base = '/api/orgs/' + orgId + '/receivables';

  // positive confere um valor em dinheiro digitado: só dígitos e separador, maior que zero e até o teto do servidor.
  // Vazio também é recusado: o campo vem preenchido, e apagá-lo é um erro, não um valor zero.
  const positive = (code) => (v, field) => {
    const cents = toCents(v, { strict: true });
    return cents !== null && cents > 0 && cents <= limits.max_cents ? '' : errorText({ code, params: { field } });
  };
  // charge confere a multa e os juros: em branco é zero; preenchidos, zero ou mais até o teto.
  const charge = (v, field) => {
    const x = String(v).trim();
    if (x === '') return '';
    const cents = toCents(x, { strict: true });
    return cents !== null && cents <= limits.max_cents ? '' : errorText({ code: 'receivable.invalid_charge', params: { field } });
  };
  // cents lê um campo de dinheiro já conferido; em branco vale zero.
  const cents = (v) => (String(v).trim() === '' ? 0 : toCents(v, { strict: true }));

  Alpine.data('orgReceivables', () => ({
    ...form(),
    loading: true,
    data: null,
    confirming: null, // o recebível de Em aberto que pede confirmação para desfazer a confirmação
    receivedConfirming: null, // o de Recebidos que pede confirmação para desfazer o recebimento
    target: null, // a proposta (Confirmar) ou o recebível (Editar) que o modal mostra
    editing: false,
    draft: { amount: '', due: '', invoice: '', note: '' },
    receiving: null, // o recebível de Em aberto que o modal Receber mostra
    rec: { on: '', amount: '', fee: '', interest: '' },
    suggestion: null, // a sugestão de multa e juros para a data escolhida (só no mensal)
    suggestionSeq: 0, // descarta a resposta de uma data que já foi trocada

    init() {
      return this.reload();
    },
    // reload lê de novo as três listas; os números valem para a hora em que chegam.
    async reload() {
      this.loading = true;
      this.errors.load = '';
      try {
        this.data = await api('GET', base);
      } catch (e) {
        this.errors.load = e.message;
      } finally {
        this.loading = false;
      }
    },

    get monthly() {
      return !!this.receiving && this.receiving.kind === 'monthly';
    },
    kindLabel(kind) {
      return kind === 'monthly' ? t('receivable.kind.monthly') : t('receivable.kind.hourly');
    },
    // "05 out. – 04 nov. 2026": os dois dias são inclusivos, em dias de calendário.
    period(item) {
      return fmt.day(item.period_start, { day: '2-digit', month: 'short' }) + ' – ' + fmt.day(item.period_end, { day: '2-digit', month: 'short', year: 'numeric' });
    },
    // rateLine é o valor por hora que o valor rende: "R$ 88,00 por hora".
    rateLine(item) {
      return item.effective_rate_cents === null || item.effective_rate_cents === undefined
        ? '' : t('receivable.rate_line', { rate: fmt.money(item.effective_rate_cents) });
    },
    belowMin() {
      return t('receivable.below_min', { min: fmt.money(limits.min_rate_cents) });
    },

    // openConfirm abre o modal Confirmar com o valor e o vencimento propostos, que o dono pode mudar.
    openConfirm(proposal) {
      this.target = proposal;
      this.editing = false;
      this.draft = {
        amount: proposal.calculated_cents > 0 ? fmt.moneyInput(proposal.calculated_cents) : '',
        due: proposal.due_date, invoice: '', note: '',
      };
      this.errors.save = '';
      this.clearFields();
      Alpine.store('modal').open('receivable-confirm', t('receivable.modal.confirm_title', { name: proposal.customer.name }), () => !this.pending);
    },
    // openEdit abre o mesmo modal com o que já foi confirmado. Vale enquanto o recebível não foi recebido.
    openEdit(row) {
      this.target = row;
      this.editing = true;
      this.draft = { amount: fmt.moneyInput(row.amount_cents), due: row.due_date, invoice: row.invoice_number || '', note: row.note || '' };
      this.errors.save = '';
      this.clearFields();
      Alpine.store('modal').open('receivable-confirm', t('receivable.modal.edit_title', { name: row.customer.name }), () => !this.pending);
    },
    // draftRate é o valor por hora que o valor digitado rende pelas horas do período, para o dono ver antes de confirmar.
    // Abaixo do piso é um aviso, não um erro.
    draftRate() {
      const target = this.target;
      const value = target ? toCents(this.draft.amount, { strict: true }) : null;
      if (!target || !(target.seconds > 0) || value === null || value <= 0) return '';
      const rate = Math.round(value * 3600 / target.seconds);
      if (rate < limits.min_rate_cents) {
        return t('receivable.modal.rate_below', { rate: fmt.money(rate), min: fmt.money(limits.min_rate_cents) });
      }
      return t('receivable.modal.rate', { rate: fmt.money(rate) });
    },
    draftBelowMin() {
      const target = this.target;
      const value = target ? toCents(this.draft.amount, { strict: true }) : null;
      return !!target && target.seconds > 0 && value !== null && value > 0 && Math.round(value * 3600 / target.seconds) < limits.min_rate_cents;
    },
    // saveConfirm confere os campos como o servidor e grava: a confirmação (POST) ou a edição (PATCH).
    saveConfirm() {
      const row = this.target;
      const spec = {
        amount_cents: [this.draft.amount, positive('receivable.invalid_amount')],
        due_date: [this.draft.due, rules.required, rules.date],
        invoice_number: [this.draft.invoice, rules.max('invoice_number')],
        note: [this.draft.note, rules.max('note')],
      };
      if (!row || !this.check(spec)) return undefined;
      const body = {
        amount_cents: toCents(this.draft.amount, { strict: true }), due_date: this.draft.due,
        invoice_number: this.draft.invoice.trim(), note: this.draft.note.trim(),
      };
      return this.run('save', async () => {
        if (this.editing) {
          await api('PATCH', base + '/' + row.id, body);
        } else {
          await api('POST', base, { customer_id: row.customer.id, period_start: row.period_start, period_end: row.period_end, ...body });
        }
        Alpine.store('modal').close();
        Alpine.store('toast').show(t(this.editing ? 'receivable.toast.updated' : 'receivable.toast.confirmed', { name: row.customer.name }));
        await this.reload();
      });
    },
    // unconfirm desfaz a confirmação: o período volta para A confirmar. Com o recebimento registrado o servidor recusa.
    unconfirm(row) {
      return this.run('undo', async () => {
        await api('DELETE', base + '/' + row.id);
        this.confirming = null;
        Alpine.store('toast').show(t('receivable.toast.unconfirmed'));
        await this.reload();
      });
    },

    // openReceive abre o modal Receber com hoje e o valor confirmado. No mensal busca a sugestão de multa e juros.
    openReceive(row) {
      this.receiving = row;
      this.rec = { on: this.data.today, amount: fmt.moneyInput(row.amount_cents), fee: '', interest: '' };
      this.suggestion = null;
      this.errors.receive = '';
      this.clearFields();
      Alpine.store('modal').open('receivable-receive', t('receivable.modal.receive_title', { name: row.customer.name }), () => !this.pending);
      return this.loadSuggestion();
    },
    // loadSuggestion pede ao servidor a multa e os juros do atraso na data escolhida. É só uma sugestão: nada é
    // preenchido até o dono usá-la.
    async loadSuggestion() {
      const row = this.receiving;
      if (!row || row.kind !== 'monthly' || !/^\d{4}-\d{2}-\d{2}$/.test(this.rec.on)) {
        this.suggestion = null;
        return;
      }
      const seq = ++this.suggestionSeq;
      try {
        const found = await api('GET', base + '/' + row.id + '/late-charges?on=' + encodeURIComponent(this.rec.on));
        if (seq === this.suggestionSeq) this.suggestion = found;
      } catch (e) {
        if (seq === this.suggestionSeq) this.suggestion = null;
      }
    },
    suggestionText() {
      const s = this.suggestion;
      return s ? t('receivable.modal.suggestion', {
        count: s.days_late, fee: fmt.money(s.late_fee_cents), interest: fmt.money(s.interest_cents), total: fmt.money(s.total_cents),
      }) : '';
    },
    // applySuggestion preenche a multa, os juros e o valor recebido (o confirmado mais os dois).
    applySuggestion() {
      const s = this.suggestion;
      if (!s) return;
      this.rec.fee = fmt.moneyInput(s.late_fee_cents);
      this.rec.interest = fmt.moneyInput(s.interest_cents);
      this.rec.amount = fmt.moneyInput(s.total_cents);
      this.clearFields();
    },
    // saveReceive confere os campos como o servidor e registra o recebimento. No por hora só a data vai: o valor é o
    // confirmado, e multa e juros não existem.
    saveReceive() {
      const row = this.receiving;
      if (!row) return undefined;
      const today = this.data.today;
      const notFuture = (v, field) => (v > today ? errorText({ code: 'receivable.received_in_future', params: { field } }) : '');
      const spec = { received_on: [this.rec.on, rules.required, rules.date, notFuture] };
      if (this.monthly) {
        // A multa e os juros são partes do valor recebido: não passam dele.
        const within = (v, field) => (cents(this.rec.fee) + cents(this.rec.interest) > cents(this.rec.amount)
          ? errorText({ code: 'receivable.charges_exceed_received', params: { field } }) : '');
        spec.received_cents = [this.rec.amount, positive('receivable.invalid_amount')];
        spec.late_fee_cents = [this.rec.fee, charge, within];
        spec.interest_cents = [this.rec.interest, charge];
      }
      if (!this.check(spec)) return undefined;
      const body = { received_on: this.rec.on };
      if (this.monthly) {
        body.received_cents = cents(this.rec.amount);
        body.late_fee_cents = cents(this.rec.fee);
        body.interest_cents = cents(this.rec.interest);
      }
      return this.run('receive', async () => {
        await api('POST', base + '/' + row.id + '/receive', body);
        Alpine.store('modal').close();
        Alpine.store('toast').show(t('receivable.toast.received', { name: row.customer.name }));
        await this.reload();
      });
    },
    // unreceive desfaz o recebimento: o recebível volta para Em aberto, sem a multa e os juros informados.
    unreceive(row) {
      return this.run('unreceive', async () => {
        await api('DELETE', base + '/' + row.id + '/receive');
        this.receivedConfirming = null;
        Alpine.store('toast').show(t('receivable.toast.unreceived'));
        await this.reload();
      });
    },
  }));
});
