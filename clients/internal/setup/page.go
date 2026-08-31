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
--dim:#7d8b9c;--faint:#4a5769;--accent:#4dd0e1;--warn:#f0b26b;--danger:#e06c75;--ok:#7bc47f;
/* Three of these were used and never defined, which is why the service picker
   came out as a white native control on a black page: an undefined variable
   makes the whole declaration invalid, so the background was never set at all
   and WebKit drew its own. --fg is the bright end of --text, --card the panel
   the picker sits on, --accent-dim the accent at rest. */
--fg:#eaf1f8;--card:#0d1219;--accent-dim:#1f6b75}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--text);font:15px/1.55 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;
-webkit-font-smoothing:antialiased}
.wrap{max-width:660px;margin:0 auto;padding:44px 28px 60px}
h1{font-size:22px;letter-spacing:.05em;margin:0;color:var(--accent)}

/* ---------- the core ---------- */
.core{display:flex;align-items:center;gap:15px;margin:0 0 4px}
.core svg{width:58px;height:58px;flex:0 0 58px;overflow:visible}

/*
 * Each ring a little brighter than the one outside it, which is what gives the
 * eye its depth — an evenly lit set of circles reads as a diagram.
 */
.core .ring circle{fill:none;stroke:var(--accent);transform-origin:60px 60px}
.core .r1 circle{stroke-width:2.5;opacity:.22;stroke-dasharray:150 76}
.core .r2 circle{stroke-width:2.5;opacity:.34;stroke-dasharray:104 66}
.core .r3 circle{stroke-width:3;opacity:.48;stroke-dasharray:88 45}
.core .r4 circle{stroke-width:3;opacity:.66;stroke-dasharray:60 38}
.core .r5 circle{stroke-width:3.5;opacity:.85;stroke-dasharray:44 26}

/*
 * Opaque, and darker than the page. Everything around it is a light source, and
 * light cannot make anything darker — so without something that actually
 * occludes, the middle fills in and the eye closes.
 */
