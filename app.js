/* flakestat docs — behaviour. No framework, no build step. */
(function () {
  'use strict';
  // Derived from this script's own URL rather than a baked-in base path.
  // A hardcoded base breaks the moment the site moves host or subpath - and it
  // fails silently, because a 404 on the index just means search returns
  // nothing. The script always knows where it was loaded from.
  var BASE = (function () {
    var el = document.currentScript ||
      (function () { var s = document.getElementsByTagName('script'); return s[s.length - 1]; })();
    try {
      return new URL('.', el.src).pathname;
    } catch (e) {
      return document.documentElement.dataset.base || '/';
    }
  })();
  var $ = function (s, r) { return (r || document).querySelector(s); };
  var $$ = function (s, r) { return Array.prototype.slice.call((r || document).querySelectorAll(s)); };

  /* ---------------------------------------------------------------- theme */
  var KEY = 'flakestat-theme', root = document.documentElement;
  function isDark() {
    var t = root.getAttribute('data-theme');
    return t ? t === 'dark' : matchMedia('(prefers-color-scheme: dark)').matches;
  }
  function paintLogos() {
    $$('img[data-light]').forEach(function (img) {
      var want = isDark() ? img.dataset.dark : img.dataset.light;
      if (want && img.getAttribute('src') !== want) img.setAttribute('src', want);
    });
  }
  paintLogos();
  var tbtn = $('#theme');
  if (tbtn) tbtn.addEventListener('click', function () {
    root.setAttribute('data-theme', isDark() ? 'light' : 'dark');
    try { localStorage.setItem(KEY, root.getAttribute('data-theme')); } catch (e) {}
    paintLogos();
  });
  matchMedia('(prefers-color-scheme: dark)').addEventListener('change', paintLogos);

  /* ----------------------------------------------------------- mobile nav */
  var mbtn = $('#menu'), sidebar = $('.sidebar');
  if (mbtn && sidebar) mbtn.addEventListener('click', function () {
    sidebar.hidden = !sidebar.hidden;
    mbtn.setAttribute('aria-expanded', String(!sidebar.hidden));
  });
  function syncSidebar() { if (sidebar && innerWidth > 900) sidebar.hidden = false; }
  addEventListener('resize', syncSidebar); syncSidebar();

  /* --------------------------------------------------------------- copy */
  function attachCopy(btn, get) {
    btn.addEventListener('click', function () {
      var text = get();
      var done = function () {
        var was = btn.textContent;
        btn.textContent = 'Copied'; btn.classList.add('done');
        setTimeout(function () { btn.textContent = was; btn.classList.remove('done'); }, 1400);
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done, function () {});
      } else {
        var ta = document.createElement('textarea');
        ta.value = text; ta.style.position = 'fixed'; ta.style.opacity = '0';
        document.body.appendChild(ta); ta.select();
        try { document.execCommand('copy'); done(); } catch (e) {}
        document.body.removeChild(ta);
      }
    });
  }
  $$('.cblock').forEach(function (block) {
    var btn = $('.copy', block);
    if (btn) attachCopy(btn, function () {
      var pane = $('.cpane:not([hidden]) pre', block) || $('pre', block);
      return pane ? pane.innerText : '';
    });
  });

  /* --------------------------------------------------------------- tabs */
  $$('.cblock[data-tabs]').forEach(function (block) {
    var tabs = $$('.tab', block), panes = $$('.cpane', block);
    tabs.forEach(function (tab, i) {
      tab.addEventListener('click', function () {
        tabs.forEach(function (t, j) { t.setAttribute('aria-selected', String(i === j)); });
        panes.forEach(function (p, j) { p.hidden = i !== j; });
        try { localStorage.setItem('flakestat-lang', tab.dataset.lang || ''); } catch (e) {}
      });
    });
  });
  // Remember the reader's language across pages.
  try {
    var lang = localStorage.getItem('flakestat-lang');
    if (lang) $$('.cblock[data-tabs] .tab').forEach(function (t) {
      if (t.dataset.lang === lang) t.click();
    });
  } catch (e) {}

  /* ------------------------------------------------------ progress + toc */
  var bar = $('#progress');
  var tocLinks = $$('.toc a');
  var heads = tocLinks.length ? $$('.content h2[id], .content h3[id]') : [];

  // On the single docs page the sidebar is the table of contents, so it gets
  // the same treatment: highlight whichever section the reader is inside.
  var sideLinks = $$('.sidebar a[data-anchor]');
  var sections = sideLinks.length ? $$('.content section[id]') : [];

  function onScroll() {
    if (bar) {
      var h = document.documentElement.scrollHeight - innerHeight;
      bar.style.width = (h > 0 ? Math.min(100, (scrollY / h) * 100) : 0) + '%';
    }
    if (heads.length) {
      var cur = heads[0], y = scrollY + 130;
      heads.forEach(function (el) { if (el.offsetTop <= y) cur = el; });
      tocLinks.forEach(function (a) {
        a.classList.toggle('on', a.getAttribute('href') === '#' + cur.id);
      });
    }
    if (sections.length) {
      var s0 = sections[0], sy = scrollY + 140;
      sections.forEach(function (el) { if (el.offsetTop <= sy) s0 = el; });
      sideLinks.forEach(function (a) {
        var on = a.dataset.anchor === s0.id;
        a.classList.toggle('on', on);
        if (on) a.setAttribute('aria-current', 'true'); else a.removeAttribute('aria-current');
      });
    }
  }
  var ticking = false;
  addEventListener('scroll', function () {
    if (ticking) return; ticking = true;
    requestAnimationFrame(function () { onScroll(); ticking = false; });
  }, { passive: true });
  onScroll();

  /* ------------------------------------------------------------- search */
  var scrim = $('#scrim'), input = $('#q'), results = $('#results'), index = null, sel = 0, hits = [];

  function load() {
    if (index) return Promise.resolve(index);
    return fetch(BASE + 'search-index.json')
      .then(function (r) {
        if (!r.ok) throw new Error('index ' + r.status);
        return r.json();
      })
      .then(function (j) { index = j; return j; });
  }
  function open() {
    if (!scrim) return;
    scrim.hidden = false; input.value = ''; results.innerHTML = '';
    document.body.style.overflow = 'hidden';
    load().then(function () { input.focus(); render(''); }, function (err) {
      results.innerHTML = '<div class="empty">Search index unavailable (' +
        esc(String(err.message || err)) + '). Reload, or browse using the contents.</div>';
    });
  }
  function close() {
    if (!scrim) return;
    scrim.hidden = true; document.body.style.overflow = '';
  }
  function esc(s) { return s.replace(/[&<>"]/g, function (c) {
    return ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]; }); }

  // Every query term must appear. Title matches outrank body matches, and an
  // exact phrase outranks scattered terms, which is what makes short queries
  // like "junit" land on the right page instead of the longest one.
  function score(item, terms, q) {
    var t = item.title.toLowerCase(), b = item.text.toLowerCase(), s = 0;
    for (var i = 0; i < terms.length; i++) {
      var w = terms[i];
      if (t.indexOf(w) === -1 && b.indexOf(w) === -1 && item.keywords.indexOf(w) === -1) return 0;
      if (t.indexOf(w) === 0) s += 60; else if (t.indexOf(w) > -1) s += 34;
      if (item.keywords.indexOf(w) > -1) s += 18;
      var n = b.split(w).length - 1;
      s += Math.min(n, 6) * 3;
    }
    if (q.length > 2 && t.indexOf(q) > -1) s += 70;
    if (q.length > 2 && b.indexOf(q) > -1) s += 22;
    return s;
  }
  function snippet(text, q) {
    var i = q ? text.toLowerCase().indexOf(q) : -1;
    var start = i > 90 ? i - 70 : 0;
    var out = text.slice(start, start + 190);
    if (start > 0) out = '…' + out;
    out = esc(out);
    if (q) out = out.replace(new RegExp('(' + q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + ')', 'ig'), '<mark>$1</mark>');
    return out;
  }
  function render(q) {
    q = q.trim().toLowerCase();
    var terms = q.split(/\s+/).filter(Boolean);
    hits = !terms.length
      ? index.slice(0, 8)
      : index.map(function (it) { return { it: it, s: score(it, terms, q) }; })
             .filter(function (x) { return x.s > 0; })
             .sort(function (a, b) { return b.s - a.s; })
             .slice(0, 12).map(function (x) { return x.it; });
    sel = 0;
    if (!hits.length) {
      results.innerHTML = '<div class="empty">No matches for “' + esc(q) + '”.</div>';
      return;
    }
    results.innerHTML = hits.map(function (it, i) {
      return '<a href="' + BASE + it.url + '" class="' + (i === 0 ? 'sel' : '') + '">' +
        '<span class="r-s">' + esc(it.section) + '</span>' +
        '<div class="r-t">' + esc(it.title) + '</div>' +
        '<div class="r-c">' + snippet(it.text, q) + '</div></a>';
    }).join('');
  }
  function move(d) {
    var links = $$('a', results); if (!links.length) return;
    links[sel] && links[sel].classList.remove('sel');
    sel = (sel + d + links.length) % links.length;
    links[sel].classList.add('sel');
    links[sel].scrollIntoView({ block: 'nearest' });
  }
  if (scrim) {
    $$('[data-search]').forEach(function (b) { b.addEventListener('click', open); });
    input.addEventListener('input', function () { render(input.value); });
    scrim.addEventListener('click', function (e) { if (e.target === scrim) close(); });
    addEventListener('keydown', function (e) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); scrim.hidden ? open() : close(); return; }
      if (e.key === '/' && scrim.hidden && !/^(INPUT|TEXTAREA)$/.test((document.activeElement || {}).tagName)) { e.preventDefault(); open(); return; }
      if (scrim.hidden) return;
      if (e.key === 'Escape') close();
      else if (e.key === 'ArrowDown') { e.preventDefault(); move(1); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); move(-1); }
      else if (e.key === 'Enter') { var l = $$('a', results)[sel]; if (l) location.href = l.href; }
    });
  }

  /* ---------------------------------------------------------- score demo */
  // A faithful port of the scoring rules for one execution context on one
  // commit: skips carry no signal, transitions are counted between adjacent
  // scored outcomes, and classification uses a Wilson lower bound so a verdict
  // needs evidence rather than a lucky flip.
  var demo = $('#demo');
  if (demo) {
    var seqEl = $('.seq', demo), MIN_RUNS = 5, FLAKY = 0.10, SUSPECT = 0.05;
    var state = 'PPFPPFPPFPPF'.split('');

    // Ported from internal/score/score.go. z is the one-sided 80% bound the
    // binary uses -- not 1.96 -- and the shape of the expression matches it
    // term for term, so the numbers shown here are the numbers you get.
    function wilson(p, n) {
      if (n <= 0) return 0;
      var z = 0.8416, z2 = z * z;
      var centre = (p + z2 / (2 * n)) / (1 + z2 / n);
      var margin = (z / (1 + z2 / n)) * Math.sqrt(p * (1 - p) / n + z2 / (4 * n * n));
      return Math.max(0, centre - margin);
    }
    function evaluate(seq) {
      var scored = seq.filter(function (c) { return c !== 'S'; });
      var passes = scored.filter(function (c) { return c === 'P'; }).length;
      var fails = scored.length - passes;
      if (!scored.length) return { verdict: 'always-skipped', score: 0, lower: 0, trans: 0, passes: 0, fails: 0, runs: 0 };
      var flips = 0, trans = 0;
      for (var i = 1; i < scored.length; i++) { trans++; if (scored[i] !== scored[i - 1]) flips++; }
      var score = trans ? flips / trans : 0;
      var lower = wilson(score, trans);
      var verdict;
      if (scored.length < MIN_RUNS) verdict = 'insufficient-data';
      else if (fails === scored.length) verdict = 'consistently-failing';
      else if (lower >= FLAKY) verdict = 'flaky';
      // Suspect deliberately uses the point estimate rather than the bound:
      // it is the "worth a look" bucket, where being early beats being sure.
      else if (score >= SUSPECT) verdict = 'suspect';
      else verdict = 'stable';
      return { verdict: verdict, score: score, lower: lower, trans: trans, flips: flips,
               passes: passes, fails: fails, runs: scored.length };
    }
    function why(r) {
      if (r.verdict === 'always-skipped') return 'Never ran, so there is nothing to measure. More runs would not help — which is why this is not “insufficient data”.';
      if (r.verdict === 'insufficient-data') return 'Only ' + r.runs + ' scored run(s), below the ' + MIN_RUNS + ' needed before any verdict is claimed.';
      if (r.verdict === 'consistently-failing') return 'Fails every time. That is broken, not flaky — the score is zero because the outcome never disagrees with itself.';
      if (r.verdict === 'flaky') return 'Changed answer ' + r.flips + ' time(s) across ' + r.trans + ' comparison(s). Even the lower bound, ' + r.lower.toFixed(2) + ', clears the ' + FLAKY + ' threshold.';
      if (r.verdict === 'suspect') return 'Disagreed ' + r.flips + ' time(s) in ' + r.trans + ' comparison(s). The point estimate clears ' + SUSPECT + ', but the lower bound (' + r.lower.toFixed(2) + ') does not reach ' + FLAKY + ' — worth watching, not worth a ticket.';
      return 'Changed answer ' + r.flips + ' time(s) across ' + r.trans + ' comparison(s) — below the ' + SUSPECT + ' threshold once sample size is accounted for.';
    }
    function draw() {
      seqEl.innerHTML = state.map(function (c, i) {
        return '<button class="' + c.toLowerCase() + '" data-i="' + i + '" title="Click to cycle pass → fail → skip">' + c + '</button>';
      }).join('');
      var r = evaluate(state);
      var cls = { flaky: 'flaky', stable: 'stable', suspect: 'suspect' }[r.verdict] || 'other';
      $('#d-verdict', demo).className = cls;
      $('#d-verdict', demo).textContent = r.verdict;
      $('#d-score', demo).textContent = r.score.toFixed(2);
      $('#d-lower', demo).textContent = r.lower.toFixed(2);
      $('#d-runs', demo).textContent = r.passes + '/' + r.fails;
      $('#d-trans', demo).textContent = r.trans;
      $('#d-why', demo).textContent = why(r);
    }
    seqEl.addEventListener('click', function (e) {
      var b = e.target.closest('button[data-i]'); if (!b) return;
      var i = +b.dataset.i, order = { P: 'F', F: 'S', S: 'P' };
      state[i] = order[state[i]]; draw();
    });
    $$('[data-demo]', demo).forEach(function (b) {
      b.addEventListener('click', function () {
        var p = b.dataset.demo;
        if (p === 'add') state.push('P');
        else if (p === 'remove') state.pop();
        else state = p.split('');
        if (state.length < 1) state = ['P'];
        draw();
      });
    });
    draw();
  }
})();
