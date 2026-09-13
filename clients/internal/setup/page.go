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
<title>PN Scripts Assistant — Setup</title>
<style>
:root{--bg:#070a0f;--raised:#0d1219;--input:#111823;--line:#1b2634;--text:#d6dee8;
--dim:#7d8b9c;--faint:#4a5769;--accent:#4dd0e1;--warn:#f0b26b;--danger:#e06c75;--ok:#7bc47f;
/* Three of these were used and never defined, which is why the service picker
   came out as a white native control on a black page: an undefined variable
   makes the whole declaration invalid, so the background was never set at all
   and WebKit drew its own. --fg is the bright end of --text, --card the panel
   the picker sits on, --accent-dim the accent at rest. */
--fg:#eaf1f8;--card:#0d1219;--accent-dim:#1f6b75;

/* The half of a select that CSS cannot reach.
   appearance:none and a background colour dress the closed box, and the list
   that drops out of it is not part of the page at all — WebKit asks GTK for a
   real menu, and GTK draws it in the system theme. So the picker looked right
   until it was opened, and then a white list appeared over a black page.
   option{} rules do not touch it either; they style a DOM element that is not
   what gets drawn. color-scheme is the one thing that does: it tells the
   engine which theme to ask for, and it covers the other native furniture —
   scrollbars, the focus ring, the text cursor — for the same reason. */
color-scheme:dark}
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
/* A row each, not a paragraph of twelve.
   .field-check was put on every one of these labels and never defined
   anywhere, and a label is inline by default — so the ten protected
   places ran together into one block of prose with the checkboxes buried
   mid-sentence, and the list that is supposed to show what the assistant
   will ask about was the hardest thing on the page to read. .muted was
   undefined for the same reason, which left every explanation at full
   strength and nothing looking secondary to anything. */
.field-check{display:flex;align-items:flex-start;gap:9px;padding:9px 0;
  border-bottom:1px solid var(--line);cursor:pointer}
.field-check:last-of-type{border-bottom:0}
.field-check input{margin:3px 0 0;flex:0 0 auto}
.field-check b{font-weight:600}
.field-check .muted{font-weight:400}
.muted{color:var(--dim)}
.models{display:flex;flex-direction:column;gap:7px;margin-top:4px}
/* A fixed width on the sample button, not a proportion. A percentage let the
   longest voice name decide how wide "Hear it" was, so the three buttons in
   the column came out three different sizes. */
.voice-row{display:flex;gap:7px;align-items:stretch;margin-top:7px}
.voice-row .pick{flex:1 1 auto;min-width:0;display:flex;flex-direction:column;
  align-items:flex-start;gap:2px;text-align:left}
.voice-row .pick span{font-weight:400;font-size:11.5px;opacity:.72}
.voice-row .hear{flex:0 0 104px;width:104px;font-size:11.5px}
.voice-row .hear:disabled{opacity:.45}
#voice-pick{margin-top:22px}
#voice-pick h3{margin:0 0 4px;font-size:13px}

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
/* Whole lines, or none.
   The height was 230px against an unstated line-height, so the box was about
   twelve and a half rows tall — and because it is scrolled to the bottom while
   something installs, the half row was at the top, permanently sliced through
   the middle of a line of build output. Both numbers are stated now and the
   height is a multiple of the line, so the top row is a row.

   break-word as well as wrapping: build output is full of long paths with no
   spaces in them, which wrap alone cannot break and which pushed the box wider
   than the column it sits in. */
pre{background:#05080c;border:1px solid var(--line);border-radius:8px;
padding:10px 12px;font-size:11.5px;line-height:18px;color:var(--dim);
max-height:234px;overflow:auto;overscroll-behavior:contain;
white-space:pre-wrap;word-break:break-word;margin:8px 0 0}
/* Classes, not ids: there can be one of these inside the card that is
   installing and another at the foot for work no card is showing. */
#progress{margin:22px 0 0}
.prog{margin-top:10px}
.card .prog, .model-progress .prog{margin-top:8px}
.prog-head{display:flex;align-items:baseline;gap:12px;font-size:12px;color:var(--dim)}
.prog-what{flex:1 1 auto;min-width:0}
.prog-pct{flex:0 0 auto;font-family:var(--mono,monospace);color:var(--text)}
/* A track that is always the same width, so the bar means something.
   Height in px rather than em: this is a rule, not text. */
.prog-track{height:6px;background:var(--input);border:1px solid var(--line);
  border-radius:4px;overflow:hidden;margin:7px 0 0}
.prog-bar{height:100%;width:0;background:var(--accent);
  transition:width .3s ease}
/* Unknown position is drawn as a stripe that moves, never as 0%: "starting"
   and "no idea how far" are different things and a bar stuck at zero says the
   wrong one. */
.prog-bar.unknown{width:100%;opacity:.35;
  background:repeating-linear-gradient(90deg,var(--accent) 0 12px,transparent 12px 24px);
  animation:creep 1s linear infinite}
@keyframes creep{from{background-position:0 0}to{background-position:24px 0}}
.prog-note{margin:6px 0 0}
/* The card doing the work is lit, so it is findable in a list of twelve. */
.card.working{border-color:var(--accent)}
.model-row.working .model-name{color:var(--accent)}
.model-progress{padding:0 0 10px}
.log-wrap{margin-top:10px}
.log-wrap>summary{cursor:pointer;font-size:11.5px;color:var(--faint);
  list-style:none;padding:2px 0}
.log-wrap>summary::-webkit-details-marker{display:none}
.log-wrap>summary::before{content:"\203A ";display:inline-block;
  transition:transform .12s}
.log-wrap[open]>summary::before{transform:rotate(90deg)}
.log-wrap>summary:hover{color:var(--dim)}
/* The band between "needed" and "optional". The first one has no space above
   it: it is the first thing in the list, and a gap there reads as a missing
   card rather than as separation. */
h3.group{margin:26px 0 2px;font-size:13px}
#reqs>h3.group:first-child{margin-top:8px}
.group-note{margin:0 0 10px}
/* A row, not a card: eight of these on one screen, each answering the same
   three questions in the same places. */
.model-row{display:flex;align-items:center;gap:12px;padding:9px 0;
  border-bottom:1px solid var(--line)}
.model-row:last-child{border-bottom:0}
.model-body{flex:1 1 auto;min-width:0}
.model-name{font-weight:600;font-size:13px;display:flex;align-items:center;gap:7px}
.model-detail{font-size:11.5px;color:var(--dim);margin-top:2px}
.model-row button{flex:0 0 84px;width:84px}
.model-here{flex:0 0 84px;width:84px;text-align:center;font-size:11.5px;color:var(--ok)}
/* Said, not offered. Same width as the button it stands in place of, so the
   column of them does not wander. */
.req-done{flex:0 0 84px;width:84px;text-align:center;font-size:11.5px;color:var(--ok)}
/* One line per check. The mark column is fixed so the ticks line up as a
   column rather than wandering with the length of each sentence.

   A grid, not a row of flex items. The note was a flex item that would not
   shrink, so a long one — the folder path, and then the list of who on the
   organisation is held back on this machine — squeezed the sentence it
   explains down to a word a line and pushed the page sideways. The sentence
   keeps a readable minimum; the note takes what is left and wraps inside it. */
.tick-row{display:grid;grid-template-columns:14px minmax(11em,1fr) auto;
  align-items:baseline;column-gap:9px;padding:5px 0;
  border-bottom:1px solid var(--line);font-size:12.5px}
.tick-row:last-of-type{border-bottom:0}
.tick-row .mark{flex:0 0 14px;width:14px;text-align:center}
.tick-row.yes .mark{color:var(--ok)}
.tick-row.no .mark{color:var(--danger)}
.tick-row.skip .mark{color:var(--warn)}
.tick-row.checking .mark{color:var(--faint)}
.tick-what{min-width:0}
.tick-note{min-width:0;color:var(--dim);font-size:11.5px;text-align:right;
  overflow-wrap:anywhere}
.cost{font-size:11.5px;color:var(--dim);margin-top:3px}
/* Lit only when the room is tight, so the ordinary case stays quiet. */
.cost.tight{color:var(--warn)}
.models-room{margin-top:-4px}
.linky-btn{margin-top:12px}
#library input[type=text]{margin:10px 0 4px}
/* Quiet tags for what a model can do, so they sit beside the name without
   competing with "recommended". */
.tag.quiet{background:transparent;border:1px solid var(--line);color:var(--dim);
  font-weight:400}
