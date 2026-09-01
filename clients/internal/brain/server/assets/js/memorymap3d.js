/*
 * The memory map.
 *
 * Every point is something the brain knows. Every line joins two memories that
 * mean similar things, and the similarity is the real distance between their
 * embeddings — the same comparison a reply uses to remember. The arrangement is
 * a force layout working from those distances, so what clusters is what the
 * brain actually treats as related. Nothing here is a shape imposed on the data.
 *
 * Two earlier versions are worth recording. The first pinned every node to a
 * ring, because the core was drawn underneath and the middle had to stay clear;
 * that produced a bare circle with everything pressed to the edge, which said
 * nothing except that there were a lot of memories. The second held them in a
 * brain-shaped outline, which was a picture of a brain rather than a picture of
 * what this brain knows. The lines were curved in both, to keep them off the
 * core — with the core gone from this panel there is no reason for a link
 * between two memories to take the long way round, so they are straight.
 */

import * as THREE from './vendor/three.module.js';
import { onRecall } from './signals.js';

const canvas = document.getElementById('memorymap');

const COLOURS = {
    project: 0x4dd0e1,
    document: 0x7bc47f,
    conversation: 0xf0b26b,
    website: 0xb39ddb,
    unknown: 0x7d8b9c,
};

/** How long the layout is allowed to settle before it is left alone. */
const SETTLE_STEPS = 420;

/** Seconds for the map to turn once. Slow enough that reading a label is never
 *  a chase. */
const SECONDS_PER_TURN = 240;

/** The radius the settled layout is scaled to, so the camera can be placed from
 *  arithmetic rather than from a number that happened to look right once. */
const LAYOUT_RADIUS = 3.6;

function dotTexture(size = 64) {
    const c = document.createElement('canvas');

    c.width = c.height = size;

    const ctx = c.getContext('2d');
    const g = ctx.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2);

    g.addColorStop(0, 'rgba(255,255,255,1)');
    g.addColorStop(0.4, 'rgba(255,255,255,0.75)');
    g.addColorStop(1, 'rgba(255,255,255,0)');

    ctx.fillStyle = g;
    ctx.fillRect(0, 0, size, size);

    return new THREE.CanvasTexture(c);
}

/*
 * A force layout in three dimensions.
 *
 * Run to a stop once rather than animated forever: the arrangement is what
 * matters, not watching it arrive, and a simulation left running is a
 * simulation still costing a processor that is busy thinking.
 */
function settle(nodes, links) {
    for (let step = 0; step < SETTLE_STEPS; step++) {
        const cooling = 1 - step / SETTLE_STEPS;

        for (let i = 0; i < nodes.length; i++) {
            const a = nodes[i];

            for (let j = i + 1; j < nodes.length; j++) {
                const b = nodes[j];

                let dx = b.x - a.x;
                let dy = b.y - a.y;
                let dz = b.z - a.z;

                let d2 = dx * dx + dy * dy + dz * dz;

                if (d2 < 0.0001) {
                    dx = Math.random() - 0.5;
                    dy = Math.random() - 0.5;
                    dz = Math.random() - 0.5;
                    d2 = 0.0001;
                }

                const force = 0.06 / d2;
                const d = Math.sqrt(d2);

                a.x -= (dx / d) * force;
                a.y -= (dy / d) * force;
                a.z -= (dz / d) * force;
                b.x += (dx / d) * force;
                b.y += (dy / d) * force;
                b.z += (dz / d) * force;
            }
        }

        for (const link of links) {
            const dx = link.b.x - link.a.x;
            const dy = link.b.y - link.a.y;
            const dz = link.b.z - link.a.z;
            const d = Math.sqrt(dx * dx + dy * dy + dz * dz) || 1;

            // Stronger similarity pulls harder and rests closer, so distance on
            // screen means something rather than being an artefact of the
            // simulation.
            const rest = 1.6 - link.strength;
            const pull = ((d - rest) / d) * 0.045 * link.strength * cooling;

            link.a.x += dx * pull;
            link.a.y += dy * pull;
            link.a.z += dz * pull;
            link.b.x -= dx * pull;
            link.b.y -= dy * pull;
            link.b.z -= dz * pull;
        }

        // A weak pull to the middle, so a loosely connected memory does not
        // drift away on its own.
        for (const n of nodes) {
            n.x -= n.x * 0.004;
            n.y -= n.y * 0.004;
            n.z -= n.z * 0.004;
        }
    }

    frame(nodes);
}

