(function () {
  "use strict";

  const logEl = document.getElementById("log");

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
        const label = "[" + evt.status + "] " + evt.path;
        const extra = evt.status === "done" ? " -> " + evt.caption : evt.status === "error" ? ": " + evt.error : "";
        logLine(evt.status, label + extra);
        if (evt.status === "complete") {
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
