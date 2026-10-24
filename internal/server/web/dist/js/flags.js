// Offline ISO 3166-1 flags, plus regional flags, from lipis/flag-icons v7.3.2 (MIT).
(function () {
  'use strict';

  const availableFlags = new Set(('ad ae af ag ai al am ao aq ar arab as asean at au aw ax az ba bb bd be bf bg bh bi bj bl bm bn bo bq br bs bt bv bw by bz ca cc cd cefta cf cg ch ci ck cl cm cn co cp cr cu cv cw cx cy cz de dg dj dk dm do dz eac ec ee eg eh er es es-ct es-ga es-pv et eu fi fj fk fm fo fr ga gb gb-eng gb-nir gb-sct gb-wls gd ge gf gg gh gi gl gm gn gp gq gr gs gt gu gw gy hk hm hn hr ht hu ic id ie il io iq ir is it je jm jo jp ke kg kh ki km kn kp kr kw ky kz la lb lc li lk lr ls lt lu lv ly ma mc md me mf mh mk ml mm mn mo mp mq mr ms mt mu mv mw mx my mz na nc ne nf ng ni nl no np nr nu nz om pa pc pe pf pg ph pk pl pm pn pr ps pt pw py qa re ro rs ru rw sa sb sc sd se sg sh sh-ac sh-hl sh-ta si sj sk sl sm sn so sr ss st sv sx sy sz tc td tf tg th tj tk tl tm tn to tr tt tv tw tz ua ug um un us uy uz va vc ve vg vi vn vu wf ws xk xx ye yt za zm zw').split(' '));
  const englishNames = typeof Intl.DisplayNames === 'function' ? new Intl.DisplayNames(['en'], { type: 'region' }) : null;
  const chineseNames = typeof Intl.DisplayNames === 'function' ? new Intl.DisplayNames(['zh-CN'], { type: 'region' }) : null;
  const aliases = new Map();

  function normalizeName(value) {
    return String(value || '').toLocaleLowerCase().replace(/[^a-z\u3400-\u9fff]/g, '');
  }

  for (const code of availableFlags) {
    if (code.length !== 2) continue;
    for (const names of [englishNames, chineseNames]) {
      if (!names) continue;
      try {
        const displayName = names.of(code.toUpperCase());
        const normalized = normalizeName(displayName);
        if (normalized && normalized !== code && normalized !== 'unknownregion' && normalized.length >= 3) aliases.set(normalized, code);
      } catch (_) { /* Unsupported display name; the ISO code and flag emoji still work. */ }
    }
  }

  // Common aliases used in node names and older configuration files.
  Object.entries({
    uk: 'gb', usa: 'us', unitedstatesofamerica: 'us',
    southkorea: 'kr', republicofkorea: 'kr', northkorea: 'kp',
    hongkong: 'hk', macau: 'mo', taiwan: 'tw',
    russia: 'ru', vietnam: 'vn', laos: 'la', moldova: 'md',
    bolivia: 'bo', tanzania: 'tz', venezuela: 've', brunei: 'bn',
    england: 'gb-eng', scotland: 'gb-sct', wales: 'gb-wls', northernireland: 'gb-nir',
    '中国香港': 'hk', '中国澳门': 'mo', '中国台湾': 'tw', '韩国': 'kr', '朝鲜': 'kp',
  }).forEach(([name, code]) => aliases.set(normalizeName(name), code));

  function directCode(value) {
    const text = String(value || '').trim().toLowerCase();
    if (availableFlags.has(text)) return text;

    // The server sends country flags as Unicode regional-indicator pairs.
    const indicators = Array.from(text).map(char => char.codePointAt(0)).filter(point => point >= 0x1f1e6 && point <= 0x1f1ff);
    if (indicators.length === 2) {
      const code = String.fromCharCode(indicators[0] - 0x1f1e6 + 65, indicators[1] - 0x1f1e6 + 65).toLowerCase();
      if (availableFlags.has(code)) return code;
    }
    return '';
  }

  function resolveCountryCode(flag, name, region) {
    for (const candidate of [flag, region]) {
      const code = directCode(candidate);
      if (code && code !== 'global') return code;
    }

    const text = normalizeName(`${flag || ''} ${region || ''} ${name || ''}`);
    if (!text || ['global', 'world', '🌐'].includes(String(flag || '').trim().toLowerCase())) return '';
    const matches = Array.from(aliases.entries()).sort((a, b) => b[0].length - a[0].length);
    for (const [alias, code] of matches) {
      if (text.includes(alias)) return code;
    }
    return '';
  }

  function getCountryName(code) {
    try {
      return chineseNames?.of(code.toUpperCase()) || englishNames?.of(code.toUpperCase()) || code.toUpperCase();
    } catch (_) {
      return code.toUpperCase();
    }
  }

  function escapeAttribute(value) {
    return String(value).replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]);
  }

  function getFlagHTML(flag, name, region) {
    const code = resolveCountryCode(flag, name, region);
    if (!code) return '<span class="flag-container" title="全球服务器"><img src="/flags/global.svg" alt="全球服务器"></span>';
    const countryName = escapeAttribute(getCountryName(code));
    return `<span class="flag-container" title="${countryName}"><img src="/flags/${code}.svg" alt="${countryName}" loading="lazy" decoding="async"></span>`;
  }

  document.addEventListener('error', event => {
    const image = event.target;
    if (image instanceof HTMLImageElement && image.matches('.flag-container img') && !image.dataset.fallback) {
      image.dataset.fallback = 'true';
      image.src = '/flags/global.svg';
      image.alt = '全球服务器';
      image.parentElement.title = '全球服务器';
    }
  }, true);

  window.TianQueFlags = { resolveCountryCode, getFlagHTML };
})();
