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
    /*
     * Two canvases: the reactor and the memory map.
     *
     * They were one, with the graph drawn over the rings. Separating them was
     * asked for and is better anyway — the reactor is what the brain is doing
     * and the map is what it knows, and they were competing for the same middle
     * of the same picture.
     *
     * One renderer still drives both. The drawing helpers take their target
     * from `ctx`, `width` and `height`, so a layer is selected by pointing
     * those at it rather than by passing a context through thirty functions.
     */
    const reactorCanvas = document.getElementById('brainmap');
    if (!reactorCanvas) return;

    const mapCanvas = document.getElementById('memorymap') || reactorCanvas;

    const layers = {
        reactor: { canvas: reactorCanvas, ctx: reactorCanvas.getContext('2d', { alpha: true }), w: 0, h: 0 },
        map: { canvas: mapCanvas, ctx: mapCanvas.getContext('2d', { alpha: true }), w: 0, h: 0 },
    };

    let ctx = layers.reactor.ctx;
    let width = 0;
    let height = 0;

    function useLayer(layer) {
        ctx = layer.ctx;
        width = layer.w;
        height = layer.h;
    }

    const COLOURS = {
        project: '#4dd0e1',
        document: '#7bc47f',
        conversation: '#f0b26b',
        unknown: '#7d8b9c',
    };

    let nodes = [];
    let links = [];
    let energy = 1;          // falls as the layout settles
    let running = true;
    let pointer = { x: -1e5, y: -1e5 };

    // Rises when memories are recalled and decays afterwards, so the core
    // brightens exactly when the brain has actually used what it knows.
    let recallPulse = 0;

    /* ---------- the view onto the map ---------- */

    /*
     * Zoom and pan apply to the memory map only.
     *
     * The reactor is deliberately outside this transform. It is the brain
     * rather than a memory: it stays the same size in the same place whatever
     * the map is doing, so it goes on reading as a status light while the field
     * is being dragged about behind it. Putting it in the transform was the
     * first thing tried and it was worse in an obvious way — zooming in to read
     * one memory blew the reactor up past the edges of the window.
     */
    const view = { scale: 1, x: 0, y: 0, touched: false };

    // Set by the drag handlers far below, but declared here with the rest of
    // the view state: draw() reads it to suppress hover while the map is being
    // moved, and a reader looking for it should find it where the other view
    // fields are.
    let dragging = false;

    // Out far enough to see the whole rim at once, in far enough to separate
    // memories that sit on top of each other. Beyond either end there is
    // nothing further to learn.
    const MIN_SCALE = 0.4;
    const MAX_SCALE = 4.5;

    /*
     * The map turns, slowly.
     *
     * A whole revolution takes five minutes. Slow enough that reading a label
     * is never a chase, fast enough that the map is visibly alive rather than a
     * picture. The rotation is applied at drawing time and changes nothing
     * about where the memories are in relation to each other — the arrangement
     * is the layout's, the turning is only the view.
     */
    const MAP_SECONDS_PER_TURN = 300;

    function mapAngle() {
        return turn(MAP_SECONDS_PER_TURN);
    }

    function applyView() {
        ctx.translate(width / 2 + view.x, height / 2 + view.y);
        ctx.scale(view.scale, view.scale);
        ctx.rotate(mapAngle());
        ctx.translate(-width / 2, -height / 2);
    }

    function toWorld(sx, sy) {
        const a = -mapAngle();
        const x = (sx - width / 2 - view.x) / view.scale;
        const y = (sy - height / 2 - view.y) / view.scale;

        return {
            x: x * Math.cos(a) - y * Math.sin(a) + width / 2,
            y: x * Math.sin(a) + y * Math.cos(a) + height / 2,
        };
    }

    function toScreen(wx, wy) {
        const a = mapAngle();
        const x = wx - width / 2;
        const y = wy - height / 2;

        return {
            x: (x * Math.cos(a) - y * Math.sin(a)) * view.scale + width / 2 + view.x,
            y: (x * Math.sin(a) + y * Math.cos(a)) * view.scale + height / 2 + view.y,
        };
    }

    /* ---------- what is actually being heard ---------- */

    /*
     * The live audio level, polled from the brain.
     *
     * This is the difference between a display that responds and one that
     * merely animates. The state alone — listening, speaking — could drive a
     * convincing-looking pulse from a timer, and that pulse would look exactly
     * the same if the microphone were muted or the voice had failed. These
     * numbers are measured off the real samples at both ends, so when the ring
     * moves there is sound, and when it is still there is not.
     *
     * Polled rather than streamed because the reading is a single number the
     * two audio paths are already computing, and a poll costs less than holding
     * a connection open for the life of the window.
     */
    let live = { source: '', level: 0, floor: 0 };
    let levelSmooth = 0;

    // Roughly six seconds of history at the poll rate, which is long enough for
    // a spoken sentence to be visible as a shape rather than a flicker.
    const WAVE_POINTS = 72;
    const wave = new Array(WAVE_POINTS).fill(0);
    let wavePos = 0;

    const LEVEL_INTERVAL = 90;

    /*
     * How much of the reading is sound somebody made.
     *
     * The raw level includes the room — a fan, a drive, traffic — and drawn
     * directly it produced a ring that thrashed about in a silent room and
     * therefore said nothing when anyone spoke. The brain measures this room's
     * floor at the start of every turn in order to know when a sentence has
     * ended, so the display subtracts the same measured number rather than a
     * guessed one, and what is left is the part worth showing.
     */
    function audible(reading) {
        const floor = reading.floor || 0;

        if (reading.level <= floor) return 0;

        return (reading.level - floor) / Math.max(0.05, 1 - floor);
    }

    async function pollLevel() {
        if (document.hidden) return;

        try {
            const r = await fetch('/api/level').then((x) => x.json());

            live = { source: r.source || '', level: r.level || 0, floor: r.floor || 0 };
        } catch {
            // A failed poll means no reading, not the last reading forever.
            live = { source: '', level: 0, floor: 0 };
        }

        wave[wavePos] = live.source ? audible(live) : 0;
        wavePos = (wavePos + 1) % WAVE_POINTS;
    }

    setInterval(pollLevel, LEVEL_INTERVAL);

    /*
     * The same reading, for anything else that wants to draw it.
     *
     * Shared rather than polled twice: the traces beside the message box and
     * the one in the voice card show exactly this number, and three pollers
     * asking the same question three times a second to get the same answer
     * would be silly. The array is handed over by reference and must not be
     * written to.
     */
    // Called when the card holding the reactor is shown again, since nothing
    // else tells a canvas that it has a size once more.
    window.brainMapResize = function () {
        resize();

        if (lastMap) layout(lastMap);
    };

    window.brainLevel = function () {
        return {
            source: live.source,
            level: live.source ? audible(live) : 0,
            smooth: levelSmooth,
            wave: wave,
            pos: wavePos,
            points: WAVE_POINTS,
        };
    };

    /* ---------- sizing ---------- */

    /*
     * How many pixels the canvases are actually drawn at.
     *
     * Measured on this machine: generating a frame costs three milliseconds for
     * the reactor and four for the map, yet only nineteen frames a second were
     * arriving and the renderer was using a whole processor core. The drawing
     * was never the problem — handing the result to the window was, because
     * these are two large canvases rasterised and composited in software on a
     * machine that is also running the model.
     *
     * That cost is proportional to area, so the area comes down. At 0.7 the
     * canvases carry about half the pixels; stretched back up by CSS the
     * difference is invisible on a picture made of glows and soft arcs, which
     * have no hard edges to lose. Measured again afterwards: a third of a core
     * instead of a whole one.
     */
    const RENDER_SCALE = 0.7;

    function measure(layer) {
        const dpr = Math.min(window.devicePixelRatio || 1, 2) * RENDER_SCALE;
        const rect = layer.canvas.getBoundingClientRect();

        // A canvas in a hidden card measures zero. Resizing to nothing would
        // throw the layout away and leave an empty canvas on the way back, so a
        // measurement of nothing is ignored rather than believed.
        if (rect.width < 2 || rect.height < 2) return false;

        layer.w = rect.width;
        layer.h = rect.height;
        layer.canvas.width = Math.floor(rect.width * dpr);
        layer.canvas.height = Math.floor(rect.height * dpr);
        layer.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

        return true;
    }

    function resize() {
        const gotReactor = measure(layers.reactor);

        if (layers.map !== layers.reactor) measure(layers.map);

        if (!gotReactor) return;

        useLayer(layers.reactor);
        energy = 1;
    }

    /* ---------- the shape the memory takes ---------- */

    /*
     * A brain, in outline: one large lobe and a smaller one behind and below.
     *
     * Taken from the shape used on the owner's own site, which draws its hero
     * as a brain-shaped cloud of points. It suits this better than the circle
     * it replaces, and it is the same kind of thing the circle was — a boundary
     * the layout is kept inside. It decides nothing about which memories end up
     * near which; that is the force layout's business, and the force layout is
     * working from real distances between real embeddings. This only says where
     * the edge is.
     *
     * Coordinates are in units of the containment radius, y downwards.
     */
    const BRAIN = {
        cerebrum: { x: -0.05, y: -0.08, rx: 0.96, ry: 0.76 },
        cerebellum: { x: 0.54, y: 0.52, rx: 0.40, ry: 0.29 },
    };

    // How far inside a lobe a point is: positive within, negative without, zero
    // exactly on the edge.
    function lobeDepth(lobe, px, py) {
        const dx = (px - lobe.x) / lobe.rx;
        const dy = (py - lobe.y) / lobe.ry;

        return 1 - (dx * dx + dy * dy);
    }

    function brainDepth(px, py) {
        return Math.max(lobeDepth(BRAIN.cerebrum, px, py),
            lobeDepth(BRAIN.cerebellum, px, py));
    }

    // Half the thickness of the body at a point, which is what gives the cloud
    // depth: thick through the middle, thin towards the edge, nothing outside.
    function brainHalf(px, py) {
        return 0.62 * Math.sqrt(Math.max(0, brainDepth(px, py)));
    }

    // Pull a point back inside, along the line from the nearer lobe's centre.
    function intoBrain(px, py) {
        const lobe = lobeDepth(BRAIN.cerebrum, px, py) >= lobeDepth(BRAIN.cerebellum, px, py)
            ? BRAIN.cerebrum
            : BRAIN.cerebellum;

        const dx = (px - lobe.x) / lobe.rx;
        const dy = (py - lobe.y) / lobe.ry;
        const d = Math.sqrt(dx * dx + dy * dy) || 1;

        return {
            x: lobe.x + ((px - lobe.x) / d) * 0.99,
            y: lobe.y + ((py - lobe.y) / d) * 0.99,
        };
    }

    /* ---------- layout ---------- */

    let lastMap = null;

    function layout(data) {
        lastMap = data;

        const categories = [...new Set(data.nodes.map((n) => n.category))];

        /*
         * Seeded by category around a disc, then left to the forces.
         *
         * It used to be pinned to a thin rim: every node held at a fixed angle
         * on a ring, because the reactor was drawn underneath and the middle
         * had to stay clear. The map has a panel of its own now, so that
         * constraint is gone and with it the shape it forced — a bare circle
         * with everything pressed against the edge, which said nothing about
         * the memories except that there were a lot of them.
         *
         * What is left is an ordinary force layout: memories that mean similar
         * things pull together, everything pushes apart a little, and the shape
         * that results is the shape of what the brain knows rather than one
         * imposed on it. Categories still start near one another, so related
         * things begin close and the layout settles sooner.
         */
        const order = data.nodes
            .map((n, i) => ({ i, c: categories.indexOf(n.category) }))
            .sort((a, b) => a.c - b.c || a.i - b.i);

        const rank = new Map(order.map((o, place) => [o.i, place]));
        const total = Math.max(data.nodes.length, 1);

        nodes = data.nodes.map((n, i) => {
            const angle = (rank.get(i) / total) * Math.PI * 2;
            const radius = Math.min(width, height) * (0.10 + Math.random() * 0.26);

            return {
                ...n,
                x: width / 2 + Math.cos(angle) * radius,
                y: height / 2 + Math.sin(angle) * radius,
                vx: 0,
                vy: 0,
                glow: 0,
                // Where this memory sits through the thickness of the shape.
                // Fixed once, so the cloud is the same body every time rather
                // than a shimmer that reshuffles itself.
                zUnit: Math.sin(i * 12.9898) * 0.94,
                sx: 0,
                sy: 0,
                sp: 1,
            };
        });

        const byId = new Map(nodes.map((n) => [n.id, n]));

        links = data.links
            .map((l) => ({ a: byId.get(l.source), b: byId.get(l.target), strength: l.strength }))
            .filter((l) => l.a && l.b);

        energy = 1;
    }

    function stepLayout() {
        useLayer(layers.map);

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

        /*
         * Held inside the shape.
         *
         * Corrected as a position rather than a force, because the simulation
         * cools on purpose: anything expressed as a force stops being applied
         * within about fifteen seconds, and a node outside the shape would
         * simply stay outside it.
         */
        const scale = Math.min(width, height) * 0.44;

        for (const n of nodes) {
            // A weak pull to the middle, so a loosely connected memory does not
            // drift to the edge and stay there.
            n.vx += (centreX - n.x) * 0.0009;
            n.vy += (centreY - n.y) * 0.0009;

            n.vx *= 0.86;
            n.vy *= 0.86;
            n.x += n.vx * energy;
            n.y += n.vy * energy;

            const ux = (n.x - centreX) / scale;
            const uy = (n.y - centreY) / scale;

            if (brainDepth(ux, uy) < 0) {
                const back = intoBrain(ux, uy);

                n.x += (centreX + back.x * scale - n.x) * 0.08;
                n.y += (centreY + back.y * scale - n.y) * 0.08;
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
     * Rotation is written as seconds per turn, not as radians per millisecond.
     *
     * It used to be the latter, and it was wrong by a factor of about two
     * hundred and fifty: the comment beside the numbers said the outermost ring
     * took six minutes to come round, and the arithmetic actually gave it just
     * under two seconds. Nobody caught it by reading, twice, because a column
     * of figures like 0.000045 does not mean anything to look at.
     *
     * A period in seconds does. Sixty is a minute; the sign is the direction.
     * A mistake in this table is now visible as a mistake.
     */
    function turn(seconds) {
        return (spin / 1000) * (Math.PI * 2) / seconds;
    }

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

        // The measured level is eased separately and faster. Slower than this
        // and the core lags visibly behind the voice; faster and it twitches on
        // individual syllables.
        levelSmooth += ((live.source ? audible(live) : 0) - levelSmooth) * 0.22;

        switch (talkState) {
            case 'listening':
            case 'speaking':
                // Driven by sound that is really there.
                //
                // The slow breath underneath is not pretending to be audio: it
                // is what the display does while the room is quiet, so that
                // waiting looks like waiting rather than like a frozen window.
                // Everything above it is the measured level.
                return 1
                    + Math.sin(now / 1400) * 0.10 * talkEnergy
                    + levelSmooth * 1.8;
            case 'thinking':
                // Nothing is making a sound while the model runs, so there is
                // no level to show and none is invented. A steady swell, which
                // says only what it knows: the brain is working.
                return 1 + 0.55 * talkEnergy;
            default:
                return 1 + 0.25 * talkEnergy;
        }
    }

    /*
     * Glow, without canvas shadow.
     *
     * The bloom used to come from ctx.shadowBlur, and it was measured at about
     * forty-five per cent of a processor core on this machine — nearly half of
     * everything the interface was doing. Canvas shadow is a real blur, and a
     * real blur over a canvas this size, several times a frame, is not
     * something a machine already running a language model can spare.
     *
     * The same look comes from drawing the shape two extra times, wider and
     * fainter, underneath itself. It is not a true Gaussian, and side by side
     * an expert could tell; against the cost it is not a close decision.
     */
    function glowStroke(paint, width, alpha, strength) {
        if (strength > 0) {
            ctx.lineWidth = width + strength * 0.55;
            ctx.globalAlpha = alpha * 0.10;
            paint();

            ctx.lineWidth = width + strength * 0.22;
            ctx.globalAlpha = alpha * 0.20;
            paint();
        }

        ctx.lineWidth = width;
        ctx.globalAlpha = alpha;
        paint();
    }

    let spin = 0;

    /*
     * The core, drawn as a wireframe globe.
     *
     * This replaced a stack of flat concentric rings. The rings were closer to
     * a control-panel dial; a turning globe is closer to the thing this is
     * meant to evoke, and it has the advantage of showing its own rotation —
     * you can see which way it is going, which a circle cannot show at all.
     *
     * All of it is ornament and none of it pretends otherwise. The one thing it
     * reads from the world is how bright it is, which follows the measured
     * audio level and how much was just recalled. Everything drawn over it —
     * the gauges, the voice trace — is real.
     */
    const GLOBE = {
        // Seconds for the sphere to come round once. Slow enough to read a
        // label across it, fast enough that a glance a minute later shows a
        // change. Written as a period rather than as radians per millisecond,
        // for the reason given on turn().
        seconds: 150,
        // How far the sphere is tilted towards the viewer: zero is edge on, one
        // is looking down the pole.
        tilt: 0.30,
        latitudes: 9,
        longitudes: 18,
        points: 620,
    };

    // Four orbits, wide and nearly flat, each at its own tilt and rate.
    const ORBITS = [
        { rx: 3.05, ry: 0.22, lean: -0.26, seconds: 300 },
        { rx: 2.70, ry: 0.50, lean: 0.10, seconds: -220 },
        { rx: 2.35, ry: 0.72, lean: 0.30, seconds: 380 },
        { rx: 2.90, ry: 0.34, lean: -0.08, seconds: -460 },
    ];

    /*
     * A four-pointed flare, the way a bright light reads through a lens.
     *
     * Drawn rather than blurred: two crossed gradients and a core. A shadow
     * blur would look much the same and cost more than everything else in this
     * function put together.
     */
    function drawFlare(x, y, size, colour, alpha) {
        const halo = ctx.createRadialGradient(x, y, 0, x, y, size);

        halo.addColorStop(0, '#ffffff');
        halo.addColorStop(0.25, colour);
        halo.addColorStop(1, colour + '00');

        ctx.globalAlpha = alpha;
        ctx.fillStyle = halo;
        ctx.beginPath();
        ctx.arc(x, y, size, 0, Math.PI * 2);
        ctx.fill();

        ctx.globalAlpha = alpha * 0.75;
        ctx.strokeStyle = colour;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(x - size * 2.6, y);
        ctx.lineTo(x + size * 2.6, y);
        ctx.moveTo(x, y - size * 1.7);
        ctx.lineTo(x, y + size * 1.7);
        ctx.stroke();
    }

    function drawGlobe(unit, energy) {
        const R = unit * 0.30;
        const lit = 0.6 + (energy - 1) * 0.45 + recallPulse * 0.3;
        const colour = talkState === 'speaking' ? '#7bffa8' : '#5fe3f5';
        const phase = turn(GLOBE.seconds);

        // The halo. This is most of why the sphere reads as lit from within.
        const halo = ctx.createRadialGradient(0, 0, R * 0.55, 0, 0, R * 1.85);

        halo.addColorStop(0, talkState === 'speaking'
            ? `rgba(123, 255, 168, ${0.16 * lit})`
            : `rgba(95, 227, 245, ${0.16 * lit})`);
        halo.addColorStop(0.45, talkState === 'speaking'
            ? `rgba(60, 170, 130, ${0.07 * lit})`
            : `rgba(40, 130, 190, ${0.07 * lit})`);
        halo.addColorStop(1, 'rgba(10, 30, 60, 0)');

        ctx.globalAlpha = 1;
        ctx.fillStyle = halo;
        ctx.beginPath();
        ctx.arc(0, 0, R * 1.85, 0, Math.PI * 2);
        ctx.fill();

        // The floor: dotted ellipses under the sphere, which is what gives it
        // somewhere to be rather than floating in nothing.
        ctx.fillStyle = colour;

        for (let ring = 0; ring < 3; ring++) {
            const rx = R * (1.30 + ring * 0.30);
            const ry = rx * 0.19;
            const dots = 60 + ring * 18;

            ctx.globalAlpha = (0.20 - ring * 0.05) * lit;
            ctx.beginPath();

            for (let i = 0; i < dots; i++) {
                const a = (i / dots) * Math.PI * 2;
                const x = Math.cos(a) * rx;
                const y = R * 1.02 + Math.sin(a) * ry;

                ctx.moveTo(x + 0.9, y);
                ctx.arc(x, y, 0.9, 0, Math.PI * 2);
            }

            ctx.fill();
        }

        // The body, so the mesh reads as the surface of something solid.
        const body = ctx.createRadialGradient(-R * 0.3, -R * 0.35, 0, 0, 0, R);

        body.addColorStop(0, `rgba(60, 150, 210, ${0.30 * lit})`);
        body.addColorStop(1, `rgba(10, 40, 80, ${0.42 * lit})`);

        ctx.fillStyle = body;
        ctx.beginPath();
        ctx.arc(0, 0, R, 0, Math.PI * 2);
        ctx.fill();

        ctx.strokeStyle = colour;
        ctx.lineWidth = 1;

        // Latitudes: circles on the sphere, which project to ellipses whose
        // width falls away towards the poles.
        ctx.globalAlpha = 0.30 * lit;
        ctx.beginPath();

        for (let i = 1; i < GLOBE.latitudes; i++) {
            const lat = (i / GLOBE.latitudes) * Math.PI - Math.PI / 2;
            const rx = R * Math.cos(lat);

            if (rx < 1) continue;

            ctx.ellipse(0, R * Math.sin(lat) * GLOBE.tilt, rx,
                Math.max(0.6, rx * GLOBE.tilt), 0, 0, Math.PI * 2);
        }

        ctx.stroke();

        // Longitudes: half-circles through the poles. Seen from here each is an
        // ellipse whose width is how far round it has turned, so the sphere
        // appears to rotate without anything being rotated.
        for (let i = 0; i < GLOBE.longitudes; i++) {
            const a = phase + (i / GLOBE.longitudes) * Math.PI;
            const face = Math.abs(Math.cos(a));

            ctx.globalAlpha = (0.08 + face * 0.26) * lit;
            ctx.beginPath();
            ctx.ellipse(0, 0, Math.max(0.6, face * R), R, 0, 0, Math.PI * 2);
            ctx.stroke();
        }

        // The limb.
        glowStroke(() => {
            ctx.beginPath();
            ctx.arc(0, 0, R, 0, Math.PI * 2);
            ctx.stroke();
        }, 1.4, 0.62 * lit, 16);

        /*
         * Points on the surface.
         *
         * Placed by the golden angle in longitude against an even spread in the
         * sine of latitude, which is what distributes points evenly over a
         * sphere instead of crowding them at the poles. Fixed positions carried
         * round by the same rotation, so it is the same body each time rather
         * than a shimmer.
         */
        for (let pass = 0; pass < 2; pass++) {
            // Back of the sphere first, dimmer, then the front over it.
            ctx.globalAlpha = (pass === 0 ? 0.22 : 0.95) * lit;
            ctx.fillStyle = pass === 0 ? colour : '#dff8fd';
            ctx.beginPath();

            for (let i = 0; i < GLOBE.points; i++) {
                const lat = Math.asin((i / GLOBE.points) * 2 - 1);
                const lon = phase + i * 2.39996;
                const front = Math.cos(lat) * Math.cos(lon);

                if ((pass === 0) === (front > 0)) continue;

                const x = Math.cos(lat) * Math.sin(lon) * R;
                const y = Math.sin(lat) * R * GLOBE.tilt
                    - Math.cos(lat) * Math.cos(lon) * R * GLOBE.tilt * 0.5;

                // Brighter where the light is, which is up and to the left.
                const size = pass === 0 ? 0.75 : 0.9 + Math.abs(front) * 1.0;

                ctx.moveTo(x + size, y);
                ctx.arc(x, y, size, 0, Math.PI * 2);
            }

            ctx.fill();
        }

        // Orbits, and one bright point running each of them.
        for (const o of ORBITS) {
            ctx.globalAlpha = 0.20 * lit;
            ctx.strokeStyle = colour;
            ctx.lineWidth = 1;
            ctx.beginPath();
            ctx.ellipse(0, 0, R * o.rx, R * o.ry, o.lean, 0, Math.PI * 2);
            ctx.stroke();

            const a = turn(o.seconds);
            const px = Math.cos(a) * R * o.rx;
            const py = Math.sin(a) * R * o.ry;

            drawFlare(
                px * Math.cos(o.lean) - py * Math.sin(o.lean),
                px * Math.sin(o.lean) + py * Math.cos(o.lean),
                R * 0.10, colour, 0.9 * lit);
        }

        ctx.globalAlpha = 1;
    }

    function drawReactor() {
        const unit = Math.min(width, height);
        const energy = stepTalkEnergy(spin);

        ctx.save();
        ctx.translate(width / 2, height / 2);

        drawGlobe(unit, energy);

        /*
         * Nothing else is drawn around the sphere.
         *
         * There were three more things here: a graduated surround ring, three
         * gauge arcs, and a ring tracing the voice. All were removed. The
         * surround was ornament competing with the orbits. The gauges were real
         * but said the same as the System monitor card, and a figure shown
         * twice in one screen is a figure someone has to reconcile. The voice
         * trace is drawn in two other places already, both larger and easier to
         * read than a thin ring behind a title.
         *
         * What is left still answers "what is it doing": the sphere's
         * brightness follows the measured sound and how much was just recalled.
         */
        ctx.restore();
        ctx.globalAlpha = 1;
    }

    /*
     * The voice ring: a trace of the sound itself.
     *
     * Each point is one measured reading, the newest at the top, older ones
     * running clockwise — so a spoken sentence appears at twelve o'clock and
     * winds away as the seconds pass. When nobody is talking every reading is
     * zero and the ring is a plain circle, which is the honest picture of a
     * quiet room.
     *
     * Cyan while it is the microphone, green while it is the brain's own voice,
     * because knowing which of you is making the sound is most of the value.
     */
    function drawVoiceRing(unit) {
        const base = unit * 0.455;
        const reach = unit * 0.035;
        const mic = live.source === 'mic';
        const colour = live.source === 'voice' ? '#7bffa8' : '#5fe3f5';

        // Fades out when there has been nothing to show for a while rather than
        // sitting there as a hard circle competing with the rings.
        const presence = Math.min(1, levelSmooth * 6 + (live.source ? 0.35 : 0));

        if (presence < 0.02) return;

        ctx.save();
        ctx.globalAlpha = 0.20 + presence * 0.62;
        ctx.strokeStyle = colour;
        ctx.lineWidth = 1.4 + presence * 1.6;
        ctx.beginPath();

        // Drawn through the midpoints between samples, with each sample as the
        // control point of a curve. Straight segments between seventy-odd
        // readings made a spiky polygon that read as a shape in its own right
        // rather than as a line following a voice.
        const at = (i) => {
            // Read backwards from the newest sample so the trace does not jump
            // by a whole turn each time the ring buffer wraps.
            const sample = wave[(wavePos - 1 - (i % WAVE_POINTS) + WAVE_POINTS * 2) % WAVE_POINTS];
            const angle = -Math.PI / 2 + (i / WAVE_POINTS) * Math.PI * 2;
            const r = base + sample * reach;

            return { x: Math.cos(angle) * r, y: Math.sin(angle) * r };
        };

        let previous = at(0);

        ctx.moveTo((previous.x + at(1).x) / 2, (previous.y + at(1).y) / 2);

        for (let i = 1; i <= WAVE_POINTS; i++) {
            const point = at(i);
            const next = at(i + 1);

            ctx.quadraticCurveTo(point.x, point.y,
                (point.x + next.x) / 2, (point.y + next.y) / 2);

            previous = point;
        }

        ctx.closePath();
        ctx.stroke();

        // A second, wider pass underneath for the bloom.
        ctx.globalAlpha = (0.20 + presence * 0.62) * 0.18;
        ctx.lineWidth = 5 + presence * 7;
        ctx.stroke();

        // A marker at the newest reading, so it is obvious which way time runs.
        const head = base + wave[(wavePos - 1 + WAVE_POINTS) % WAVE_POINTS] * reach;


        ctx.globalAlpha = 0.5 + presence * 0.5;
        ctx.fillStyle = mic ? '#cdf6fb' : colour;
        ctx.beginPath();
        ctx.arc(0, -head, 2.4 + presence * 2, 0, Math.PI * 2);
        ctx.fill();

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
        { key: 'storage', r: 0.425, from: -2.5, to: -1.2, colour: '#4dd0e1' },
        { key: 'recall', r: 0.425, from: -0.5, to: 0.8, colour: '#8fe9f2' },
        { key: 'queue', r: 0.425, from: 1.5, to: 2.8, colour: '#f0b26b' },
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
        const outer = unit * 0.487;
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
            ctx.arc(0, 0, outer * 0.88, start, start + Math.PI / 2 - 0.56);
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
    /*
     * Drawn in screen coordinates, not map coordinates.
     *
     * The reticle and the label are annotations on what the pointer is over, so
     * they stay the size they were designed at. Inside the map transform they
     * would grow with the zoom, and at four times in the label was wider than
     * the window.
     */
    function drawReticle(at) {
        const r = 20;

        ctx.save();
        ctx.translate(at.x, at.y);
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
        ctx.moveTo(at.x - r - 8, at.y);
        ctx.lineTo(at.x - r + 2, at.y);
        ctx.moveTo(at.x + r - 2, at.y);
        ctx.lineTo(at.x + r + 8, at.y);
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
    function draw() {
        useLayer(layers.reactor);
        ctx.clearRect(0, 0, width, height);

        recallPulse *= 0.985;

        /*
         * No drifting web behind the sphere.
         *
         * There was one, from when this canvas was the background of the whole
         * window and needed something in the corners. Inside a panel with a
         * globe in it, it is another thing in the same square inch competing
         * for attention — and it was the largest remaining piece of drawing per
         * frame. The field of memories in the map panel is the real version of
         * what it was imitating.
         */
        drawReactor();

        useLayer(layers.map);
        ctx.clearRect(0, 0, width, height);

        // From here to the matching restore, everything is drawn in map
        // coordinates: this is the layer that zooms and pans.
        ctx.save();
        applyView();

        /*
         * Links first, so nodes sit on top of them.
         *
         * Drawn as arcs following the rim rather than as straight lines.
         *
         * A straight line between two memories on opposite sides of the ring is
         * a chord straight through the middle, and with a couple of hundred of
         * them the centre of the picture became a solid mesh — the reactor was
         * behind a ball of wool, and no structure was readable in the wool
         * either. Interpolating in polar coordinates instead bends each link
         * around the ring. Nothing is hidden by this: every link still joins
         * exactly the two memories it did, and its strength still sets how
         * brightly it is drawn. It simply takes the long way round.
         */
        ctx.strokeStyle = '#4dd0e1';
        ctx.lineWidth = 0.9;

        // The centre of the map in map coordinates, which is what the node
        // positions are in.
        const centreX = width / 2;
        const centreY = height / 2;

        /*
         * Drawn in a handful of batches rather than one stroke per link.
         *
         * Each stroke() is a draw call, and with a couple of hundred links that
         * was a couple of hundred of them every frame — measured at about half
         * a processor between this and the node glow below, on a machine that
         * is also running the model. Links only differ by how bright they are,
         * so they are bucketed by strength and each bucket is drawn as one
         * path. Four calls instead of two hundred and seventy-eight, and the
         * picture is the same to within a step of opacity.
         */
        const BANDS = 4;
        const banded = [];

        for (let i = 0; i < BANDS; i++) banded.push([]);

        for (const l of links) {
            const band = Math.min(BANDS - 1, Math.floor((l.strength - 0.5) * 2 * BANDS));

            banded[Math.max(0, band)].push(l);
        }

        for (let band = 0; band < BANDS; band++) {
            if (!banded[band].length) continue;

            ctx.globalAlpha = 0.07 + (band / (BANDS - 1)) * 0.19;
            ctx.beginPath();

            for (const l of banded[band]) {
                const ax = l.a.x - centreX;
                const ay = l.a.y - centreY;
                const bx = l.b.x - centreX;
                const by = l.b.y - centreY;

                const ra = Math.sqrt(ax * ax + ay * ay);
                const rb = Math.sqrt(bx * bx + by * by);

                const from = Math.atan2(ay, ax);
                let sweep = Math.atan2(by, bx) - from;

                // The short way round, so a link across the seam does not
                // travel most of the circle to get somewhere close by.
                while (sweep > Math.PI) sweep -= Math.PI * 2;
                while (sweep < -Math.PI) sweep += Math.PI * 2;

                const STEPS = 10;

                for (let i = 0; i <= STEPS; i++) {
                    const t = i / STEPS;
                    const angle = from + sweep * t;

                    // Dipped inward at the middle, so links between neighbours
                    // are visibly separate from the rim itself.
                    const r = (ra + (rb - ra) * t) * (1 - Math.sin(t * Math.PI) * 0.13);

                    const x = centreX + Math.cos(angle) * r;
                    const y = centreY + Math.sin(angle) * r;

                    if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
                }
            }

            ctx.stroke();
        }

        let hovered = null;

        // The pointer is compared in map coordinates, and the tolerance is
        // divided by the zoom. Without that division a node is easy to hit when
        // zoomed in and nearly impossible when zoomed out, because the target
        // is a fixed distance in map units while the pointer moves in screen
        // ones.
        const reach = 150 / (view.scale * view.scale);
        const aim = dragging ? { x: -1e5, y: -1e5 } : toWorld(pointer.x, pointer.y);

        for (const n of nodes) {
            const dx = n.x - aim.x;
            const dy = n.y - aim.y;
            const near = dx * dx + dy * dy < reach;

            if (near) hovered = n;

            const colour = COLOURS[n.category] || COLOURS.unknown;
            const size = 3 + n.glow * 4.5 + (near ? 2 : 0);
            const lit = n.glow > 0.01 || near;

            /*
             * Shadow blur only on the few nodes that are lit.
             *
             * Every node used to carry a shadowBlur, which meant a hundred and
             * twenty-seven blurred fills per frame. Canvas shadow is the most
             * expensive thing on this page by a wide margin and it was being
             * spent almost entirely on nodes at rest, where the blur is a faint
             * halo nobody can see. It is kept for the handful that are actually
             * glowing — the memories a reply just used, and whatever the
             * pointer is over — which is where it means something.
             */
            if (lit) {
                ctx.globalAlpha = 0.24 * (n.glow || 0.7);
                ctx.fillStyle = colour;
                ctx.beginPath();
                ctx.arc(n.x, n.y, size * 4, 0, Math.PI * 2);
                ctx.fill();

                ctx.shadowColor = colour;
                ctx.shadowBlur = 6 + n.glow * 14;
            }

            ctx.globalAlpha = 0.55 + n.glow * 0.45;
            ctx.fillStyle = colour;
            ctx.beginPath();
            ctx.arc(n.x, n.y, size, 0, Math.PI * 2);
            ctx.fill();

            if (lit) ctx.shadowBlur = 0;

            n.glow *= 0.97;
        }

        ctx.restore();

        if (hovered) {
            const at = toScreen(hovered.x, hovered.y);

            drawReticle(at);
            drawLabel(hovered, at);
        }

        drawViewHint();
        drawViewState();

        ctx.globalAlpha = 1;
    }

    /*
     * A one-line note saying the map can be moved.
     *
     * Shown only until the first zoom or drag. An interface that can be
     * manipulated and does not say so is a feature nobody finds; one that keeps
     * saying so after you have learned it is nagging.
     */
    let hintFade = 1;

    function drawViewHint() {
        if (view.touched) hintFade *= 0.94;

        if (hintFade < 0.02) return;

        ctx.save();
        ctx.globalAlpha = 0.34 * hintFade;
        ctx.fillStyle = '#7d8b9c';
        ctx.font = '10px ui-monospace, monospace';
        ctx.textAlign = 'center';
        // Clear of the composer, which floats over the bottom of the canvas.
        ctx.fillText('scroll to zoom · drag to move · double-click to reset',
            width / 2, height - 104);
        ctx.restore();
    }

    /*
     * A readout of the current zoom, shown only while it is not 1.
     *
     * Once the map can be moved it is possible to get lost in it, and the way
     * back has to be visible rather than remembered.
     */
    function drawViewState() {
        if (Math.abs(view.scale - 1) < 0.01 && !view.x && !view.y) return;

        ctx.save();
        ctx.globalAlpha = 0.5;
        ctx.fillStyle = '#8fe9f2';
        ctx.font = '10px ui-monospace, monospace';
        ctx.textAlign = 'left';
        ctx.fillText(`${view.scale.toFixed(1)}x`, 14, height - 104);
        ctx.restore();
    }

    function drawLabel(node, at) {
        const padding = 7;
        ctx.font = '11px ui-monospace, monospace';

        const text = node.label;
        const w = ctx.measureText(text).width + padding * 2;
        const x = Math.min(Math.max(at.x - w / 2, 4), width - w - 4);
        const y = at.y - 26;

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

        /*
         * Thirty frames a second, not sixty.
         *
         * Nothing here moves fast enough to need more. The outermost ring takes
         * six minutes to come round and the map five; at sixty frames a second
         * the second half of every pair was redrawing a picture identical to
         * the first. On a machine whose processor is already running the model,
         * that half was coming out of the answers.
         *
         * Dragging is the exception, because a map being dragged is the one
         * thing here that follows a hand.
         */
        const busy = energy >= 0.02
            || levelSmooth > 0.01
            || talkState === 'thinking'
            || talkState === 'speaking'
            || recallPulse > 0.02;

        const interval = dragging ? 16 : busy ? 33 : 60;

        spin = now;

        if (now - last >= interval) {
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

    let dragFrom = null;

    function pointerAt(e) {
        const rect = mapCanvas.getBoundingClientRect();

        return { x: e.clientX - rect.left, y: e.clientY - rect.top };
    }

    mapCanvas.addEventListener('mousemove', (e) => {
        const at = pointerAt(e);

        pointer.x = at.x;
        pointer.y = at.y;

        if (!dragging || !dragFrom) return;

        // Panning moves the view by the screen distance travelled, not the map
        // distance. Dividing by the scale here would make the map slide out
        // from under the pointer at any zoom other than 1.
        view.x = dragFrom.vx + (at.x - dragFrom.x);
        view.y = dragFrom.vy + (at.y - dragFrom.y);
    });

    mapCanvas.addEventListener('mousedown', (e) => {
        if (e.button !== 0) return;

        const at = pointerAt(e);

        dragging = true;
        view.touched = true;
        dragFrom = { x: at.x, y: at.y, vx: view.x, vy: view.y };
        mapCanvas.style.cursor = 'grabbing';

        // The layout has usually cooled to a stop by the time anyone explores
        // it. A little energy here lets the graph settle around wherever the
        // view ends up rather than sitting rigid.
        energy = Math.max(energy, 0.15);
    });

    window.addEventListener('mouseup', () => {
        dragging = false;
        dragFrom = null;
        mapCanvas.style.cursor = '';
    });

    mapCanvas.addEventListener('mouseleave', () => {
        pointer.x = pointer.y = -1e5;
    });

    /*
     * Zoom about the pointer rather than the centre.
     *
     * Zooming about the centre is easier to write and much worse to use: the
     * thing you are leaning towards slides away as you zoom in, so you spend
     * the whole time chasing it with the pan. Holding the point under the
     * pointer still means you zoom into whatever you were looking at.
     */
    mapCanvas.addEventListener('wheel', (e) => {
        e.preventDefault();

        const at = pointerAt(e);
        const before = toWorld(at.x, at.y);

        const step = Math.exp(-e.deltaY * 0.0014);
        const scale = Math.min(MAX_SCALE, Math.max(MIN_SCALE, view.scale * step));

        if (scale === view.scale) return;

        view.scale = scale;
        view.touched = true;

        const after = toScreen(before.x, before.y);

        view.x += at.x - after.x;
        view.y += at.y - after.y;
    }, { passive: false });

    mapCanvas.addEventListener('dblclick', () => {
        view.scale = 1;
        view.x = 0;
        view.y = 0;
        energy = Math.max(energy, 0.2);
    });

    // Nothing should be simulated for a window nobody is looking at.
    document.addEventListener('visibilitychange', () => {
        running = !document.hidden;
        if (running) requestAnimationFrame(frame);
    });

    /*
     * A resize changes where the rim is, so the layout is rebuilt to match.
     *
     * Without this the nodes keep positions worked out for the old size and are
     * then dragged into the new band from wherever they happened to be, which
     * is how the graph ended up lopsided every time the window was maximised.
     */
    window.addEventListener('resize', () => {
        resize();

        if (lastMap) layout(lastMap);
    });

    resize();
    window.brainMapReload();
    requestAnimationFrame(frame);
})();
