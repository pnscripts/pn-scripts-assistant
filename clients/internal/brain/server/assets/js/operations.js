/*
 * The operations view: what this machine is doing, in detail.
 *
 * Built from the layout somebody watching a machine assembles for themselves
 * out of terminals — the cards, the processors, what is running, the service
 * they care about — on the grounds that the only reason it is four terminals
 * is that nothing offered it in one place.
 *
 * Two rules run through the drawing. Nothing is invented: a figure the machine
 * does not publish is drawn as a gap and said as "not reported", never as a
 * zero, because zero and unknown look identical on a bar and mean opposite
 * things. And nothing is identified by colour alone: every series is named in
 * a legend and every value is written out, so the picture survives a
 * colour-blind reader and a monochrome screenshot.
 */
(function () {

/** How often the view refreshes. Every second while it is on screen, because
 *  that is the interval the readings are taken at; and not at all while it is
 *  not, since drawing a chart nobody is looking at is pure heat. */
const TICK = 1000;

// Every word a person reads here is in i18n.js, in English and Bulgarian.
const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
const bg = () => !!(window.pnLang && window.pnLang() === 'bg');

// A string the server names by key, or what it said when there is none.
function said(key, fallback, vars) {
    const text = T(key, vars);

    return text === key ? fallback : text;
}

// A figure with the decimal point a reader of the page expects.
function decimal(text) {
    return bg() ? String(text).replace(/(\d)\.(\d)/g, '$1,$2') : String(text);
}

// Text from somewhere else, made safe for the markup this file builds.
function escape(text) {
    return String(text == null ? '' : text)
        .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

const SERIES_A = 'var(--series-a)';
const SERIES_B = 'var(--series-b)';

function el(id) {
    return document.getElementById(id);
}

function visible() {
    const view = document.querySelector('.view[data-view="operations"]');

    return view && !view.hidden;
}

/* ---------- saying numbers the way people say them ---------- */

function bytes(n) {
    if (!isFinite(n) || n < 0) return '—';

    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0;

    while (n >= 1024 && i < units.length - 1) {
        n /= 1024;
        i += 1;
    }

    return decimal(`${n < 10 && i > 0 ? n.toFixed(1) : Math.round(n)}${units[i]}`);
}

function rate(n) {
    return isFinite(n) && n >= 0 ? `${bytes(n)}/s` : '—';
}

function percent(n, places = 0) {
    return isFinite(n) && n >= 0 ? decimal(`${n.toFixed(places)}%`) : '—';
}

// Uptime the way it is said out loud, not as a count of seconds.
function forHowLong(seconds) {
    if (!isFinite(seconds) || seconds <= 0) return '—';

    const d = Math.floor(seconds / 86400);
    const h = Math.floor((seconds % 86400) / 3600);
    const m = Math.floor((seconds % 3600) / 60);

    if (d) return T('ops.daysHours', { d, h });
    if (h) return T('ops.hoursMinutes', { h, m });

    return T('ops.minutes', { m });
}

/*
 * How hard is too hard.
 *
 * Status colours, which are reserved for state and never used as a series
 * colour. The thresholds are the ones that mean something to somebody watching:
 * a core over 85% is the bottleneck, over 95% it is the whole story.
 */
function pressure(value) {
    if (!isFinite(value) || value < 0) return '';
    if (value >= 95) return 'hot';
    if (value >= 85) return 'warn';

    return '';
}

/* ---------- the charts ---------- */

/*
 * One plot, one axis, any number of series.
 *
 * Never two scales on one chart: the two figures here are both percentages, so
 * they share an axis honestly. The throughput chart is bytes per second for
 * both of its series, and shares one for the same reason.
 *
 * Gaps are gaps. A reading the machine declined to give breaks the line rather
 * than being drawn as zero, because a line that dips to the floor says the
 * machine went quiet and that is not what happened.
 */
function plot(where, { series, max, format, ticks = 3 }) {
    const box = el(where);

    if (!box) return;

    const width = box.clientWidth || 400;
    const height = box.clientHeight || 118;
    const pad = { left: 34, right: 6, top: 6, bottom: 14 };
    const w = Math.max(10, width - pad.left - pad.right);
    const h = Math.max(10, height - pad.top - pad.bottom);

    const points = series[0] ? series[0].values.length : 0;

    if (points < 2) {
        box.innerHTML = `<p class="note tiny">${escape(T('ops.fewReadings'))}</p>`;

        return;
    }

    const top = Math.max(max || 0, ...series.flatMap((s) => s.values.filter((v) => v >= 0)), 1);
    const x = (i) => pad.left + (i / (points - 1)) * w;
    const y = (v) => pad.top + h - (Math.min(v, top) / top) * h;

    const svg = [`<svg viewBox="0 0 ${width} ${height}" preserveAspectRatio="none" role="img">`];

    // Recessive grid, and the axis labelled at both ends of the scale rather
    // than at every step — the shape is the message, the numbers are the check.
    for (let i = 0; i <= ticks; i += 1) {
        const value = (top / ticks) * i;
        const at = y(value);

        svg.push(`<line class="ops-grid-line" x1="${pad.left}" x2="${width - pad.right}" y1="${at}" y2="${at}"/>`);
        svg.push(`<text class="ops-axis-text" x="0" y="${at + 3}">${format(value)}</text>`);
    }

    series.forEach((s) => {
        const colour = s.colour;

        /*
         * Broken into runs at every gap, so a missing reading leaves a hole in
         * the line instead of a plunge to the floor.
         */
        let run = [];
        const runs = [];

        s.values.forEach((v, i) => {
            if (!isFinite(v) || v < 0) {
                if (run.length > 1) runs.push(run);

                run = [];

                return;
            }

            run.push([x(i), y(v)]);
        });

        if (run.length > 1) runs.push(run);

        runs.forEach((r) => {
            const line = r.map(([px, py], i) => `${i ? 'L' : 'M'}${px.toFixed(1)} ${py.toFixed(1)}`).join(' ');
            const floor = pad.top + h;
            const area = `${line} L${r[r.length - 1][0].toFixed(1)} ${floor} L${r[0][0].toFixed(1)} ${floor} Z`;

            svg.push(`<path class="ops-area" d="${area}" fill="${colour}"/>`);
            svg.push(`<path class="ops-line" d="${line}" stroke="${colour}"/>`);
        });
    });

    svg.push(`<line class="ops-crosshair" id="${where}-cross" x1="0" x2="0" y1="${pad.top}" y2="${pad.top + h}" style="display:none"/>`);

    series.forEach((s, i) => {
        svg.push(`<circle class="ops-dot" id="${where}-dot-${i}" cx="0" cy="0" fill="${s.colour}" style="display:none"/>`);
    });

    svg.push('</svg>');

    box.innerHTML = svg.join('');

    hover(box, { where, series, points, x, y, format, pad, h });
}

/*
 * The crosshair and the tooltip.
 *
 * A chart drawn in a page is interactive whether or not anybody built the
 * interaction, so the choice is between answering "what was that spike at
 * 14:02" and refusing to. The hit target is the whole plot, not the line.
 */
function hover(box, { where, series, points, x, y, format, pad, h }) {
    const svg = box.querySelector('svg');
    const cross = el(`${where}-cross`);
    const dots = series.map((_, i) => el(`${where}-dot-${i}`));

    let tip = box.querySelector('.ops-tip');

    if (!tip) {
        tip = document.createElement('div');
        tip.className = 'ops-tip';
        tip.hidden = true;
        box.appendChild(tip);
    }

    const leave = () => {
        tip.hidden = true;

        if (cross) cross.style.display = 'none';

        dots.forEach((d) => { if (d) d.style.display = 'none'; });
    };

    box.onmouseleave = leave;

    box.onmousemove = (e) => {
        const rect = svg.getBoundingClientRect();
        const at = ((e.clientX - rect.left) / rect.width) * (svg.viewBox.baseVal.width || rect.width);
        const i = Math.max(0, Math.min(points - 1,
            Math.round(((at - pad.left) / (x(points - 1) - x(0))) * (points - 1))));

        const px = x(i);

        if (cross) {
            cross.setAttribute('x1', px);
            cross.setAttribute('x2', px);
            cross.style.display = '';
        }

        const lines = [];

        series.forEach((s, n) => {
            const v = s.values[i];
            const known = isFinite(v) && v >= 0;

            if (dots[n]) {
                if (known) {
                    dots[n].setAttribute('cx', px);
                    dots[n].setAttribute('cy', y(v));
                    dots[n].style.display = '';
                } else {
                    dots[n].style.display = 'none';
                }
            }

            lines.push(`<u>${s.name}</u> ${known ? format(v) : escape(T('ops.notReported'))}`);
        });

        const when = series[0].times && series[0].times[i];

        tip.innerHTML = (when ? `<u>${when}</u><br>` : '') + lines.join('<br>');
        tip.hidden = false;

        // Kept inside the panel: a tooltip that hangs off the right-hand edge
        // is a tooltip nobody can read.
        const offset = Math.min(px + 12, box.clientWidth - tip.offsetWidth - 4);

        tip.style.left = `${Math.max(0, offset)}px`;
        tip.style.top = `${pad.top + 2}px`;
    };
}

function legend(where, entries) {
    const box = el(where);

    if (!box) return;

    box.innerHTML = entries
        .map((e, i) => `<span><i${i ? ' data-series="b"' : ''}></i>${e}</span>`)
        .join('');
}

/* ---------- drawing what came back ---------- */

/*
 * A panel with nothing in it is hidden, not emptied.
 *
 * The whole point of this column is that it describes the machine it is
 * running on. A heading with no rows under it is a claim that something is
 * missing, and on a desktop with no battery nothing is missing.
 */
function showCard(id, has) {
    const card = el(id);

    if (card) card.hidden = !has;

    return has;
}

/*
 * How hot is too hot, by the chip's own numbers.
 *
 * A processor package is designed to run at eighty degrees and a disk is not,
 * so a single threshold across both is how a dashboard learns to cry wolf. The
 * kernel publishes each chip's own high and critical points where the chip
 * declares them; only where it declares neither does this fall back to
 * something generic, and says so by using a wider one.
 */
function heat(sensor) {
    const { celsius, high_c: high, critical_c: crit } = sensor;

    if (crit > 0 && celsius >= crit) return 'hot';
    if (high > 0 && celsius >= high) return 'warn';
    if (high <= 0 && crit <= 0 && celsius >= 90) return 'warn';

    return '';
}

function drawTemperatures(list) {
    if (!showCard('ops-temps-card', list && list.length)) return;

    el('ops-temps').innerHTML = list.map((t) => {
        // Scaled to the critical point where the chip gives one, so the bar
        // means "how close to the limit" rather than "out of a hundred".
        const top = t.critical_c > 0 ? t.critical_c : 100;
        const state = heat(t);
        const limit = t.critical_c > 0
            ? `<span class="ops-temp-limit"> / ${Math.round(t.critical_c)}°</span>`
            : '';

        return `<div class="ops-bar-row">
            <span class="ops-bar-label" title="${t.chip}">${t.label}</span>
            <div class="ops-bar"><div${state ? ` data-state="${state}"` : ''}
                style="width:${Math.min(100, (t.celsius / top) * 100).toFixed(1)}%"></div></div>
            <span class="ops-bar-value">${decimal(t.celsius.toFixed(1))}°${limit}</span>
        </div>`;
    }).join('');
}

function drawFans(list) {
    if (!showCard('ops-fans-card', list && list.length)) return;

    el('ops-fans').innerHTML = list.map((f) => `<div class="row">
        <span class="row-label">${f.label}</span>
        <span class="row-value">${escape(T('ops.rpm', { n: Math.round(f.rpm).toLocaleString(bg() ? 'bg-BG' : 'en-GB') }))}</span>
    </div>`).join('');
}

function drawPower(power) {
    if (!showCard('ops-power-card', !!power)) return;

    const state = power.percent <= 10 && power.state !== 'charging' ? 'hot'
        : power.percent <= 25 && power.state !== 'charging' ? 'warn' : '';

    const named = power.state
        ? said('ops.power.' + power.state.replace(/\W+/g, '_'), power.state)
        : T('ops.battery');

    const marked = state ? ' data-state="' + state + '"' : '';
    const watts = power.watts >= 0 ? ' · ' + decimal(power.watts.toFixed(1)) + 'W' : '';

    el('ops-power').innerHTML = `<div class="ops-bar-row">
        <span class="ops-bar-label">${escape(named)}</span>
        <div class="ops-bar"><div${marked}
            style="width:${Math.min(100, power.percent)}%"></div></div>
        <span class="ops-bar-value">${Math.round(power.percent)}%${watts}</span>
    </div>`;
}

function drawLinks(list) {
    if (!showCard('ops-links-card', list && list.length)) return;

    el('ops-links').innerHTML = list.map((l) => {
        const speed = l.speed_mbps > 0
            ? `${l.speed_mbps >= 1000 ? `${l.speed_mbps / 1000}G` : `${l.speed_mbps}M`}`
            : (l.wireless ? T('ops.wireless') : '');

        const moving = l.state === 'up' && l.in_per_second >= 0
            ? T('ops.inOut', { in: rate(l.in_per_second), out: rate(l.out_per_second) })
            : said(`ops.link.${String(l.state).replace(/\W+/g, '_')}`, l.state);

        return `<div class="row${l.state === 'up' ? '' : ' row-quiet'}">
            <span class="row-label">${l.name}${speed ? ` · ${speed}` : ''}</span>
            <span class="row-value">${moving}</span>
        </div>`;
    }).join('');
}

function drawDisks(list) {
    const disks = (list || []).filter((d) => d.total_bytes > 0);

    if (!showCard('ops-disks-card', disks.length)) return;

    el('ops-disks').innerHTML = disks.map((d) => {
        const used = d.total_bytes - d.free_bytes;
        const full = (used / d.total_bytes) * 100;
        const state = full >= 95 ? 'hot' : full >= 85 ? 'warn' : '';

        return `<div class="ops-bar-row">
            <span class="ops-bar-label" title="${d.mount_point}">${
                d.current ? escape(T('ops.theBrain')) : d.mount_point.split('/').pop() || '/'}</span>
            <div class="ops-bar"><div${state ? ` data-state="${state}"` : ''}
                style="width:${full.toFixed(1)}%"></div></div>
            <span class="ops-bar-value">${escape(T('ops.free', { size: bytes(d.free_bytes) }))}</span>
        </div>`;
    }).join('');
}

function drawCores(cores) {
    const box = el('ops-cores');

    if (!box) return;

    box.innerHTML = (cores || []).map((v, i) => {
        const known = isFinite(v) && v >= 0;
        const state = pressure(v);

        return `<div class="ops-core">
            <span class="ops-core-n">${i}</span>
            <div class="ops-core-track">
                <div class="ops-core-fill"${state ? ` data-state="${state}"` : ''}
                     style="width:${known ? Math.min(100, v).toFixed(1) : 0}%"></div>
            </div>
            <span class="ops-core-value">${known ? `${Math.round(v)}%` : '—'}</span>
        </div>`;
    }).join('');
}

function drawBar(fill, value, text, used, total, state) {
    const bar = el(fill);
    const label = el(text);

    if (bar) {
        const known = isFinite(value) && value >= 0;

        bar.style.width = `${known ? Math.min(100, value) : 0}%`;

        if (state) {
            bar.dataset.state = state;
        } else {
            delete bar.dataset.state;
        }
    }

    if (label) {
        label.textContent = total
            ? `${bytes(used)} / ${bytes(total)}`
            : T('ops.none');
    }
}

/*
 * The cards.
 *
 * Built once and then updated in place, rather than rewritten every second.
 * Rewriting was simpler and wrong in two ways: it threw away the chart under
 * each card and drew a new one every tick, and it destroyed whatever the
 * pointer was hovering over at the moment of the refresh — so a tooltip could
 * not be read on a panel that refreshes once a second.
 */
let cardsBuilt = 0;

function drawGPUs(gpus, note, history, times) {
    const box = el('ops-gpus');
    const why = el('ops-gpu-note');

    if (!box) return;

    if (why) {
        why.hidden = !note;
        why.textContent = gpuNote(note);
    }

    const cards = gpus || [];

    /*
     * A machine with nothing this can read shows no graphics panel at all,
     * unless there is something to say about why — a card whose own tool is
     * installed and will not answer is worth a sentence; a machine that simply
     * has integrated graphics and nothing else is not.
     */
    showCard('ops-graphics-card', cards.length > 0 || !!note);

    if (!cards.length) {
        if (cardsBuilt !== 0) {
            box.innerHTML = '';
            cardsBuilt = 0;
        }

        return;
    }

    if (cardsBuilt !== cards.length) {
        box.innerHTML = cards.map((g, i) => `<div class="ops-gpu" data-card="${i}">
            <div class="ops-gpu-head">
                <span class="ops-gpu-name"><span class="ops-gpu-index">${g.index}</span> ${g.name}</span>
                <span class="ops-gpu-figures" data-figures></span>
            </div>
            <div class="ops-bars">
                <div class="ops-bar-row">
                    <span class="ops-bar-label" data-busy-label>${escape(T('ops.busy'))}</span>
                    <div class="ops-bar"><div data-busy></div></div>
                    <span class="ops-bar-value" data-busy-value>—</span>
                </div>
                <div class="ops-bar-row" data-mem-row${g.memory_total_bytes > 0 ? '' : ' hidden'}>
                    <span class="ops-bar-label">${escape(T('ops.memory'))}</span>
                    <div class="ops-bar"><div data-series="b" data-mem></div></div>
                    <span class="ops-bar-value" data-mem-value>—</span>
                </div>
            </div>
            <figure class="ops-figure">
                <figcaption>${escape(T('ops.cardChart', { n: g.index }))}</figcaption>
                <div class="ops-legend" id="ops-gpu-chart-${i}-legend"></div>
                <div class="ops-chart ops-chart-short" id="ops-gpu-chart-${i}"></div>
            </figure>
        </div>`).join('');

        cardsBuilt = cards.length;
    }

    cards.forEach((g, i) => {
        const card = box.querySelector(`.ops-gpu[data-card="${i}"]`);

        if (!card) return;

        const figures = [
            g.temperature_c >= 0 ? `<b>${Math.round(g.temperature_c)}°C</b>` : null,
            g.fan_percent >= 0 ? `${escape(T('ops.fan'))} <b>${Math.round(g.fan_percent)}%</b>` : null,
            g.power_watts >= 0
                ? `<b>${Math.round(g.power_watts)}W</b>${g.power_cap_watts >= 0 ? ` / ${Math.round(g.power_cap_watts)}` : ''}`
                : null,
            g.clock_mhz >= 0 ? `<b>${Math.round(g.clock_mhz)}</b>MHz` : null,
        ].filter(Boolean).join(' · ');

        card.querySelector('[data-figures]').innerHTML = figures || escape(T('ops.noFigures'));

        const busy = card.querySelector('[data-busy]');
        const state = pressure(g.util_percent);

        busy.style.width = `${g.util_percent >= 0 ? Math.min(100, g.util_percent) : 0}%`;

        if (state) {
            busy.dataset.state = state;
        } else {
            delete busy.dataset.state;
        }

        /*
         * Named for what it actually is.
         *
         * A part that publishes no busy figure has one derived from its clock
         * — how close it is running to its maximum — and calling that "busy"
         * puts an integrated chip at 100% beside a card at 100% while meaning
         * two different things by it.
         */
        card.querySelector('[data-busy-label]').textContent = g.util_from_clock ? T('ops.clock') : T('ops.busy');

        card.querySelector('[data-busy-value]').textContent = g.util_from_clock && g.clock_mhz >= 0
            ? `${Math.round(g.clock_mhz)} / ${Math.round(g.clock_max_mhz)}MHz`
            : percent(g.util_percent);

        // An integrated part has no memory of its own to report, so the row is
        // not there rather than reading 0B / 0B.
        const memRow = card.querySelector('[data-mem-row]');

        if (g.memory_total_bytes > 0) {
            memRow.hidden = false;

            card.querySelector('[data-mem]').style.width =
                `${g.memory_percent >= 0 ? Math.min(100, g.memory_percent) : 0}%`;

            card.querySelector('[data-mem-value]').textContent =
                `${bytes(g.memory_used_bytes)} / ${bytes(g.memory_total_bytes)}`;
        } else {
            memRow.hidden = true;
        }

        /*
         * One chart per card rather than all of them on one plot.
         *
         * Three cards holding a model each move together, and six lines on one
         * axis is a picture of nothing. Small multiples say the same thing and
         * stay readable.
         */
        /*
         * Only the series this part actually reports.
         *
         * An integrated chip has no memory figure, and a legend entry for a
         * line that is not drawn is a claim that the line is at zero.
         */
        const drawn = [
            {
                name: escape(g.util_from_clock ? T('ops.clock') : T('ops.busy')),
                colour: SERIES_A,
                values: history.map((h) => (h.gpu || [])[i]),
                times,
            },
        ];

        if (g.memory_total_bytes > 0) {
            drawn.push({
                name: escape(T('ops.memoryUsed')),
                colour: SERIES_B,
                values: history.map((h) => (h.gpu_memory || [])[i]),
                times,
            });
        }

        legend(`ops-gpu-chart-${i}-legend`, drawn.length > 1 ? drawn.map((d) => d.name) : []);
        plot(`ops-gpu-chart-${i}`, { max: 100, format: (v) => `${Math.round(v)}%`, series: drawn });
    });
}

// Processor time as the server says it ("2h 19m", "1m 05s"), in the page's units.
function clock(said) {
    if (!said) return '—';
    if (!bg()) return said;

    return String(said)
        .replace(/(\d+)h\b/g, (m, n) => T('ops.clockH', { n }))
        .replace(/(\d+)m\b/g, (m, n) => T('ops.clockM', { n }))
        .replace(/(\d+)s\b/g, (m, n) => T('ops.clockS', { n }));
}

function drawProcesses(list) {
    const body = el('ops-processes');

    if (!body) return;

    body.innerHTML = (list || []).map((p) => `<tr${p.ours ? ' class="ours"' : ''}>
        <td class="num">${p.pid}</td>
        <td>${p.user || '—'}</td>
        <td class="num">${percent(p.cpu_percent, 1)}</td>
        <td class="num">${percent(p.mem_percent, 1)}</td>
        <td class="num">${bytes(p.rss_bytes)}</td>
        <td class="num">${clock(p.cpu_time)}</td>
        <td class="command" title="${(p.command || '').replace(/"/g, '&quot;')}">${p.command || ''}</td>
    </tr>`).join('');
}

function drawBrain(brain, machine) {
    const box = el('ops-brain-rows');

    if (!box || !brain) return;

    const step = brain.step || {};
    const models = brain.models || {};

    const rows = [
        [T('ops.answeringWith'), brain.model || '—'],
        [T('ops.doingNow'), step.busy ? (step.note || step.kind || T('ops.working')) : T('ops.nothing')],
        [T('ops.modelRoles'),
            [models.talk, models.work, models.reason].filter(Boolean).join(' · ') || '—'],
        [T('ops.memoryAt'), brain.root || '—'],
        [T('ops.network'), T('ops.inOut', { in: rate(machine.network_in_per_second), out: rate(machine.network_out_per_second) })],
        [T('ops.disk'), T('ops.readWritten', { read: rate(machine.disk_read_per_second), written: rate(machine.disk_write_per_second) })],
    ];

    if (brain.storage && brain.storage.free_bytes) {
        rows.push([T('ops.roomLeft'), T('ops.free', { size: bytes(brain.storage.free_bytes) })]);
    }

    box.innerHTML = rows.map(([what, value]) => `<div class="row">
        <span class="row-label">${escape(what)}</span>
        <span class="row-value">${escape(value)}</span>
    </div>`).join('');
}

/* ---------- the refresh ---------- */

let failed = false;

async function refresh() {
    if (!visible()) return;

    let data;

    try {
        data = await fetch('/api/operations').then((r) => r.json());
    } catch {
        return;
    }

    const missing = el('ops-unavailable');

    if (!data.available) {
        if (missing) {
            missing.hidden = false;
            missing.textContent = T('ops.unavailable');
        }

        if (!failed) {
            failed = true;
            document.querySelector('.ops-grid').hidden = true;
        }

        return;
    }

    if (missing) missing.hidden = true;

    const m = data.machine || {};
    const history = data.history || [];

    el('ops-uptime').textContent = forHowLong(m.uptime_seconds);
    el('ops-load').textContent = (m.load_average || []).map((v) => decimal(v.toFixed(2))).join(' ') || '—';
    el('ops-tasks').textContent = m.tasks
        ? T('ops.tasksChip', { tasks: m.tasks, threads: m.threads, running: m.running })
        : '—';
    el('ops-window').textContent = T('ops.window', { n: history.length, s: data.seconds_back });

    drawCores(m.cores);

    drawBar('ops-mem-fill', m.memory_total_bytes
        ? (m.memory_used_bytes / m.memory_total_bytes) * 100
        : -1, 'ops-mem-value', m.memory_used_bytes, m.memory_total_bytes,
        pressure(m.memory_total_bytes ? (m.memory_used_bytes / m.memory_total_bytes) * 100 : -1));

    drawBar('ops-swap-fill', m.swap_total_bytes
        ? (m.swap_used_bytes / m.swap_total_bytes) * 100
        : -1, 'ops-swap-value', m.swap_used_bytes, m.swap_total_bytes, null);

    const swapBar = el('ops-swap-fill');

    if (swapBar) swapBar.dataset.series = 'b';

    const times = history.map((h) => new Date(h.at).toLocaleTimeString(bg() ? 'bg-BG' : 'en-GB', {
        hour: '2-digit', minute: '2-digit', second: '2-digit',
    }));

    gpuKey = data.gpu_note_key || '';
    gpuWhy = data.gpu_note_why || '';
    drawGPUs(m.gpus, m.gpu_note, history, times);
    drawTemperatures(m.temperatures);
    drawFans(m.fans);
    drawPower(m.power);
    drawLinks(m.links);
    drawDisks(data.disks);
    drawProcesses(m.processes);
    drawBrain(data.brain, m);

    legend('ops-cpu-legend', [escape(T('ops.processor')), escape(T('ops.memory'))]);
    plot('ops-cpu-chart', {
        max: 100,
        format: (v) => `${Math.round(v)}%`,
        series: [
            { name: escape(T('ops.processor')), colour: SERIES_A, values: history.map((h) => h.cpu), times },
            { name: escape(T('ops.memory')), colour: SERIES_B, values: history.map((h) => h.memory), times },
        ],
    });

    legend('ops-io-legend', [escape(T('ops.networkOut')), escape(T('ops.diskWritten'))]);
    plot('ops-io-chart', {
        format: (v) => bytes(v),
        series: [
            { name: escape(T('ops.networkOut')), colour: SERIES_A, values: history.map((h) => h.network_out), times },
            { name: escape(T('ops.diskWritten')), colour: SERIES_B, values: history.map((h) => h.disk_write), times },
        ],
    });
}

/*
 * How long answering takes, in the same view as the temperatures.
 *
 * Measured from the last word somebody said, so it includes the two stages the
 * program is responsible for — waiting out the silence and starting a voice —
 * which are invisible from anywhere else and are the entire delay on a machine
 * where the model is quick.
 *
 * Read far less often than the sensors: it changes only when somebody speaks.
 */
async function pace() {
    let data;

    try {
        const res = await fetch('/api/pace');
        if (!res.ok) return;
        data = await res.json();
    } catch (err) {
        return;
    }

    const card = document.getElementById('ops-pace-card');
    if (!card) return;

    card.hidden = false;

    const t = data.typical || {};
    const ms = (v) => (v >= 1000
        ? T('ops.seconds', { s: decimal((v / 1000).toFixed(1)) })
        : T('ops.ms', { ms: Math.round(v) }));

    const verdictArgs = Object.assign({}, data.verdict_args || {});

    if (verdictArgs.s) verdictArgs.s = decimal(verdictArgs.s);

    document.getElementById('ops-pace-verdict').textContent = data.verdict_key
        ? said(`ops.pace.verdict.${data.verdict_key}`, data.verdict || '', verdictArgs)
        : (data.verdict || '');

    /*
     * Each stage against the whole wait, not against a fixed scale.
     *
     * The point of the picture is which stage owns the time, and on this
     * machine one of them owns nearly all of it — a fixed scale would render
     * every bar full and say nothing.
     */
    const stages = [
        [T('ops.stage.hearing'), t.hearing],
        [T('ops.stage.thinking'), t.thinking],
        [T('ops.stage.writing'), t.writing],
        [T('ops.stage.speaking'), t.speaking],
    ].filter(([, v]) => v > 0);

    const worst = Math.max(1, ...stages.map(([, v]) => v));

    document.getElementById('ops-pace').innerHTML = stages.map(([what, v]) => `
        <div class="ops-bar-row">
            <span class="ops-bar-label">${escape(what)}</span>
            <div class="ops-bar"><div style="width:${Math.round((v / worst) * 100)}%"></div></div>
            <span class="ops-bar-value">${ms(v)}</span>
        </div>`).join('');

    const target = data.target || 800;
    const fixed = data.fixed || {};

    document.getElementById('ops-pace-model').textContent = t.model
        ? t.model + (t.tools ? ' ' + T('ops.withTools') : '')
            + ' · ' + T(data.over === 1 ? 'ops.paceOverOne' : 'ops.paceOver', { n: data.over })
            + ' · ' + T('ops.paceTarget', { ms: target, silence: fixed.silence_ms, tokens: fixed.tool_words })
        : '';

    /*
     * And the offer, where the program can actually do the thing.
     *
     * A download of a couple of gigabytes is not something to start on
     * somebody's behalf because a measurement suggested it, so it is a button
     * and it names what it will fetch — the sentence and the download must not
     * be able to drift apart.
     */
    document.getElementById('ops-pace-findings').innerHTML =
        (data.findings || []).map((f) => {
            const offer = f.fetch
                ? ' <button type="button" class="model-action" data-fetch-model="' + escape(f.fetch) + '">'
                    + escape(T('ops.get', { name: f.fetch })) + '</button>'
                : '';

            return `<div class="row">
            <span class="row-label">${escape(said('ops.stage.' + f.stage, f.stage))} · ${ms(f.costs_ms)}</span>
            <span class="row-value">${escape(finding(f))}${offer}</span>
        </div>`;
        }).join('');

    for (const button of document.querySelectorAll('[data-fetch-model]')) {
        button.onclick = async () => {
            const name = button.dataset.fetchModel;

            button.disabled = true;
            button.textContent = T('ops.fetching', { name });

            try {
                await fetch('/api/models/pull', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name }),
                });
            } catch (err) {
                button.disabled = false;
                button.textContent = T('ops.get', { name });

                return;
            }

            // It carries on in the background and reports on the progress
            // line, which is where every other long fetch in this program
            // reports.
            button.textContent = T('ops.fetchingWatch', { name });
        };
    }
}

