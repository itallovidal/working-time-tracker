// A volta do Trello depois de a pessoa autorizar. O Trello devolve o token no fragmento da URL
// (#token=...), que o navegador não manda ao servidor: este script o lê, apaga o endereço do histórico e
// o entrega, com o state da query, ao servidor, que confere o cookie da conexão e guarda a integração.
(function () {
  const token = new URLSearchParams(location.hash.replace(/^#/, '')).get('token') || '';
  const state = new URLSearchParams(location.search).get('state') || '';
  history.replaceState(null, '', location.pathname);

  // O servidor responde para onde ir; aceitamos só um caminho do próprio site.
  const go = (to) => location.replace(typeof to === 'string' && to.startsWith('/') && !to.startsWith('//') && !to.includes('\\') ? to : '/');

  fetch('/integrations/trello/token', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ state, token }),
  })
    .then((res) => res.json())
    .then((data) => go(data && data.redirect))
    .catch(() => go('/'));
})();
