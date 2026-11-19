// Applies the theme before the first paint (no flash), and paints the
// browser's own bars (theme-color) with the page background. Kept outside
// the bundle because the Content-Security-Policy forbids inline scripts.
;(function () {
  var mode = 'system'
  try { mode = localStorage.getItem('db.theme') || 'system' } catch {}
  var dark = mode === 'dark' || (mode === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
  var meta = document.querySelector('meta[name="theme-color"]')
  if (meta) meta.setAttribute('content', dark ? '#0a0a0c' : '#f2f3f5')
})()
