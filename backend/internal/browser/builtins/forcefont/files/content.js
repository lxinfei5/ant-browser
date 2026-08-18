/* Vendored from local Tampermonkey script
 * 强制字体（Monaco + 95% 微软雅黑） v3.1.3
 * @grant none. No remote URLs, no @require.
 */
(function () {
  'use strict';

  /* ==================== 配置区 ==================== */

  // true：保留网站原有的代码块和代码编辑器字体
  // false：代码块也使用 Monaco + 微软雅黑
  const SKIP_CODE = true;

  // 检测未知图标字体。超大网页感觉卡顿时可以设为 false。
  const SMART_ICON_RESCUE = true;

  // 微软雅黑相对于网页原字号的缩放比例
  const YAHEI_SIZE_ADJUST = '95%';

  /*
   * 限制雅黑别名只覆盖 CJK 表意文字、中文标点与全角字符。
   * 没有这一行时，雅黑的拉丁字形会抢在 Monaco 前面渲染英文/数字。
   * 末尾四项让中文语境的弯引号、破折号、省略号也使用雅黑。
   */
  const CJK_UNICODE_RANGE =
    'U+2E80-2EFF, U+3000-303F, U+3400-4DBF, U+4E00-9FFF, ' +
    'U+F900-FAFF, U+FF00-FFEF, U+20000-2A6DF, ' +
    'U+2013-2014, U+2018-201D, U+2026';

  /*
   * 字体回退顺序（逐字形回退）：
   * 中文/中文标点 → 缩放到 95% 的微软雅黑（FF 别名经 unicode-range 限定 CJK 区间，
   *                 不含拉丁字形，放在栈首也不会截胡英文）
   * 英文、数字     → Monaco
   * 没有 Monaco    → Consolas（Windows 自带等宽兜底）
   * 没有微软雅黑   → 苹方等系统中文字体
   */
  const FONT_STACK = [
    '"FF Microsoft YaHei"',
    '"Monaco"',
    '"Consolas"',
    '"Microsoft YaHei"',
    '"Microsoft YaHei UI"',
    '"PingFang SC"',
    '"Hiragino Sans GB"',
    '"Noto Sans CJK SC"',
    '"Source Han Sans SC"',
    '"WenQuanYi Micro Hei"',
    '"Apple Color Emoji"',
    '"Segoe UI Emoji"',
    '"Noto Color Emoji"',
    '"Segoe UI Symbol"',
    'sans-serif'
  ].join(', ');

  /* =============================================== */

  const STYLE_ID = '__force_font_style__';

  const ICON_ROOTS = [
    'svg',
    'use',
    'ion-icon',

    '[data-icon]',
    '[data-ff-skip]',

    // 通用图标类名
    '[class~="icon" i]',
    '[class^="icon-" i]',
    '[class*=" icon-" i]',
    '[class$="-icon" i]',
    '[class*="-icon " i]',

    '[class~="glyph" i]',
    '[class^="glyph-" i]',
    '[class*=" glyph-" i]',

    '[class~="symbol" i]',
    '[class^="symbol-" i]',
    '[class*=" symbol-" i]',

    // Font Awesome
    '[class~="fa" i]',
    '[class~="fas" i]',
    '[class~="far" i]',
    '[class~="fal" i]',
    '[class~="fab" i]',
    '[class~="fad" i]',
    '[class^="fa-" i]',
    '[class*=" fa-" i]',

    // 常见图标库
    '[class~="anticon" i]',
    '[class*="anticon-" i]',
    '[class~="iconfont" i]',
    '[class*="iconfont-" i]',
    '[class*="glyphicon" i]',
    '[class*="dashicon" i]',
    '[class*="codicon" i]',
    '[class*="octicon" i]',
    '[class*="ionicon" i]',
    '[class*="material-icons" i]',
    '[class*="material-symbols" i]',

    // 品牌 Logo
    '[class~="logo" i]',
    '[class^="logo-" i]',
    '[class*=" logo-" i]',
    '[class$="-logo" i]',
    '[class*="-logo " i]'
  ];

  const CODE_ROOTS = [
    'code',
    'pre',
    'kbd',
    'samp',

    '[class~="code" i]',
    '[class^="code-" i]',
    '[class*=" code-" i]',
    '[class~="hljs" i]',
    '[class~="highlight" i]',
    '[class^="language-" i]',
    '[class*=" language-" i]',

    // 在线代码编辑器
    '[class*="monaco-editor" i]',
    '[class*="codemirror" i]',
    '[class*="ace_editor" i]'
  ];

  function includeDescendants(selectors) {
    return selectors.flatMap(selector => [
      selector,
      `${selector} *`
    ]);
  }

  const ICON_TREE_SELECTORS = includeDescendants(ICON_ROOTS);

  const CODE_TREE_SELECTORS = SKIP_CODE
    ? includeDescendants(CODE_ROOTS)
    : [];

  const SKIP_SELECTORS = [
    ...ICON_TREE_SELECTORS,
    ...CODE_TREE_SELECTORS
  ];

  const ICON_TREE_SELECTOR = ICON_TREE_SELECTORS.join(',');
  const CODE_TREE_SELECTOR = CODE_TREE_SELECTORS.join(',');

  /*
   * FF Microsoft YaHei 是微软雅黑的本地别名。
   * unicode-range 把它限制在 CJK 区间：中文走雅黑（size-adjust 95%），
   * 拉丁字符回退给 Monaco，不会被雅黑的拉丁字形截胡。
   */
  const CSS = `
    @font-face {
      font-family: "FF Microsoft YaHei";
      src: local("Microsoft YaHei Light");
      font-style: normal;
      font-weight: 300;
      font-display: swap;
      size-adjust: ${YAHEI_SIZE_ADJUST};
      unicode-range: ${CJK_UNICODE_RANGE};
    }

    @font-face {
      font-family: "FF Microsoft YaHei";
      src: local("Microsoft YaHei");
      font-style: normal;
      font-weight: 400;
      font-display: swap;
      size-adjust: ${YAHEI_SIZE_ADJUST};
      unicode-range: ${CJK_UNICODE_RANGE};
    }

    @font-face {
      font-family: "FF Microsoft YaHei";
      src: local("Microsoft YaHei Bold");
      font-style: normal;
      font-weight: 700;
      font-display: swap;
      size-adjust: ${YAHEI_SIZE_ADJUST};
      unicode-range: ${CJK_UNICODE_RANGE};
    }

    :where(*:not(:is(${SKIP_SELECTORS.join(',')}))) {
      font-family: ${FONT_STACK} !important;
    }
  `;

  /* -------------------- 样式注入 -------------------- */

  let styleElement = null;

  function injectCSS() {
    if (styleElement?.isConnected) return true;

    const existing = document.getElementById(STYLE_ID);

    if (existing) {
      styleElement = existing;
      return true;
    }

    const parent = document.head || document.documentElement;
    if (!parent) return false;

    styleElement = document.createElement('style');
    styleElement.id = STYLE_ID;
    styleElement.textContent = CSS;
    parent.appendChild(styleElement);

    return true;
  }

  injectCSS();

  /* -------------------- 图标检测 -------------------- */

  // BMP PUA + Unicode Plane 15/16 PUA
  const PUA_RE =
    /[\uE000-\uF8FF\u{F0000}-\u{FFFFD}\u{100000}-\u{10FFFD}]/u;

  const ICON_FONT_RE =
    /icon|glyph|font\s?awesome|material\s?(icons|symbols)|anticon|iconfont|ionicons|feather|remix|typicons|entypo|dashicons|octicons|codicon|bootstrap.?icons/i;

  function hasDirectPuaText(el) {
    for (const node of el.childNodes) {
      if (
        node.nodeType === Node.TEXT_NODE &&
        PUA_RE.test(node.nodeValue || '')
      ) {
        return true;
      }
    }

    return false;
  }

  function pseudoLooksLikeIcon(el, pseudo) {
    let computedStyle;

    try {
      computedStyle = getComputedStyle(el, pseudo);
    } catch {
      return false;
    }

    if (!computedStyle) return false;

    const content = computedStyle.content;

    if (
      !content ||
      content === 'none' ||
      content === 'normal' ||
      content === '""' ||
      content === "''"
    ) {
      return false;
    }

    return (
      PUA_RE.test(content) ||
      ICON_FONT_RE.test(computedStyle.fontFamily)
    );
  }

  function usesIconFont(el) {
    return (
      hasDirectPuaText(el) ||
      pseudoLooksLikeIcon(el, '::before') ||
      pseudoLooksLikeIcon(el, '::after')
    );
  }

  function shouldInspect(el) {
    if (!el.isConnected) return false;

    // 跳过 SVG 等非 HTML 元素
    if (el.namespaceURI !== 'http://www.w3.org/1999/xhtml') {
      return false;
    }

    if (
      el.matches(
        'script,style,link,meta,head,title,template,noscript'
      )
    ) {
      return false;
    }

    // 已知图标无需进行 getComputedStyle 检测
    if (
      ICON_TREE_SELECTOR &&
      el.matches(ICON_TREE_SELECTOR)
    ) {
      return false;
    }

    // 跳过代码块及其全部后代
    if (
      SKIP_CODE &&
      CODE_TREE_SELECTOR &&
      el.matches(CODE_TREE_SELECTOR)
    ) {
      return false;
    }

    /*
     * 叶子节点最可能是图标。
     * 带 class 的非叶子节点也可能通过伪元素显示图标。
     */
    return (
      el.childElementCount === 0 ||
      el.matches('i,span,a,button,label,li,[class]')
    );
  }

  /* -------------------- 共享分片队列 -------------------- */

  const pendingElements = new Set();
  let drainScheduled = false;

  const scheduleIdle = window.requestIdleCallback
    ? callback =>
        window.requestIdleCallback(callback, {
          timeout: 300
        })
    : callback =>
        window.setTimeout(
          () =>
            callback({
              didTimeout: true,
              timeRemaining: () => 0
            }),
          16
        );

  function scheduleDrain() {
    if (
      drainScheduled ||
      pendingElements.size === 0
    ) {
      return;
    }

    drainScheduled = true;
    scheduleIdle(drainQueue);
  }

  function drainQueue(deadline) {
    drainScheduled = false;

    const startedAt = performance.now();
    let processed = 0;

    for (const el of pendingElements) {
      pendingElements.delete(el);

      if (shouldInspect(el) && usesIconFont(el)) {
        el.setAttribute('data-ff-skip', '1');
      }

      processed++;

      // 每批最多处理 250 个元素或占用约 8ms
      if (
        processed >= 250 ||
        performance.now() - startedAt >= 8 ||
        (
          !deadline.didTimeout &&
          deadline.timeRemaining() < 1
        )
      ) {
        break;
      }
    }

    if (pendingElements.size > 0) {
      scheduleDrain();
    }
  }

  function enqueueTree(root) {
    if (!(root instanceof Element)) return;

    pendingElements.add(root);

    for (const el of root.querySelectorAll('*')) {
      pendingElements.add(el);
    }

    scheduleDrain();
  }

  /* -------------------- 启动与动态监听 -------------------- */

  function start() {
    injectCSS();

    if (
      SMART_ICON_RESCUE &&
      document.documentElement
    ) {
      enqueueTree(document.documentElement);
    }

    const observer = new MutationObserver(mutations => {
      // 网站替换或清空 head 时重新注入样式
      if (!styleElement?.isConnected) {
        injectCSS();
      }

      if (!SMART_ICON_RESCUE) return;

      for (const mutation of mutations) {
        for (const node of mutation.addedNodes) {
          if (node.nodeType === Node.ELEMENT_NODE) {
            enqueueTree(node);
          }
        }
      }
    });

    observer.observe(document, {
      childList: true,
      subtree: true
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener(
      'DOMContentLoaded',
      start,
      { once: true }
    );
  } else {
    start();
  }
})();