/*
 * Scale the settled layout to a known size.
 *
 * The forces have no natural unit: the same arrangement comes out twice as wide
 * with twice as many memories, because every node pushes on every other. Left
 * alone, the camera framed it correctly at a hundred memories and sat inside
 * the cloud at a hundred and thirty. What the layout decides is the shape and
 * the relative distances; how large that shape is drawn is this program's
 * business, not the simulation's.
 */
function frame(nodes) {
    let middle = { x: 0, y: 0, z: 0 };

    for (const n of nodes) {
        middle.x += n.x / nodes.length;
        middle.y += n.y / nodes.length;
        middle.z += n.z / nodes.length;
    }

    let furthest = 0;

    for (const n of nodes) {
        n.x -= middle.x;
        n.y -= middle.y;
        n.z -= middle.z;

        furthest = Math.max(furthest, Math.hypot(n.x, n.y, n.z));
    }

    if (furthest < 0.001) return;

    const scale = LAYOUT_RADIUS / furthest;

    for (const n of nodes) {
        n.x *= scale;
        n.y *= scale;
        n.z *= scale;
    }
}

export function startMemoryMap() {
    if (!canvas) return;

    const renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true });

    renderer.setClearAlpha(0);

    const scene = new THREE.Scene();
    const camera = new THREE.PerspectiveCamera(45, 1, 0.1, 200);
    const world = new THREE.Group();

    scene.add(world);

    let nodes = [];
    let links = [];
    let cloud = null;
    let wires = null;
    let sizes = null;
    let tints = null;

    /* ---------- how the map is looked at ---------- */

    // Far enough back that a sphere of LAYOUT_RADIUS fits the shorter side of
    // the panel, with a margin. Worked out rather than guessed, because a
    // guessed distance framed a hundred memories and sat inside a hundred and
    // thirty.
    const fitted = (LAYOUT_RADIUS / Math.tan((45 * Math.PI) / 360)) * 1.25;

    const view = { yaw: 0, pitch: 0.25, distance: fitted, spin: true };
    const pointer = new THREE.Vector2(-10, -10);
    const raycaster = new THREE.Raycaster();

    raycaster.params.Points.threshold = 0.16;

    let dragging = false;
    let dragged = 0;

    // Set once the reader has turned the map by hand, after which it is theirs.
    let moved = false;
    let from = null;
    let selected = -1;
    let hovered = -1;

    canvas.addEventListener('mousedown', (e) => {
        dragging = true;
        dragged = 0;
        from = { x: e.clientX, y: e.clientY, yaw: view.yaw, pitch: view.pitch };
        view.spin = false;
    });

    window.addEventListener('mouseup', () => { dragging = false; });

    canvas.addEventListener('mousemove', (e) => {
        const rect = canvas.getBoundingClientRect();

        pointer.x = ((e.clientX - rect.left) / rect.width) * 2 - 1;
        pointer.y = -((e.clientY - rect.top) / rect.height) * 2 + 1;

        if (!dragging || !from) return;

        dragged += Math.abs(e.clientX - from.x) + Math.abs(e.clientY - from.y);
        moved = true;
        view.yaw = from.yaw + (e.clientX - from.x) * 0.006;
        view.pitch = Math.max(-1.4, Math.min(1.4, from.pitch + (e.clientY - from.y) * 0.006));
    });

    /*
     * The map holds still while the pointer is on it.
     *
     * It turns slowly so that it reads as alive, which is fine to look at and
     * unpleasant to aim at: a point you are reaching for drifts out from under
     * the cursor. Stopping while the pointer is over it costs nothing — the
     * rotation is only a view, and the moment you look away it resumes.
     */
    canvas.addEventListener('mouseenter', () => { view.spin = false; });

    canvas.addEventListener('mouseleave', () => {
        pointer.set(-10, -10);

        // Left alone again — unless the reader has taken hold of it, in which
        // case where they put it is where it stays.
        if (!moved) view.spin = true;
    });

    canvas.addEventListener('wheel', (e) => {
        e.preventDefault();
        view.distance = Math.max(3, Math.min(24, view.distance * Math.exp(e.deltaY * 0.0012)));
    }, { passive: false });

    // A click is a click, not the end of a drag.
    canvas.addEventListener('click', () => {
        if (dragged > 6) return;

        select(hovered);
    });

    canvas.addEventListener('dblclick', () => {
        moved = false;
        view.spin = true;
        view.distance = fitted;
    });

    /* ---------- selection ---------- */

    function neighboursOf(index) {
        const near = new Set();

        for (const link of links) {
            if (link.a.index === index) near.add(link.b.index);
            if (link.b.index === index) near.add(link.a.index);
        }

        return near;
    }

    function select(index) {
        selected = index;

        if (index < 0) {
            paint();
            canvas.dispatchEvent(new CustomEvent('memory-cleared', { bubbles: true }));

            return;
        }

        const node = nodes[index];
        const near = neighboursOf(index);
        const related = [...near].map((i) => nodes[i]);

        paint();

        canvas.dispatchEvent(new CustomEvent('memory-selected', {
            bubbles: true,
            detail: {
                id: node.id,
                label: node.label,
                category: node.category,
                related: related.map((n) => ({ id: n.id, label: n.label, category: n.category })),
            },
        }));
    }

    /*
     * Colour and size, recomputed when something changes rather than every
     * frame. With a selection, its neighbours stay lit and everything else goes
     * quiet — which is the question "what is this connected to?" answered in
     * the picture rather than in a list.
     */
    function paint() {
        if (!cloud) return;

        const near = selected >= 0 ? neighboursOf(selected) : null;

        for (let i = 0; i < nodes.length; i++) {
            const node = nodes[i];
            const base = new THREE.Color(COLOURS[node.category] ?? COLOURS.unknown);

            let strength = 1;

            if (near) {
                strength = i === selected ? 1.9 : near.has(i) ? 1.2 : 0.22;
            } else if (node.glow > 0) {
                strength = 1 + node.glow;
            }

            base.multiplyScalar(strength);

            tints[i * 3] = base.r;
            tints[i * 3 + 1] = base.g;
            tints[i * 3 + 2] = base.b;

            const size = 0.115 + Math.min(0.11, node.degree * 0.007);

            sizes[i] = i === selected ? size * 2.2
                : i === hovered ? size * 1.6
                    : node.glow > 0 ? size * (1 + node.glow) : size;
        }

        cloud.geometry.attributes.color.needsUpdate = true;
        cloud.geometry.attributes.size.needsUpdate = true;

        if (wires) {
            wires.material.opacity = selected >= 0 ? 0.08 : 0.16;
        }
    }

    /* ---------- building ---------- */

    async function load() {
        let data;

        try {
            data = await fetch('/api/memory-map').then((r) => r.json());
        } catch {
            return;
        }

        const byId = new Map();

        nodes = (data.nodes || []).map((n, index) => {
            const node = {
                ...n,
                index,
                degree: 0,
                glow: 0,
                // Seeded on a sphere so the layout starts spread out rather
                // than exploding from a single point.
                x: (Math.random() - 0.5) * 6,
                y: (Math.random() - 0.5) * 6,
                z: (Math.random() - 0.5) * 6,
            };

            byId.set(n.id, node);

            return node;
        });

        links = (data.links || [])
            .map((l) => ({ a: byId.get(l.source), b: byId.get(l.target), strength: l.strength }))
            .filter((l) => l.a && l.b);

        for (const link of links) {
            link.a.degree++;
            link.b.degree++;
        }

        settle(nodes, links);

        build();
        paint();
    }

    function build() {
        if (cloud) world.remove(cloud);
        if (wires) world.remove(wires);

        const positions = new Float32Array(nodes.length * 3);

        tints = new Float32Array(nodes.length * 3);
        sizes = new Float32Array(nodes.length);

        for (let i = 0; i < nodes.length; i++) {
            positions[i * 3] = nodes[i].x;
            positions[i * 3 + 1] = nodes[i].y;
            positions[i * 3 + 2] = nodes[i].z;
        }

        const geometry = new THREE.BufferGeometry();

        geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));
        geometry.setAttribute('color', new THREE.BufferAttribute(tints, 3));
        geometry.setAttribute('size', new THREE.BufferAttribute(sizes, 1));

        // A small shader, only so each point can carry its own size. Sizes
        // matter here: a memory joined to many others is drawn larger, which is
        // a real property of the graph rather than decoration.
        const material = new THREE.ShaderMaterial({
            uniforms: { map: { value: dotTexture() } },
            vertexShader: `
                attribute float size;
                varying vec3 tint;
                void main() {
                    tint = color;
                    vec4 seen = modelViewMatrix * vec4(position, 1.0);
                    gl_PointSize = size * 380.0 / -seen.z;
                    gl_Position = projectionMatrix * seen;
                }`,
            fragmentShader: `
                uniform sampler2D map;
                varying vec3 tint;
                void main() {
                    vec4 dot = texture2D(map, gl_PointCoord);
                    if (dot.a < 0.05) discard;
                    gl_FragColor = vec4(tint, dot.a);
                }`,
            transparent: true,
            depthWrite: false,
            vertexColors: true,
        });

        cloud = new THREE.Points(geometry, material);
        world.add(cloud);

        // Straight lines. There is nothing in the middle to route around.
        const wire = new Float32Array(links.length * 6);

        links.forEach((link, i) => {
            wire[i * 6] = link.a.x;
            wire[i * 6 + 1] = link.a.y;
            wire[i * 6 + 2] = link.a.z;
            wire[i * 6 + 3] = link.b.x;
            wire[i * 6 + 4] = link.b.y;
            wire[i * 6 + 5] = link.b.z;
        });

        const wireGeometry = new THREE.BufferGeometry();

        wireGeometry.setAttribute('position', new THREE.BufferAttribute(wire, 3));

        wires = new THREE.LineSegments(wireGeometry, new THREE.LineBasicMaterial({
            color: 0x4dd0e1,
            transparent: true,
            opacity: 0.22,
            depthWrite: false,
        }));

        world.add(wires);
    }

    /* ---------- the loop ---------- */

    function resize() {
        const rect = canvas.getBoundingClientRect();

        if (rect.width < 2 || rect.height < 2) return false;

        renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
        renderer.setSize(rect.width, rect.height, false);
        camera.aspect = rect.width / rect.height;
        camera.updateProjectionMatrix();

        return true;
    }

    let last = performance.now();

    function frame(now) {
        requestAnimationFrame(frame);

        const delta = Math.min(0.1, (now - last) / 1000);

        last = now;

        if (!resize() || !cloud) return;

        if (view.spin) view.yaw += (delta * Math.PI * 2) / SECONDS_PER_TURN;

        camera.position.set(
            Math.cos(view.pitch) * Math.sin(view.yaw) * view.distance,
            Math.sin(view.pitch) * view.distance,
            Math.cos(view.pitch) * Math.cos(view.yaw) * view.distance);
        camera.lookAt(0, 0, 0);

        // What the pointer is over, so a click knows what it hit.
        raycaster.setFromCamera(pointer, camera);

        const hits = raycaster.intersectObject(cloud);
        const nowHovered = hits.length ? hits[0].index : -1;

        if (nowHovered !== hovered) {
            hovered = nowHovered;
            canvas.style.cursor = hovered >= 0 ? 'pointer' : 'grab';
            paint();

            canvas.dispatchEvent(new CustomEvent('memory-hovered', {
                bubbles: true,
                detail: hovered >= 0 ? { label: nodes[hovered].label } : null,
            }));
        }

        let fading = false;

        for (const node of nodes) {
            if (node.glow > 0) {
                node.glow = Math.max(0, node.glow - delta * 0.5);
                fading = true;
            }
        }

        if (fading) paint();

        renderer.render(scene, camera);
    }

    // A reply lights up the memories it used, and so does a search.
    onRecall((ids) => {
        const used = new Set(ids);

        for (const node of nodes) {
            if (used.has(node.id)) node.glow = 1;
        }

        paint();
    });

    window.brainMapReload = load;

    /*
     * Reloaded whenever the brain knows a different number of things.
     *
     * The map was drawn once at startup and again only when somebody pressed
     * Remember on a lesson — so memories arriving any other way never appeared.
     * Learning a folder put fifty-nine projects in the brain and the map went
     * on showing the two conversations it had held when the page opened: a
     * picture of the memory that was wrong by an order of magnitude, in the
     * panel whose only job is to show what it knows.
     *
     * The count rather than a timer, because rebuilding a graph of sixty nodes
     * restarts the layout, and doing that on a schedule would keep an
     * untouched map permanently unsettled.
     */
    let knownCount = -1;

    async function reloadIfChanged() {
        if (document.hidden) return;

        try {
            const body = await fetch('/api/status', { headers: { Accept: 'application/json' } })
                .then((r) => r.json());

            const status = body && body.data !== undefined ? body.data : body;
            const count = (status.memory && status.memory.facts) || 0;

            if (count === knownCount) return;

            const first = knownCount < 0;

            knownCount = count;

            // Not on the first reading: load() below is about to run anyway,
            // and two builds racing each other is one wasted layout.
            if (!first) load();
        } catch {
            // A failed poll means try again shortly, not stop watching.
        }
    }

    setInterval(reloadIfChanged, 4000);
    reloadIfChanged();

    load();
    requestAnimationFrame(frame);
}
