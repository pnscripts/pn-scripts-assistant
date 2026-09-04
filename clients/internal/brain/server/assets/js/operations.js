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

    return `${n < 10 && i > 0 ? n.toFixed(1) : Math.round(n)}${units[i]}`;
}

function rate(n) {
    return isFinite(n) && n >= 0 ? `${bytes(n)}/s` : '—';
}

function percent(n, places = 0) {
    return isFinite(n) && n >= 0 ? `${n.toFixed(places)}%` : '—';
}

// Uptime the way it is said out loud, not as a count of seconds.
function forHowLong(seconds) {
    if (!isFinite(seconds) || seconds <= 0) return '—';

    const d = Math.floor(seconds / 86400);
    const h = Math.floor((seconds % 86400) / 3600);
    const m = Math.floor((seconds % 3600) / 60);

    if (d) return `${d}d ${h}h`;
    if (h) return `${h}h ${m}m`;

    return `${m}m`;
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
        box.innerHTML = '<p class="note tiny">Not enough readings yet.</p>';

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

            lines.push(`<u>${s.name}</u> ${known ? format(v) : 'not reported'}`);
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
            : 'none';
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
        why.textContent = note || '';
    }

    const cards = gpus || [];

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
                    <span class="ops-bar-label">busy</span>
                    <div class="ops-bar"><div data-busy></div></div>
                    <span class="ops-bar-value" data-busy-value>—</span>
                </div>
                <div class="ops-bar-row">
                    <span class="ops-bar-label">memory</span>
                    <div class="ops-bar"><div data-series="b" data-mem></div></div>
                    <span class="ops-bar-value" data-mem-value>—</span>
                </div>
            </div>
            <figure class="ops-figure">
                <figcaption>Card ${g.index}, last three minutes</figcaption>
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
            g.fan_percent >= 0 ? `fan <b>${Math.round(g.fan_percent)}%</b>` : null,
            g.power_watts >= 0
                ? `<b>${Math.round(g.power_watts)}W</b>${g.power_cap_watts >= 0 ? ` / ${Math.round(g.power_cap_watts)}` : ''}`
                : null,
            g.clock_mhz >= 0 ? `<b>${Math.round(g.clock_mhz)}</b>MHz` : null,
        ].filter(Boolean).join(' · ');

        card.querySelector('[data-figures]').innerHTML = figures || 'no figures reported';

        const busy = card.querySelector('[data-busy]');
        const state = pressure(g.util_percent);

        busy.style.width = `${g.util_percent >= 0 ? Math.min(100, g.util_percent) : 0}%`;

        if (state) {
            busy.dataset.state = state;
        } else {
            delete busy.dataset.state;
        }

        card.querySelector('[data-busy-value]').textContent = percent(g.util_percent);

        card.querySelector('[data-mem]').style.width =
            `${g.memory_percent >= 0 ? Math.min(100, g.memory_percent) : 0}%`;

        card.querySelector('[data-mem-value]').textContent =
            `${bytes(g.memory_used_bytes)} / ${bytes(g.memory_total_bytes)}`;

        /*
         * One chart per card rather than all of them on one plot.
         *
         * Three cards holding a model each move together, and six lines on one
         * axis is a picture of nothing. Small multiples say the same thing and
         * stay readable.
         */
        legend(`ops-gpu-chart-${i}-legend`, ['busy', 'memory used']);
        plot(`ops-gpu-chart-${i}`, {
            max: 100,
            format: (v) => `${Math.round(v)}%`,
            series: [
                { name: 'busy', colour: SERIES_A, values: history.map((h) => (h.gpu || [])[i]), times },
                { name: 'memory used', colour: SERIES_B, values: history.map((h) => (h.gpu_memory || [])[i]), times },
            ],
        });
    });
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
        <td class="num">${p.cpu_time || '—'}</td>
        <td class="command" title="${(p.command || '').replace(/"/g, '&quot;')}">${p.command || ''}</td>
    </tr>`).join('');
}

function drawBrain(brain, machine) {
    const box = el('ops-brain-rows');

    if (!box || !brain) return;

    const step = brain.step || {};
    const models = brain.models || {};

    const rows = [
        ['answering with', brain.model || '—'],
        ['doing now', step.busy ? (step.note || step.kind || 'working') : 'nothing'],
        ['talking / working / reasoning',
            [models.talk, models.work, models.reason].filter(Boolean).join(' · ') || '—'],
        ['memory kept at', brain.root || '—'],
        ['network', `${rate(machine.network_in_per_second)} in · ${rate(machine.network_out_per_second)} out`],
        ['disk', `${rate(machine.disk_read_per_second)} read · ${rate(machine.disk_write_per_second)} written`],
    ];

    if (brain.storage && brain.storage.free_bytes) {
        rows.push(['room left', `${bytes(brain.storage.free_bytes)} free`]);
    }

    box.innerHTML = rows.map(([what, value]) => `<div class="row">
        <span class="row-label">${what}</span>
        <span class="row-value">${value}</span>
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
            missing.textContent = data.why
                || 'This machine does not publish these readings.';
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
    el('ops-load').textContent = (m.load_average || []).map((v) => v.toFixed(2)).join(' ') || '—';
    el('ops-tasks').textContent = m.tasks
        ? `${m.tasks} · ${m.threads} thr · ${m.running} running`
        : '—';
    el('ops-window').textContent = `${history.length} readings · ${data.seconds_back}s`;

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

    const times = history.map((h) => new Date(h.at).toLocaleTimeString([], {
        hour: '2-digit', minute: '2-digit', second: '2-digit',
    }));

    drawGPUs(m.gpus, m.gpu_note, history, times);
    drawProcesses(m.processes);
    drawBrain(data.brain, m);

    legend('ops-cpu-legend', ['processor', 'memory']);
    plot('ops-cpu-chart', {
        max: 100,
        format: (v) => `${Math.round(v)}%`,
        series: [
            { name: 'processor', colour: SERIES_A, values: history.map((h) => h.cpu), times },
            { name: 'memory', colour: SERIES_B, values: history.map((h) => h.memory), times },
        ],
    });

    legend('ops-io-legend', ['network out', 'disk written']);
    plot('ops-io-chart', {
        format: (v) => bytes(v),
        series: [
            { name: 'network out', colour: SERIES_A, values: history.map((h) => h.network_out), times },
            { name: 'disk written', colour: SERIES_B, values: history.map((h) => h.disk_write), times },
        ],
    });
}

refresh();
setInterval(refresh, TICK);

// And immediately when the view is opened, rather than up to a second later.
for (const button of document.querySelectorAll('[data-view="operations"]')) {
    button.addEventListener('click', () => setTimeout(refresh, 30));
}

}());