.model-row.here .model-name{color:var(--ok)}
.tag.warn{background:var(--warn);color:#1c1206}
.api-held{font-size:12px;color:var(--ok);margin:0 0 4px}
.api-held::before{content:"\2713 ";font-weight:700}
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

/* ---------- the stage ----------
   The world the steps stand in; the program's own page explains it at length.
   Everything hangs off .has-stage, set only once WebGL has started, so without
   it this page is exactly the flat one it was. */
.stage{position:fixed;inset:0;width:100vw;height:100vh;display:block;z-index:0;pointer-events:none}
html.has-stage .wrap{position:relative;z-index:1}
.stage-frame{position:relative}
.stage-frame.stage-moving{overflow:visible}
.stage-moving>.stage-shown{position:absolute;left:0;top:0;backface-visibility:hidden;will-change:transform}
.stage-moving>.stage-shown[hidden]{display:block}
.step .sub{margin-bottom:10px}
.status{color:var(--dim);font-size:12.5px}
.status.bad{color:var(--danger)}
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
  <div class="core" aria-label="PN Scripts Assistant">
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
    <h1>PN Scripts Assistant</h1>
  </div>

  <!--
      A greeting, and nothing else.

      This line has been wrong twice by trying to carry more than a greeting.
      It promised setup "only happens once", which stopped being true when it
      became reopenable, and then explained at length that nothing would be
      installed yet — a claim that describes the next few minutes and hides
      what the whole page is for.

      The last step says all of that where it applies, next to the list of what
      will actually be installed and how large it is. Said here as well it was
      only length in front of the first question.
  -->
  <p class="sub">Let's get your machine ready. It takes a few minutes.</p>

  <div class="machine" id="machine">checking your machine…</div>

  <!-- Setup as steps, in the order the decisions actually depend on each
       other: where it lives, what it thinks with, what has to be installed,
       and what is optional. Each carries what somebody needs to decide, which
       is the part a list of checkboxes leaves out. -->
  <ol class="steps" id="steps"></ol>

  <!-- The steps stand in the same world the program's panels do, and this
       is the frame the one in front fills. Only the steps are inside it: the
       bar above and the buttons below are how somebody moves between them,
       and they stay where they are while the step turns. -->
  <div class="step-stage" id="step-stage">

  <div class="step" id="step-welcome">
    <h2>A private assistant, on your own computer</h2>
    <p class="sub">It listens, answers, remembers what you tell it, and reads
      the files you point it at. All of that happens here — nothing you say
      leaves this machine unless you choose to use a paid service, and it goes
      on working with the network unplugged.</p>
    <p class="sub">Setting it up takes a few minutes and one large download.
      This will say what each piece is for, how big it is and where it goes,
      before it fetches anything.</p>
    <div id="welcome-cost"></div>
  </div>

  <div class="step" id="step-where" hidden>
    <h2>Where to keep it</h2>
    <p class="sub">Everything the assistant learns — what you tell it, what it reads,
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
    <div id="models-section"></div>
    <!-- Its own container, outside the one that is rebuilt every two seconds.
         See renderApiChoice. -->
    <div id="api-section"></div>
    <!-- The whole library goes last, below the paid-API card rather than
         above it. Expanded, it is sixty rows of models — and the card offering
         the alternative to all of them was sitting underneath, which made
         "use a paid API" something you found by scrolling past four hundred
         reasons not to. -->
    <div id="library-section"></div>
  </div>

  <div class="step" id="step-needs" hidden>
    <h2>What it needs</h2>
    <p class="sub">These are the pieces the assistant runs on. Each one says what it
      is for, what stops working without it, and where it goes on this machine.
      Setup installs them for you — nothing here needs a terminal.</p>
    <div id="reqs"></div>
  </div>

  <div class="step" id="step-name" hidden>
    <h2>What to call it</h2>
    <p class="sub">It answers to its name, so this is both what it is called
      and the word that wakes it when you speak.</p>

    <!-- Only the typed fields are inside the form. The voice picker below
         redraws on every poll — it has to, because the human voices appear
         partway through an install — and wrapping it here would freeze it.
         See identityBeingEdited. -->
    <div id="name-form"></div>

    <div id="voice-pick"></div>
  </div>

  <div class="step" id="step-privacy" hidden>
    <h2>What it asks you about</h2>
    <p class="sub">One switch decides how much it stops to ask, and how much
      may leave this machine. It is the same switch as under Permissions, and
      you can change it there at any time.</p>

    <!-- The one switch first, because everything below it depends on the
         answer: on "never stop" the files listed underneath are recorded
         rather than asked about, and a list of questions that will not be
         asked should not be read as if it will. -->
    <div id="freedom-pick" class="models"></div>
    <p class="sub" id="freedom-files"></p>

    <div id="privacy-rules"></div>
    <label class="field">
      <span>Anything else it should ask about</span>
      <input id="privacy-add" type="text" autocomplete="off"
             placeholder="/a/folder/  or  a-file-name.txt">
    </label>
    <div id="privacy-yours"></div>
  </div>

  <div class="step" id="step-apply" hidden>
    <h2>Ready</h2>
    <!-- It used to describe what pressing Apply would do. Each step installs
         its own things now, so by the time somebody arrives here it has all
         happened — and the useful thing to show is what is on the machine,
         not a promise about what might be. -->
    <p class="sub">This is what is on your machine now, and where each piece
      of it lives.</p>
    <div id="ready-ticks"></div>
    <div id="overview"></div>
    <div id="plan"></div>

  </div>

  </div>

  <!-- What is happening, under whichever step is doing it.
       It lived inside the last step while everything installed at the end.
       Each step installs its own things now, so it sits below all of them and
       follows the work rather than the reader. The log is still complete,
       behind a fold: several thousand lines of cmake output is the honest
       answer to "how much longer" and an unreadable one. -->
  <div id="progress" hidden></div>

  <p class="gate" id="gate" hidden></p>

  <div class="footer">
    <button id="back" class="ghost" hidden>Back</button>
    <span class="status" id="status"></span>
    <span class="spacer"></span>
    <button id="next" hidden>Next</button>
    <button id="continue" hidden disabled>Start the program</button>
  </div>
</div>

<script>
const el = id => document.getElementById(id);
let busy = false;

/*
 * Closing the window in the middle of an install asks first.
 *
 * It used to be silent and permanent. The process dies, whatever was
 * downloading stops, and — before part files — the half of it that had
 * arrived kept the real name and was counted as installed ever after. The
 * download is safe now; the interruption is still worth a question, because
 * some of these are twenty minutes of building and starting again is starting
 * from the beginning.
 *
 * The browser decides the wording, which is fine: the only job here is that
 * closing during an install is a decision rather than an accident.
 */
window.addEventListener("beforeunload", (e) => {
  if (!busy) return;

  e.preventDefault();

  // Old browsers want a returned string; current ones want preventDefault.
  e.returnValue = "Something is still installing. Closing now stops it.";

  return e.returnValue;
});

async function get(p){ const r = await fetch(p); return r.json(); }

// Sends a choice and hands back what the server made of it.
async function post(p, body){
  const r = await fetch(p, {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify(body || {}),
  });

  return r.json().catch(() => ({}));
}

function machineLine(h){
  const bits = [h.cores + " cores", h.ram_gb + "GB RAM"];
  bits.push(h.has_gpu ? "GPU: " + h.gpu : "no discrete GPU");
  return bits.join("  ·  ");
}

/*
 * Everything needed is chosen already; everything else is not.
 *
 * Setup used to make no distinction: twelve cards, each with its own Install
 * button, and a line at the bottom naming what was "still to deal with". So
 * the shortest path to a working assistant was to read twelve entries and
 * work out which four mattered — and the longest was to install all of it,
 * including a recogniser that takes minutes to compile, before finding out
 * whether any of it was wanted.
 *
 * Neither is the right default. What it cannot run without is chosen here so
 * that pressing Next twice gives somebody a working assistant; what it can
 * run without is left alone, listed under a heading that says it can be added
 * later — which is true, and now true from inside the program rather than by
 * running setup again.
 */
function renderRequirements(state){
  el("reqs").textContent = "";

  /*
   * The chat model is not listed here, because the next step is entirely
   * about it.
   *
   * It is a requirement and it does block, so it belongs in the checks — but
   * "Chat model · needs Ollama first" sitting in a list of programs, one step
   * before a screen offering eight models to choose between, asks the same
   * question twice and answers it differently each time.
   */
  const needed = (state.requirements || [])
    .filter(r => !r.optional && r.name !== "Chat model");
  const extras = (state.requirements || []).filter(r => r.optional);

  const onPaid = brainWay === "paid";

  heading("reqs", onPaid ? "For a brain on this machine" : "Needed to run",
    onPaid
      ? "You chose a paid service, so none of these are required — it can "
        + "answer without them. Install them if you also want it to work "
        + "offline and to remember things: the embedding model is what "
        + "memory is made of."
      : "Without these it cannot answer at all. Install anything marked "
        + "missing before going on.");

  /*
   * The escape hatch that used to be here is gone, along with what it escaped.
   *
   * It jumped forward to the paid-API screen, because this step came first and
   * demanded a gigabyte and a half from somebody who had no use for it. That
   * question is asked two steps earlier now, and the answer is what decides
   * whether anything here is required — so there is nothing left to escape.
   */

  installAllButton("reqs", needed, "Install what's needed");

  needed.forEach(r => requirementCard(r, state));

  if (extras.length){
    heading("reqs", "Add when you want them",
      "None of these are needed to start. Install any you want now, or add "
      + "them later from inside the program — you will not have to come back "
      + "here.");

    installAllButton("reqs", extras, "Install the extras");

    extras.forEach(r => requirementCard(r, state));
  }
}

/*
 * One press for a whole group, beside the press-by-press buttons.
 *
 * Both are wanted and they answer different people. Somebody who cares which
 * pieces land on their machine has a button per piece and a card explaining
 * each; somebody who wants an assistant has one button and a number. The list
 * that made you press four buttons to reach a working program was serving the
 * first person at the second one's expense.
 *
 * It sends the whole group to /apply, which already installs in order, stops
 * at the first failure and reports which step it is on — so each card lights
 * up and shows its own bar in turn, with nothing new to write.
 */
function installAllButton(into, group, label){
  const todo = group.filter(r => r.state !== "ok" && r.installable);
  if (todo.length < 2) return;

  /*
   * Anything wanting a password goes last.
   *
   * A run stops at the first failure and a dismissed password box is one, so
   * an apt package early in the list could abandon everything queued behind
   * it. Ordered here rather than on the server because the server's order is
   * the dependency order, which is right and is a different question.
   */
  const ordered = [...todo].sort((a, b) =>
    (a.needs_password ? 1 : 0) - (b.needs_password ? 1 : 0));

  let total = 0;
  ordered.forEach(r => { total += sizeInGB(r.size); });

  const b = document.createElement("button");
  b.className = "ghost linky-btn";
  b.disabled = busy;
  b.textContent = label + " · " + ordered.length + " things"
    + (total > 0
      ? ", about " + (total < 1
        ? Math.round(total * 1024) + "MB"
        : total.toFixed(1) + "GB")
      : "");

  b.onclick = () => installAll(ordered.map(r => r.name));

  el(into).appendChild(b);
}

async function installAll(names){
  if (!names.length) return;

  // Set here rather than waited for: the server marks itself busy the moment
  // the request lands, but the next poll is up to two seconds away, and a
  // button that stays live for two seconds after being pressed gets pressed
  // twice.
  busy = true;
  refresh();

  await post("/apply", {steps: names});
  refresh();
}

// heading puts a titled band between groups of cards.
function heading(into, title, note){
  const h = document.createElement("h3");
  h.className = "group";
  h.textContent = title;
  el(into).appendChild(h);

  const p = document.createElement("p");
  p.className = "sub group-note";
  p.textContent = note;
  el(into).appendChild(p);
}

/*
 * sizeInGB reads "~4.7GB" or "488MB" back into a number.
 *
 * The sizes are written for people — a tilde, a unit, sometimes a space — and
 * are compared against free space only to decide whether to light a warning.
 * Anything unparseable returns 0, which lights nothing: a missing warning is
 * better than one raised by a string this did not understand.
 */
function sizeInGB(text){
  if (!text) return 0;

  const m = String(text).match(/([\d.]+)\s*([KMG])i?B/i);
  if (!m) return 0;

  const n = parseFloat(m[1]);
  if (!isFinite(n)) return 0;

  switch (m[2].toUpperCase()){
    case "G": return n;
    case "M": return n / 1024;
    case "K": return n / (1024 * 1024);
    default:  return 0;
  }
}

function requirementCard(r, state){
  [r].forEach(r => {
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

    /*
     * What it costs, and whether there is room for it.
     *
     * Both were known and neither was said: this offered to fetch a gigabyte
     * onto a disk whose free space it had already measured. Together on one
     * line because they are one question — "1.3GB, and 38GB free where it
     * goes" answers it, while either number alone does not.
     *
     * Only for things not yet installed. Beside something already on the
     * machine, a download size is a fact about the past.
     */
    if (!ok && (r.size || r.free_gb)){
      const cost = document.createElement("div");
      cost.className = "cost";

      const parts = [];

      if (r.size) parts.push(r.size + " to download");
      if (r.free_gb) parts.push(r.free_gb + "GB free where it goes");

      cost.textContent = parts.join(" · ");

      // Lit when the download would not comfortably fit, which is the one
      // time this line needs to be read rather than glanced at.
      const wants = sizeInGB(r.size);

      if (wants && r.free_gb && r.free_gb < wants * 1.5){
        cost.classList.add("tight");
      }

      body.appendChild(cost);
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
     * A button only where there is something to do.
     *
     * Every installed piece used to carry an Update button, which made a list
     * of twelve read as twelve outstanding jobs when eleven of them were
     * finished and current. The check already knows the difference — it
     * reports "outdated" separately from "ok" — and the page was throwing that
     * away by treating anything installed as updatable.
     *
     * So: Install what is missing, Update what is behind, and say "installed"
     * for the rest. A word rather than a button, because there is nothing to
     * press: pressing it would refetch a version already on the machine.
     */
    if (ok){
      const done = document.createElement("span");
      done.className = "req-done";
      done.textContent = "installed";
      row.appendChild(done);
    } else if (r.installable){
      /*
       * Chosen now, done at the end.
       *
       * The button used to install the moment it was pressed, which made every
       * decision on this page final as soon as it was made. Adding to a list
       * instead means somebody can pick four things, look at what that adds up
       * to, and take one back out.
       */
      /*
       * Pressed, done — not pressed, queued.
       *
       * These used to add to a list carried out on the last step, so that
       * four things could be chosen and reconsidered before any of them
       * happened. That was the right shape for a page whose last step did
       * everything, and it is the wrong shape for one where each step
       * finishes its own work: it left somebody on the models step wondering
       * whether Ollama was there, because "chosen" and "installed" looked the
       * same and neither was true yet.
       */
      const b = document.createElement("button");
      b.textContent = r.state === "outdated" ? "Update" : "Install";
      b.className = "ghost";
      b.disabled = busy;
      b.onclick = () => install(r.name);
      row.appendChild(b);
    }

    card.appendChild(row);

    // The bar goes in the card doing the work, not at the foot of a list of
    // twelve. See progressBlock.
    if (state && whatIsInstalling(state) === r.name){
      card.classList.add("working");
      card.appendChild(progressBlock(state));
    }

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
 * The whole library: loaded once, then filtered in the page.
 *
 * Filtered here rather than re-fetched because a search that waits on a
 * website between keystrokes is a search nobody finishes.
 */
let libraryShown = false;
let libraryModels = null;
let libraryError = "";
let librarySearch = "";

async function loadLibrary(){
  refresh();

  try {
    const res = await (await fetch("/library")).json();

    if (!res.ok){
      libraryError = res.error || "I could not reach the library.";
      libraryModels = [];
    } else {
      libraryModels = res.models || [];
      libraryError = "";
    }
  } catch (e){
    libraryError = "I could not reach the library.";
    libraryModels = [];
  }

  refresh();
}

function renderLibrary(){
  const box = el("library-section");
  if (!box) return;

  box.textContent = "";

  if (libraryModels === null){
    const wait = document.createElement("p");
    wait.className = "sub";
    wait.textContent = "Reading ollama's library…";
    box.appendChild(wait);

    return;
  }

  if (libraryError){
    const err = document.createElement("p");
    err.className = "sub";
    err.textContent = libraryError;
    box.appendChild(err);

    return;
  }

  const head = document.createElement("h3");
  head.className = "group";
  head.textContent = "Every model there is";
  box.appendChild(head);

  const note = document.createElement("p");
  note.className = "sub group-note";
  note.textContent = libraryModels.length + " to choose from, the ones this "
    + "machine can run first. Anything here installs the same way.";
  box.appendChild(note);

  const find = document.createElement("input");
  find.type = "text";
  find.placeholder = "Search by name — llama, qwen, vision, code…";
  find.value = librarySearch;
  find.oninput = () => { librarySearch = find.value; renderLibrary(); find.focus(); };
  box.appendChild(find);

  const want = librarySearch.trim().toLowerCase();

  const shown = libraryModels.filter(m => !want
    || m.name.toLowerCase().includes(want)
    || (m.what || "").toLowerCase().includes(want)
    || (m.can || []).some(c => c.toLowerCase().includes(want)));

  const list = document.createElement("div");
  list.className = "models";

  // A cap, with the count said plainly. Four hundred rows in one page is slow
  // to draw and no easier to read than the first fifty and a search box.
  shown.slice(0, 60).forEach(m => list.appendChild(libraryRow(m)));

  box.appendChild(list);

  if (shown.length > 60){
    const cut = document.createElement("p");
    cut.className = "sub";
    cut.textContent = "Showing 60 of " + shown.length + ". Search to narrow it.";
    box.appendChild(cut);
  }

  if (!shown.length){
    const none = document.createElement("p");
    none.className = "sub";
    none.textContent = "Nothing matches that.";
    box.appendChild(none);
  }
}

function libraryRow(m){
  const row = document.createElement("div");
  row.className = "model-row" + (m.installed ? " here" : "");

  const body = document.createElement("div");
  body.className = "model-body";

  const name = document.createElement("div");
  name.className = "model-name";
  name.textContent = m.name;

  (m.can || []).forEach(c => {
    const tag = document.createElement("span");
    tag.className = "tag quiet";
    tag.textContent = c;
    name.appendChild(tag);
  });

  if (!m.fits && m.needs_gb){
    const warn = document.createElement("span");
    warn.className = "tag warn";
    warn.textContent = "wants " + m.needs_gb + "GB";
    name.appendChild(warn);
  }

  body.appendChild(name);

  const detail = document.createElement("div");
  detail.className = "model-detail";
  detail.textContent = [m.what, m.pulls ? m.pulls + " pulls" : ""]
    .filter(Boolean).join(" · ");
  body.appendChild(detail);

  row.appendChild(body);

  if (m.installed){
    const here = document.createElement("span");
    here.className = "model-here";
    here.textContent = "installed";
    row.appendChild(here);
  } else {
    const b = document.createElement("button");
    b.className = "ghost";
    b.textContent = "Install";
    b.disabled = busy;
    b.onclick = () => pullModel(m.name);
    row.appendChild(b);
  }

  return row;
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

  // Which companies are done. Redrawn every time — it is text, it holds no
  // typing, and it is the one part of this card that changes.
  renderSavedKeys(state);

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

/*
 * Which kind of brain, before which one.
 *
 * Both were shown at once: eight local models, then the paid-API card, then a
 * button revealing four hundred more. For somebody who had decided to use a
 * paid service, all of that was a list of things they had already chosen not
 * to do — and the card they wanted sat underneath it.
 *
 * They are alternatives, so this asks which alternative first and shows only
 * that one. Switching back is one press, and having both is still allowed —
 * a key saved alongside a local model gives the router something to fall back
 * to — but it is a thing somebody does deliberately, not the default view.
 */
let brainWay = "";

function renderBrainChoice(state){
  const box = el("brain-section");
  box.textContent = "";

  const hasModel = state.requirements.some(r => r.name === "Chat model" && r.state === "ok");

  /*
   * Decided once, from what is already here.
   *
   * Somebody who has saved a key is on the paid path and should not have to
   * say so again; somebody with a model installed is on the local one. With
   * neither, local is the default, because it is the private answer and this
   * program's whole argument is that private is the better default.
   */
  if (!brainWay){
    if (state.has_api_key && !hasModel) brainWay = "paid";
    else brainWay = "local";
  }

  const h = document.createElement("h2");
  h.textContent = "Choose a brain";
  box.appendChild(h);

  const note = document.createElement("p");
  note.className = "sub";
  note.textContent = "It needs a language model to think with. Run one on this "
    + "machine, or send your messages to a paid service. You can change this "
    + "later, or have both.";
  box.appendChild(note);

  const pick = document.createElement("div");
  pick.className = "models";

  [
    {
      id: "local", title: "Run one here",
      why: state.hardware.can_local
        ? "Free and private. Nothing you say leaves this computer, and it "
          + "keeps working with the network unplugged."
        : "Free and private, but this machine has " + state.hardware.ram_gb
          + "GB of RAM — enough to work, slowly enough to be frustrating.",
    },
    {
      id: "paid", title: "Use a paid API",
      why: "Stronger answers and fast on any machine. It costs per use, and "
        + "your messages go to whichever company you choose.",
    },
  ].forEach(o => {
    const b = document.createElement("button");
    b.className = "ghost pick" + (brainWay === o.id ? " chosen" : "");

    const name = document.createElement("strong");
    name.textContent = o.title;

    // Marked on the local one, and only when this machine can actually do it.
    if (o.id === "local" && state.hardware.can_local){
      const tag = document.createElement("span");
      tag.className = "tag";
      tag.textContent = "private";
      name.appendChild(tag);
    }

    b.appendChild(name);

    const why = document.createElement("span");
    why.textContent = o.why;
    b.appendChild(why);

    b.onclick = () => { brainWay = o.id; refresh(); };

    pick.appendChild(b);
  });

  box.appendChild(pick);

  renderApiChoice(state);
  renderModelList(state);
}

/*
 * renderModelList is every model this can install, with what each costs.
 *
 * A list rather than two cards. Two was enough while the last step did the
 * installing and the question was only "which one" — but the question people
 * actually have is "what can I run on this machine", and a choice between
 * Quickest and Balanced answers a narrower one than it appears to.
 *
 * Everything is listed, including what will not fit: a list filtered down to
 * what fits cannot be told apart from a short list, and somebody wondering why
 * their machine is not offered the big one gets no answer from an absence. It
 * is marked and left choosable — it is their machine.
 */
/*
 * renderWelcome says what this is about to cost, before anything is fetched.
 *
 * The number nobody was given. Setup asked for a folder, a model and a
 * handful of permissions and only then, on the screen that downloads, said how
 * large any of it was — by which point somebody on a metered line had already
 * agreed to the shape of the thing. Said first instead, from the same sizes
 * the requirement cards carry, so it cannot drift from what actually happens.
 */
function renderWelcome(state){
  const box = el("welcome-cost");
  if (!box) return;

  box.textContent = "";

  const machine = document.createElement("p");
  machine.className = "sub";
  machine.textContent = machineLine(state.hardware);
  box.appendChild(machine);

  // The recommended path: what it cannot run without, plus the model it would
  // choose for this machine. Not the extras — those are a later decision and
  // counting them here would overstate the price of getting started.
  let total = 0;

  (state.requirements || []).forEach(r => {
    if (r.optional || r.state === "ok") return;
    if (r.name === "Chat model") return;

    total += sizeInGB(r.size);
  });

  const pick = (state.model_options || []).find(m => m.recommended);
  if (pick) total += sizeInGB(pick.size);

  if (total <= 0) return;

  const cost = document.createElement("p");
  cost.className = "sub";
  cost.textContent = "On this machine that is about "
    + (total < 1 ? Math.round(total * 1024) + "MB" : total.toFixed(1) + "GB")
    + " to download. You can choose a smaller brain, or use a paid service and "
    + "download almost nothing.";
  box.appendChild(cost);
}

function renderModelList(state){
  const box = el("models-section");
  if (!box) return;

  box.textContent = "";

  // Not on the paid path: these are the thing somebody there has chosen not
  // to do, and the library button below them opens four hundred more.
  const lib = el("library-section");

  if (brainWay !== "local"){
    if (lib) lib.textContent = "";

    return;
  }

  const options = state.model_options || [];
  if (!options.length) return;

  /*
   * No heading, and no second description.
   *
   * The card above already says "Run one here" and what it means, and this
   * repeated both a centimetre below it — the same title twice and two
   * sentences making the same promise. Once the choice became explicit, the
   * section under it stopped needing to introduce itself.
   *
   * What is left is the one thing the card does not say: that installing
   * several is allowed and which of them answers is decided later.
   */
  const note = document.createElement("p");
  note.className = "sub group-note";
  note.textContent = "Install as many as you like — you choose which one "
    + "answers later, and can add any of the others from inside the program.";
  box.appendChild(note);

  /*
   * Where they land and what is left there, once, above the list.
   *
   * Once rather than on every row: it is the same disk for all of them, and
   * eight rows each repeating the same free-space figure would be noise. It
   * is worth saying at all because it is not the drive chosen on the first
   * step — ollama keeps its models in its own directory wherever the brain
   * was put, which is the single most surprising thing on this page for
   * somebody who moved the brain to a big disk to hold them.
   */
  const room = (state.requirements || []).find(r => r.name === "Chat model");

  if (room && (room.where || room.free_gb)){
    const where = document.createElement("p");
    where.className = "sub group-note models-room";
    where.textContent = "They go to " + (room.where || "ollama's own folder")
      + (room.free_gb ? " — " + room.free_gb + "GB free there" : "");
    box.appendChild(where);
  }

  const list = document.createElement("div");
  list.className = "models";

  options.forEach(o => {
    const row = document.createElement("div");
    row.className = "model-row" + (o.installed ? " here" : "");

    const body = document.createElement("div");
    body.className = "model-body";

    const name = document.createElement("div");
    name.className = "model-name";
    name.textContent = o.label;

    if (o.recommended){
      const tag = document.createElement("span");
      tag.className = "tag";
      tag.textContent = "recommended";
      name.appendChild(tag);
    }

    if (!o.fits){
      /*
       * Said as a fact about the machine, not as a refusal.
       *
       * "Needs 32GB" is checkable and leaves the decision where it belongs.
       * "Too big" is this program deciding for somebody what their own
       * computer can do, on a guess about memory.
       */
      const warn = document.createElement("span");
      warn.className = "tag warn";
      warn.textContent = "wants " + o.needs_gb + "GB";
      name.appendChild(warn);
    }

    body.appendChild(name);

    const detail = document.createElement("div");
    detail.className = "model-detail";
    detail.textContent = o.model + " · " + o.size + " · " + o.speed;
    body.appendChild(detail);

    row.appendChild(body);

    if (o.installed){
      /*
       * Installed, and possibly the one that answers.
       *
       * Only offered as a choice when there is a choice to make: with one
       * model installed, "which should answer?" is a question with one answer
       * and the row just says it is here. The second model is what turns it
       * into a decision.
       */
      const several = (state.model_options || []).filter(m => m.installed).length > 1;
      const answering = state.chosen_model === o.model
        || state.chosen_model === o.model + ":latest";

      if (!several){
        const here = document.createElement("span");
        here.className = "model-here";
        here.textContent = answering ? "answers" : "installed";
        row.appendChild(here);
      } else {
        const b = document.createElement("button");
        b.className = "ghost pick" + (answering ? " chosen" : "");
        b.style.flex = "0 0 84px";
        b.style.width = "84px";
        b.textContent = answering ? "answers" : "use this";
        b.disabled = busy;
        b.onclick = async () => {
          await post("/chosen-model", {model: o.model});
          refresh();
        };
        row.appendChild(b);
      }
    } else {
      const b = document.createElement("button");
      b.className = "ghost";
      b.textContent = "Install";
      b.disabled = busy;
      b.onclick = () => pullModel(o.model);
      row.appendChild(b);
    }

    list.appendChild(row);

    /*
     * A model's progress goes under its own row.
     *
     * Outside the row rather than inside it: the row is a line of flex boxes
     * across, and a progress bar is a block below. Wrapped so it still reads
     * as belonging to the row above it.
     */
    if (whatIsInstalling(state) === "model:" + o.model){
      row.classList.add("working");

      const under = document.createElement("div");
      under.className = "model-progress";
      under.appendChild(progressBlock(state));
      list.appendChild(under);
    }
  });

  box.appendChild(list);

  /*
   * And the rest of them, for anybody who asks.
   *
   * Eight is the right number to show first and the wrong number to show
   * only: the honest answer to "are those the only ones?" is no, and a list
   * of eight that does not say so has answered with a smaller truth. Behind a
   * press because a few hundred entries is not a choice, it is a search — and
   * because fetching it reaches the network, which is somebody's decision to
   * make rather than a page's.
   */
  const more = el("library-section");
  if (!more) return;

  more.textContent = "";
  more.id = "library-section";

  if (!libraryShown){
    const open = document.createElement("button");
    open.className = "ghost linky-btn";
    open.textContent = "Show every model there is";
    open.onclick = () => { libraryShown = true; loadLibrary(); };
    more.appendChild(open);

    return;
  }

  renderLibrary();
}

/*
 * renderSavedKeys says which companies already have a key here.
 *
 * Names, never keys. It answers the question somebody has after saving one —
 * "did that work, and can I add another" — which the card could not answer
 * before because it vanished at exactly that moment.
 */
function renderSavedKeys(state){
  const box = el("api-saved");
  if (!box) return;

  const names = {anthropic: "Anthropic", openai: "OpenAI", openrouter: "OpenRouter"};
  const held = state.api_keys || [];

  box.textContent = "";

  if (!held.length) return;

  held.forEach(id => {
    const row = document.createElement("div");
    row.className = "api-held";
    row.textContent = (names[id] || id) + " — key saved";
    box.appendChild(row);
  });

  const more = document.createElement("p");
  more.className = "sub";
  more.textContent = held.length === 1
    ? "You can add another company below — it keeps one key for each."
    : "Choose which one answers in Settings, at any time.";
  box.appendChild(more);
}

/*
 * renderApiChoice builds the paid-API card once and then leaves it alone.
 *
 * It used to be rebuilt with everything else, and everything else is rebuilt
 * every two seconds by the poll. A <select> and a password field cannot
 * survive that: the dropdown was destroyed under the cursor mid-choice, the
 * provider snapped back to the first one, and a key being typed was wiped
 * between one character and the next. Choosing OpenAI and saving a key for it
 * was not difficult, it was impossible — the control was replaced before the
 * choice could be used.
 *
 * The folder field on the first step already solved this and said why: the
 * field is left alone so it does not fight whoever is typing in it. This is
 * the same rule, applied to the other two controls somebody has to type into.
 */
function renderApiChoice(state){
  const box = el("api-section");
  if (!box) return;

  const hasModel = state.requirements.some(r => r.name === "Chat model" && r.state === "ok");

  /*
   * Shown even once a key is saved, because one is not the limit.
   *
   * It used to hide itself the moment any key existed, which made saving a
   * second one impossible: the card that saves keys disappeared as soon as
   * the first was saved. The settings file has always had a separate place
   * for each company, so holding all three was supported everywhere except
   * the one screen where they are entered.
   *
   * It still goes away once a model runs here and no key has been saved —
   * somebody who chose a local brain does not need a paid one explained at
   * them — but never once there is something in it to see.
   */
  /*
   * Shown on the paid path, and once there is a key to see.
   *
   * It used to hide itself the moment any key existed, which made saving a
   * second one impossible: the card that saves keys disappeared as soon as
   * the first was saved. The settings file has always had a separate place
   * for each company — holding all three was supported everywhere except the
   * one screen where they are entered.
   */
  box.hidden = brainWay !== "paid" && !(state.api_keys || []).length;

  if (box.dataset.ready === "1"){
    /*
     * Only what follows the machine, and only when it is not what somebody is
     * looking at. Recommending the API on a machine that cannot run a model
     * is a fact about the machine and it does not change while setup is open;
     * re-reading it here would be an excuse to touch the card again.
     */
    return;
  }

  box.dataset.ready = "1";

  const api = document.createElement("div");
  api.className = "choice" + (state.hardware.can_local ? "" : " rec");
  /*
   * The title and the pitch live on the choice card above. What is left here
   * is the one fact that card cannot carry, because it is about this machine
   * rather than about the choice: the key does not leave it.
   */
  const ap = document.createElement("p");
  ap.className = "sub group-note";
  ap.textContent = "The key is stored only on this computer.";
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
  /*
   * Every company, from the one list the rest of the program uses.
   *
   * This used to be three of them written out here, which is why there were
   * three: the picker, the settings file, the key check and the router each
   * held their own copy, and adding a company meant finding all four. Almost
   * all of them speak the same protocol as OpenAI, which is the only reason
   * the list can be long — a company is a name and an address, not a client
   * to write.
   */
  const services = state.services || [];

  const choose = document.createElement("select");

  services.forEach(sv => {
    const opt = document.createElement("option");
    opt.value = sv.id;
    opt.textContent = sv.name;
    choose.appendChild(opt);

  });

  api.appendChild(choose);

  const saved = document.createElement("div");
  saved.id = "api-saved";
  api.appendChild(saved);

  const where = document.createElement("p");
  where.className = "sub";
  api.appendChild(where);

  const input = document.createElement("input");
  input.type = "password";
  api.appendChild(input);

  /*
   * A field for the address, shown only for the entry that needs one.
   *
   * Everything else has a fixed endpoint and asking for it would be asking
   * somebody to type a URL this program already knows.
   */
  const addr = document.createElement("input");
  addr.type = "text";
  addr.placeholder = "http://localhost:1234/v1";
  addr.hidden = true;
  api.insertBefore(addr, input);

  const describe = () => {
    const sv = services.find(x => x.id === choose.value) || services[0];
    if (!sv) return;

    addr.hidden = !sv.custom;
    input.hidden = sv.needs_key === false && !sv.custom;

    // The prefix as the hint, where the company commits to one. Several do
    // not, and inventing an example would teach somebody to distrust a key
    // that is perfectly good.
    input.placeholder = sv.prefix ? sv.prefix + "…" : "your key";

    where.textContent = sv.custom
      ? sv.note + " Give its address above; most need no key."
      : "Get a key at " + sv.where + (sv.note ? " — " + sv.note : "");
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
      body: JSON.stringify({
        key: input.value,
        provider: choose.value,
        base_url: addr.hidden ? "" : addr.value,
      })
    }).then(r => r.json());
    if (!res.ok){ err.textContent = res.error; err.hidden = false; return; }

    // The key goes, the company stays: somebody saving a second key is more
    // likely to want the picker where they left it than reset to the first.
    input.value = "";
    refresh();
  };
  api.appendChild(ab);
  box.appendChild(api);
}


/*
 * install and pullModel start the work and let the poll report it.
 *
 * busy is set here rather than waited for: the server sets its own the moment
 * the request lands, but the next poll is up to two seconds away, and a row
 * whose button stays live for two seconds after being pressed gets pressed
 * again. Neither touches the log any more — progressBlock owns the panel and
 * shows it whenever there is something to show, on whichever step is doing it.
 */
async function install(name){
  // Updating is the same request; the server re-runs the installer, which
  // fetches the current release and puts it over what is there.
  busy = true;
  refresh();

  await fetch("/install?name=" + encodeURIComponent(name));
  refresh();
}

async function pullModel(model){
  busy = true;
  refresh();

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
/*
 * The steps, in the order the work has to happen in.
 *
 * "What it needs" comes before the models now, and that is the whole of the
 * reordering: a model cannot be downloaded before the thing that runs models
 * exists. While everything installed at the end, the order of the steps was
 * only the order of the questions and either arrangement read fine. Now that
 * each step installs what it lists, the steps are the order of the work, and
 * asking somebody to choose a model on a machine with no Ollama would offer
 * them a button that could not work.
 */
/*
 * The steps, in the order the answers depend on each other.
 *
 * "What it needs" used to come first, which put the demand before the decision:
 * somebody intending to use a paid service was told to install Ollama and an
 * embedding model in order to reach the screen where they say they want
 * neither. That was patched with a button to escape it, which is evidence the
 * order was wrong rather than a fix.
 *
 * So the fork comes first and everything after is filtered by the answer. What
 * counts as required on the needs step is now a consequence of how somebody
 * said it should think, and nothing irrelevant is ever demanded.
 */
const STEPS = [
  {id: "welcome", title: "Welcome", shown: () => true, blocks: () => ""},

  {id: "brain", title: "How it thinks", shown: () => true,
   blocks: st => {
     // Something to think with, and it has to be here rather than chosen:
     // this step installs what it lists.
     const localHere = (st.requirements || [])
       .some(r => r.name === "Chat model" && r.state === "ok");

     if (localHere || st.has_api_key) return "";

     return "Install a model to run here, or save a key for a paid API.";
   }},

  {id: "where", title: "Where it lives",
   shown: st => (st.drives || []).length > 1,
   /*
    * Never blocks: there is always a sensible default, and the step exists to
    * let somebody change it rather than to demand an answer they may not have
    * an opinion about.
    */
   blocks: () => ""},

  {id: "needs", title: "What it needs", shown: () => true,
   blocks: st => {
     /*
      * Required is a consequence of the answer two steps back.
      *
      * Ollama runs models on this machine and the embedding model is one of
      * them, so both exist to serve a local brain. On the paid path they are
      * worth having and are not needed — the step says so and lets somebody
      * through, rather than barring the way over a thing they chose not to
      * use.
      *
      * Read from brainWay rather than from whether a key happens to exist:
      * somebody who has chosen the paid path and not yet pasted a key is
      * still on the paid path, and should not be told to install a gigabyte
      * and a half on the way to the box they are about to type into.
      */
     if (brainWay === "paid") return "";

     const missing = (st.requirements || [])
       .filter(r => !r.optional && r.state !== "ok" && r.name !== "Chat model")
       .map(r => r.name);

     if (!missing.length) return "";

     return "Still to install: " + missing.join(", ") + ".";
   }},

  /*
   * Name and voice, after the install rather than beside the other questions.
   *
   * The two human voices do not exist until piper does, and a picker offering
   * buttons that cannot play is asking somebody to choose between things they
   * cannot hear.
   *
   * Never blocks: it has a name already, and keeping the one it came with is
   * a perfectly good answer from somebody who does not want to give it another.
   */
  {id: "name", title: "Name and voice", shown: () => true, blocks: () => ""},

  {id: "privacy", title: "What it asks you about", shown: () => true,
   /*
    * Never blocks. Every rule starts switched on, so somebody who reads none
    * of it and presses Next has the protective answer — which is the only
    * defensible default for a question about credentials.
    */
   blocks: () => ""},

  {id: "apply", title: "Ready", shown: () => true, blocks: () => ""},
];

// Which step is on screen. Moved by Back and Next, and once at the start by
// placeAtFirstUnfinished.
let step = 0;

/*
 * Where to open, which is not always the beginning.
 *
 * Once, on the first load. After that the step is wherever the person has
 * navigated to, and moving it under them as requirements are satisfied would
 * take the page out of their hands.
 */
let placed = false;

function visibleSteps(state){ return STEPS.filter(s => s.shown(state)); }


function placeAtFirstUnfinished(state){
  if (placed) return;

  placed = true;

  /*
   * A first run starts at the first step. Every time.
   *
   * This used to open on the first step that *blocked*, which is a different
   * thing and quietly skipped the most consequential question on the page:
   * "Where to keep it" does not block, because there is always a default
   * folder — so somebody setting this up for the first time was put on step
   * two, and the drive their assistant would live on was decided for them by
   * a step they never saw. On a machine with a small home disk and a large
   * external one, that is the wrong answer chosen silently.
   *
   * Guarded by "placed" like everything else here: deciding this on every
   * poll would drag somebody back to step one every two seconds for as long
   * as anything remained uninstalled. No backticks in this file, either —
   * the whole page is one Go raw string and a backtick ends it.
   */
  if (state.blocking > 0){
    step = 0;

    return;
  }

  const shown = visibleSteps(state);

  for (let i = 0; i < shown.length; i++){
    if (shown[i].blocks && shown[i].blocks(state)){
      step = i;

      return;
    }
  }

  step = shown.length - 1;
}


/*
 * What it will stop and ask about, offered as a choice rather than announced.
 *
 * Each rule says what it covers and why, because "/.aws/" is not a thing
 * anybody can make a decision about — what is in it is the decision. Every one
 * starts on, so the answer for somebody who skips this step is the protective
 * one.
 */
/*
 * The one switch, in the words the program uses for it.
 *
 * Copied from Permissions rather than paraphrased: somebody who picks
 * "never stop" here and later opens that page should find the same sentence
 * beside the same choice, not a second description that might mean something
 * slightly different.
 */
const FREEDOM = [
  {level: "ask", name: "Ask me first — and nothing leaves this machine",
   means: "It asks before anything that changes something, and nothing leaves this "
     + "machine — no hosted model, no web."},
  {level: "granted", name: "Do what I have allowed — the web is open, the model stays here",
   means: "It does what you have already allowed and asks about the rest. The web "
     + "is open; the model answering you stays on this machine."},
  {level: "everything", name: "Never stop, never refuse — hosted models, the web, memory, all of it",
   means: "It does anything it can, without asking, and nothing is held back: "
     + "hosted models, the web, and what it has learned about you may all be sent. "
     + "Everything is still recorded."},
];

// Set while a choice is on its way, so a poll landing in between does not
// put the tick back on the old answer and make the click look ignored.
let freedomSaving = "";

function renderFreedomChoice(state){
  const box = el("freedom-pick");
  if (!box) return;

  const chosen = freedomSaving || state.freedom || "ask";

  box.textContent = "";

  FREEDOM.forEach(f => {
    const pick = document.createElement("button");
    pick.type = "button";
    pick.className = "ghost pick" + (chosen === f.level ? " chosen" : "");
    pick.disabled = freedomSaving !== "";

    const name = document.createElement("strong");
    name.textContent = f.name;

    const means = document.createElement("span");
    means.textContent = f.means;

    pick.append(name, means);

    pick.onclick = async () => {
      freedomSaving = f.level;
      renderFreedomChoice(state);

      try {
        await post("/freedom", {level: f.level});
      } finally {
        freedomSaving = "";
        refresh();
      }
    };

    box.appendChild(pick);
  });

  const files = el("freedom-files");

  if (files){
    files.textContent = chosen === "everything"
      ? "With the switch on never stop, these files do not stop it either. Reading "
        + "one is written down with the reason it would have asked, and nothing waits "
        + "for you."
      : "These are the files where it stops and puts the request to you first, "
        + "whatever else you have allowed — and it remembers what you answer, so it "
        + "asks once rather than every time.";
  }
}

function renderPrivacyChoice(state){
  renderFreedomChoice(state);

  const box = el("privacy-rules");

  if (!box || !state.protection) return;

  const off = new Set(state.protection.off || []);

  box.textContent = "";

  (state.protection.rules || []).forEach(rule => {
    const row = document.createElement("label");
    row.className = "field-check";

    const tick = document.createElement("input");
    tick.type = "checkbox";
    tick.checked = rule.fixed || !off.has(rule.id);
    tick.disabled = rule.fixed;

    const text = document.createElement("span");
    text.innerHTML = "<b>" + rule.what + "</b>" +
      (rule.fixed ? " <span class=\"muted\">always</span>" : "") +
      "<br><span class=\"muted\">" + rule.why + "</span>";

    tick.onchange = async () => {
      /*
       * Read at click time, not at draw time.
       *
       * This used to build its new list from the set captured when the row was
       * drawn, and the server replaces the whole list rather than merging. So
       * two quick clicks on different rules both started from the same stale
       * picture and the second silently undid the first — on the one screen in
       * the program where a setting quietly not taking is a file left
       * unprotected.
       */
      const live = new Set((lastState && lastState.protection
        && lastState.protection.off) || off);

      if (tick.checked) live.delete(rule.id); else live.add(rule.id);

      await post("/protection", {off: [...live]});
      refresh();
    };

    row.append(tick, text);
    box.appendChild(row);
  });

  const yours = el("privacy-yours");

  if (yours){
    yours.textContent = "";

    (state.protection.yours || []).forEach(own => {
      const row = document.createElement("div");
      row.className = "chosen";
      row.textContent = own;

      const drop = document.createElement("button");
      drop.type = "button";
      drop.className = "linky";
      drop.textContent = "remove";
      drop.onclick = async () => {
        await post("/protection", {
          yours: (state.protection.yours || []).filter(y => y !== own),
        });
        refresh();
      };

      row.appendChild(drop);
      yours.appendChild(row);
    });
  }

  const add = el("privacy-add");

  if (add && !add.dataset.wired){
    add.dataset.wired = "1";

    add.onkeydown = async e => {
      if (e.key !== "Enter") return;

      e.preventDefault();

      const what = add.value.trim();

      if (!what) return;

      add.value = "";
      await post("/protection", {yours: [...(state.protection.yours || []), what]});
      refresh();
    };
  }
}


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


/*
 * Where an install has got to, drawn wherever it belongs.
 *
 * It used to be one panel at the foot of the page. That reads fine while the
 * page is short and badly once it is not: installing Ollama from a list of
 * twelve pieces put the bar below all twelve, so the thing being installed was
 * scrolled off the top while a bar at the bottom said 503MB of 1.3GB, and
 * nothing on screen connected the two.
 *
 * So it is built here and placed by the caller — inside the card of the piece
 * being installed, and at the foot only when whatever is installing is not on
 * screen to hold it.
 */
/*
 * Whether the log is folded open, remembered across redraws.
 *
 * The card is rebuilt every two seconds while something installs, and a fresh
 * <details> starts closed — so opening the technical details lasted until the
 * next poll and then shut itself, over and over. The same shape of bug as the
 * provider picker that could not be chosen from: state living in a DOM node
 * that something else is busy replacing.
 */
let logOpen = false;

function progressBlock(state){
  const box = document.createElement("div");
  box.className = "prog";

  const done = !state.busy;

  const head = document.createElement("div");
  head.className = "prog-head";

  const what = document.createElement("span");
  what.id = "";
  what.className = "prog-what";
  what.textContent = done
    ? (state.apply_failed ? "Stopped part way" : "Done")
    : (state.step_name || "Working…");
  head.appendChild(what);

  const pct = document.createElement("span");
  pct.className = "prog-pct";
  head.appendChild(pct);

  box.appendChild(head);

  const track = document.createElement("div");
  track.className = "prog-track";

  const bar = document.createElement("div");
  bar.className = "prog-bar";
  track.appendChild(bar);
  box.appendChild(track);

  const percent = state.percent;

  if (done){
    bar.style.width = state.apply_failed ? "0%" : "100%";
  } else if (percent >= 0){
    pct.textContent = percent + "%";
    bar.style.width = percent + "%";
  } else {
    // Running, position unknown — a moving stripe rather than a bar at zero.
    // "Starting" and "no idea how far" are different things.
    bar.className = "prog-bar unknown";
  }

  /*
   * The last line that says something, not the last line.
   *
   * cmake and ollama both end their output with blank lines and redraw codes,
   * so "the last line" was regularly empty — a note appearing and vanishing
   * several times a second while the rest of the panel sat still.
   */
  const lines = (state.log || "").replace(/\r/g, "\n").split("\n")
    .map(l => l.replace(/\u001b\[[0-9;?]*[A-Za-z]/g, "").trim())
    .filter(l => l && !l.startsWith("["));

  if (!done && lines.length){
    const note = document.createElement("p");
    note.className = "sub prog-note";
    note.textContent = lines[lines.length - 1];
    box.appendChild(note);
  }

  const fold = document.createElement("details");
  fold.className = "log-wrap";
  fold.open = logOpen;
  fold.ontoggle = () => { logOpen = fold.open; };

  const sum = document.createElement("summary");
  sum.textContent = "Show the technical details";
  fold.appendChild(sum);

  const pre = document.createElement("pre");
  pre.textContent = state.log || "";
  fold.appendChild(pre);

  // Scrolled to the end once it is in the document, which is the only moment
  // the height is known. A live log is read at the bottom, and this one is
  // replaced wholesale every couple of seconds, so there is no scroll position
  // of somebody's own to preserve.
  setTimeout(() => { pre.scrollTop = pre.scrollHeight; }, 0);

  box.appendChild(fold);

  return box;
}

/*
 * whatIsInstalling names the card the progress belongs in.
 *
 * The raw step name from the server: "Ollama", or "model:qwen2.5-coder:7b".
 * Empty when nothing is running.
 */
function whatIsInstalling(state){
  return state.busy ? (state.step_target || "") : "";
}

/*
 * renderFootProgress is the fallback, for work no card on screen is showing.
 *
 * Installing the menu entry belongs to no card; so does anything started on a
 * step somebody has since walked away from. Without this they would run with
 * nothing on screen saying so, which is the state this whole panel exists to
 * prevent — but with it shown unconditionally, a card and the foot would draw
 * the same bar twice.
 */
function renderFootProgress(state){
  const box = el("progress");
  if (!box) return;

  box.textContent = "";

  if (!state.log){
    box.hidden = true;

    return;
  }

  // Claimed by a card already on screen? Then it is being shown there.
  const target = whatIsInstalling(state);

  if (target && document.querySelector(".card.working, .model-row.working")){
    box.hidden = true;

    return;
  }

  box.hidden = false;
  box.appendChild(progressBlock(state));
}

function renderOverview(state){
  const box = el("overview");
  box.textContent = "";

  const rows = [];

  rows.push(["Everything it learns", state.chosen_drive || "the default folder", true]);

  /*
   * What is here, and what is not.
   *
   * This used to list what pressing Apply was about to do, which was the right
   * thing to show while the last step did the installing. Nothing is pending
   * by the time somebody reaches this step now, so the question it answers has
   * changed from "what will happen" to "what happened" — and the answer worth
   * having includes the optional pieces that were skipped, because otherwise
   * the only way to find out that there is no voice is to notice the silence.
   */
  const here = (state.requirements || []).filter(r => r.state === "ok");
  const skipped = (state.requirements || []).filter(r => r.state !== "ok");

  here.forEach(r => rows.push([r.name, r.where || "—", false]));

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

  if (skipped.length){
    const note = document.createElement("p");
    note.className = "ov-note";
    note.textContent = "Not installed: " + skipped.map(r => r.name).join(", ")
      + ". You can add any of them later from inside the program.";
    box.appendChild(note);
  }
}

/*
 * The readiness ticks: what was checked, rather than what was installed.
 *
 * Fetched on its own schedule, not from /state — two of these ask a model a
 * question, and /state is read every two seconds by a page that must stay
 * responsive. The server answers immediately with whatever the slow ones have
 * reached, so this never waits either.
 */
let readyTicks = null;
let readyFetchedFor = "";

function renderReadyTicks(state){
  const box = el("ready-ticks");
  if (!box) return;

  // Re-asked when an install finishes, because that is exactly when the
  // answer changes — and not on every poll, because two of them are slow.
  const key = String(state.busy) + ":" + step;

  if (readyFetchedFor !== key){
    readyFetchedFor = key;

    get("/ready").then(got => { readyTicks = got.ticks || []; drawTicks(); })
      .catch(() => {});
  }

  drawTicks();
}

function drawTicks(){
  const box = el("ready-ticks");
  if (!box || !readyTicks) return;

  box.textContent = "";

  const h = document.createElement("h3");
  h.className = "group";
  h.textContent = "Checked, not assumed";
  box.appendChild(h);

  readyTicks.forEach(t => {
    const row = document.createElement("div");
    row.className = "tick-row " + t.state;

    const mark = document.createElement("span");
    mark.className = "mark";
    mark.textContent = t.state === "yes" ? "✓"
      : t.state === "no" ? "✗"
      : t.state === "checking" ? "…" : "!";
    row.appendChild(mark);

    const what = document.createElement("span");
    what.className = "tick-what";
    what.textContent = t.what;
    row.appendChild(what);

    const note = document.createElement("span");
    note.className = "tick-note";
    note.textContent = t.note || "";
    row.appendChild(note);

    box.appendChild(row);
  });

  const again = document.createElement("button");
  again.className = "ghost linky-btn";
  again.textContent = "Check again";
  again.onclick = async () => {
    readyTicks = null;
    readyFetchedFor = "";
    await post("/ready", {});
    refresh();
  };
  box.appendChild(again);
}

/*
 * The name and the language, which are the only typed answers in the wizard.
 *
 * identityTouched plus focus, rather than the build-once dataset.ready used by
 * the folder and API cards. Build-once means never updating, which is right
 * for a card whose only live content is a span and wrong here: this step sits
 * beside a voice picker that changes as piper arrives, and freezing the pair
 * would leave "not yet" beside two voices that had finished installing.
 *
 * The guard is the running program's own — a flag plus contains(activeElement)
 * — so the poll never writes over somebody mid-word, and never over a change
 * they made and clicked away from without saving.
 */
let identity = null;
let identityTouched = false;
let identitySaid = "";

function identityBeingEdited(){
  const form = el("name-form");

  if (!form) return false;

  return identityTouched || form.contains(document.activeElement);
}

function renderIdentity(state){
  const box = el("name-form");
  if (!box) return;

  if (identity === null){
    identity = {};

    get("/identity").then(got => { identity = got; renderIdentity(state); })
      .catch(() => {});

    return;
  }

  if (!identity.languages) return;

  // Never under somebody's hands. See identityBeingEdited.
  if (identityBeingEdited()) return;

  box.textContent = "";

  const nameRow = document.createElement("div");
  nameRow.className = "folder-row";

  const name = document.createElement("input");
  name.type = "text";
  name.id = "identity-name";
  name.value = identity.name || identity.default_name || "";
  name.placeholder = identity.default_name || "Assistant";
  name.oninput = () => { identityTouched = true; };
  name.onkeydown = e => { if (e.key === "Enter") saveIdentity(); };
  nameRow.appendChild(name);

  const save = document.createElement("button");
  save.className = "ghost";
  save.textContent = "Save";
  save.onclick = () => saveIdentity();
  nameRow.appendChild(save);

  box.appendChild(nameRow);

  const wake = document.createElement("p");
  wake.className = "sub";
  wake.textContent = "Say this word and it starts listening. "
    + "You can add other things it answers to later.";
  box.appendChild(wake);

  const lh = document.createElement("h3");
  lh.className = "group";
  lh.textContent = "The language you speak";
  box.appendChild(lh);

  const ln = document.createElement("p");
  ln.className = "sub group-note";
  ln.textContent = "Told rather than guessed. Left to guess at a language it "
    + "is unsure of, it translates instead of writing down what you said — so "
    + "you get an answer in English about something you did not ask.";
  box.appendChild(ln);

  const pick = document.createElement("select");

  identity.languages.forEach(l => {
    const o = document.createElement("option");
    o.value = l.code;
    o.textContent = l.name;
    pick.appendChild(o);
  });

  pick.value = identity.language || "";
  pick.onchange = () => {
    identity.language = pick.value;
    saveIdentity();
  };

  box.appendChild(pick);

  if (identitySaid){
    const said = document.createElement("p");
    said.className = "sub";
    said.textContent = identitySaid;
    box.appendChild(said);
  }
}

async function saveIdentity(){
  const field = el("identity-name");
  const chosen = identity || {};

  const res = await post("/identity", {
    name: field ? field.value : chosen.name,
    language: chosen.language || "",
  });

  identityTouched = false;

  if (res && res.ok === false){
    identitySaid = res.error || "That could not be saved.";
  } else {
    identitySaid = "Saved.";

    // Re-read, so the wake word the server derived is what is shown.
    identity = null;
  }

  refresh();
}

/*
 * The voice picker, on the last step.
 *
 * voiceFetchedFor is what the list was last fetched for: the voices appear
 * partway through the install, so a list read once at the top of setup would
 * still say "not yet" beside a voice that had finished downloading two
 * minutes earlier.
 */
let voiceState = null;
let voiceFetchedFor = null;
let voiceSpeaking = "";
let voiceNote = "";

function renderVoices(state){
  const speaking = (state.requirements || []).find(r => r.name === "Voice (speaking)");
  const key = (speaking ? speaking.state + ":" + speaking.detail : "none") + ":" + state.busy;

  if (voiceFetchedFor !== key){
    voiceFetchedFor = key;
    get("/voices").then(v => { voiceState = v; drawVoices(); }).catch(() => {});
  }

  drawVoices();
}

function drawVoices(){
  const box = el("voice-pick");
  if (!box) return;

  box.textContent = "";

  const voices = (voiceState && voiceState.voices) || [];
  if (!voices.length) return;

  const h = document.createElement("h3");
  h.textContent = "How it will sound";
  box.appendChild(h);

  const p = document.createElement("p");
  p.className = "sub";
  p.textContent = "Press Hear it to listen to each one, then pick the one you want. "
    + "You can change this later in Settings.";
  box.appendChild(p);

  voices.forEach(v => {
    const row = document.createElement("div");
    row.className = "voice-row";

    const pick = document.createElement("button");
    pick.className = "ghost pick" + (voiceState.chosen === v.kind ? " chosen" : "");
    pick.disabled = !v.available || voiceSpeaking !== "";
    pick.onclick = () => chooseVoice(v.kind);

    const name = document.createElement("strong");
    name.textContent = v.name;
    pick.appendChild(name);

    const detail = document.createElement("span");
    detail.textContent = v.detail;
    pick.appendChild(detail);

    row.appendChild(pick);

    const hear = document.createElement("button");
    hear.className = "ghost hear";
    hear.textContent = voiceSpeaking === v.kind ? "speaking…"
      : (v.available ? "▶ Hear it" : "not yet");
    hear.disabled = !v.available || voiceSpeaking !== "";
    hear.onclick = () => hearVoice(v.kind);
    row.appendChild(hear);

    box.appendChild(row);
  });

  if (voiceNote){
    const n = document.createElement("p");
    n.className = "sub";
    n.textContent = voiceNote;
    box.appendChild(n);
  }
}

/*
 * One voice at a time.
 *
 * The request only returns once the words have finished, so every button
 * stays disabled for exactly as long as something is talking. Two samples
 * playing over each other would be the one thing this feature exists to
 * prevent somebody hearing.
 */
async function hearVoice(kind){
  voiceSpeaking = kind;
  voiceNote = "";
  drawVoices();

  try {
    const r = await (await fetch("/hear?kind=" + encodeURIComponent(kind),
      {method: "POST"})).json();

    if (!r.ok) voiceNote = r.error || "That voice could not speak.";
  } catch (e){
    voiceNote = "That voice could not speak.";
  }

  voiceSpeaking = "";
  drawVoices();
}

async function chooseVoice(kind){
  try {
    const r = await (await fetch("/voice?kind=" + encodeURIComponent(kind),
      {method: "POST"})).json();

    if (r.ok){
      if (voiceState) voiceState.chosen = kind;
      voiceNote = "";
    } else {
      voiceNote = r.error || "That voice could not be chosen.";
    }
  } catch (e){
    voiceNote = "That voice could not be chosen.";
  }

  drawVoices();
}

/*
 * The most recent answer from the server, for handlers that fire between
 * polls and must not act on the picture they were drawn with.
 */
let lastState = null;

async function refresh(){
  const state = await get("/state");
  lastState = state;
  busy = state.busy;

  el("machine").textContent = machineLine(state.hardware);
  /*
   * The fork first, then everything that depends on the answer.
   *
   * renderBrainChoice is what sets brainWay, and the needs step's gate reads
   * it. Run the other way round, the very first paint judged the requirements
   * against an unanswered question — so somebody who had chosen a paid
   * service saw "Without these it cannot answer at all" for a frame before it
   * corrected itself.
   */
  renderBrainChoice(state);
  renderRequirements(state);
  renderDriveChoice(state);
  renderWelcome(state);
  renderFolderChoice(state);
  renderPrivacyChoice(state);
  renderOverview(state);
  renderReadyTicks(state);
  renderIdentity(state);
  renderVoices(state);
  renderFootProgress(state);
  placeAtFirstUnfinished(state);
  renderSteps(state);

  const hasBrain = state.requirements.some(r => r.name === "Chat model" && r.state === "ok")
    || state.has_api_key;
  const ready = state.blocking === 0 && hasBrain && !busy;

  el("continue").disabled = !ready;
  /*
   * A failed apply outranks the requirement count.
   *
   * "Everything is ready" is derived from how many blocking requirements are
   * left, which an apply that stopped on an optional one leaves at zero — so
   * the page said everything was ready immediately below the word "Failed".
   */
  el("status").textContent = busy
    ? "Working…"
    : (state.apply_failed
        ? "That did not finish — see the log above."
        : (ready ? "Everything is ready." : (state.blocking + " requirement(s) still needed")));

  el("status").classList.toggle("bad", !busy && !!state.apply_failed);
}


el("continue").onclick = async () => {
  el("continue").disabled = true;
  el("status").textContent = "Starting your assistant…";
  await fetch("/done");
};

refresh();
setInterval(refresh, 2000);
</script>
<!--
    The steps, put into the world.

    In the order setup goes through them rather than the order they are
    written in, so the next step is the one beside this one and going on is a
    short turn. A module on its own, after everything else: the page works
    completely without it, and a machine with no WebGL simply never gets past
    the first line of it.
-->
<script type="module">
import { startStage } from "/stage/stage.js";

const titles = new Map((typeof STEPS === "undefined" ? [] : STEPS)
  .map(s => [document.getElementById("step-" + s.id), s.title]));

startStage({
  frame: document.getElementById("step-stage"),
  panels: [...titles.keys()],
  title: box => titles.get(box) || "",
});
</script>
</body>
</html>`
