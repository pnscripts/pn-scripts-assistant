package setup

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
<title>PN Brain — Setup</title>
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
/* One button per model, stacked, each naming what it is and what it costs in
   time. A row of three would put the names first and the difference last,
   which is the wrong way round: the difference is what is being chosen. */
.models{display:flex;flex-direction:column;gap:7px;margin-top:4px}
.models button{display:flex;flex-direction:column;align-items:flex-start;gap:2px;
  text-align:left;padding:9px 12px;width:100%}
.models button span{font-weight:400;font-size:11.5px;opacity:.72}

/* The service picker sits above its key field and explains where to get one. */
select{font:inherit;font-size:13px;padding:7px 9px;border-radius:7px;
  background:var(--card);color:var(--fg);border:1px solid var(--line);
  width:100%;margin-bottom:6px}
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
.ask{background:var(--raised);border:1px solid var(--line);border-radius:10px;padding:16px;margin-bottom:26px}
.ask-note{color:var(--dim);font-size:12px;margin:0 0 12px;line-height:1.45}
#ask-form{display:flex;gap:8px}
#ask-form input{flex:1;margin-bottom:0}
#ask-log{margin-bottom:11px}
.qa{margin-bottom:13px}
.qa .q{font-size:12.5px;color:var(--faint);margin:0 0 4px}
.qa .a{font-size:13px;white-space:pre-wrap;margin:0;line-height:1.5}
#ask-suggestions{display:flex;flex-wrap:wrap;gap:6px;margin-bottom:11px}
.chip{background:var(--input);border:1px solid var(--line);color:var(--dim);border-radius:14px;
padding:4px 11px;font-size:11.5px;cursor:pointer}
.chip:hover{border-color:var(--accent);color:var(--accent)}
</style>
</head>
<body>
<div class="wrap">
  <h1>PN Brain</h1>
  <p class="sub">Let's get your machine ready. This takes a few minutes, and only happens once.</p>

  <div class="machine" id="machine">checking your machine…</div>

  <div class="ask">
    <h2 style="margin-top:0">Questions?</h2>
    <p class="ask-note">While setup runs, you can ask about it here. This is a short list of
      written answers, not the assistant — that arrives when setup finishes.</p>
    <div id="ask-log"></div>
    <div id="ask-suggestions"></div>
    <form id="ask-form">
      <input type="text" id="ask-input" placeholder="Ask about setup…" autocomplete="off">
      <button type="submit">Ask</button>
    </form>
  </div>

  <h2>Requirements</h2>
  <div id="reqs"></div>

  <div id="brain-section"></div>

  <pre id="log" hidden></pre>

  <div class="footer">
    <span class="status" id="status"></span>
    <span class="spacer"></span>
    <button id="continue" disabled>Continue to PN Brain</button>
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

    /*
     * Install what is missing, and update what is not.
     *
     * The button used to appear only for things that were absent, so anything
     * already installed could never be changed from here: an Ollama from a
     * year ago reported "ok" for ever, and the only way to move it on was to
     * find where it lived and delete it by hand. That is precisely the
     * knowledge this page exists to spare somebody.
     *
     * The same action either way — it fetches the current release and puts it
     * in place — so the only difference is the word, and the word matters:
     * "Install" beside a tick would read as though something were wrong.
     */
    if (r.installable){
      const b = document.createElement("button");
      b.textContent = ok ? "Update" : "Install";
      // ghost, which this page already uses for the quieter action: an
      // update is not urgent, and a button identical to the one beside a
      // missing part would say that it was.
      b.className = ok ? "ghost" : "";
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
  note.textContent = "PN Brain needs a language model to think with. You can run one on this "
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

  /*
   * One button per model, rather than one button.
   *
   * The single suggestion was the right answer for somebody with no way to
   * judge between eight names, but it made a trade on their behalf — a larger
   * model that answers well and slowly — and that is exactly the trade people
   * differ on. Somebody who would rather wait two seconds than get the better
   * answer could not see that the option existed.
   *
   * The recommendation keeps its place in the order rather than being lifted
   * to the top: on a machine where the sensible default is the middle one,
   * showing it first would hide that something faster exists.
   */
  const options = state.model_options || [];

  if (options.length > 1){
    const pick = document.createElement("div");
    pick.className = "models";

    options.forEach(o => {
      const b = document.createElement("button");
      b.className = o.recommended ? "" : "ghost";
      b.disabled = busy;
      b.onclick = () => pullModel(o.model);

      const name = document.createElement("strong");
      name.textContent = o.label;
      b.appendChild(name);

      const detail = document.createElement("span");
      detail.textContent = o.model + " · " + o.size + " · " + o.speed;
      b.appendChild(detail);

      pick.appendChild(b);
    });

    local.appendChild(pick);
  } else {
    const lb = document.createElement("button");
    lb.textContent = "Download " + state.recommended_model.model;
    lb.disabled = busy;
    lb.onclick = () => pullModel(state.recommended_model.model);
    local.appendChild(lb);
  }

  box.appendChild(local);

  // API
  const api = document.createElement("div");
  api.className = "choice" + (state.hardware.can_local ? "" : " rec");
  const ah = document.createElement("h3");
  ah.textContent = "Use a paid API";
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
    + "and your messages go to whichever company you choose. The key is stored only on "
    + "this computer.";
  api.appendChild(ap);

  /*
   * Which company, because they are not interchangeable.
   *
   * Privacy here is decided by where a request goes, and somebody who agreed
   * to send their conversation to one has not agreed to the rest — least of
   * all OpenRouter, where it may be served by any of the companies behind it.
   * Naming where to get each key is the difference between a choice and a
   * default nobody noticed making.
   */
  const services = [
    {id: "anthropic", name: "Anthropic", hint: "sk-ant-…",
     where: "console.anthropic.com"},
    {id: "openai", name: "OpenAI", hint: "sk-…",
     where: "platform.openai.com/api-keys"},
    {id: "openrouter", name: "OpenRouter", hint: "sk-or-…",
     where: "openrouter.ai/keys — one key, models from every major company"},
  ];

  const choose = document.createElement("select");

  services.forEach(sv => {
    const opt = document.createElement("option");
    opt.value = sv.id;
    opt.textContent = sv.name;
    choose.appendChild(opt);
  });

  api.appendChild(choose);

  const where = document.createElement("p");
  where.className = "sub";
  api.appendChild(where);

  const input = document.createElement("input");
  input.type = "password";
  api.appendChild(input);

  const describe = () => {
    const sv = services.find(x => x.id === choose.value) || services[0];
    input.placeholder = sv.hint;
    where.textContent = "Get a key at " + sv.where;
  };

  choose.onchange = describe;
  describe();

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
      body: JSON.stringify({key: input.value, provider: choose.value})
    }).then(r => r.json());
    if (!res.ok){ err.textContent = res.error; err.hidden = false; return; }
    input.value = "";
    refresh();
  };
  api.appendChild(ab);
  box.appendChild(api);
}

