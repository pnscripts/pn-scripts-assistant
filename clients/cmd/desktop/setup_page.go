package main

// setupPage is the first-run experience, served by the app itself.
//
// Embedded as a string rather than a separate asset so the desktop binary stays
// a single file with nothing to install alongside it — which is the whole point
// of a program that is supposed to set itself up.
//
// The tone matters here as much as the mechanics. Someone arriving with no
// local model and no API key needs to understand a real trade-off (free, private
// and slower versus paid, fast and better) well enough to choose, and the
// honest answer depends on hardware they may never have thought about. So the
// page states what their machine can actually do, recommends accordingly, and
// says what each choice costs.
const setupPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Pnexus — Setup</title>
<style>
:root{--bg:#070a0f;--raised:#0d1219;--input:#111823;--line:#1b2634;--text:#d6dee8;
--dim:#7d8b9c;--faint:#4a5769;--accent:#4dd0e1;--warn:#f0b26b;--danger:#e06c75;--ok:#7bc47f}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--text);font:15px/1.55 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;
-webkit-font-smoothing:antialiased}
.wrap{max-width:660px;margin:0 auto;padding:44px 28px 60px}
h1{font-size:22px;letter-spacing:.05em;margin:0 0 4px;color:var(--accent)}
.sub{color:var(--dim);font-size:13.5px;margin:0 0 30px}
h2{font-size:11px;letter-spacing:.14em;text-transform:uppercase;color:var(--faint);
margin:32px 0 12px;font-weight:600}
.card{background:var(--raised);border:1px solid var(--line);border-radius:10px;padding:15px;margin-bottom:10px}
.row{display:flex;gap:11px;align-items:flex-start}
.mark{font-family:ui-monospace,monospace;flex:none;width:15px;text-align:center;padding-top:1px}
.ok .mark{color:var(--ok)} .missing .mark{color:var(--danger)} .optional .mark{color:var(--warn)}
.name{font-weight:600;font-size:14px}
.detail{color:var(--faint);font-size:11.5px;font-family:ui-monospace,monospace;margin-left:7px}
.why{color:var(--dim);font-size:12.5px;margin-top:2px}
.consequence{color:var(--warn);font-size:12.5px;margin-top:5px}
.hint{color:var(--dim);font-size:12.5px;margin-top:5px}
.hint a{color:var(--accent)}
button{font:inherit;font-size:13px;font-weight:600;border:none;border-radius:7px;
padding:8px 15px;cursor:pointer;background:var(--accent);color:#04191c}
button:hover:not(:disabled){filter:brightness(1.13)}
button:disabled{opacity:.4;cursor:default}
button.ghost{background:transparent;border:1px solid var(--line);color:var(--dim)}
button.ghost:hover:not(:disabled){border-color:var(--accent);color:var(--accent)}
.spacer{flex:1}
.machine{font-family:ui-monospace,monospace;font-size:12px;color:var(--dim);
background:var(--input);border:1px solid var(--line);border-radius:8px;padding:11px 13px;margin-bottom:16px}
.choice{border:1px solid var(--line);border-radius:10px;padding:16px;margin-bottom:11px;background:var(--raised)}
.choice.rec{border-color:var(--accent-dim,#1f6b75)}
.choice h3{margin:0 0 3px;font-size:15px}
.tag{font-size:10px;letter-spacing:.1em;text-transform:uppercase;color:#04191c;
background:var(--accent);border-radius:9px;padding:2px 8px;margin-left:8px;vertical-align:2px}
.pros{color:var(--dim);font-size:12.5px;margin:7px 0 13px}
input[type=password],input[type=text]{width:100%;background:var(--input);border:1px solid var(--line);
border-radius:7px;color:var(--text);padding:9px 11px;font:inherit;font-size:13px;margin-bottom:9px}
pre{background:#05080c;border:1px solid var(--line);border-radius:8px;padding:12px;
font-size:11.5px;color:var(--dim);max-height:230px;overflow:auto;white-space:pre-wrap;margin:12px 0 0}
.footer{margin-top:32px;display:flex;align-items:center;gap:12px}
.status{color:var(--dim);font-size:12.5px}
.err{color:var(--danger);font-size:12.5px;margin-top:6px}
</style>
</head>
<body>
<div class="wrap">
  <h1>Pnexus</h1>
  <p class="sub">Let's get your machine ready. This takes a few minutes, and only happens once.</p>

  <div class="machine" id="machine">checking your machine…</div>

  <h2>Requirements</h2>
  <div id="reqs"></div>

  <div id="brain-section"></div>

  <pre id="log" hidden></pre>

  <div class="footer">
    <span class="status" id="status"></span>
    <span class="spacer"></span>
    <button id="continue" disabled>Continue to Pnexus</button>
  </div>
</div>

<script>
const el = id => document.getElementById(id);
let busy = false;

async function get(p){ const r = await fetch(p); return r.json(); }

function machineLine(h){
  const bits = [h.cores + " cores", h.ram_gb + "GB RAM"];
  bits.push(h.has_gpu ? "GPU: " + h.gpu : "no discrete GPU");
  return bits.join("  ·  ");
}

function renderRequirements(state){
  el("reqs").textContent = "";
  state.requirements.forEach(r => {
    const ok = r.state === "ok";
    const cls = ok ? "ok" : (r.optional ? "optional" : "missing");
    const card = document.createElement("div");
    card.className = "card " + cls;

    const row = document.createElement("div");
    row.className = "row";

    const mark = document.createElement("span");
    mark.className = "mark";
    mark.textContent = ok ? "✓" : (r.optional ? "!" : "✗");

    const body = document.createElement("div");
    body.style.flex = "1";

    const name = document.createElement("div");
    name.className = "name";
    name.textContent = r.name;
    if (r.detail){
      const d = document.createElement("span");
      d.className = "detail";
      d.textContent = r.detail;
      name.appendChild(d);
    }
    body.appendChild(name);

    const why = document.createElement("div");
    why.className = "why";
    why.textContent = r.why;
    body.appendChild(why);

    if (!ok){
      const c = document.createElement("div");
      c.className = "consequence";
      c.textContent = "Without this: " + r.consequence;
      body.appendChild(c);

      if (!r.installable && r.manual_hint){
        const h = document.createElement("div");
        h.className = "hint";
        h.textContent = r.manual_hint;
        body.appendChild(h);
      }
    }

    row.append(mark, body);

    if (!ok && r.installable){
      const b = document.createElement("button");
      b.textContent = "Install";
      b.disabled = busy;
      b.onclick = () => install(r.name);
      row.appendChild(b);
    }

    card.appendChild(row);
    el("reqs").appendChild(card);
  });
}

function renderBrainChoice(state){
  const box = el("brain-section");
  box.textContent = "";

  const hasModel = state.requirements.some(r => r.name === "Chat model" && r.state === "ok");
  if (hasModel || state.has_api_key) return;

  const h = document.createElement("h2");
  h.textContent = "Choose a brain";
  box.appendChild(h);

  const note = document.createElement("p");
  note.className = "sub";
  note.style.marginBottom = "14px";
  note.textContent = "Pnexus needs a language model to think with. You can run one on this "
    + "machine for free, or use a paid API. You can change this later, or use both.";
  box.appendChild(note);

  // Local
  const local = document.createElement("div");
  local.className = "choice" + (state.hardware.can_local ? " rec" : "");
  const lh = document.createElement("h3");
  lh.textContent = "Run it locally";
  if (state.hardware.can_local){
    const t = document.createElement("span");
    t.className = "tag";
    t.textContent = "recommended";
    lh.appendChild(t);
  }
  local.appendChild(lh);

  const lp = document.createElement("p");
  lp.className = "pros";
  lp.textContent = state.hardware.can_local
    ? "Free, private, works offline. Suggested for your machine: "
      + state.recommended_model.model + " (" + state.recommended_model.size + ") — "
      + state.recommended_model.speed + "."
    : "Your machine has " + state.hardware.ram_gb + "GB of RAM, which is really too "
      + "little to run a model well. It would work, but slowly enough to be frustrating.";
  local.appendChild(lp);

  const lb = document.createElement("button");
  lb.textContent = "Download " + state.recommended_model.model;
  lb.disabled = busy;
  lb.onclick = pullModel;
  local.appendChild(lb);
  box.appendChild(local);

  // API
  const api = document.createElement("div");
  api.className = "choice" + (state.hardware.can_local ? "" : " rec");
  const ah = document.createElement("h3");
  ah.textContent = "Use the Anthropic API";
  if (!state.hardware.can_local){
    const t = document.createElement("span");
    t.className = "tag";
    t.textContent = "recommended";
    ah.appendChild(t);
  }
  api.appendChild(ah);

  const ap = document.createElement("p");
  ap.className = "pros";
  ap.textContent = "Much stronger answers and fast on any machine, but it costs per use "
    + "and your messages go to Anthropic. Get a key at console.anthropic.com — it is stored "
    + "only on this computer.";
  api.appendChild(ap);

  const input = document.createElement("input");
  input.type = "password";
  input.placeholder = "sk-ant-…";
  api.appendChild(input);

  const err = document.createElement("div");
  err.className = "err";
  err.hidden = true;
  api.appendChild(err);

  const ab = document.createElement("button");
  ab.className = "ghost";
  ab.textContent = "Save key";
  ab.onclick = async () => {
    err.hidden = true;
    const res = await fetch("/api-key", {
      method: "POST",
      headers: {"Content-Type":"application/json"},
      body: JSON.stringify({key: input.value})
    }).then(r => r.json());
    if (!res.ok){ err.textContent = res.error; err.hidden = false; return; }
    input.value = "";
    refresh();
  };
  api.appendChild(ab);
  box.appendChild(api);
}

async function install(name){
  busy = true;
  el("log").hidden = false;
  await fetch("/install?name=" + encodeURIComponent(name));
  refresh();
}

async function pullModel(){
  busy = true;
  el("log").hidden = false;
  await fetch("/choose-model");
  refresh();
}

async function refresh(){
  const state = await get("/state");
  busy = state.busy;

  el("machine").textContent = machineLine(state.hardware);
  renderRequirements(state);
  renderBrainChoice(state);

  if (state.log){
    el("log").hidden = false;
    el("log").textContent = state.log;
    el("log").scrollTop = el("log").scrollHeight;
  }

  const hasBrain = state.requirements.some(r => r.name === "Chat model" && r.state === "ok")
    || state.has_api_key;
  const ready = state.blocking === 0 && hasBrain && !busy;

  el("continue").disabled = !ready;
  el("status").textContent = busy
    ? "Working…"
    : (ready ? "Everything is ready." : (state.blocking + " requirement(s) still needed"));
}

el("continue").onclick = async () => {
  el("continue").disabled = true;
  el("status").textContent = "Starting Pnexus…";
  await fetch("/done");
};

refresh();
setInterval(refresh, 2000);
</script>
</body>
</html>`
