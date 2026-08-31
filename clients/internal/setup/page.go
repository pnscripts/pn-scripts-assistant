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

/*
 * Choosing marks the button; it does not restyle the row.
 *
 * Selected and recommended used to be drawn the same way, so picking the
 * other option made the solid highlight jump across and read as the two
 * swapping places — when nothing had moved. A tick and a lit border say
 * "this one" without changing what anything else looks like.
 */
.pick{position:relative;padding-right:34px !important}
.pick.chosen{border-color:var(--accent);color:var(--fg);opacity:1}
.pick.chosen::after{content:"✓";position:absolute;right:12px;top:50%;
  transform:translateY(-50%);color:var(--accent);font-weight:700}
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

/*
 * The step bar.
 *
 * Named rather than numbered, because "2 of 3" tells somebody where they are
 * and not what they are being asked. The one they are on is lit; the ones
 * behind are still readable and can be gone back to; the ones ahead are dim,
 * because they may not all appear — a machine with one disk skips the first.
 */
.steps{display:flex;gap:0;list-style:none;padding:0;margin:0 0 18px;
  font-size:12px;flex-wrap:wrap}

/* What will happen, numbered in the order it happens. */
.plan{margin:0 0 14px;padding-left:20px;font-size:13px;line-height:1.7}
.plan li{color:var(--fg)}
.steps li{padding:5px 12px;border-bottom:2px solid var(--line);color:var(--dim);
  transition:color .15s,border-color .15s}
.steps li[data-state="now"]{color:var(--fg);border-bottom-color:var(--accent)}
.steps li[data-state="done"]{color:var(--dim);border-bottom-color:var(--accent);
  opacity:.75}
.steps li[data-state="done"]:hover{color:var(--fg)}
.steps li[data-state="later"]{opacity:.5}

/* One step at a time, so the page is never longer than the decision on it. */
.step h2{margin-top:0}
.step .sub{margin-bottom:10px}
.status{color:var(--dim);font-size:12.5px}
.err{color:var(--danger);font-size:12.5px;margin-top:6px}
.ask{background:var(--raised);border:1px solid var(--line);border-radius:10px;padding:16px;margin-bottom:26px}
/* Closed it is one quiet line; open it is what it always was. */
.ask summary{cursor:pointer;font-size:12.5px;color:var(--dim);padding:2px 0;
  list-style:none}
.ask summary::-webkit-details-marker{display:none}
.ask summary::before{content:"› ";opacity:.7}
.ask[open] summary::before{content:"⌄ "}
.ask summary:hover{color:var(--fg)}
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

  <!-- Setup as steps, in the order the decisions actually depend on each
       other: where it lives, what it thinks with, what has to be installed,
       and what is optional. Each carries what somebody needs to decide, which
       is the part a list of checkboxes leaves out. -->
  <ol class="steps" id="steps"></ol>

  <div class="step" id="step-where">
    <h2>Where to keep it</h2>
    <p class="sub">Everything PN Brain learns — what you tell it, what it reads,
      every conversation — lives in one folder. Nothing else on your computer is
      touched, and deleting that folder removes the brain completely.</p>
    <p class="sub">Choosing now is free. Moving it later copies the lot and
      deletes the original, which takes a while and is risky on a drive that
      might be unplugged.</p>
    <div id="drive-section"></div>
  </div>

  <div class="step" id="step-brain" hidden>
    <div id="brain-section"></div>
  </div>

  <div class="step" id="step-needs" hidden>
    <h2>What it needs</h2>
    <p class="sub">These are the pieces PN Brain runs on. Each one says what it
      is for and what stops working without it. Setup installs them for you —
      nothing here needs a terminal.</p>
    <div id="reqs"></div>
  </div>

  <div class="step" id="step-apply" hidden>
    <h2>Ready</h2>
    <p class="sub">Nothing has been installed yet. Here is what will happen when
      you start — it downloads several gigabytes, so it is worth a look before
      you begin.</p>
    <div id="plan"></div>
  </div>

  <!-- The questions stay at the bottom, closed, on every step. A several
       gigabyte download is exactly when somebody has questions and nothing to
       ask; it is just not what the page is for. -->
  <details class="ask">
    <summary>Questions about any of this?</summary>
    <p class="ask-note">A short list of written answers, not the assistant —
      that arrives when setup finishes.</p>
    <div id="ask-log"></div>
    <div id="ask-suggestions"></div>
    <form id="ask-form">
      <input type="text" id="ask-input" placeholder="Ask about setup…" autocomplete="off">
      <button type="submit">Ask</button>
    </form>
  </details>

  <pre id="log" hidden></pre>

  <div class="footer">
    <button id="back" class="ghost" hidden>Back</button>
    <span class="status" id="status"></span>
    <span class="spacer"></span>
    <button id="next" hidden>Next</button>
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
      /*
       * Chosen now, done at the end.
       *
       * The button used to install the moment it was pressed, which made every
       * decision on this page final as soon as it was made. Adding to a list
       * instead means somebody can pick four things, look at what that adds up
       * to, and take one back out.
       */
      const chosen = planned.has(r.name);

      const b = document.createElement("button");
      b.textContent = ok ? "Update" : "Install";
      b.className = "ghost pick" + (chosen ? " chosen" : "");
      b.disabled = busy;
      b.onclick = () => plan(r.name, (ok ? "Update " : "Install ") + r.name);
      row.appendChild(b);
    }

    card.appendChild(row);
    el("reqs").appendChild(card);
  });
}