async function install(name){
  // Updating is the same request; the server re-runs the installer, which
  // fetches the current release and puts it over what is there.
  busy = true;
  el("log").hidden = false;
  await fetch("/install?name=" + encodeURIComponent(name));
  refresh();
}

async function pullModel(model){
  busy = true;
  el("log").hidden = false;
  // Which model, so the choice made on the page is the one that arrives.
  await fetch("/choose-model", {
    method: "POST",
    headers: {"Content-Type":"application/json"},
    body: JSON.stringify({model: model || ""}),
  });
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

async function loadSuggestions(){
  const {suggestions} = await get("/suggestions");
  const box = el("ask-suggestions");
  box.textContent = "";
  suggestions.forEach(q => {
    const c = document.createElement("span");
    c.className = "chip";
    c.textContent = q;
    c.onclick = () => askQuestion(q);
    box.appendChild(c);
  });
}

async function askQuestion(question){
  const res = await fetch("/ask", {
    method:"POST", headers:{"Content-Type":"application/json"},
    body: JSON.stringify({question})
  }).then(r => r.json());

  const qa = document.createElement("div");
  qa.className = "qa";

  const q = document.createElement("p");
  q.className = "q";
  q.textContent = question;

  const a = document.createElement("p");
  a.className = "a";
  // textContent: these answers are fixed strings, but the question is echoed
  // back and that is user input.
  a.textContent = res.answer;

  qa.append(q, a);
  el("ask-log").appendChild(qa);
  el("ask-input").value = "";
}

el("ask-form").onsubmit = e => {
  e.preventDefault();
  const q = el("ask-input").value.trim();
  if (q) askQuestion(q);
};

loadSuggestions();

el("continue").onclick = async () => {
  el("continue").disabled = true;
  el("status").textContent = "Starting PN Brain…";
  await fetch("/done");
};

refresh();
setInterval(refresh, 2000);
</script>
</body>
</html>`
