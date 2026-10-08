// A ajuda: marca no sumário a seção que está na tela e, no celular, recolhe o sumário.
// Sem JavaScript os links do sumário continuam funcionando como âncoras comuns.
document.addEventListener('alpine:init', () => {
  Alpine.data('helpPage', () => {
    // Fora do objeto do componente: o Alpine embrulha o que fica nele em proxies, e o
    // MediaQueryList, o observador e os elementos não funcionam assim.
    const narrow = window.matchMedia('(max-width: 860px)');
    let root;
    let toc;
    let bar;
    let links = [];
    let marks = []; // os grupos e as seções, na ordem da página
    let pinned = ''; // o item que a pessoa clicou, até ela rolar por conta própria
    let settledAt = null; // onde a página parou depois de rolar até o item clicado
    let settleTimer = 0;
    let frame = 0;

    // O quanto do alto da tela é da barra superior: o sumário gruda abaixo dela e os trechos param
    // abaixo dela. No celular ela quebra em várias linhas, então a altura vem da própria barra.
    function fit() {
      if (bar) root.style.setProperty('--help-top', `${bar.offsetHeight + 16}px`);
    }

    // A linha de leitura: a seção cujo topo já passou dela é a que a pessoa está lendo.
    function line() {
      return parseFloat(getComputedStyle(root).getPropertyValue('--help-top')) + 24;
    }

    // Destaca o item do sumário de `id` (ou nenhum, com ''), mantendo-o à vista quando o sumário
    // rola sozinho.
    function mark(id) {
      for (const a of links) {
        if (id !== '' && a.getAttribute('href') === `#${id}`) {
          a.setAttribute('aria-current', 'true');
          reveal(a);
        } else {
          a.removeAttribute('aria-current');
        }
      }
    }

    function reveal(link) {
      if (narrow.matches) return;
      const box = toc.getBoundingClientRect();
      const r = link.getBoundingClientRect();
      if (r.top < box.top) toc.scrollTop -= box.top - r.top + 8;
      else if (r.bottom > box.bottom) toc.scrollTop += r.bottom - box.bottom + 8;
    }

    function refresh() {
      frame = 0;
      if (pinned !== '') return mark(pinned);
      const items = marks.filter((m) => m.hasAttribute('data-help-section'));
      const atEnd = window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 2;
      if (atEnd && items.length) return mark(items[items.length - 1].id);
      const y = line();
      let current = '';
      for (const m of marks) {
        if (m.getBoundingClientRect().top > y) break;
        current = m.id;
      }
      mark(current);
    }

    function schedule() {
      if (!frame) frame = requestAnimationFrame(refresh);
    }

    function unpin() {
      if (pinned === '') return;
      pinned = '';
      settledAt = null;
      schedule();
    }

    // O item clicado vale até a página assentar nele (a rolagem suave dispara scroll a cada quadro)
    // e a pessoa rolar de novo, por qualquer meio, inclusive arrastando a barra de rolagem.
    function pin(id) {
      pinned = id;
      settledAt = null;
      clearTimeout(settleTimer);
      settleTimer = setTimeout(() => { settledAt = window.scrollY; }, 300);
    }

    function onScroll() {
      if (pinned !== '') {
        if (settledAt !== null && Math.abs(window.scrollY - settledAt) > 4) return unpin();
        if (settledAt === null) {
          clearTimeout(settleTimer);
          settleTimer = setTimeout(() => { settledAt = window.scrollY; }, 200);
        }
      }
      schedule();
    }

    return {
      init() {
        root = this.$root;
        toc = root.querySelector('[data-help-toc]');
        bar = document.querySelector('.topbar');
        links = [...toc.querySelectorAll('a[href^="#"]')];
        marks = [...root.querySelectorAll('.help-group, [data-help-section]')];

        fit();
        if (bar) new ResizeObserver(() => { fit(); schedule(); }).observe(bar);

        // O servidor entrega o sumário aberto (vale sem JavaScript). No celular ele começa
        // recolhido; ao alargar a janela, volta a ficar aberto.
        const place = () => {
          toc.open = !narrow.matches;
        };
        place();
        narrow.addEventListener('change', place);

        // Recolher o sumário muda a altura da página: com uma âncora no endereço, volta ao trecho.
        const hash = decodeURIComponent(location.hash.slice(1));
        const target = hash && document.getElementById(hash);
        if (target) {
          pin(hash);
          if (narrow.matches) requestAnimationFrame(() => target.scrollIntoView({ behavior: 'instant', block: 'start' }));
        }

        window.addEventListener('scroll', onScroll, { passive: true });
        window.addEventListener('resize', schedule);
        // Rolar com a roda, o dedo ou o teclado solta o item clicado: o destaque volta a seguir a leitura.
        for (const ev of ['wheel', 'touchmove', 'keydown']) window.addEventListener(ev, unpin, { passive: true });
        schedule();
      },

      // Clique no sumário: rola até o trecho, com o sumário já recolhido no celular.
      go(ev) {
        const a = ev.target.closest('a[href^="#"]');
        if (!a) return;
        const id = decodeURIComponent(a.getAttribute('href').slice(1));
        const target = document.getElementById(id);
        if (!target) return;
        ev.preventDefault();
        if (narrow.matches) toc.open = false;
        history.replaceState(null, '', `#${id}`);
        pin(id);
        mark(id);
        requestAnimationFrame(() => target.scrollIntoView({ block: 'start' }));
      },
    };
  });
});