/*
 * Where the brain keeps itself.
 *
 * Only shown when there is a choice to make. On a machine with one disk this
 * is a question with one answer, and asking it would be a step that teaches
 * somebody nothing and delays them by one click.
 */
function renderDriveChoice(state){
  const box = el("drive-section");
  box.textContent = "";

  const drives = state.drives || [];
  if (drives.length < 2) return;

  /*
   * No heading here.
   *
   * The step this sits in already has one, and the explanation above it. Both
   * were printed, so the page said "Where to keep it" twice with two versions
   * of the same paragraph between them.
   */
  const list = document.createElement("div");
  list.className = "models";

  drives.forEach(d => {
    const chosen = state.chosen_drive === d.path;

    const b = document.createElement("button");
    b.className = "ghost pick" + (chosen ? " chosen" : "");
    b.disabled = busy;
    b.onclick = async () => {
      const res = await fetch("/choose-drive", {
        method: "POST",
        headers: {"Content-Type":"application/json"},
        body: JSON.stringify({path: d.path}),
      }).then(r => r.json());

      if (!res.ok){ alert(res.error); return; }

      refresh();
    };

    const name = document.createElement("strong");
    name.textContent = d.mount === "/" ? "This computer" : d.mount;
    if (d.removable){
      // Worth saying plainly: a brain on a drive that gets unplugged is a
      // brain that is missing, and nothing else on this page would warn them.
      name.textContent += " — removable";
    }
    b.appendChild(name);

    const detail = document.createElement("span");
    detail.textContent = d.free_gb + "GB free of " + d.total_gb + "GB · " + d.path;
    b.appendChild(detail);

    list.appendChild(b);
  });

  box.appendChild(list);
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
      const chosen = planned.has("model:" + o.model);

      /*
       * Chosen and recommended are different things and must look it.
       *
       * Both used to be drawn as the solid button, so picking the other one
       * made the highlight jump across and read as the two swapping places —
       * when nothing had moved and only the selection had changed.
       * Recommended is a fact about this machine and stays where it is;
       * chosen is a state of the button and is marked on the button.
       */
      const b = document.createElement("button");
      b.className = "ghost pick" + (chosen ? " chosen" : "");
      b.disabled = busy;
      b.onclick = () => {
        // One model, not four: choosing another replaces the last.
        [...planned.keys()]
          .filter(k => k.startsWith("model:"))
          .forEach(k => planned.delete(k));

        if (!chosen) planned.set("model:" + o.model, "Download " + o.model + " (" + o.size + ")");

        refresh();
      };

      const name = document.createElement("strong");
      name.textContent = o.label;

      if (o.recommended){
        const tag = document.createElement("span");
        tag.className = "tag";
        tag.textContent = "recommended";
        name.appendChild(tag);
      }

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

/*
 * Setup as steps, in the order the decisions depend on each other.
 *
 * Where it lives comes first, because everything after it is written there and
 * a choice made afterwards means moving gigabytes rather than naming a folder.
 * Then what it thinks with, then what has to be installed to run that — the
 * last of which is the only step somebody cannot answer without the first two.
 *
 * A step with nothing to decide is left out rather than shown empty: on a
 * machine with one disk, "where to keep it" is a question with one answer, and
 * a step that answers itself is a click that teaches nobody anything.
 */
const STEPS = [
  {id: "where", title: "Where to keep it",
   shown: st => (st.drives || []).length > 1},
  {id: "brain", title: "Its brain", shown: () => true},
  {id: "needs", title: "What it needs", shown: () => true},
  {id: "apply", title: "Ready", shown: () => true},
];

/*
 * What has been chosen but not yet done.
 *
 * Setup collects decisions and the last step carries them out, rather than
 * each button acting the moment it is pressed. Somebody picking a drive, a
 * model and two optional pieces should be able to change their mind about any
 * of them without having already downloaded five gigabytes of the first
 * answer.
 */
const planned = new Map();

function plan(name, describe){
  if (planned.has(name)) planned.delete(name);
  else planned.set(name, describe);

  refresh();
}

/*
 * renderPlan lists what will happen, in the order it will happen.
 *
 * Ordered by dependency rather than by when it was chosen: a model cannot be
 * downloaded before the thing that runs models exists, and a list that reads
 * in a different order from the one it runs in would make a failure halfway
 * through impossible to follow.
 */
function renderPlan(state){
  const box = el("plan");
  box.textContent = "";

  const missing = (state.requirements || [])
    .filter(r => r.state !== "ok" && !r.optional)
    .map(r => r.name);

  const order = [...planned.keys()].sort((a, b) => {
    const rank = n => n.startsWith("model:") ? 2 : (missing.includes(n) ? 0 : 1);

    return rank(a) - rank(b);
  });

  if (state.chosen_drive){
    const where = document.createElement("p");
    where.className = "sub";
    where.textContent = "It will keep itself in " + state.chosen_drive;
    box.appendChild(where);
  }

  if (!order.length){
    const none = document.createElement("p");
    none.className = "sub";
    none.textContent = "Nothing left to install. You can go straight in.";
    box.appendChild(none);

    return;
  }

  const list = document.createElement("ol");
  list.className = "plan";

  order.forEach(name => {
    const li = document.createElement("li");
    li.textContent = planned.get(name);
    list.appendChild(li);
  });

  box.appendChild(list);

  const go = document.createElement("button");
  go.textContent = "Install all of it";
  go.disabled = busy;
  go.onclick = async () => {
    busy = true;
    el("log").hidden = false;
    await fetch("/apply", {
      method: "POST",
      headers: {"Content-Type":"application/json"},
      body: JSON.stringify({steps: order}),
    });
    refresh();
  };

  box.appendChild(go);
}

let step = 0;

function visibleSteps(state){ return STEPS.filter(s => s.shown(state)); }

function renderSteps(state){
  const shown = visibleSteps(state);

  if (step >= shown.length) step = shown.length - 1;
  if (step < 0) step = 0;

  const bar = el("steps");
  bar.textContent = "";

  shown.forEach((s, i) => {
    const li = document.createElement("li");
    li.textContent = s.title;
    li.dataset.state = i === step ? "now" : (i < step ? "done" : "later");

    // Going back is allowed; skipping ahead is not, because a later step
    // depends on the answer to an earlier one.
    if (i < step){
      li.onclick = () => { step = i; refresh(); };
      li.style.cursor = "pointer";
    }

    bar.appendChild(li);
  });

  STEPS.forEach(s => {
    const box = el("step-" + s.id);
    const at = shown[step];

    box.hidden = !at || at.id !== s.id;
  });

  const back = el("back");
  const next = el("next");

  back.hidden = step === 0;
  next.hidden = step >= shown.length - 1;
  next.disabled = busy;
}

el("back").onclick = () => { step -= 1; refresh(); };
el("next").onclick = () => { step += 1; refresh(); };

async function refresh(){
  const state = await get("/state");
  busy = state.busy;

  el("machine").textContent = machineLine(state.hardware);
  renderRequirements(state);
  renderDriveChoice(state);
  renderBrainChoice(state);
  renderPlan(state);
  renderSteps(state);

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
