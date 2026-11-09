// A tela A receber do modo solo, só do dono, em três listas (Decisão 24): Em andamento (o período atual de cada
// cliente, ainda aberto), A receber (o que fechou e não entrou, calculado ou ajustado) e Recebidos. Receber registra a
// data, o valor e o número da nota (no mensal, com a multa e os juros que o atraso sugere); Editar ajusta o valor e o
// vencimento antes; cada passo se desfaz. Uma conta sem id é a calculada, que nada grava até ser recebida ou ajustada.
// As contas do período e do atraso são do servidor: a tela só confere o que ele também confere, com as mesmas regras e
// o erro embaixo do campo.
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
  // dayNumber é o dia YYYY-MM-DD contado em dias, para a diferença entre dois dias de calendário sem fuso.
  const dayNumber = (day) => Date.UTC(+day.slice(0, 4), +day.slice(5, 7) - 1, +day.slice(8, 10)) / 86400000;
  // periodQuery identifica o período calculado nas rotas sem id.
  const periodQuery = (row) => 'customer_id=' + encodeURIComponent(row.customer.id)
    + '&period_start=' + encodeURIComponent(row.period_start) + '&period_end=' + encodeURIComponent(row.period_end);

  Alpine.data('orgReceivables', () => ({
    ...form(),
    loading: true,
    data: null,
    receivedConfirming: null, // o de Recebidos que pede confirmação para desfazer o recebimento
    target: null, // a conta de A receber que o modal Editar mostra
    resetting: false, // a confirmação de voltar ao valor calculado, no modal Editar
    draft: { amount: '', due: '', invoice: '', note: '' },
    receiving: null, // a conta (ou o mês em andamento) que o modal Receber mostra
    // monthly diz que a conta do modal Receber é de contrato mensal (com multa e juros). É um campo, e não um getter de
    // receiving: o x-show do getter nem sempre se refazia ao trocar a conta, e o modal do mensal mostrava a dica do por
    // hora.
    monthly: false,
    advance: false, // o modal Receber é de um mês ainda em andamento
    rec: { on: '', amount: '', fee: '', interest: '', invoice: '' },
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

    kindLabel(kind) {
      return kind === 'monthly' ? t('receivable.kind.monthly') : t('receivable.kind.hourly');
    },
    // period é o período em datas curtas, "06/11 – 05/12", e com o ano quando não é o de hoje: as listas trazem do mês
    // corrente para trás, e o ano só pesa no histórico.
    period(item) {
      const year = this.data ? this.data.today.slice(0, 4) : '';
      const opts = (day) => (day.slice(0, 4) === year
        ? { day: '2-digit', month: '2-digit' }
        : { day: '2-digit', month: '2-digit', year: 'numeric' });
      return fmt.day(item.period_start, opts(item.period_start)) + ' – ' + fmt.day(item.period_end, opts(item.period_end));
    },
    // amountOf é o valor da conta (o calculado, na conta calculada).
    amountOf(item) {
      return item.amount_cents;
    },
    // progress é onde o período em andamento está: "dia 4 de 30", "fecha em 26 dias" e a parte já corrida.
    progress(row) {
      const today = dayNumber(this.data.today);
      const start = dayNumber(row.period_start);
      const end = dayNumber(row.period_end);
      const days = end - start + 1;
      const day = Math.min(Math.max(today - start + 1, 1), days);
      const left = end - today;
      let closes = t('receivable.in_progress.closes_in', { count: left });
      if (left <= 0) closes = t('receivable.in_progress.closes_today');
      else if (left === 1) closes = t('receivable.in_progress.closes_tomorrow');
      return { day: t('receivable.in_progress.day_of', { day, days }), closes, pct: Math.round(day * 100 / days) };
    },
    // dueBadge é o selo do vencimento: atrasado em vermelho, hoje e amanhã em amarelo, e os outros neutros.
    dueBadge(row) {
      if (row.late) return { cls: 'badge-danger', text: t('receivable.due.late', { count: row.days_late }) };
      const left = dayNumber(row.due_date) - dayNumber(this.data.today);
      if (left <= 0) return { cls: 'badge-warn', text: t('receivable.due.today') };
      if (left === 1) return { cls: 'badge-warn', text: t('receivable.due.tomorrow') };
      return { cls: '', text: t('receivable.due.in', { count: left }) };
    },
    // rateLine é o valor por hora que o valor rende: "R$ 88,00 por hora".
    rateLine(item) {
      return item.effective_rate_cents === null || item.effective_rate_cents === undefined
        ? '' : t('receivable.rate_line', { rate: fmt.money(item.effective_rate_cents) });
    },
    belowMin() {
      return t('receivable.below_min', { min: fmt.money(limits.min_rate_cents) });
    },

    // openEdit abre o modal Editar com o que a conta tem hoje: o calculado, ou o que já foi ajustado.
    openEdit(row) {
      this.target = row;
      this.resetting = false;
      this.draft = {
        amount: row.amount_cents > 0 ? fmt.moneyInput(row.amount_cents) : '',
        due: row.due_date, invoice: row.invoice_number || '', note: row.note || '',
      };
      this.errors.save = '';
      this.clearFields();
      Alpine.store('modal').open('receivable-edit', t('receivable.modal.edit_title', { name: row.customer.name }), () => !this.pending);
    },
    // draftRate é o valor por hora que o valor digitado rende pelas horas do período, para o dono ver antes de salvar.
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
    // saveEdit confere os campos como o servidor e grava: a conta calculada passa a ser gravada (POST), e a ajustada é
    // alterada (PATCH). Ela continua em A receber.
    saveEdit() {
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
        if (row.id) {
          await api('PATCH', base + '/' + row.id, body);
        } else {
          await api('POST', base, { customer_id: row.customer.id, period_start: row.period_start, period_end: row.period_end, ...body });
        }
        Alpine.store('modal').close();
        Alpine.store('toast').show(t('receivable.toast.adjusted', { name: row.customer.name }));
        await this.reload();
      });
    },
    // reset volta a conta ajustada ao valor calculado: o recebível gravado some e o período volta a ser calculado.
    reset() {
      const row = this.target;
      if (!row || !row.id) return undefined;
      return this.run('save', async () => {
        await api('DELETE', base + '/' + row.id);
        this.resetting = false;
        Alpine.store('modal').close();
        Alpine.store('toast').show(t('receivable.toast.reset'));
        await this.reload();
      });
    },

    // openReceive abre o modal Receber com hoje, o valor da conta e o número da nota. advance diz que é o mês em
    // andamento, recebido adiantado. No mensal busca a sugestão de multa e juros.
    openReceive(row, advance) {
      this.receiving = row;
      this.monthly = row.kind === 'monthly';
      this.advance = !!advance;
      this.rec = { on: this.data.today, amount: fmt.moneyInput(this.amountOf(row)), fee: '', interest: '', invoice: row.invoice_number || '' };
      this.suggestion = null;
      this.errors.receive = '';
      this.clearFields();
      const title = advance ? 'receivable.modal.advance_title' : 'receivable.modal.receive_title';
      Alpine.store('modal').open('receivable-receive', t(title, { name: row.customer.name }), () => !this.pending);
      return this.loadSuggestion();
    },
    // loadSuggestion pede ao servidor a multa e os juros do atraso na data escolhida: pela conta gravada, ou pelo
    // período calculado. É só uma sugestão: nada é preenchido até o dono usá-la.
    async loadSuggestion() {
      const row = this.receiving;
      if (!row || row.kind !== 'monthly' || !/^\d{4}-\d{2}-\d{2}$/.test(this.rec.on)) {
        this.suggestion = null;
        return;
      }
      const on = 'on=' + encodeURIComponent(this.rec.on);
      const url = row.id ? base + '/' + row.id + '/late-charges?' + on : base + '/late-charges?' + periodQuery(row) + '&' + on;
      const seq = ++this.suggestionSeq;
      try {
        const found = await api('GET', url);
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
    // applySuggestion preenche a multa, os juros e o valor recebido (o da conta mais os dois).
    applySuggestion() {
      const s = this.suggestion;
      if (!s) return;
      this.rec.fee = fmt.moneyInput(s.late_fee_cents);
      this.rec.interest = fmt.moneyInput(s.interest_cents);
      this.rec.amount = fmt.moneyInput(s.total_cents);
      this.clearFields();
    },
    // saveReceive confere os campos como o servidor e registra o recebimento: da conta gravada (POST /:id/receive) ou
    // do período calculado, num passo só (POST /receive). No por hora o valor é o da conta, e multa e juros não existem.
    saveReceive() {
      const row = this.receiving;
      if (!row) return undefined;
      const today = this.data.today;
      const notFuture = (v, field) => (v > today ? errorText({ code: 'receivable.received_in_future', params: { field } }) : '');
      const spec = {
        received_on: [this.rec.on, rules.required, rules.date, notFuture],
        invoice_number: [this.rec.invoice, rules.max('invoice_number')],
      };
      if (this.monthly) {
        // A multa e os juros são partes do valor recebido: não passam dele.
        const within = (v, field) => (cents(this.rec.fee) + cents(this.rec.interest) > cents(this.rec.amount)
          ? errorText({ code: 'receivable.charges_exceed_received', params: { field } }) : '');
        spec.received_cents = [this.rec.amount, positive('receivable.invalid_amount')];
        spec.late_fee_cents = [this.rec.fee, charge, within];
        spec.interest_cents = [this.rec.interest, charge];
      }
      if (!this.check(spec)) return undefined;
      const body = { received_on: this.rec.on, invoice_number: this.rec.invoice.trim() };
      if (this.monthly) {
        body.received_cents = cents(this.rec.amount);
        body.late_fee_cents = cents(this.rec.fee);
        body.interest_cents = cents(this.rec.interest);
      }
      return this.run('receive', async () => {
        if (row.id) {
          await api('POST', base + '/' + row.id + '/receive', body);
        } else {
          await api('POST', base + '/receive', { customer_id: row.customer.id, period_start: row.period_start, period_end: row.period_end, ...body });
        }
        Alpine.store('modal').close();
        Alpine.store('toast').show(t('receivable.toast.received', { name: row.customer.name }));
        await this.reload();
      });
    },
    // unreceive desfaz o recebimento: a conta volta para A receber, ajustada, sem a multa e os juros informados.
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
