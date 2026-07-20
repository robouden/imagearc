(function () {
  "use strict";

  const logEl = document.getElementById("log");

  // --- Progress ---
  let progressTotal = 0;
  let progressDone = 0;
  const progressEl = document.getElementById("progress");
  function resetProgress() {
    progressTotal = 0;
    progressDone = 0;
    progressEl.hidden = false;
    renderProgress();
  }
  function renderProgress() {
    const pct = progressTotal > 0 ? Math.round((progressDone / progressTotal) * 100) : 0;
    document.getElementById("progress-bar").style.width = pct + "%";
    document.getElementById("progress-label").textContent = progressDone + " / " + progressTotal;
    document.getElementById("progress-pct").textContent = pct + "%";
  }

  function logLine(cls, text) {
    const div = document.createElement("div");
    div.className = cls;
    div.textContent = text;
    logEl.appendChild(div);
    logEl.scrollTop = logEl.scrollHeight;
  }

  // --- Batch caption ---
  let stream = null;

  document.getElementById("start").addEventListener("click", async () => {
    const body = {
      folder: document.getElementById("folder").value,
      provider: document.getElementById("provider").value,
      model: document.getElementById("model").value,
      recurse: document.getElementById("recurse").checked,
      dryRun: document.getElementById("dryrun").checked,
    };
    if (!body.folder) {
      logLine("error", "enter a folder path first");
      return;
    }
    logEl.textContent = "";
    logLine("started", "starting batch...");
    resetProgress();

    try {
      const res = await fetch("/api/caption", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!res.ok) {
        logLine("error", "failed to start: " + (await res.text()));
        return;
      }
    } catch (e) {
      logLine("error", "failed to start: " + e);
      return;
    }

    if (stream) stream.close();
    stream = new EventSource("/api/stream");
    stream.onmessage = (ev) => {
      try {
        const evt = JSON.parse(ev.data);
        if (evt.status === "start") {
          progressTotal = evt.total || 0;
          progressDone = 0;
          renderProgress();
          return;
        }
        const label = "[" + evt.status + "] " + evt.path;
        const extra = evt.status === "done" ? " -> " + evt.caption : evt.status === "error" ? ": " + evt.error : "";
        logLine(evt.status, label + extra);
        if (evt.status === "done" || evt.status === "error" || evt.status === "skipped") {
          progressDone++;
          renderProgress();
        }
        if (evt.status === "complete") {
          if (progressTotal === 0) progressTotal = progressDone;
          progressDone = progressTotal;
          renderProgress();
          logLine("done", "batch complete");
          stream.close();
        }
      } catch (e) {
        // ignore malformed frames
      }
    };
    stream.onerror = () => {
      // connection closed by server when batch is done, or dropped; nothing to retry.
    };
  });

  // --- Metadata editor ---
  document.getElementById("meta-load").addEventListener("click", async () => {
    const path = document.getElementById("meta-path").value;
    const statusEl = document.getElementById("meta-status");
    if (!path) return;
    statusEl.textContent = "loading...";
    try {
      const res = await fetch("/api/metadata?path=" + encodeURIComponent(path));
      if (!res.ok) {
        statusEl.textContent = "error: " + (await res.text());
        return;
      }
      const m = await res.json();
      document.getElementById("meta-caption").value = m.caption || "";
      document.getElementById("meta-keywords").value = (m.keywords || []).join(", ");
      document.getElementById("meta-byline").value = m.byline || "";
      document.getElementById("meta-location").value = m.location || "";
      statusEl.textContent = "loaded";
    } catch (e) {
      statusEl.textContent = "error: " + e;
    }
  });

  document.getElementById("meta-save").addEventListener("click", async () => {
    const statusEl = document.getElementById("meta-status");
    const path = document.getElementById("meta-path").value;
    if (!path) return;
    const body = {
      path: path,
      caption: document.getElementById("meta-caption").value,
      keywords: document
        .getElementById("meta-keywords")
        .value.split(",")
        .map((s) => s.trim())
        .filter(Boolean),
      byline: document.getElementById("meta-byline").value,
      location: document.getElementById("meta-location").value,
    };
    statusEl.textContent = "saving...";
    try {
      const res = await fetch("/api/metadata", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      statusEl.textContent = res.ok ? "saved" : "error: " + (await res.text());
    } catch (e) {
      statusEl.textContent = "error: " + e;
    }
  });

  // --- Model dropdown (populate from provider) ---
  const modelSel = document.getElementById("model");
  const providerSel = document.getElementById("provider");
  function setModelOptions(models, note) {
    modelSel.innerHTML = "";
    if (!models.length) {
      const opt = document.createElement("option");
      opt.value = "";
      opt.textContent = note || "no models found";
      opt.disabled = true;
      modelSel.appendChild(opt);
      return;
    }
    models.forEach((m) => {
      const opt = document.createElement("option");
      opt.value = m;
      opt.textContent = m;
      modelSel.appendChild(opt);
    });
  }
  async function loadModels() {
    setModelOptions([], "loading…");
    try {
      const res = await fetch("/api/models?provider=" + encodeURIComponent(providerSel.value));
      if (!res.ok) {
        setModelOptions([], (await res.text()) || "unavailable");
        return;
      }
      const data = await res.json();
      setModelOptions(data.models || [], "no models found");
    } catch (e) {
      setModelOptions([], "error loading models");
    }
  }
  providerSel.addEventListener("change", loadModels);
  loadModels();

  // --- Folder picker ---
  const picker = document.getElementById("picker");
  const pickerList = document.getElementById("picker-list");
  const pickerPath = document.getElementById("picker-path");
  let pickerTarget = null; // input id to fill
  let pickerCur = "";      // current directory

  async function pickerOpen(targetId) {
    pickerTarget = targetId;
    const seed = document.getElementById(targetId).value || "";
    picker.hidden = false;
    await pickerLoad(seed);
  }

  async function pickerLoad(path) {
    try {
      const res = await fetch("/api/browse?path=" + encodeURIComponent(path || ""));
      if (!res.ok) { pickerPath.textContent = "cannot open: " + (await res.text()); return; }
      const data = await res.json();
      pickerCur = data.path;
      pickerPath.textContent = data.path;
      pickerList.innerHTML = "";
      if (!data.dirs.length) {
        const li = document.createElement("li");
        li.className = "empty";
        li.textContent = "no subfolders";
        pickerList.appendChild(li);
      }
      data.dirs.forEach((name) => {
        const li = document.createElement("li");
        li.textContent = "📁 " + name;
        li.addEventListener("click", () => pickerLoad(data.path.replace(/\/$/, "") + "/" + name));
        pickerList.appendChild(li);
      });
      document.getElementById("picker-up").dataset.parent = data.parent || "";
    } catch (e) {
      pickerPath.textContent = "error: " + e;
    }
  }

  document.querySelectorAll("[data-browse]").forEach((b) =>
    b.addEventListener("click", () => pickerOpen(b.dataset.browse)));
  document.getElementById("picker-up").addEventListener("click", (e) => {
    const parent = e.currentTarget.dataset.parent;
    if (parent) pickerLoad(parent);
  });
  document.getElementById("picker-close").addEventListener("click", () => (picker.hidden = true));
  picker.addEventListener("click", (e) => { if (e.target === picker) picker.hidden = true; });
  document.getElementById("picker-pick").addEventListener("click", () => {
    if (pickerTarget) document.getElementById(pickerTarget).value = pickerCur;
    picker.hidden = true;
  });

  // --- Library ---
  function activateTab(id) {
    document.querySelectorAll(".tab").forEach((x) => x.classList.toggle("is-active", x.dataset.tab === id));
    document.querySelectorAll(".panel").forEach((p) => p.classList.toggle("is-active", p.dataset.panel === id));
  }

  const grid = document.getElementById("lib-grid");
  const libCount = document.getElementById("lib-count");

  async function libSearch() {
    const q = document.getElementById("lib-q").value.trim();
    const kw = document.getElementById("lib-keyword").value.trim();
    libCount.textContent = "searching…";
    try {
      const params = new URLSearchParams();
      if (q) params.set("q", q);
      if (kw) params.set("keyword", kw);
      params.set("limit", "200");
      const res = await fetch("/api/search?" + params.toString());
      if (!res.ok) { libCount.textContent = "error: " + (await res.text()); return; }
      const data = await res.json();
      renderGallery(data.photos || []);
      libCount.textContent = data.total + " photo" + (data.total === 1 ? "" : "s") +
        (data.total > (data.photos || []).length ? " (showing first " + data.photos.length + ")" : "");
    } catch (e) {
      libCount.textContent = "error: " + e;
    }
  }

  function renderGallery(photos) {
    grid.innerHTML = "";
    if (!photos.length) {
      grid.innerHTML = '<p class="empty-note">No photos. Index a folder, or run a caption batch first.</p>';
      return;
    }
    photos.forEach((p) => {
      const cell = document.createElement("figure");
      cell.className = "cell";
      const img = document.createElement("img");
      img.loading = "lazy";
      img.src = "/api/thumb?path=" + encodeURIComponent(p.path);
      img.alt = p.filename;
      const cap = document.createElement("figcaption");
      cap.textContent = p.caption || p.filename;
      cell.append(img, cap);
      cell.addEventListener("click", () => openLightbox(p));
      grid.appendChild(cell);
    });
  }

  // --- Lightbox ---
  const lightbox = document.getElementById("lightbox");
  function openLightbox(p) {
    document.getElementById("lightbox-img").src = "/api/image?path=" + encodeURIComponent(p.path);
    document.getElementById("lightbox-caption").textContent = p.caption || p.filename;
    document.getElementById("lightbox-keywords").textContent = p.keywords || "";
    const meta = document.getElementById("lightbox-meta");
    meta.innerHTML = "";
    const bits = [];
    if (p.date) bits.push(p.date);
    if (p.lat != null && p.lon != null) bits.push(p.lat.toFixed(5) + ", " + p.lon.toFixed(5));
    meta.textContent = bits.join("  ·  ");
    if (p.lat != null && p.lon != null) {
      const a = document.createElement("a");
      a.href = "https://www.openstreetmap.org/?mlat=" + p.lat + "&mlon=" + p.lon + "#map=15/" + p.lat + "/" + p.lon;
      a.target = "_blank";
      a.rel = "noopener";
      a.className = "map-link";
      a.textContent = " ↗ map";
      meta.appendChild(a);
    }
    document.getElementById("lightbox-edit").onclick = () => {
      document.getElementById("meta-path").value = p.path;
      lightbox.hidden = true;
      activateTab("metadata");
      document.getElementById("meta-load").click();
    };
    lightbox.hidden = false;
  }
  document.getElementById("lightbox-close").addEventListener("click", () => (lightbox.hidden = true));
  lightbox.addEventListener("click", (e) => { if (e.target === lightbox) lightbox.hidden = true; });

  let libTimer = null;
  function libSearchDebounced() {
    clearTimeout(libTimer);
    libTimer = setTimeout(libSearch, 250);
  }
  // Map view
  let map = null, geoLayer = null, geoLoaded = false;
  function showView(which) {
    const grid = which === "grid";
    document.getElementById("lib-grid").hidden = !grid;
    document.getElementById("lib-map").hidden = grid;
    document.getElementById("view-grid").classList.toggle("is-active", grid);
    document.getElementById("view-map").classList.toggle("is-active", !grid);
    if (!grid) initMap();
  }
  function initMap() {
    if (typeof L === "undefined") { libCount.textContent = "map library unavailable (no internet?)"; return; }
    if (!map) {
      map = L.map("lib-map").setView([20, 0], 2);
      L.tileLayer("https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png", {
        attribution: "&copy; OpenStreetMap", maxZoom: 19,
      }).addTo(map);
      geoLayer = L.layerGroup().addTo(map);
    }
    setTimeout(() => map.invalidateSize(), 60);
    if (!geoLoaded) { geoLoaded = true; loadGeo(); }
  }
  async function loadGeo() {
    if (!map) return;
    try {
      const res = await fetch("/api/geo");
      if (!res.ok) return;
      const data = await res.json();
      geoLayer.clearLayers();
      const pts = [];
      (data.photos || []).forEach((p) => {
        if (p.lat == null || p.lon == null) return;
        const m = L.marker([p.lat, p.lon]).addTo(geoLayer);
        const pop = document.createElement("div");
        pop.className = "map-pop";
        const img = document.createElement("img");
        img.src = "/api/thumb?path=" + encodeURIComponent(p.path);
        const cap = document.createElement("div");
        cap.textContent = p.caption || p.filename;
        pop.append(img, cap);
        pop.addEventListener("click", () => openLightbox(p));
        m.bindPopup(pop);
        pts.push([p.lat, p.lon]);
      });
      if (pts.length) map.fitBounds(pts, { padding: [30, 30], maxZoom: 14 });
      libCount.textContent = pts.length + " geotagged photo" + (pts.length === 1 ? "" : "s");
    } catch (e) {}
  }
  document.getElementById("view-grid").addEventListener("click", () => showView("grid"));
  document.getElementById("view-map").addEventListener("click", () => showView("map"));

  document.getElementById("lib-search").addEventListener("click", libSearch);
  document.getElementById("lib-q").addEventListener("input", libSearchDebounced);
  document.getElementById("lib-keyword").addEventListener("input", libSearchDebounced);
  document.getElementById("lib-q").addEventListener("keydown", (e) => { if (e.key === "Enter") { clearTimeout(libTimer); libSearch(); } });
  document.getElementById("lib-keyword").addEventListener("keydown", (e) => { if (e.key === "Enter") { clearTimeout(libTimer); libSearch(); } });

  // Index folder into the library (streams progress)
  document.getElementById("lib-index-btn").addEventListener("click", async () => {
    const folder = document.getElementById("lib-folder").value;
    if (!folder) return;
    const bar = document.getElementById("lib-progress");
    const barFill = document.getElementById("lib-progress-bar");
    const barLabel = document.getElementById("lib-progress-label");
    const barPct = document.getElementById("lib-progress-pct");
    let total = 0, done = 0;
    const paint = () => {
      const pct = total ? Math.round((done / total) * 100) : 0;
      barFill.style.width = pct + "%";
      barLabel.textContent = done + " / " + total;
      barPct.textContent = pct + "%";
    };
    bar.hidden = false; paint();
    try {
      const res = await fetch("/api/index", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ folder: folder, recurse: document.getElementById("lib-recurse").checked }),
      });
      if (!res.ok) { libCount.textContent = "index error: " + (await res.text()); return; }
    } catch (e) { libCount.textContent = "index error: " + e; return; }
    const es = new EventSource("/api/stream");
    es.onmessage = (ev) => {
      try {
        const evt = JSON.parse(ev.data);
        if (evt.status === "start") { total = evt.total || 0; done = 0; paint(); return; }
        if (evt.status === "done" || evt.status === "error") { done++; paint(); }
        if (evt.status === "complete") { done = total; paint(); es.close(); libSearch(); if (map) loadGeo(); }
      } catch (e) {}
    };
  });

  // Refresh: re-scan all indexed folders
  document.getElementById("lib-refresh").addEventListener("click", async () => {
    const bar = document.getElementById("lib-progress");
    const barFill = document.getElementById("lib-progress-bar");
    const barLabel = document.getElementById("lib-progress-label");
    const barPct = document.getElementById("lib-progress-pct");
    let total = 0, done = 0;
    const paint = () => {
      const pct = total ? Math.round((done / total) * 100) : 0;
      barFill.style.width = pct + "%";
      barLabel.textContent = done + " / " + total;
      barPct.textContent = pct + "%";
    };
    bar.hidden = false; paint();
    try {
      const res = await fetch("/api/refresh", { method: "POST" });
      if (!res.ok) { libCount.textContent = "refresh error: " + (await res.text()); return; }
    } catch (e) { libCount.textContent = "refresh error: " + e; return; }
    const es = new EventSource("/api/stream");
    es.onmessage = (ev) => {
      try {
        const evt = JSON.parse(ev.data);
        if (evt.status === "start") { total = evt.total || 0; done = 0; paint(); return; }
        if (evt.status === "done" || evt.status === "error" || evt.status === "skipped") { done++; paint(); }
        if (evt.status === "complete") { es.close(); libSearch(); if (map) loadGeo(); }
      } catch (e) {}
    };
  });

  // --- Dashboard ---
  function bars(el, items) {
    el.innerHTML = "";
    const max = items.reduce((m, x) => Math.max(m, x.count), 0) || 1;
    items.forEach((x) => {
      const li = document.createElement("li");
      const label = document.createElement("span");
      label.className = "bar-label";
      label.textContent = x.label;
      const track = document.createElement("span");
      track.className = "bar-track";
      const fill = document.createElement("span");
      fill.className = "bar-fill";
      fill.style.width = Math.round((x.count / max) * 100) + "%";
      track.appendChild(fill);
      const n = document.createElement("span");
      n.className = "bar-count";
      n.textContent = x.count;
      li.append(label, track, n);
      el.appendChild(li);
    });
    if (!items.length) el.innerHTML = '<li class="empty-note">—</li>';
  }
  async function loadDashboard() {
    try {
      const res = await fetch("/api/stats");
      if (!res.ok) return;
      const s = await res.json();
      document.getElementById("dash-total").textContent = s.total;
      document.getElementById("dash-captioned").textContent = s.captioned;
      document.getElementById("dash-geotagged").textContent = s.geotagged || 0;
      bars(document.getElementById("dash-keywords"), s.topKeywords || []);
      bars(document.getElementById("dash-years"), s.years || []);
      bars(document.getElementById("dash-locations"), s.locations || []);
      bars(document.getElementById("dash-bylines"), s.bylines || []);
    } catch (e) {}
  }
  document.getElementById("dash-refresh").addEventListener("click", loadDashboard);

  // Lazy-load per tab
  let libLoaded = false, dashLoaded = false;
  document.querySelectorAll(".tab").forEach((t) => {
    t.addEventListener("click", () => {
      if (t.dataset.tab === "library" && !libLoaded) { libLoaded = true; libSearch(); }
      if (t.dataset.tab === "dashboard" && !dashLoaded) { dashLoaded = true; loadDashboard(); }
    });
  });

  // --- Catalog ---
  document.getElementById("cat-build").addEventListener("click", async () => {
    const statusEl = document.getElementById("cat-status");
    const body = {
      folder: document.getElementById("cat-folder").value,
      output: document.getElementById("cat-output").value || "catalog.csv",
    };
    if (!body.folder) return;
    statusEl.textContent = "building...";
    try {
      const res = await fetch("/api/catalog", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!res.ok) {
        statusEl.textContent = "error: " + (await res.text());
        return;
      }
      const data = await res.json();
      statusEl.textContent = "wrote " + data.rows + " rows to " + data.output;
    } catch (e) {
      statusEl.textContent = "error: " + e;
    }
  });
})();
