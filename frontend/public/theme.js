// Sets the theme before first paint: the saved choice, else the system preference.
// A file, not an inline script: the server's Content-Security-Policy is script-src 'self'.
// The key is shared with src/shared/lib/theme.ts.
(function () {
  var theme = 'dark';
  try {
    var saved = localStorage.getItem('protolab-theme');
    if (saved === 'light' || saved === 'dark') theme = saved;
    else if (window.matchMedia('(prefers-color-scheme: light)').matches) theme = 'light';
  } catch (e) {}
  document.documentElement.setAttribute('data-theme', theme);
})();
