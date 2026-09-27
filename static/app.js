function copyPermalink(btn) {
  var permalink = btn.dataset.permalink;
  navigator.clipboard.writeText(location.origin + permalink).then(function() {
    var orig = btn.textContent;
    btn.textContent = '✓';
    setTimeout(function() { btn.textContent = orig; }, 1500);
  });
}

async function toggleLike(btn) {
  var postId = btn.dataset.postId;
  var resp;
  try {
    resp = await fetch('/posts/' + postId + '/like', { method: 'POST' });
  } catch (_) {
    return;
  }

  var data;
  try {
    data = await resp.json();
  } catch (_) {
    return;
  }

  if (resp.status === 429) {
    var orig = btn.textContent;
    btn.textContent = '制限中…';
    setTimeout(function() { btn.textContent = orig; }, 1800);
    return;
  }

  if (data.liked) {
    btn.innerHTML = '♥ ' + data.count;
    btn.classList.add('liked');
    btn.dataset.liked = 'true';
  }
}

// X（Twitter）の埋め込みがあるページでだけ公式ウィジェットを読み込む。
document.addEventListener('DOMContentLoaded', function() {
  if (!document.querySelector('.link-embed-x .twitter-tweet')) return;
  var s = document.createElement('script');
  s.src = 'https://platform.twitter.com/widgets.js';
  s.async = true;
  s.charset = 'utf-8';
  document.body.appendChild(s);
});