.core .pupil{fill:#08060a}
.core .rim{fill:none;stroke:var(--accent);stroke-width:2;opacity:.9}

/*
 * Not together, and not all the same way round.
 *
 * Rings turning in step read as one solid object rotating, which is a wheel.
 * Different speeds in alternating directions is what makes it look like
 * something focusing rather than something spinning.
 */
.core .ring{transform-origin:60px 60px;animation:turn linear infinite}
.core .r1{animation-duration:34s}
.core .r2{animation-duration:23s;animation-direction:reverse}
.core .r3{animation-duration:17s}
.core .r4{animation-duration:12s;animation-direction:reverse}
.core .r5{animation-duration:8s}

@keyframes turn{to{transform:rotate(360deg)}}

/*
 * Still, for anybody who asked for that.
 *
 * Continuous motion is a real problem for some people rather than a taste, and
 * the shape carries the identity perfectly well without turning.
 */
@media (prefers-reduced-motion: reduce){
  .core .ring{animation:none}
}
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
/*
 * appearance:none, or none of the rest of this applies.
 *
 * WebKit draws a select with the platform's own control unless told not to,
 * and the platform's own control here is a light one — so the picker stayed
 * white however dark the page around it was told to be. The arrow is drawn
 * back in as a background image, since removing the appearance removes that
 * too.
 */
select{font:inherit;font-size:13px;padding:8px 30px 8px 11px;border-radius:7px;
  background:var(--input);color:var(--text);border:1px solid var(--line);
  width:100%;margin-bottom:6px;appearance:none;-webkit-appearance:none;
  background-image:url("data:image/svg+xml;charset=utf-8,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6'%3E%3Cpath d='M1 1l4 4 4-4' stroke='%237d8b9c' stroke-width='1.5' fill='none' stroke-linecap='round'/%3E%3C/svg%3E");
  background-repeat:no-repeat;background-position:right 11px center}
select:focus,input[type=text]:focus,input[type=password]:focus{
  outline:none;border-color:var(--accent)}
select option{background:var(--input);color:var(--text)}
input::placeholder{color:var(--faint)}
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
.gate{color:var(--warn);font-size:12.5px;margin:18px 0 0}
.folder-h{font-size:13px;margin:22px 0 5px;color:var(--dim);font-weight:600}
.folder-row{display:flex;gap:8px;align-items:flex-start}
.folder-row input{flex:1;margin-bottom:0}
.folder-row button{white-space:nowrap}
.overview{border:1px solid var(--line);border-radius:10px;overflow:hidden;margin-bottom:14px}
.ov-row{display:flex;gap:14px;padding:9px 13px;border-bottom:1px solid var(--line);
  align-items:baseline}
.ov-row:last-child{border-bottom:none}
.ov-what{flex:0 0 42%;font-size:12.5px;color:var(--dim)}
.ov-strong{color:var(--fg);font-weight:600}
.ov-where{flex:1;font-family:ui-monospace,monospace;font-size:11.5px;
  color:var(--faint);word-break:break-all}
.ov-note{color:var(--dim);font-size:12px;line-height:1.5;margin:0 0 14px}
.where{color:var(--faint);font-size:11.5px;margin-top:3px;
  font-family:ui-monospace,monospace;word-break:break-all}
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
</style>
</head>
<body>
<div class="wrap">
  <!--
      The core, rather than the program's name written out.

      This is the same eye the assistant watches from, drawn the same way: five
      rings of decreasing radius around a dark middle, each a little brighter
      than the last, turning slowly and not together. The dark centre is the
      part that makes it an eye — without it this is a target, and with it the
      light has somewhere to be coming from.

      In SVG rather than the real one. The real core is WebGL with bloom, and
      setup runs on a machine that has nothing installed yet and may have no
      working graphics drivers at all — which is the worst possible moment to
      meet a blank rectangle where the program's face should be. Flat shapes
      and two CSS animations cannot fail that way.
  -->
  <div class="core" aria-label="PN Brain">
    <svg viewBox="0 0 120 120" role="img" aria-hidden="true">
      <defs>
        <radialGradient id="halo">
          <stop offset="0%" stop-color="var(--accent)" stop-opacity=".33"/>
          <stop offset="55%" stop-color="var(--accent)" stop-opacity=".07"/>
          <stop offset="100%" stop-color="var(--accent)" stop-opacity="0"/>
        </radialGradient>
      </defs>

      <circle cx="60" cy="60" r="58" fill="url(#halo)"/>

      <!-- Gapped rings, so that turning is visible at all: a full circle
           rotating looks exactly like a full circle standing still. -->
      <g class="ring r1"><circle cx="60" cy="60" r="52"/></g>
      <g class="ring r2"><circle cx="60" cy="60" r="43"/></g>
      <g class="ring r3"><circle cx="60" cy="60" r="34"/></g>
      <g class="ring r4"><circle cx="60" cy="60" r="26"/></g>
      <g class="ring r5"><circle cx="60" cy="60" r="19"/></g>

      <circle class="pupil" cx="60" cy="60" r="13"/>
      <circle class="rim" cx="60" cy="60" r="13"/>
    </svg>
    <h1>PN Brain</h1>
  </div>

  <!--
      It no longer only happens once, so it no longer says so.
      Setup can be reopened from the settings, from the icon's right-click menu
      or with "brain setup", which makes "only happens once" a promise the
      program stopped keeping the moment those existed. What is worth saying
      instead is the thing somebody actually needs to know before starting:
      that reading it costs nothing, because nothing is done until the end.
  -->
  <p class="sub">Let's get your machine ready. It takes a few minutes, and nothing is
    installed or changed until you apply it at the end. You can come back and change
    any of this later.</p>

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
    <div id="folder-section"></div>
  </div>

  <div class="step" id="step-brain" hidden>
    <div id="brain-section"></div>
  </div>

  <div class="step" id="step-needs" hidden>
    <h2>What it needs</h2>
    <p class="sub">These are the pieces PN Brain runs on. Each one says what it
      is for, what stops working without it, and where it goes on this machine.
      Setup installs them for you — nothing here needs a terminal.</p>
    <div id="reqs"></div>
  </div>

  <div class="step" id="step-apply" hidden>
    <h2>Ready</h2>
    <p class="sub">Nothing has been installed or changed yet. Everything below
      is what will happen when you press Apply — every folder that gets made,
      every piece that gets installed and where it lands, and every download
      with its size. Read it before you agree to it.</p>
    <div id="overview"></div>
    <div id="plan"></div>
  </div>

  <pre id="log" hidden></pre>

  <p class="gate" id="gate" hidden></p>

  <div class="footer">
    <button id="back" class="ghost" hidden>Back</button>
    <span class="status" id="status"></span>
    <span class="spacer"></span>
    <button id="next" hidden>Next</button>
    <button id="continue" hidden disabled>Continue to PN Brain</button>
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

    /*
     * Where it is, or where it is about to go.
     *
     * Setup asks somebody to agree to installing things on their own machine
     * and named none of the places any of it would land. Shown for the
     * installed ones too, because "where is it?" is a question this program is
     * supposed to answer from inside itself rather than send somebody to a
     * terminal to work out.
     */
    if (r.where){
      const wh = document.createElement("div");
      wh.className = "where";
      wh.textContent = (ok ? "Installed at " : "Will go to ") + r.where;
      body.appendChild(wh);
    }

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

/*
 * And the exact folder, for somebody who has one in mind.
 *
 * The drive buttons pick a folder per drive, which is the right default and
 * was until now the only possibility. Somebody who keeps everything under one
 * directory has a place they want this, and the page gave them no way to say
 * so — the answer to "can I choose where on the drive?" was no, for no reason
 * other than that nothing asked.
 */
function renderFolderChoice(state){
  const box = el("folder-section");
  if (!box) return;

  if (box.dataset.ready === "1"){
    // Only the parts that follow the state; the field is left alone so it does
    // not fight whoever is typing in it.
    const shown = el("folder-current");
    if (shown) shown.textContent = state.chosen_drive || "";

    return;
  }

  box.dataset.ready = "1";
  box.textContent = "";

  const h = document.createElement("h3");
  h.className = "folder-h";
  h.textContent = "Or somewhere else entirely";
  box.appendChild(h);

  const note = document.createElement("p");
  note.className = "sub";
  note.textContent = "Give a full path. It is checked before it is accepted, so a "
    + "folder that cannot be written to is refused here rather than half way "
    + "through installing.";
  box.appendChild(note);

  const now = document.createElement("p");
  now.className = "where";
  now.id = "folder-current";
  now.textContent = state.chosen_drive || "";
  box.appendChild(now);

  const row = document.createElement("div");
  row.className = "folder-row";

  const field = document.createElement("input");
  field.type = "text";
  field.id = "folder-input";
  field.placeholder = state.chosen_drive || "/path/to/a/folder";
  field.autocomplete = "off";

  const use = document.createElement("button");
  use.className = "ghost";
  use.textContent = "Use this folder";

  const err = document.createElement("p");
  err.className = "err";
  err.hidden = true;

  use.onclick = async () => {
    const path = field.value.trim();
    if (!path) return;

    use.disabled = true;
    err.hidden = true;

    const res = await fetch("/choose-folder", {
      method: "POST", headers: {"Content-Type": "application/json"},
      body: JSON.stringify({path})
    }).then(r => r.json());

    use.disabled = false;

    if (!res.ok){
      err.textContent = res.error;
      err.hidden = false;

      return;
    }

    field.value = "";
    refresh();
  };

  field.onkeydown = e => { if (e.key === "Enter"){ e.preventDefault(); use.onclick(); } };

  row.append(field, use);
  box.append(row, err);
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
  local.appendChild(lh);

  /*
   * The word "recommended" appears once on this page.
   *
   * It was on the card and again on the model inside it, which read as the
   * same recommendation made twice and left it unclear which of the two was
   * being recommended. The card is marked by its lit border, which is what the
   * border was for; the badge belongs on the thing that is actually pressed.
   */

  const lp = document.createElement("p");
  lp.className = "pros";
  lp.textContent = state.hardware.can_local
    ? "Free, private, works offline. Nothing you say leaves this computer, and "
      + "it keeps working with the network unplugged."
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
/*
 * The steps, and what each one will not let you leave without.
 *
 * "blocks" returns why the step is not finished, or "" when it is. Setup used
 * to let somebody walk to the end having answered nothing, and then fail at
 * Apply — which asks four questions, ignores whether they were answered, and
 * reports the problem at the point where it is most expensive to discover.
 *
 * Only genuinely required things block. The optional pieces are optional, and
 * a step that insisted on them would be lying about the word.
 */
const STEPS = [
  {id: "where", title: "Where to keep it",
   shown: st => (st.drives || []).length > 1,
   /*
    * Never blocks: there is always a sensible default, and the step exists to
    * let somebody change it rather than to demand an answer they may not have
    * an opinion about.
    */
   blocks: () => ""},

  {id: "brain", title: "Its brain", shown: () => true,
   blocks: st => {
     // A model to think with, from somewhere. Either a local one that is
     // installed or about to be, or a key for a paid service.
     const localPlanned = [...planned.keys()].some(k => k.startsWith("model:"));
     const localHere = (st.requirements || [])
       .some(r => r.name === "Chat model" && r.state === "ok");

     if (localPlanned || localHere || st.has_api_key) return "";

     return "Choose a model to run here, or save a key for a paid API.";
   }},

  {id: "needs", title: "What it needs", shown: () => true,
   blocks: st => {
     /*
      * Everything that blocks has to be either present or chosen.
      *
      * Not installed — chosen. Nothing installs until Apply, so the question
      * this step asks is whether the plan covers what is missing, and the
      * answer to "you have not dealt with Ollama" should arrive here rather
      * than three steps later.
      */
     const localPlanned = [...planned.keys()].some(k => k.startsWith("model:"));

     const unhandled = (st.requirements || []).filter(r => {
       if (r.optional || r.state === "ok" || planned.has(r.name)) return false;

       /*
        * The model chosen on the previous step is the chat model.
        *
        * They are the same download under two names, so asking for it again
        * here reads as the page having forgotten the answer — and pressing
        * Install would queue the same several gigabytes twice.
        */
       if (r.name === "Chat model" && localPlanned) return false;

       return true;
     });

     if (!unhandled.length) return "";

     return "Still to deal with: " + unhandled.map(r => r.name).join(", ") + ".";
   }},

  {id: "apply", title: "Ready", shown: () => true, blocks: () => ""},
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
/*
 * The whole picture, before anybody agrees to it.
 *
 * The last step listed what would be installed and nothing about where any of
 * it would land — so the thing being agreed to was "install four pieces,
 * somewhere". This says where the brain's own folder goes, where each piece
 * goes, and which of them ignore the drive that was chosen.
 *
 * That last part matters and is easy to get wrong: the models are by far the
 * largest download here and they go to ollama's own directory whatever drive
 * the brain was put on. Somebody who moved the brain to a big disk
 * specifically to hold them would otherwise find that out afterwards.
 */
function renderOverview(state){
  const box = el("overview");
  box.textContent = "";

  const rows = [];

  rows.push(["Everything it learns", state.chosen_drive || "the default folder", true]);

  const willInstall = (state.requirements || []).filter(r => planned.has(r.name));
  const already = (state.requirements || []).filter(
    r => !planned.has(r.name) && r.state === "ok" && r.where);

  willInstall.forEach(r => rows.push([r.name + " — will be installed", r.where || "—", false]));
  already.forEach(r => rows.push([r.name + " — already here", r.where || "—", false]));

  const models = [...planned.entries()].filter(([k]) => k.startsWith("model:"));

  models.forEach(([, describe]) => rows.push([describe, state.model_dir || "ollama's own folder", false]));

  const table = document.createElement("div");
  table.className = "overview";

  rows.forEach(([what, where, strong]) => {
    const line = document.createElement("div");
    line.className = "ov-row";

    const a = document.createElement("div");
    a.className = "ov-what" + (strong ? " ov-strong" : "");
    a.textContent = what;

    const b = document.createElement("div");
    b.className = "ov-where";
    b.textContent = where;

    line.append(a, b);
    table.appendChild(line);
  });

  box.appendChild(table);

  /*
   * Said plainly, because it contradicts what the first step implies.
   *
   * "Where to keep it" reads as though it governs everything setup is about
   * to put on the machine, and for the models it does not.
   */
  if (models.length){
    const caveat = document.createElement("p");
    caveat.className = "ov-note";
    caveat.textContent = "The models do not go in the folder chosen above — ollama "
      + "keeps them in its own directory, shown against each one. Everything PN "
      + "Brain itself learns does go where you chose.";
    box.appendChild(caveat);
  }

  if (!willInstall.length && !models.length){
    const none = document.createElement("p");
    none.className = "ov-note";
    none.textContent = "Nothing will be installed — everything needed is already here.";
    box.appendChild(none);
  }
}

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
  const cont = el("continue");
  const at = shown[step];
  const last = step >= shown.length - 1;

  back.hidden = step === 0;
  next.hidden = last;

  /*
   * Continue belongs to the end, not to every screen.
   *
   * Offered on all four steps it invited somebody to skip the remaining
   * questions without ever saying that is what it did, sitting next to Next
   * and looking like the more decisive of the two.
   */
  cont.hidden = !last;

  // Why the way forward is shut, said on the step that shut it.
  const why = at && at.blocks ? at.blocks(state) : "";

  next.disabled = busy || !!why;

  const gate = el("gate");
  gate.textContent = why;
  gate.hidden = !why;
}

el("back").onclick = () => { step -= 1; refresh(); };
el("next").onclick = () => { step += 1; refresh(); };

async function refresh(){
  const state = await get("/state");
  busy = state.busy;

  el("machine").textContent = machineLine(state.hardware);
  renderRequirements(state);
  renderDriveChoice(state);
  renderFolderChoice(state);
  renderBrainChoice(state);
  renderOverview(state);
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
