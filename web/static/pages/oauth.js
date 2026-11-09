// A tela de consentimento do OAuth do MCP (/oauth/authorize): a pessoa logada deixa, ou não, o Claude agir em nome
// dela. O servidor já conferiu o pedido; o "Permitir" e o "Cancelar" o devolvem a /api/oauth/consent, que confere de
// novo e responde o endereço de volta ao app (com o código, ou com o access_denied). A tela só vai para lá.
document.addEventListener('alpine:init', () => {
  const { api, form } = WTT;

  Alpine.data('oauthConsent', () => ({
    ...form(),
    answer(approve) {
      return this.run('consent', async () => {
        const req = WTT.boot.oauth;
        const res = await api('POST', '/api/oauth/consent', {
          client_id: req.client_id,
          redirect_uri: req.redirect_uri,
          state: req.state,
          response_type: 'code',
          code_challenge: req.code_challenge,
          code_challenge_method: 'S256',
          resource: req.resource,
          scope: req.scope,
          approve,
        });
        // O pending continua ligado até a página sair: um segundo clique não manda o pedido de novo.
        this.pending = 'leaving';
        location.href = res.redirect;
        await new Promise(() => {});
      });
    },
    // otherAccount sai desta conta e volta ao login, que depois traz a pessoa de novo para esta tela.
    otherAccount() {
      return this.run('consent', async () => {
        try {
          await api('POST', '/api/auth/logout');
        } finally {
          location.href = '/login?out=1&next=' + encodeURIComponent(WTT.boot.authorizeURL || location.pathname + location.search);
        }
      });
    },
  }));
});
