function copyPermalink(btn) {
  var permalink = btn.dataset.permalink;
  navigator.clipboard.writeText(location.origin + permalink).then(function() {
    var orig = btn.textContent;
    btn.textContent = '✓';
    setTimeout(function() { btn.textContent = orig; }, 1500);
  });
}