/*
 * A finding in the page's language: its two halves by key, with the model
 * names it carries; the English the server wrote when there is no key.
 */
function finding(f) {
    const args = Object.assign({}, f.args || {});

    for (const name of ['model', 'working']) {
        if (name in args && !args[name]) args[name] = T('ops.theModel');
    }

    const because = said(`ops.pace.${f.key}.because`, '', args);
    const change = said(`ops.pace.${f.key}.change`, '', args);

    return f.key && because && change ? `${because}. ${change}.` : `${f.because}. ${f.change}.`;
}

// The graphics note, by the key the server names it with when it has one.
let gpuKey = '';
let gpuWhy = '';

function gpuNote(note) {
    if (!note) return '';

    return gpuKey ? said(`ops.gpu.${gpuKey}`, note, { why: gpuWhy }) : note;
}

refresh();
pace();
setInterval(refresh, TICK);

// Drawn again at once in the other language, rather than at the next tick.
window.addEventListener('pn-language', () => {
    cardsBuilt = -1; // the cards' own labels are written when they are built
    refresh();
    pace();
});
setInterval(pace, 15000);

// And immediately when the view is opened, rather than up to a second later.
for (const button of document.querySelectorAll('[data-view="operations"]')) {
    button.addEventListener('click', () => {
        setTimeout(refresh, 30);
        setTimeout(pace, 30);
    });
}

}());
