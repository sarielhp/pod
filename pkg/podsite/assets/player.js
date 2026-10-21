(function () {
  var feedURL = new URL("feed.xml", location.href).href;
  var subscribe = document.getElementById("subscribe-app");
  if (subscribe) subscribe.href = "intent://" + feedURL + "#Intent;scheme=antennapod-subscribe;package=de.danoeh.antennapod;end";
  var copy = document.getElementById("copy-feed");
  if (copy) copy.addEventListener("click", function () {
    var done = function () { copy.textContent = "Copied"; };
    if (navigator.clipboard && window.isSecureContext) navigator.clipboard.writeText(feedURL).then(done);
    else window.prompt("Feed URL", feedURL);
  });
  var cards = Array.prototype.slice.call(document.querySelectorAll(".episode-card[data-src]"));
  var bar = document.getElementById("player");
  if (!bar || !cards.length) return;
  var audio = new Audio();
  audio.preload = "none";
  var ui = {
    title: bar.querySelector(".p-title"),
    toggle: bar.querySelector(".p-toggle"),
    back: bar.querySelector(".p-back"),
    fwd: bar.querySelector(".p-fwd"),
    seek: bar.querySelector(".p-seek"),
    cur: bar.querySelector(".p-cur"),
    dur: bar.querySelector(".p-dur"),
    speed: bar.querySelector(".p-speed")
  };
  var current = null;
  var objectURL = null;
  var lastSaved = 0;
  var speeds = [1, 1.25, 1.5, 1.75, 2, 0.75];
  var speedIndex = 0;

  function store(key, value) {
    try {
      if (value === null) localStorage.removeItem(key);
      else localStorage.setItem(key, String(value));
    } catch (e) {}
  }

  function load(key) {
    try { return localStorage.getItem(key); } catch (e) { return null; }
  }

  function clock(sec) {
    if (!isFinite(sec)) return "0:00";
    sec = Math.floor(sec);
    var h = Math.floor(sec / 3600);
    var m = Math.floor((sec % 3600) / 60);
    var s = sec % 60;
    var tail = (m < 10 && h ? "0" : "") + m + ":" + (s < 10 ? "0" : "") + s;
    return h ? h + ":" + tail : tail;
  }

  var dbPromise = null;
  function db() {
    if (!dbPromise) {
      dbPromise = new Promise(function (resolve, reject) {
        if (!window.indexedDB) return reject(new Error("no indexedDB"));
        var req = indexedDB.open("pod-offline", 1);
        req.onupgradeneeded = function () { req.result.createObjectStore("episodes"); };
        req.onsuccess = function () { resolve(req.result); };
        req.onerror = function () { reject(req.error); };
      });
    }
    return dbPromise;
  }

  function tx(mode, fn) {
    return db().then(function (d) {
      return new Promise(function (resolve, reject) {
        var req = fn(d.transaction("episodes", mode).objectStore("episodes"));
        req.onsuccess = function () { resolve(req.result); };
        req.onerror = function () { reject(req.error); };
      });
    });
  }

  function absolute(card) { return new URL(card.dataset.src, location.href).href; }

  function setPlayIcons() {
    cards.forEach(function (c) {
      var playing = c === current && !audio.paused;
      c.classList.toggle("playing", c === current);
      c.querySelector(".ep-play").textContent = playing ? "Pause" : "Play";
    });
    ui.toggle.textContent = audio.paused ? "Play" : "Pause";
  }

  function releaseObjectURL() {
    if (objectURL) URL.revokeObjectURL(objectURL);
    objectURL = null;
  }

  function select(card) {
    var url = absolute(card);
    current = card;
    ui.title.textContent = card.dataset.title;
    bar.hidden = false;
    document.body.classList.add("has-player");
    releaseObjectURL();
    audio.playbackRate = speeds[speedIndex];
    var saved = parseFloat(load("pod:pos:" + url));
    if (saved > 0) audio.addEventListener("loadedmetadata", function once() {
      audio.removeEventListener("loadedmetadata", once);
      audio.currentTime = saved;
    });
    setMediaSession(card);

    var btn = card.querySelector(".ep-offline");
    if (btn && btn.dataset.saved) {
      return tx("readonly", function (s) { return s.get(url); })
        .catch(function () { return null; })
        .then(function (blob) {
          if (current !== card) return;
          if (blob) objectURL = URL.createObjectURL(blob);
          audio.src = objectURL || url;
        });
    }
    audio.src = url;
    return Promise.resolve();
  }

  function setMediaSession(card) {
    if (!("mediaSession" in navigator)) return;
    var cover = document.querySelector(".cover-img");
    navigator.mediaSession.metadata = new MediaMetadata({
      title: card.dataset.title,
      artist: document.title,
      artwork: cover ? [{ src: cover.src }] : []
    });
  }

  function playAudio() {
    var p = audio.play();
    if (p && p.catch) {
      p.catch(function (err) {
        console.error("Audio playback error:", err);
        setPlayIcons();
      });
    }
  }

  function togglePlay(card) {
    if (card !== current) {
      select(card).then(function () { playAudio(); });
      return;
    }
    if (audio.paused) playAudio(); else audio.pause();
  }

  function skip(delta) {
    audio.currentTime = Math.max(0, Math.min(audio.duration || Infinity, audio.currentTime + delta));
  }

  audio.addEventListener("play", setPlayIcons);
  audio.addEventListener("pause", setPlayIcons);
  audio.addEventListener("error", function () {
    console.error("Audio error:", audio.error ? audio.error.message || audio.error.code : "unknown");
    setPlayIcons();
  });
  audio.addEventListener("loadedmetadata", function () { ui.seek.max = Math.floor(audio.duration); ui.dur.textContent = clock(audio.duration); });
  audio.addEventListener("timeupdate", function () {
    ui.seek.value = Math.floor(audio.currentTime);
    ui.cur.textContent = clock(audio.currentTime);
    var now = Date.now();
    if (current && now - lastSaved > 3000) {
      lastSaved = now;
      store("pod:pos:" + absolute(current), Math.floor(audio.currentTime));
    }
  });
  audio.addEventListener("ended", function () {
    if (current) store("pod:pos:" + absolute(current), null);
    setPlayIcons();
  });

  ui.toggle.addEventListener("click", function () { if (current) togglePlay(current); });
  ui.back.addEventListener("click", function () { skip(-15); });
  ui.fwd.addEventListener("click", function () { skip(30); });
  ui.seek.addEventListener("input", function () { audio.currentTime = Number(ui.seek.value); });
  ui.speed.addEventListener("click", function () {
    speedIndex = (speedIndex + 1) % speeds.length;
    audio.playbackRate = speeds[speedIndex];
    ui.speed.textContent = speeds[speedIndex] + "x";
  });

  if ("mediaSession" in navigator) {
    navigator.mediaSession.setActionHandler("play", function () { audio.play(); });
    navigator.mediaSession.setActionHandler("pause", function () { audio.pause(); });
    navigator.mediaSession.setActionHandler("seekbackward", function () { skip(-15); });
    navigator.mediaSession.setActionHandler("seekforward", function () { skip(30); });
  }

  function fetchWithProgress(url, onProgress) {
    return fetch(url).then(function (resp) {
      if (!resp.ok) throw new Error("HTTP " + resp.status);
      var total = Number(resp.headers.get("Content-Length")) || 0;
      var type = resp.headers.get("Content-Type") || "audio/mpeg";
      if (!resp.body || !resp.body.getReader) return resp.blob();
      var reader = resp.body.getReader();
      var chunks = [];
      var got = 0;
      function pump() {
        return reader.read().then(function (r) {
          if (r.done) return new Blob(chunks, { type: type });
          chunks.push(r.value);
          got += r.value.length;
          if (total) onProgress(Math.round((got / total) * 100));
          return pump();
        });
      }
      return pump();
    });
  }

  function wireOffline(card) {
    var btn = card.querySelector(".ep-offline");
    var url = absolute(card);
    var busy = false;

    function show(saved) {
      btn.textContent = saved ? "Saved offline ✓ (remove)" : "Save offline";
      btn.dataset.saved = saved ? "1" : "";
    }

    tx("readonly", function (s) { return s.getKey(url); })
      .then(function (key) { show(key !== undefined); })
      .catch(function () { btn.disabled = true; btn.textContent = "Offline unavailable"; });

    btn.addEventListener("click", function () {
      if (busy) return;
      if (btn.dataset.saved) {
        tx("readwrite", function (s) { return s.delete(url); }).then(function () { show(false); });
        return;
      }
      busy = true;
      btn.textContent = "Saving 0%";
      fetchWithProgress(url, function (pct) { btn.textContent = "Saving " + pct + "%"; })
        .then(function (blob) { return tx("readwrite", function (s) { return s.put(blob, url); }); })
        .then(function () { show(true); })
        .catch(function () { btn.textContent = "Failed — retry"; })
        .then(function () { busy = false; });
    });
  }

  cards.forEach(function (card) {
    card.querySelector(".ep-play").addEventListener("click", function () { togglePlay(card); });
    wireOffline(card);
  });
})();
