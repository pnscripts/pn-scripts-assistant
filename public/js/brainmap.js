/*
 * The brain map: a living picture of what PN Brain knows.
 *
 * Every node is a real memory and every line is a real relationship — the same
 * embedding similarity that drives recall. Nothing here is decorative geometry.
 * When a cluster looks dense it is because those memories genuinely cluster,
 * and when the brain recalls something during a conversation, that node lights
 * up because it was actually used.
 *
 * Written against a plain canvas rather than a graph library: the whole thing
 * is a force simulation and a draw loop, it has to survive being embedded in a
 * single self-contained binary with no build step, and a library would be
 * larger than the code it replaced.
 *
 * The restraint that matters is cost. This runs behind a chat window on a
 * machine that may be doing CPU-only inference at the same time, so it stops
 * completely when the tab is hidden, settles to a low frame rate once the
 * layout stops moving, and never simulates more nodes than it draws.
 */

(function () {
    const canvas = document.getElementById('brainmap');
    if (!canvas) return;

    const ctx = canvas.getContext('2d', { alpha: true });

    const COLOURS = {
        project: '#4dd0e1',
        document: '#7bc47f',
        conversation: '#f0b26b',
        unknown: '#7d8b9c',
    };

    let nodes = [];
    let links = [];
    let particles = [];
    let width = 0;
    let height = 0;
    let energy = 1;          // falls as the layout settles
    let running = true;
    let pointer = { x: -1e5, y: -1e5 };

    /* ---------- sizing ---------- */

    function resize() {
        const dpr = Math.min(window.devicePixelRatio || 1, 2);
        const rect = canvas.getBoundingClientRect();

        width = rect.width;
        height = rect.height;
        canvas.width = Math.floor(width * dpr);
        canvas.height = Math.floor(height * dpr);
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

        seedParticles();
        energy = 1;
    }

    /* ---------- drifting field ---------- */

    function seedParticles() {
        // Density by area, so a small window is not a blizzard and a large one
        // is not empty.
        // Dense enough to read as a field rather than dust. Capped so a large
        // monitor does not quietly become a particle benchmark.
        const count = Math.min(Math.round((width * height) / 2600), 520);
        particles = [];

        for (let i = 0; i < count; i++) {
            particles.push({
                x: Math.random() * width,
                y: Math.random() * height,
                vx: (Math.random() - 0.5) * 0.22,
                vy: (Math.random() - 0.5) * 0.22,
                r: Math.random() * 1.6 + 0.4,
                a: Math.random() * 0.55 + 0.2,
            });
        }
    }

    function stepParticles() {
        for (const p of particles) {
            p.x += p.vx;
            p.y += p.vy;

            // Wrap rather than bounce: bouncing makes edges visible and the
            // field should feel unbounded.
            if (p.x < 0) p.x += width;
            if (p.x > width) p.x -= width;
            if (p.y < 0) p.y += height;
            if (p.y > height) p.y -= height;
        }
    }

    /* ---------- layout ---------- */

    function layout(data) {
        const categories = [...new Set(data.nodes.map((n) => n.category))];

        // Each category gets its own region, so the eye can find "projects"
        // without reading a legend. Within a region, position is settled by the
        // force pass below.
        nodes = data.nodes.map((n, i) => {
            const band = categories.indexOf(n.category);
            const angle = (band / Math.max(categories.length, 1)) * Math.PI * 2
                + (i / data.nodes.length) * 0.9;
            const radius = Math.min(width, height) * (0.16 + Math.random() * 0.24);

            return {
                ...n,
                x: width / 2 + Math.cos(angle) * radius,
                y: height / 2 + Math.sin(angle) * radius,
                vx: 0,
                vy: 0,
                glow: 0,
            };
        });

        const byId = new Map(nodes.map((n) => [n.id, n]));

        links = data.links
            .map((l) => ({ a: byId.get(l.source), b: byId.get(l.target), strength: l.strength }))
            .filter((l) => l.a && l.b);

        energy = 1;
    }

    function stepLayout() {
        if (energy < 0.02) return;

        const centreX = width / 2;
        const centreY = height / 2;

        // Repulsion, capped: without a cap two nodes that land on top of each
        // other fling themselves off screen.
        for (let i = 0; i < nodes.length; i++) {
            const a = nodes[i];

            for (let j = i + 1; j < nodes.length; j++) {
                const b = nodes[j];
                let dx = b.x - a.x;
                let dy = b.y - a.y;
                let d2 = dx * dx + dy * dy;

                if (d2 > 26000) continue;
                if (d2 < 1) { dx = Math.random() - 0.5; dy = Math.random() - 0.5; d2 = 1; }

                const force = 220 / d2;
                const d = Math.sqrt(d2);
                const fx = (dx / d) * force;
                const fy = (dy / d) * force;

                a.vx -= fx; a.vy -= fy;
                b.vx += fx; b.vy += fy;
            }
        }

        // Links pull, in proportion to how related the memories actually are.
        for (const l of links) {
            const dx = l.b.x - l.a.x;
            const dy = l.b.y - l.a.y;
            const d = Math.sqrt(dx * dx + dy * dy) || 1;
            const rest = 90;
            const force = ((d - rest) / d) * 0.0016 * l.strength;

            l.a.vx += dx * force; l.a.vy += dy * force;
            l.b.vx -= dx * force; l.b.vy -= dy * force;
        }

        for (const n of nodes) {
            // A gentle pull inward keeps the graph on screen without a hard boundary.
            n.vx += (centreX - n.x) * 0.0012;
            n.vy += (centreY - n.y) * 0.0012;

            n.vx *= 0.86;
            n.vy *= 0.86;
            n.x += n.vx * energy;
            n.y += n.vy * energy;
        }

        energy *= 0.994;
    }

    /* ---------- drawing ---------- */

    /** How far apart two particles may drift and still be joined. */
    const LINK_DISTANCE = 72;

    /**
     * Adds one thread to the current path if the pair is close enough.
     *
     * Every thread goes into a single path stroked once at the end. Stroking
     * each line separately is thousands of draw calls a frame; one path is one.
     * The trade is that they all share an opacity rather than fading with
     * distance, which at this size is not visible.
     */
    function thread(a, b, maxD2) {
        const dx = a.x - b.x;
        const dy = a.y - b.y;

        if (dx * dx + dy * dy > maxD2) return;

        ctx.moveTo(a.x, a.y);
        ctx.lineTo(b.x, b.y);
    }

    function draw() {
        ctx.clearRect(0, 0, width, height);

        // Threads between particles that drift close together.
        //
        // Comparing every pair would be 520 x 520 / 2 = about 135,000 distance
        // tests per frame, behind a chat window on a machine that may be running
        // a model on its CPU. Instead the field is bucketed into a grid whose
        // cell is the link distance, so a particle can only reach its own cell
        // and the ones touching it. Only half the neighbours are visited, since
        // the other half will visit back. That turns the cost into roughly the
        // number of particles times how many share a cell.
        const cell = LINK_DISTANCE;
        const cols = Math.max(1, Math.ceil(width / cell));
        const rows = Math.max(1, Math.ceil(height / cell));
        const buckets = new Map();

        for (const p of particles) {
            const key = (Math.min(Math.floor(p.y / cell), rows - 1) * cols)
                + Math.min(Math.floor(p.x / cell), cols - 1);
            const bucket = buckets.get(key);
            if (bucket) bucket.push(p); else buckets.set(key, [p]);
        }

        ctx.strokeStyle = '#4dd0e1';
        ctx.lineWidth = 0.4;
        ctx.beginPath();

        // Right, down-left, down, down-right — plus the cell itself. Going only
        // forward means each pair is considered once.
        const NEIGHBOURS = [[1, 0], [-1, 1], [0, 1], [1, 1]];
        const maxD2 = LINK_DISTANCE * LINK_DISTANCE;

        for (let row = 0; row < rows; row++) {
            for (let col = 0; col < cols; col++) {
                const here = buckets.get(row * cols + col);
                if (!here) continue;

                for (let i = 0; i < here.length; i++) {
                    for (let j = i + 1; j < here.length; j++) {
                        thread(here[i], here[j], maxD2);
                    }
                }

                for (const [dc, dr] of NEIGHBOURS) {
                    const nc = col + dc;
                    const nr = row + dr;
                    if (nc < 0 || nc >= cols || nr >= rows) continue;

                    const there = buckets.get(nr * cols + nc);
                    if (!there) continue;

                    for (const a of here) {
                        for (const b of there) thread(a, b, maxD2);
                    }
                }
            }
        }

        ctx.globalAlpha = 0.14;
        ctx.stroke();

        for (const p of particles) {
            ctx.globalAlpha = p.a;
            ctx.fillStyle = '#4dd0e1';
            ctx.beginPath();
            ctx.arc(p.x, p.y, p.r, 0, Math.PI * 2);
            ctx.fill();
        }

        // Links first, so nodes sit on top of them.
        for (const l of links) {
            ctx.globalAlpha = 0.12 + (l.strength - 0.5) * 0.9;
            ctx.strokeStyle = '#4dd0e1';
            ctx.lineWidth = 0.9;
            ctx.beginPath();
            ctx.moveTo(l.a.x, l.a.y);
            ctx.lineTo(l.b.x, l.b.y);
            ctx.stroke();
        }

        let hovered = null;

        for (const n of nodes) {
            const dx = n.x - pointer.x;
            const dy = n.y - pointer.y;
            const near = dx * dx + dy * dy < 150;

            if (near) hovered = n;

            const colour = COLOURS[n.category] || COLOURS.unknown;
            const size = 3 + n.glow * 4.5 + (near ? 2 : 0);

            if (n.glow > 0.01 || near) {
                ctx.globalAlpha = 0.24 * (n.glow || 0.7);
                ctx.fillStyle = colour;
                ctx.beginPath();
                ctx.arc(n.x, n.y, size * 4, 0, Math.PI * 2);
                ctx.fill();
            }

            ctx.globalAlpha = 0.78 + n.glow * 0.22;
            ctx.fillStyle = colour;
            ctx.beginPath();
            ctx.arc(n.x, n.y, size, 0, Math.PI * 2);
            ctx.fill();

            n.glow *= 0.97;
        }

        if (hovered) drawLabel(hovered);

        ctx.globalAlpha = 1;
    }

    function drawLabel(node) {
        const padding = 7;
        ctx.font = '11px ui-monospace, monospace';

        const text = node.label;
        const w = ctx.measureText(text).width + padding * 2;
        const x = Math.min(Math.max(node.x - w / 2, 4), width - w - 4);
        const y = node.y - 26;

        ctx.globalAlpha = 0.92;
        ctx.fillStyle = '#0d1219';
        ctx.strokeStyle = COLOURS[node.category] || COLOURS.unknown;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.roundRect(x, y, w, 20, 5);
        ctx.fill();
        ctx.stroke();

        ctx.globalAlpha = 1;
        ctx.fillStyle = '#d6dee8';
        ctx.fillText(text, x + padding, y + 14);
    }

    /* ---------- loop ---------- */

    let last = 0;

    function frame(now) {
        if (!running) return;

        // Once the layout has settled there is nothing to animate but drift, so
        // drop to roughly 20fps. This sits behind a chat window on a machine
        // that may be running a model on the CPU.
        const interval = energy < 0.02 ? 50 : 16;

        if (now - last >= interval) {
            stepParticles();
            stepLayout();
            draw();
            last = now;
        }

        requestAnimationFrame(frame);
    }

    /* ---------- public: light up what was recalled ---------- */

    window.brainMapRecall = function (ids) {
        for (const n of nodes) {
            if (ids.includes(n.id)) n.glow = 1;
        }
    };

    window.brainMapReload = async function () {
        try {
            const data = await fetch('/api/memory-map').then((r) => r.json());
            layout(data);
        } catch {
            /* An empty map is a fine fallback; never block the chat on this. */
        }
    };

    /* ---------- wiring ---------- */

    canvas.addEventListener('mousemove', (e) => {
        const rect = canvas.getBoundingClientRect();
        pointer.x = e.clientX - rect.left;
        pointer.y = e.clientY - rect.top;
    });

    canvas.addEventListener('mouseleave', () => {
        pointer.x = pointer.y = -1e5;
    });

    // Nothing should be simulated for a window nobody is looking at.
    document.addEventListener('visibilitychange', () => {
        running = !document.hidden;
        if (running) requestAnimationFrame(frame);
    });

    window.addEventListener('resize', resize);

    resize();
    window.brainMapReload();
    requestAnimationFrame(frame);
})();
