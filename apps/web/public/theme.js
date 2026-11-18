// Applies the theme before the first paint (no flash). Kept outside the
// bundle because the Content-Security-Policy forbids inline scripts.
;(function () {
  var mode = 'system'
  try { mode = localStorage.getItem('db.theme') || 'system' } catch {}
  var dark = mode === 'dark' || (mode === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
})()
