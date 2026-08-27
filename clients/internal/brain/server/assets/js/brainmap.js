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

    // Rises when memories are recalled and decays afterwards, so the core
    // brightens exactly when the brain has actually used what it knows.
    let recallPulse = 0;

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

        // Categories are laid out as arcs around the rim, each arc sized by how
        // many memories are in it.
        //
        // The first version gave every category a single fixed angle and let the
        // force pass spread things out from there. With a hundred memories in
        // two or three categories that produced two dense knots on one side of
        // the ring and an empty rest, and the force pass could not fix it: the
        // simulation cools within seconds, so wherever the seeding puts a node
        // is very close to where it stays. Sizing the arcs by population fills
        // the rim evenly while keeping each category contiguous, which is what
        // made the regions worth having in the first place.
        const order = data.nodes
            .map((n, i) => ({ i, c: categories.indexOf(n.category) }))
            .sort((a, b) => a.c - b.c || a.i - b.i);

        const rank = new Map(order.map((o, place) => [o.i, place]));
        const total = Math.max(data.nodes.length, 1);

        nodes = data.nodes.map((n, i) => {
            const angle = (rank.get(i) / total) * Math.PI * 2;
            // Seeded in the band the layout is held in, so the first frames are
            // already composed instead of showing the graph climb out of the
            // reactor it was previously seeded inside.
            const radius = Math.min(width, height) * (0.47 + Math.random() * 0.13);

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

        // The graph is held in a band outside the rings rather than merely
        // pushed off the centre.
        //
        // Scattered across the whole canvas it fought the reactor: two things
        // competing for the same space and neither reading clearly. Confined to
        // an annulus it becomes a rim of memory around the instrument, which is
        // both what it is and what the references do with their outer bands.
        const unit = Math.min(width, height);
        const inner = unit * 0.455;
        const outer = unit * 0.62;

        for (const n of nodes) {
            // The band does the containing now, so the old pull toward the
            // centre would only drag nodes back into the reactor.

            const dx = n.x - centreX;
            const dy = n.y - centreY;

            n.vx *= 0.86;
            n.vy *= 0.86;
            n.x += n.vx * energy;
            n.y += n.vy * energy;

            // Containment corrects the position, not the velocity.
            //
            // The simulation cools on purpose: energy decays so the graph
            // settles instead of trembling forever. Anything expressed as a
            // force therefore stops being applied within about fifteen seconds
            // — which is what went wrong on the first attempt. The band was a
            // force, energy ran out before the nodes reached it, and they froze
            // in their opening scatter with clumps stranded in the corners. A
            // position correction does not cool, so the band holds for as long
            // as the window is open.
            const bd = Math.sqrt(dx * dx + dy * dy) || 1;

            if (bd < inner || bd > outer) {
                const step = ((bd < inner ? inner : outer) - bd) * 0.06;

                n.x += (dx / bd) * step;
                n.y += (dy / bd) * step;
            }
        }

        energy *= 0.994;
    }

    /* ---------- the reactor ---------- */

    /*
     * Concentric rings behind the memory graph.
     *
     * This is the part that is deliberately decorative, and it is the only
     * part. Everything drawn over it — nodes, links, the glow when something is
     * recalled — is real: real memories, real embedding similarity, real use.
     * The references this was drawn from are full of invented readouts, and
     * inventing readouts is the one thing worth refusing, because a display
     * that shows made-up numbers teaches you to ignore all of them.
     *
     * So the rings carry no data. They rotate, they frame the graph, and they
     * are honest about being furniture.
     */

    /*
     * Ring speeds.
     *
     * Slow on purpose. This sits behind a conversation somebody is reading, and
     * anything turning fast enough to notice is competing with the words. These
     * are roughly a fifth of what they were: the outermost takes about a minute
     * to come round, and the innermost a little under fifteen seconds.
     *
     * Speed carries meaning instead — see talkEnergy, which lifts them while
     * the brain is listening or speaking.
     */
    const RINGS = [
        // Outermost: thin, graduated, almost a scale.
        { r: 0.430, speed: -0.000030, width: 1.2, alpha: 0.30, gaps: 1, gapSize: 0, ticks: 160, glow: 6 },
        // The heavy one. Four bright arcs, wide gaps, strong bloom — this is
        // the ring the references are built around.
        { r: 0.380, speed: 0.000048, width: 7.0, alpha: 0.85, gaps: 4, gapSize: 0.62, glow: 26 },
        { r: 0.335, speed: -0.000065, width: 1.6, alpha: 0.50, gaps: 1, gapSize: 0, dots: 90, glow: 8 },
        { r: 0.300, speed: 0.000082, width: 3.5, alpha: 0.70, gaps: 7, gapSize: 0.16, glow: 16 },
        { r: 0.255, speed: -0.000110, width: 1.4, alpha: 0.40, gaps: 1, gapSize: 0, ticks: 72, glow: 6 },
        { r: 0.215, speed: 0.000150, width: 9.0, alpha: 0.55, gaps: 2, gapSize: 0.95, glow: 30 },
        { r: 0.170, speed: -0.000200, width: 2.2, alpha: 0.75, gaps: 5, gapSize: 0.22, glow: 14 },
        { r: 0.130, speed: 0.000260, width: 1.4, alpha: 0.55, gaps: 1, gapSize: 0, dots: 48, glow: 8 },
        { r: 0.100, speed: -0.000340, width: 4.0, alpha: 0.80, gaps: 3, gapSize: 0.38, glow: 20 },
    ];

    /*
     * What the reactor does while the brain is listening, thinking or speaking.
     *
     * The state is real — it comes from the conversation loop, not a timer — so
     * the centre of the picture tells you what the brain is doing without you
     * reading the button. Listening breathes slowly. Thinking turns faster and
     * steadily. Speaking pulses with the voice.
     */
    let talkState = 'idle';
    let talkEnergy = 0;

    window.brainMapState = function (state) {
        talkState = state || 'idle';
    };

    function stepTalkEnergy(now) {
        const target = { listening: 0.45, thinking: 1, speaking: 0.8 }[talkState] || 0;

        // Eased rather than switched, so a change of state is a swell instead
        // of a jump.
        talkEnergy += (target - talkEnergy) * 0.05;

        switch (talkState) {
            case 'listening':
                // A slow breath, the way something waiting looks.
                return 1 + Math.sin(now / 900) * 0.25 * talkEnergy;
            case 'speaking':
                // Faster and less even, closer to speech.
                return 1 + (Math.sin(now / 180) + Math.sin(now / 310)) * 0.3 * talkEnergy;
            case 'thinking':
                return 1 + 2.2 * talkEnergy;
            default:
                return 1 + talkEnergy;
        }
    }

    let spin = 0;

    function drawReactor() {
        const cx = width / 2;
        const cy = height / 2;
        const unit = Math.min(width, height);
        const energy = stepTalkEnergy(spin);

        ctx.save();
        ctx.translate(cx, cy);

        for (const ring of RINGS) {
            const radius = unit * ring.r * (1 + (energy - 1) * 0.04);
            const angle = spin * ring.speed * 1000 * energy;

            const colour = talkState === 'speaking' ? '#7bffa8' : '#5fe3f5';

            ctx.strokeStyle = colour;
            ctx.lineWidth = ring.width;

            // The bloom is what separates this from a wireframe. Canvas shadow
            // is expensive, so it is applied per ring rather than per segment.
            ctx.shadowColor = colour;
            ctx.shadowBlur = (ring.glow || 0) * (0.7 + energy * 0.3);

            // Rings of dots rather than a line, which the references use to
            // break up the concentric bands.
            if (ring.dots) {
                ctx.globalAlpha = ring.alpha * (0.75 + energy * 0.25);
                ctx.fillStyle = colour;

                for (let i = 0; i < ring.dots; i++) {
                    const a = angle + (i / ring.dots) * Math.PI * 2;
                    const big = i % 6 === 0;

                    ctx.beginPath();
                    ctx.arc(Math.cos(a) * radius, Math.sin(a) * radius,
                        big ? ring.width * 1.1 : ring.width * 0.55, 0, Math.PI * 2);
                    ctx.fill();
                }

                ctx.shadowBlur = 0;

                continue;
            }

            // Broken arcs rather than closed circles: a gap reads as machinery,
            // a full circle reads as a loading spinner.
            for (let i = 0; i < ring.gaps; i++) {
                const start = angle + (i / ring.gaps) * Math.PI * 2;
                const end = start + (Math.PI * 2) / ring.gaps - ring.gapSize;

                ctx.globalAlpha = ring.alpha * (0.75 + energy * 0.25);
                ctx.beginPath();
                ctx.arc(0, 0, radius, start, end);
                ctx.stroke();
            }

            ctx.shadowBlur = 0;

            if (ring.ticks) {
                ctx.globalAlpha = ring.alpha * 0.8;
                ctx.lineWidth = 1.3;
                ctx.shadowBlur = 4;

                for (let i = 0; i < ring.ticks; i++) {
                    const a = angle + (i / ring.ticks) * Math.PI * 2;
                    const long = i % 6 === 0;
                    const inner = radius - (long ? 9 : 4);

                    ctx.beginPath();
                    ctx.moveTo(Math.cos(a) * inner, Math.sin(a) * inner);
                    ctx.lineTo(Math.cos(a) * radius, Math.sin(a) * radius);
                    ctx.stroke();
                }

                ctx.shadowBlur = 0;
            }
        }

        // The core. Its brightness follows how much was just recalled, so the
        // one thing at the centre of the picture is not decoration after all.
        const core = unit * 0.055 * (1 + (energy - 1) * 0.12);
        const reach = core * (2.4 + recallPulse * 2 + (energy - 1) * 0.8);
        const glow = ctx.createRadialGradient(0, 0, 0, 0, 0, reach);

        const lit = 0.20 + recallPulse * 0.5 + (energy - 1) * 0.22;

        if (talkState === 'speaking') {
            glow.addColorStop(0, `rgba(150, 240, 170, ${lit})`);
            glow.addColorStop(0.5, `rgba(123, 196, 127, ${0.07 + recallPulse * 0.18})`);
            glow.addColorStop(1, 'rgba(123, 196, 127, 0)');
        } else {
            glow.addColorStop(0, `rgba(120, 235, 245, ${lit})`);
            glow.addColorStop(0.5, `rgba(77, 208, 225, ${0.07 + recallPulse * 0.18})`);
            glow.addColorStop(1, 'rgba(77, 208, 225, 0)');
        }

        ctx.globalAlpha = 1;
        ctx.fillStyle = glow;
        ctx.beginPath();
        ctx.arc(0, 0, reach, 0, Math.PI * 2);
        ctx.fill();

        const coreColour = talkState === 'speaking' ? '#b6ffd0' : '#cdf6fb';

        ctx.globalAlpha = 0.95;
        ctx.strokeStyle = coreColour;
        ctx.shadowColor = coreColour;
        ctx.shadowBlur = 30;
        ctx.lineWidth = 3;
        ctx.beginPath();
        ctx.arc(0, 0, core, 0, Math.PI * 2);
        ctx.stroke();
        ctx.shadowBlur = 0;

        // Inner detail. Three arcs and a filled centre, which is what makes it
        // read as a reactor rather than a circle.
        ctx.lineWidth = 1;
        ctx.globalAlpha = 0.55;

        for (let i = 0; i < 3; i++) {
            const a = -spin * 0.0009 + (i / 3) * Math.PI * 2;

            ctx.beginPath();
            ctx.arc(0, 0, core * 0.62, a, a + 1.5);
            ctx.stroke();
        }

        ctx.globalAlpha = 1;
        ctx.fillStyle = '#ffffff';
        ctx.shadowColor = coreColour;
        ctx.shadowBlur = 34;
        ctx.beginPath();
        ctx.arc(0, 0, core * 0.34, 0, Math.PI * 2);
        ctx.fill();
        ctx.shadowBlur = 0;

        drawGauges(unit);

        ctx.restore();
        ctx.globalAlpha = 1;
    }

    /*
     * Radial gauges around the reactor.
     *
     * The references have rings of these everywhere, attached to nothing. These
     * are attached to something: how full the drive is, how much of what the
     * brain knows was just recalled, and how much of its review queue is
     * waiting. A gauge that moves for a reason is worth looking at; one that
     * moves because it looks good teaches you to stop looking.
     */
    const gauges = { storage: 0, recall: 0, queue: 0 };

    window.brainMapGauges = function (values) {
        Object.assign(gauges, values);
    };

    const GAUGE_ARCS = [
        { key: 'storage', r: 0.265, from: -2.5, to: -1.2, colour: '#4dd0e1' },
        { key: 'recall', r: 0.265, from: -0.5, to: 0.8, colour: '#8fe9f2' },
        { key: 'queue', r: 0.265, from: 1.5, to: 2.8, colour: '#f0b26b' },
    ];

    function drawGauges(unit) {
        for (const g of GAUGE_ARCS) {
            const radius = unit * g.r;
            const value = Math.max(0, Math.min(1, gauges[g.key] || 0));

            // The track, so an empty gauge still reads as a gauge rather than
            // as something missing.
            ctx.globalAlpha = 0.10;
            ctx.strokeStyle = g.colour;
            ctx.lineWidth = 3;
            ctx.beginPath();
            ctx.arc(0, 0, radius, g.from, g.to);
            ctx.stroke();

            if (value <= 0) continue;

            ctx.globalAlpha = 0.55;
            ctx.beginPath();
            ctx.arc(0, 0, radius, g.from, g.from + (g.to - g.from) * value);
            ctx.stroke();
        }
    }

    /* ---------- the frame ---------- */

    /*
     * The instrument surround: an outer graduated ring and corner brackets.
     *
     * There was a rotating sweep here. It was removed because it made the whole
     * thing read as radar, and this is not a radar — nothing is being scanned
     * for, nothing arrives from a bearing. It is a reactor with a memory graph
     * around it, and a sweeping line told a different story than the truth.
     *
     * All of it is ornament and none of it carries a number. That line is held
     * everywhere in this file — the rings, the sweep and the graduations exist
     * to make the thing feel like an instrument, while every value shown
     * anywhere on the canvas is something the brain actually measured.
     */

    function drawOuterFrame() {
        const cx = width / 2;
        const cy = height / 2;
        const unit = Math.min(width, height);

        ctx.save();
        ctx.translate(cx, cy);

        // A wide ring near the edge of the canvas, graduated like a bearing
        // scale, turning slowly against the inner rings.
        const outer = unit * 0.455;
        const angle = spin * 0.00006;

        ctx.strokeStyle = '#4dd0e1';
        ctx.globalAlpha = 0.14;
        ctx.lineWidth = 1;

        for (let i = 0; i < 120; i++) {
            const a = angle + (i / 120) * Math.PI * 2;
            const major = i % 10 === 0;
            const len = major ? 14 : 6;

            ctx.beginPath();
            ctx.moveTo(Math.cos(a) * (outer - len), Math.sin(a) * (outer - len));
            ctx.lineTo(Math.cos(a) * outer, Math.sin(a) * outer);
            ctx.stroke();
        }

        // Four arcs just inside it, with wide gaps at the diagonals.
        ctx.globalAlpha = 0.20;
        ctx.lineWidth = 1.6;

        for (let q = 0; q < 4; q++) {
            const start = -angle * 2.2 + q * (Math.PI / 2) + 0.28;

            ctx.beginPath();
            ctx.arc(0, 0, outer - 26, start, start + Math.PI / 2 - 0.56);
            ctx.stroke();
        }

        ctx.restore();
        ctx.globalAlpha = 1;

        drawCornerBrackets();
    }

    // Brackets at the canvas corners, which is what makes the whole surface
    // read as a viewport rather than a background.
    function drawCornerBrackets() {
        const m = 18;
        const len = 34;

        ctx.globalAlpha = 0.35;
        ctx.strokeStyle = '#4dd0e1';
        ctx.lineWidth = 1.4;

        const corners = [
            [m, m, 1, 1],
            [width - m, m, -1, 1],
            [m, height - m, 1, -1],
            [width - m, height - m, -1, -1],
        ];

        for (const [x, y, dx, dy] of corners) {
            ctx.beginPath();
            ctx.moveTo(x + dx * len, y);
            ctx.lineTo(x, y);
            ctx.lineTo(x, y + dy * len);
            ctx.stroke();
        }

        ctx.globalAlpha = 1;
    }

    // A reticle on whatever the pointer is over, so hovering feels like
    // targeting rather than like a tooltip.
    function drawReticle(node) {
        const r = 20;

        ctx.save();
        ctx.translate(node.x, node.y);
        ctx.rotate(spin * 0.0012);

        ctx.globalAlpha = 0.7;
        ctx.strokeStyle = '#8fe9f2';
        ctx.lineWidth = 1.2;

        for (let q = 0; q < 4; q++) {
            const start = q * (Math.PI / 2) + 0.35;

            ctx.beginPath();
            ctx.arc(0, 0, r, start, start + Math.PI / 2 - 0.7);
            ctx.stroke();
        }

        ctx.restore();

        ctx.globalAlpha = 0.35;
        ctx.beginPath();
        ctx.moveTo(node.x - r - 8, node.y);
        ctx.lineTo(node.x - r + 2, node.y);
        ctx.moveTo(node.x + r - 2, node.y);
        ctx.lineTo(node.x + r + 8, node.y);
        ctx.stroke();

        ctx.globalAlpha = 1;
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

        drawOuterFrame();
        drawReactor();
        recallPulse *= 0.985;

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
            ctx.globalAlpha = 0.10 + (l.strength - 0.5) * 0.55;
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

            ctx.globalAlpha = 0.55 + n.glow * 0.45;
            ctx.fillStyle = colour;
            ctx.shadowColor = colour;
            ctx.shadowBlur = 6 + n.glow * 14;
            ctx.beginPath();
            ctx.arc(n.x, n.y, size, 0, Math.PI * 2);
            ctx.fill();
            ctx.shadowBlur = 0;

            n.glow *= 0.97;
        }

        if (hovered) {
            drawReticle(hovered);
            drawLabel(hovered);
        }

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

        spin = now;

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

        // Scaled by how much was recalled, so a question answered from one
        // memory does not look like one answered from a dozen.
        recallPulse = Math.min(1, (ids.length || 0) / 10);
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
