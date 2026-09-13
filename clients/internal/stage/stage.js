/*
 * The stage: the one three.js scene the whole interface stands in.
 *
 * Every panel is a place. They stand on a ring around the brain's light, and
 * going from one to another is the camera turning to face it — so the page
 * has an inside, and where something is stays where it is.
 *
 * The panels are the real ones. Drawing the text, the fields and the scrolling
 * lists on a canvas would mean rebuilding by hand what the page already does
 * properly — typing, selecting, copying, being read aloud — and doing all of it
 * worse. So three.js draws the world, and what stands in it is the page, placed
 * by the same camera through CSS transforms: the arithmetic of three.js's own
 * CSS3DRenderer, applied to each panel where it already is. Its renderer moves
 * every element into a container of its own, and a panel picked up and put down
 * loses its scroll position, its canvases' sizes and every assumption made by
 * the code that fills it about where it lives.
 *
 * At rest nothing is transformed. The panel in front of you is flat, exactly
 * where the layout put it, so its text is as sharp as it ever was and scrolling
 * it costs what it always did; the world behind is drawn once and left alone
 * until something in it changes. The scene only runs while it moves. The page
 * was already the most expensive thing this program does, and a backdrop
 * redrawn sixty times a second to look the same would make it more so.
 *
 * The geometry, once, because everything below leans on it. The camera is an
 * ordinary three.js camera covering the window, looking through the middle of
 * the frame rather than the middle of the window — the part of the screen the
 * panels live in, beside the column and under the bar — and at rest it stands
 * exactly as far from the panel in front of it as makes one unit of the world
 * one pixel of the screen. So a panel the size of its frame, at that distance,
 * covers the frame to the pixel, in WebGL and in CSS alike, and a turn of the
 * camera moves both by the same matrices.
 */

import * as THREE from './three.module.js';

/** Vertical field of view. Narrow, so a panel at an angle stays readable. */
const FOV = 36;

/** How long a turn takes, and how much longer a turn half-way round does. */
const TURN_MS = 620;
const FAR_TURN_MS = 560;

/** Stepping back to see everything, and coming in again from there. */
const STEP_BACK_MS = 900;

/** At rest, how often to look at whether the light has changed colour. */
const REST_LOOK_MS = 500;

/** Stepping back redraws no faster than this: nothing out there is quick. */
const OVERVIEW_FRAME_MS = 33;

const stillness = matchMedia('(prefers-reduced-motion: reduce)');

function cssValue(name, fallback) {
    const raw = getComputedStyle(document.documentElement).getPropertyValue(name).trim();

    return raw || fallback;
}

function eased(t) {
    return t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2;
}

function small(value) {
    return Math.abs(value) < 1e-10 ? 0 : value;
}

/*
 * The two matrices, written the way CSS reads them.
 *
 * The same flips CSS3DRenderer makes: the screen's y runs down and the world's
 * runs up, so the camera's second row and the object's second column change
 * sign. Anything else here and a panel and its outline part company on the
 * first turn.
 */
function cameraCSS(matrix) {
    const e = matrix.elements;

    return `matrix3d(${small(e[0])},${small(-e[1])},${small(e[2])},${small(e[3])},` +
        `${small(e[4])},${small(-e[5])},${small(e[6])},${small(e[7])},` +
        `${small(e[8])},${small(-e[9])},${small(e[10])},${small(e[11])},` +
        `${small(e[12])},${small(-e[13])},${small(e[14])},${small(e[15])})`;
}

function objectCSS(matrix) {
    const e = matrix.elements;

    return `matrix3d(${small(e[0])},${small(e[1])},${small(e[2])},${small(e[3])},` +
        `${small(-e[4])},${small(-e[5])},${small(-e[6])},${small(-e[7])},` +
        `${small(e[8])},${small(e[9])},${small(e[10])},${small(e[11])},` +
        `${small(e[12])},${small(e[13])},${small(e[14])},${small(e[15])})`;
}

/** A dependable scatter, so the dust does not reshuffle when the window moves. */
function scatter(seed) {
    let state = seed >>> 0;

    return () => {
        state = (state * 1664525 + 1013904223) >>> 0;

        return state / 4294967296;
    };
}

/** Light around the middle rather than in it, as the core draws its own. */
function haloTexture(size = 256) {
    const c = document.createElement('canvas');

    c.width = c.height = size;

    const ctx = c.getContext('2d');
    const g = ctx.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2);

    g.addColorStop(0, 'rgba(255,255,255,0.55)');
    g.addColorStop(0.12, 'rgba(255,255,255,0.28)');
    g.addColorStop(0.38, 'rgba(255,255,255,0.08)');
    g.addColorStop(1, 'rgba(255,255,255,0)');

    ctx.fillStyle = g;
    ctx.fillRect(0, 0, size, size);

    const texture = new THREE.CanvasTexture(c);

    texture.colorSpace = THREE.SRGBColorSpace;

    return texture;
}

function roundedPath(ctx, x, y, w, h, r) {
    ctx.beginPath();
    ctx.moveTo(x + r, y);
    ctx.arcTo(x + w, y, x + w, y + h, r);
    ctx.arcTo(x + w, y + h, x, y + h, r);
    ctx.arcTo(x, y + h, x, y, r);
    ctx.arcTo(x, y, x + w, y, r);
    ctx.closePath();
}

function roundedOutline(w, h, r) {
    const shape = new THREE.Shape();
    const x = -w / 2;
    const y = -h / 2;

    shape.moveTo(x + r, y);
    shape.lineTo(x + w - r, y);
    shape.quadraticCurveTo(x + w, y, x + w, y + r);
    shape.lineTo(x + w, y + h - r);
    shape.quadraticCurveTo(x + w, y + h, x + w - r, y + h);
    shape.lineTo(x + r, y + h);
    shape.quadraticCurveTo(x, y + h, x, y + h - r);
    shape.lineTo(x, y + r);
    shape.quadraticCurveTo(x, y, x + r, y);

    return new THREE.BufferGeometry().setFromPoints(shape.getPoints(6));
}

function circle(radius, segments = 128) {
    const points = [];

    for (let i = 0; i < segments; i++) {
        const a = (i / segments) * Math.PI * 2;

        points.push(new THREE.Vector3(Math.sin(a) * radius, 0, Math.cos(a) * radius));
    }

    return new THREE.BufferGeometry().setFromPoints(points);
}

/*
 * An icon, as a picture a canvas can hold.
 *
 * Read off the page's own markup rather than drawn a second time, so a panel
 * out in the world wears the same mark as its entry in the column. A picture
 * that would taint the canvas is refused rather than uploaded — WebGL throws on
 * a tainted texture, and a panel with a name and no icon is better than a
 * stage that stops drawing.
 */
function iconPicture(svg, colour, done) {
    if (!svg) return;

    const copy = svg.cloneNode(true);

    copy.setAttribute('width', '128');
    copy.setAttribute('height', '128');
    copy.setAttribute('style', `color:${colour}`);

    const image = new Image();

    image.onload = () => {
        try {
            const probe = document.createElement('canvas').getContext('2d');

            probe.drawImage(image, 0, 0, 1, 1);
            probe.getImageData(0, 0, 1, 1);
            done(image);
        } catch {
            // Tainted: the name alone, then.
        }
    };

    image.src = 'data:image/svg+xml;charset=utf-8,' +
        encodeURIComponent(new XMLSerializer().serializeToString(copy));
}

/*
 * startStage puts the panels of one page into the world.
 *
 *   frame    the element a panel at rest fills
 *   panels   the panels, in the order they stand round the ring; the one not
 *            hidden is the one in front
 *   fill     whether every panel is exactly the frame's size (the program's
 *            views) or as tall as its own contents (setup's steps)
 *   title, count, icon   what a panel out in the world is labelled with
 *   go       how to make a panel the current one, for stepping back
 *   pulse    { colour, level } of the brain, or nothing
 *   button   what steps back and returns, if the page offers it
 *   turner   { root, prev, here, next }: the panel in front by name, and a
 *            turn to either side, if the page offers it
 *
 * Returns null where there is no WebGL, and the page carries on flat — which
 * is exactly how it was before any of this.
 */
export function startStage(options) {
    const {
        frame,
        fill = false,
        title = () => '',
        count = () => '',
        icon = () => null,
        go = null,
        pulse = null,
        button = null,
        turner = null,
    } = options;

    const panels = (options.panels || []).filter(Boolean);

    if (!frame || panels.length < 2) return null;

    const root = document.documentElement;
    const canvas = document.createElement('canvas');

    canvas.className = 'stage';
    canvas.setAttribute('aria-hidden', 'true');

    let renderer;

    try {
        renderer = new THREE.WebGLRenderer({ canvas, antialias: true, powerPreference: 'low-power' });
    } catch (err) {
        console.warn('The stage could not start, so the panels stay flat.', err);

        return null;
    }

    // Never more pixels than the screen has: this is a backdrop, and on a
    // processor that draws its own pixels every extra one is paid for.
    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1));

    document.body.prepend(canvas);
    root.classList.add('has-stage');
    frame.classList.add('stage-frame');

    const accent = cssValue('--accent', '#4dd0e1');
    const ground = new THREE.Color(cssValue('--bg', '#04080f'));

    const scene = new THREE.Scene();

    scene.background = ground;
    scene.fog = new THREE.Fog(ground, 1, 2);

    const camera = new THREE.PerspectiveCamera(FOV, 1, 10, 200000);

    /* ---------- the measurements everything is placed from ---------- */

    const world = {
        width: 0,           // the window
        height: 0,
        near: 0,            // how far a panel stands at rest: one unit, one pixel
        radius: 0,          // the ring
        step: (Math.PI * 2) / panels.length,
        frame: { left: 0, top: 0, width: 0, height: 0 },
        eye: { x: 0, y: 0 },    // where the camera looks through, on the screen
    };

    /*
     * Where the camera looks through: the middle of the frame.
     *
     * The middle of the window was the first answer, and it put the ring's
     * nearest panels behind the bar at the bottom of any window small enough
     * for the bars to take a real share of it. A frame whose height follows
     * its contents — setup's steps — is anchored on the space below its top
     * instead, which does not move when a taller step replaces a shorter one.
     */
    function measureFrame() {
        const r = frame.getBoundingClientRect();

        world.frame = { left: r.left, top: r.top, width: r.width, height: r.height };
        world.eye = {
            x: r.left + r.width / 2,
            y: fill ? r.top + r.height / 2 : r.top + Math.max(240, world.height - r.top) / 2,
        };

        // The same camera, its middle moved onto the frame: nothing about the
        // scale changes, so the one-unit-one-pixel distance still holds.
        camera.setViewOffset(world.width, world.height,
            world.width / 2 - world.eye.x, world.height / 2 - world.eye.y,
            world.width, world.height);
    }

    /* ---------- the places ---------- */

    const edgeMaterial = new THREE.LineBasicMaterial({
        color: accent, transparent: true, opacity: 0.3, depthWrite: false,
    });
    const hitMaterial = new THREE.MeshBasicMaterial({ visible: false, side: THREE.DoubleSide });
    const unitPlane = new THREE.PlaneGeometry(1, 1);

    const places = panels.map((panel, index) => {
        const slot = new THREE.Group();
        const edge = new THREE.LineLoop(new THREE.BufferGeometry(), edgeMaterial.clone());
        const picture = document.createElement('canvas');
        const texture = new THREE.CanvasTexture(picture);

        texture.colorSpace = THREE.SRGBColorSpace;

        // One-sided: a panel across the ring is seen from behind, through the
        // middle, and its name would read backwards through every card.
        const cover = new THREE.Mesh(unitPlane, new THREE.MeshBasicMaterial({
            map: texture, transparent: true, depthWrite: false,
        }));
        const hit = new THREE.Mesh(unitPlane, hitMaterial);

        edge.position.z = 0.5;
        hit.userData.index = index;

        slot.add(cover, edge, hit);
        scene.add(slot);

        return { panel, index, slot, edge, cover, hit, picture, texture, height: 0, icon: null, hover: false, drawnFor: -1 };
    });

    let here = Math.max(0, places.findIndex((p) => !p.panel.hidden));

    function heightOf(place) {
        if (fill) return world.frame.height;

        return place.height || world.frame.height;
    }

    /*
     * Where a place's panel sits inside its slot.
     *
     * The slot's origin is the middle of the window, because that is where the
     * camera looks; the frame is wherever the layout put it — to the right of
     * the column, below the bar. So the panel is offset inside its slot by
     * exactly that, and at rest lands on the frame rather than on the middle of
     * the screen.
     */
    function sizePlace(place) {
        const f = world.frame;
        const h = heightOf(place);
        const x = f.left + f.width / 2 - world.eye.x;
        const y = -(f.top + h / 2 - world.eye.y);

        place.cover.scale.set(f.width, h, 1);
        place.cover.position.set(x, y, 0);

        // A picture drawn for a panel of one shape, stretched over another,
        // pulls its name tall and thin.
        if (Math.abs(h / Math.max(1, f.width) - place.drawnFor) > 0.02) drawCover(place);
        place.hit.scale.copy(place.cover.scale);
        place.hit.position.copy(place.cover.position);

        place.edge.geometry.dispose();
        place.edge.geometry = roundedOutline(f.width, h, Math.min(18, h / 4));
        place.edge.position.set(x, y, 0.5);

        const angle = place.index * world.step;

        place.slot.position.set(Math.sin(angle) * world.radius, 0, Math.cos(angle) * world.radius);
        place.slot.rotation.set(0, angle, 0);
        place.slot.updateMatrixWorld(true);
    }

    /*
     * What a panel out in the world shows: its mark, its name, and the number
     * its entry in the column shows. Nothing invented to make it look busy —
     * a panel that is not in front is not being read, so it says what it is
     * and how much is waiting in it, which is what the column says too.
     */
    function drawCover(place) {
        const f = world.frame;
        const h = heightOf(place);
        const width = 720;
        const height = Math.max(160, Math.round(width * h / Math.max(1, f.width)));
        const c = place.picture;

        place.drawnFor = h / Math.max(1, f.width);

        /*
         * A new texture for a new shape.
         *
         * three.js gives a texture its storage on the card once, at the size
         * it first had, and later pictures are copied into that — so a canvas
         * that has changed size is squeezed into the old one, and the name on
         * it comes out tall and thin.
         */
        if (c.width !== width || c.height !== height) {
            c.width = width;
            c.height = height;

            place.texture.dispose();
            place.texture = new THREE.CanvasTexture(c);
            place.texture.colorSpace = THREE.SRGBColorSpace;
            place.cover.material.map = place.texture;
            place.cover.material.needsUpdate = true;
        }

        const ctx = c.getContext('2d');

        // Drawn again over the last picture when the shape has not changed,
        // and a translucent fill over itself darkens every time.
        ctx.clearRect(0, 0, width, height);

        const fillStyle = ctx.createLinearGradient(0, 0, width * 0.4, height);

        fillStyle.addColorStop(0, 'rgba(20, 44, 66, 0.72)');
        fillStyle.addColorStop(1, 'rgba(9, 16, 26, 0.86)');

        roundedPath(ctx, 3, 3, width - 6, height - 6, 20);
        ctx.fillStyle = fillStyle;
        ctx.fill();
        ctx.lineWidth = 2;
        ctx.strokeStyle = 'rgba(77, 208, 225, 0.22)';
        ctx.stroke();

        const name = (title(place.panel) || '').toUpperCase();
        const badge = String(count(place.panel) || '');
        const mark = Math.round(Math.min(height * 0.2, 112));
        const size = Math.round(Math.min(46, width / Math.max(8, name.length * 0.9)));
        const middle = height / 2;

        if (place.icon) {
            ctx.globalAlpha = 0.9;
            ctx.drawImage(place.icon, width / 2 - mark / 2, middle - mark - size * 0.4, mark, mark);
            ctx.globalAlpha = 1;
        }

        ctx.font = `600 ${size}px ui-sans-serif, system-ui, sans-serif`;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'top';
        ctx.fillStyle = accent;
        ctx.fillText(name, width / 2, place.icon ? middle + size * 0.2 : middle - size / 2, width - 60);

        if (badge) {
            const pill = Math.round(size * 1.1);
            const text = `${badge} waiting`;

            ctx.font = `600 ${Math.round(pill * 0.55)}px ui-sans-serif, system-ui, sans-serif`;

            const tw = ctx.measureText(text).width + pill;
            const top = (place.icon ? middle + size * 0.2 : middle - size / 2) + size * 1.5;

            roundedPath(ctx, width / 2 - tw / 2, top, tw, pill, pill / 2);
            ctx.fillStyle = 'rgba(240, 178, 107, 0.16)';
            ctx.fill();
            ctx.strokeStyle = 'rgba(240, 178, 107, 0.55)';
            ctx.stroke();
            ctx.fillStyle = '#f0b26b';
            ctx.textBaseline = 'middle';
            ctx.fillText(text, width / 2, top + pill / 2);
        }

        place.texture.needsUpdate = true;
    }

    for (const place of places) {
        iconPicture(icon(place.panel), accent, (image) => {
            place.icon = image;
            drawCover(place);
            draw();
        });
    }

    /* ---------- the floor, the dust and the light ---------- */

    const floorMaterial = new THREE.LineBasicMaterial({
        color: accent, transparent: true, opacity: 0.13, depthWrite: false,
    });
    const ringMaterial = new THREE.LineBasicMaterial({
        color: accent, transparent: true, opacity: 0.3, depthWrite: false,
    });

    let floor = null;
    let dust = null;

    const light = new THREE.Group();
    const tint = new THREE.Color(accent);

    const halo = new THREE.Sprite(new THREE.SpriteMaterial({
        map: haloTexture(), color: tint, transparent: true, opacity: 0.45,
        blending: THREE.AdditiveBlending, depthWrite: false, fog: false,
    }));
    const heart = new THREE.LineSegments(
        new THREE.EdgesGeometry(new THREE.IcosahedronGeometry(1, 1)),
        new THREE.LineBasicMaterial({ color: tint, transparent: true, opacity: 0.55, depthWrite: false, fog: false }),
    );
    const orbitA = new THREE.LineLoop(circle(1, 96), new THREE.LineBasicMaterial({
        color: tint, transparent: true, opacity: 0.4, depthWrite: false, fog: false,
    }));
    const orbitB = new THREE.LineLoop(circle(1, 96), orbitA.material);

    orbitA.rotation.set(1.15, 0, 0.35);
    orbitB.rotation.set(-0.9, 0, -0.6);
    light.add(halo, heart, orbitA, orbitB);
    scene.add(light);

    function buildFloor() {
        if (floor) {
            floor.traverse((o) => o.geometry && o.geometry.dispose());
            scene.remove(floor);
        }

        floor = new THREE.Group();

        const R = world.radius;

        for (const k of [0.32, 0.6, 1.3, 1.75, 2.35]) {
            floor.add(new THREE.LineLoop(circle(R * k), floorMaterial));
        }

        // The ring the panels stand on, a little brighter than the rest.
        floor.add(new THREE.LineLoop(circle(R), ringMaterial));

        const spokes = [];

        for (let i = 0; i < places.length * 2; i++) {
            const a = (i / (places.length * 2)) * Math.PI * 2;
            const from = i % 2 === 0 ? R * 0.32 : R * 0.6;

            spokes.push(
                new THREE.Vector3(Math.sin(a) * from, 0, Math.cos(a) * from),
                new THREE.Vector3(Math.sin(a) * R * 2.35, 0, Math.cos(a) * R * 2.35),
            );
        }

        floor.add(new THREE.LineSegments(new THREE.BufferGeometry().setFromPoints(spokes), floorMaterial));

        // A thread from the floor to the light, so the light stands on something.
        floor.add(new THREE.Line(new THREE.BufferGeometry().setFromPoints([
            new THREE.Vector3(0, 0, 0), new THREE.Vector3(0, world.height * 0.62, 0),
        ]), ringMaterial));

        floor.position.y = -world.height * 0.62;
        scene.add(floor);

        if (dust) {
            dust.geometry.dispose();
            scene.remove(dust);
        }

        const random = scatter(7);
        const positions = new Float32Array(420 * 3);

        for (let i = 0; i < 420; i++) {
            const a = random() * Math.PI * 2;
            const r = R * (0.15 + random() * 2.3);

            positions[i * 3] = Math.sin(a) * r;
            positions[i * 3 + 1] = (random() - 0.4) * world.height * 2.4;
            positions[i * 3 + 2] = Math.cos(a) * r;
        }

        const geometry = new THREE.BufferGeometry();

        geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));

        dust = new THREE.Points(geometry, new THREE.PointsMaterial({
            color: accent, size: 2, sizeAttenuation: false, transparent: true, opacity: 0.42, depthWrite: false,
        }));
        scene.add(dust);

        halo.scale.setScalar(R * 0.62);
        heart.scale.setScalar(R * 0.045);
        orbitA.scale.setScalar(R * 0.1);
        orbitB.scale.setScalar(R * 0.135);
    }

    function build() {
        world.width = window.innerWidth;
        world.height = window.innerHeight;

        renderer.setSize(world.width, world.height, false);
        camera.aspect = world.width / world.height;
        camera.updateProjectionMatrix();

        world.near = (world.height / 2) / Math.tan(THREE.MathUtils.degToRad(FOV / 2));

        measureFrame();

        // Far enough round that neighbours never touch, whatever the window.
        world.radius = Math.max(
            (world.frame.width * 1.25) / (2 * Math.sin(world.step / 2)),
            world.near * 1.4,
        );

        for (const place of places) {
            sizePlace(place);
            drawCover(place);
        }

        buildFloor();
    }

    /* ---------- the camera ---------- */

    const pose = { angle: 0, dist: 0, lift: 0, look: 0 };

    function restPose(index) {
        return { angle: index * world.step, dist: world.radius + world.near, lift: 0, look: 0 };
    }

    /** How far the wheel has moved the camera in or out, stepped back. */
    let overviewZoom = 1;

    /*
     * Stepped back far enough for the whole ring to fit the frame.
     *
     * Worked out from the frame rather than from the ring alone, because the
     * frame is what there is to see it in: on a wide screen that is most of
     * the window, and on a small one it can be a strip between two bars.
     * Allowed to spill a little past the sides, under the column, where there
     * is glass to see it through; not past the top and bottom, where there is
     * a bar.
     */
    function overviewPose(angle = pose.angle) {
        const f = world.frame;
        const R = world.radius;
        const visible = fill ? f.height : Math.max(240, world.height - Math.max(0, f.top));
        const across = ((R * 2.3 + f.width) * world.near) / Math.max(200, f.width * 1.3);
        const deep = ((R * 0.85 + visible * 0.7) * world.near) / Math.max(150, visible * 0.92);
        const distance = Math.max(across, deep, R * 1.6) * overviewZoom;

        return {
            angle,
            dist: distance * 0.94,
            lift: distance * 0.34,
            look: -R * 0.12,
        };
    }

    function aim() {
        camera.position.set(Math.sin(pose.angle) * pose.dist, pose.lift, Math.cos(pose.angle) * pose.dist);
        camera.lookAt(0, pose.look, 0);
        camera.updateMatrixWorld();

        // What is far from the camera fades into the ground colour, measured
        // from wherever the camera is, so stepping back does not fog the lot.
        const from = camera.position.length();

        scene.fog.near = Math.max(1, from - world.radius * 1.1);
        scene.fog.far = from + world.radius * 1.7;
    }

    /* ---------- panels standing in the world ---------- */

    /** The places whose real panel is out in the world rather than flat. */
    const shown = new Set();
    let moving = false;

    /*
     * How tall the frame was the last time it was at rest, as the browser
     * reported it rather than as asked for.
     *
     * By the time the page's hidden attribute has changed, the step being left
     * has already gone from the flow and the frame has already shrunk — it is
     * only that nothing has laid it out yet. Asking its size then lays it out
     * short, and a page scrolled further down than a short frame allows is
     * pulled back up in the same moment: the world measured the page at one
     * place and the panels were drawn at another. Holding the height from
     * before, which the observer below keeps without asking, is what stops
     * the frame shrinking at all.
     */
    let restHeight = 0;

    function begin() {
        if (moving) return;

        if (!fill && restHeight) frame.style.minHeight = `${restHeight}px`;

        measureFrame();

        for (const place of places) sizePlace(place);

        moving = true;
        frame.classList.add('stage-moving');
    }

    function stand(place) {
        if (shown.has(place)) return;

        const f = world.frame;
        const style = place.panel.style;

        shown.add(place);
        place.panel.classList.add('stage-shown');
        style.width = `${f.width}px`;

        if (fill) style.height = `${f.height}px`;

        // Where the camera looks through, in the panel's coordinates — where
        // CSS has to put the vanishing point for the two pictures to agree.
        style.transformOrigin = `${world.eye.x - f.left}px ${world.eye.y - f.top}px`;

        if (!fill) {
            place.height = place.panel.offsetHeight;
            sizePlace(place);

            const tallest = Math.max(restHeight, ...[...shown].map((p) => p.height || 0));

            frame.style.minHeight = `${tallest}px`;
        }

    }

    const toCamera = new THREE.Vector3();
    const facingOut = new THREE.Vector3();

    function placePanels() {
        const lens = `perspective(${world.near}px) translateZ(${world.near}px) ${cameraCSS(camera.matrixWorldInverse)}`;

        for (const place of shown) {
            const angle = place.index * world.step;

            facingOut.set(Math.sin(angle), 0, Math.cos(angle));
            toCamera.copy(camera.position).sub(place.slot.position).normalize();

            // Edge-on or turned away, a panel is a sliver of smeared text at
            // best and a mirror image at worst.
            place.panel.style.visibility = facingOut.dot(toCamera) > 0.18 ? '' : 'hidden';
            place.panel.style.transform = `${lens} ${objectCSS(place.slot.matrixWorld)}`;
        }
    }

    function flatten() {
        for (const place of shown) {
            const style = place.panel.style;

            place.panel.classList.remove('stage-shown', 'stage-leaving');
            style.transform = '';
            style.transformOrigin = '';
            style.width = '';
            style.height = '';
            style.visibility = '';
        }

        shown.clear();
        moving = false;
        frame.classList.remove('stage-moving');
        frame.style.minHeight = '';

        if (!fill && places[here]) places[here].height = places[here].panel.offsetHeight;
    }

    /* ---------- moving ---------- */

    let flight = null;

    function flyTo(target, done) {
        const from = { ...pose };
        let turn = target.angle - from.angle;

        turn = Math.atan2(Math.sin(turn), Math.cos(turn));

        const far = Math.min(1, Math.abs(turn) / Math.PI);
        const restDistance = world.radius + world.near;
        const rest = Math.abs(from.dist - restDistance) < 1 && Math.abs(target.dist - restDistance) < 1;
        const outThere = from.dist > restDistance * 1.3 && target.dist > restDistance * 1.3;

        let length = rest ? TURN_MS + FAR_TURN_MS * far : outThere ? TURN_MS * 0.8 : STEP_BACK_MS;

        if (stillness.matches) length = 0;

        flight = {
            from,
            to: { ...target, angle: from.angle + turn },
            start: performance.now(),
            length,
            far,
            rest,
            done,
        };

        wake();
    }

    function advance(now) {
        const f = flight;
        const t = f.length ? Math.min(1, (now - f.start) / f.length) : 1;
        const k = eased(t);

        for (const key of ['angle', 'dist', 'lift', 'look']) {
            pose[key] = f.from[key] + (f.to[key] - f.from[key]) * k;
        }

        /*
         * A turn from one panel to the next steps back as it goes.
         *
         * Turned on the spot, the camera would swing the panel in front of it
         * across the screen at arm's length, which reads as the page being
         * thrown rather than as walking to another one. Stepping back for the
         * middle of the turn shows both panels, and how far it steps back
         * follows how far round the other one is.
         */
        if (f.rest) {
            const arc = Math.sin(Math.PI * k);

            pose.dist += world.near * (0.55 + 1.4 * f.far) * arc;
            pose.lift += world.near * 0.2 * f.far * arc;
        }

        if (t >= 1) {
            flight = null;
            pose.angle = Math.atan2(Math.sin(pose.angle), Math.cos(pose.angle));

            if (f.done) f.done();
        }
    }

    function settle() {
        if (overviewOpen) return;

        flatten();
    }

    /* ---------- drawing ---------- */

    let running = false;
    let then = 0;
    let drawnAt = 0;
    let shownLevel = 0;
    let shownTint = '';
    let lost = false;

    function lightUp(delta) {
        const reading = pulse ? pulse() : null;
        const colour = (reading && reading.colour) || accent;

        if (colour !== shownTint) {
            shownTint = colour;
            tint.set(colour);
            halo.material.color.copy(tint);
            heart.material.color.copy(tint);
            orbitA.material.color.copy(tint);
        }

        const level = reading ? Math.max(0, Math.min(1, reading.level || 0)) : 0;

        shownLevel += (level - shownLevel) * Math.min(1, delta * 6);

        halo.material.opacity = 0.4 + shownLevel * 0.45;
        heart.rotation.y += delta * (0.2 + shownLevel);
        heart.rotation.x += delta * 0.07;
        orbitA.rotation.z += delta * 0.12;
        orbitB.rotation.z -= delta * 0.09;

        if (dust) dust.rotation.y += delta * 0.006;
    }

    function draw() {
        if (lost) return;

        aim();

        if (shown.size) placePanels();

        for (const place of places) {
            // The panel in front, at rest, is the page itself: an outline or a
            // cover behind it would show through every translucent card.
            const front = place.index === here && !moving;
            const angle = place.index * world.step;

            facingOut.set(Math.sin(angle), 0, Math.cos(angle));
            toCamera.copy(camera.position).sub(place.slot.position).normalize();

            // The backs of the panels across the ring are what gives stepping
            // back its shape; from in front of a panel they are only lines
            // crossing the words.
            const turnedAway = facingOut.dot(toCamera) < 0;

            place.cover.visible = !front && !shown.has(place);
            place.edge.visible = !front && (overviewOpen || !turnedAway);
            place.edge.material.opacity = place.hover ? 1 : shown.has(place) ? 0.22 : 0.3;
            place.cover.material.opacity = place.hover ? 1 : 0.82;
        }

        // The shapes inside the light are for when the world is being looked
        // at. At rest they would sit behind the conversation, turning slowly
        // under the words.
        heart.visible = orbitA.visible = orbitB.visible = moving;

        renderer.render(scene, camera);
        drawnAt = performance.now();

        // Counted so that "it draws nothing at rest" can be checked rather
        // than believed, as the core's frames are.
        window.brainStageFrames = (window.brainStageFrames || 0) + 1;
    }

    function wake() {
        if (running || lost) return;

        running = true;
        then = performance.now();
        requestAnimationFrame(tick);
    }

    function tick(now) {
        const delta = Math.min(0.1, Math.max(0, (now - then) / 1000));

        then = now;

        const flying = !!flight;

        if (flight) advance(now);

        lightUp(delta);

        // Stepped back with nothing moving, there is no hurry.
        if (flying || dragging || !overviewOpen || now - drawnAt >= OVERVIEW_FRAME_MS) draw();

        if ((flight || overviewOpen) && !document.hidden && !lost) {
            requestAnimationFrame(tick);
        } else {
            running = false;
        }
    }

    // At rest, the light's colour is still the brain's status; looked at twice
    // a second and drawn only when it has changed.
    setInterval(() => {
        if (running || document.hidden || lost || !pulse) return;

        const reading = pulse();

        if (reading && reading.colour && reading.colour !== shownTint) {
            lightUp(0);
            draw();
        }
    }, REST_LOOK_MS);

    /* ---------- following the page ---------- */

    function arrive(index) {
        const before = places[here];

        begin();

        if (before && index !== here) {
            stand(before);
            before.panel.classList.add('stage-leaving');
        }

        here = index;
        stand(places[here]);
        sayWhere();

        if (overviewOpen) leaveOverview();

        // Placed now rather than on the first frame: until then both panels
        // are out of the flow with no transform, lying flat on top of each
        // other, and a browser that is slow to hand out a frame paints that.
        aim();
        placePanels();

        flyTo(restPose(here), settle);
    }

    /*
     * The page decides which panel is in front; the stage follows.
     *
     * Every way of going somewhere in this program ends in a panel's hidden
     * attribute changing — the column, the buttons inside the panels, code
     * that sends somebody to their tasks. Watching that attribute is the one
     * place all of them meet, and it leaves the code that decides unaware
     * there is a stage at all. The observer runs before the next paint, so the
     * panel being left never blinks out before it turns away.
     */
    const watcher = new MutationObserver(() => {
        const index = places.findIndex((p) => !p.panel.hidden);

        if (index < 0 || index === here) return;

        arrive(index);
    });

    for (const place of places) {
        watcher.observe(place.panel, { attributes: true, attributeFilter: ['hidden'] });
    }

    /* ---------- stepping back ---------- */

    let overviewOpen = false;
    let dragging = null;
    let highlighted = -1;

    const hint = document.createElement('p');

    hint.className = 'stage-hint';
    hint.hidden = true;
    hint.textContent = 'Every panel, where it stands. Drag to turn round, click one to go to it, Esc to go back.';
    document.body.append(hint);

    function openOverview() {
        if (overviewOpen || lost) return;

        begin();
        stand(places[here]);
        aim();
        placePanels();

        for (const place of places) {
            if (!shown.has(place)) drawCover(place);
        }

        overviewOpen = true;
        root.classList.add('stage-overview');

        if (button) button.setAttribute('aria-pressed', 'true');

        const f = world.frame;

        hint.style.left = `${f.left + f.width / 2}px`;
        // Along the top, where there is only sky: at the bottom it sat over
        // the panel nearest the camera, which is the one most likely chosen.
        hint.style.top = `${Math.max(8, f.top) + 14}px`;
        hint.hidden = false;
        highlight(here);

        flyTo(overviewPose(), null);
    }

    function leaveOverview() {
        overviewOpen = false;
        dragging = null;
        root.classList.remove('stage-overview');
        hint.hidden = true;
        highlight(-1);

        if (button) button.setAttribute('aria-pressed', 'false');
    }

    function closeOverview() {
        if (!overviewOpen) return;

        leaveOverview();
        flyTo(restPose(here), settle);
    }

    function highlight(index) {
        highlighted = index;

        for (const place of places) place.hover = place.index === index;

        canvas.style.cursor = index >= 0 && overviewOpen ? 'pointer' : '';
        wake();
    }

    function choose(index) {
        if (index < 0) return;

        if (index === here || !go) {
            closeOverview();

            return;
        }

        leaveOverview();
        go(places[index].panel);

        // If the page would not go there, come back to where it is rather than
        // being left standing out in the world.
        if (places.findIndex((p) => !p.panel.hidden) !== index) flyTo(restPose(here), settle);
    }

    const pointer = new THREE.Vector2();
    const raycaster = new THREE.Raycaster();

    function placeUnder(event) {
        const r = canvas.getBoundingClientRect();

        pointer.set(((event.clientX - r.left) / r.width) * 2 - 1, -((event.clientY - r.top) / r.height) * 2 + 1);
        raycaster.setFromCamera(pointer, camera);

        const hits = raycaster.intersectObjects(places.map((p) => p.hit), false);

        return hits.length ? hits[0].object.userData.index : -1;
    }

    canvas.addEventListener('pointerdown', (event) => {
        if (!overviewOpen || flight) return;

        dragging = { x: event.clientX, angle: pose.angle, moved: false };
        canvas.setPointerCapture(event.pointerId);
    });

    canvas.addEventListener('pointermove', (event) => {
        if (!overviewOpen) return;

        if (dragging) {
            const dx = event.clientX - dragging.x;

            if (Math.abs(dx) > 4) dragging.moved = true;

            if (dragging.moved && !flight) {
                pose.angle = dragging.angle - dx * 0.0045;
                canvas.style.cursor = 'grabbing';
                wake();
            }

            return;
        }

        const index = flight ? -1 : placeUnder(event);

        if (index !== highlighted) highlight(index);
    });

    canvas.addEventListener('pointerup', (event) => {
        if (!dragging) return;

        const click = !dragging.moved;

        dragging = null;
        canvas.style.cursor = '';

        if (click) choose(placeUnder(event));
    });

    canvas.addEventListener('wheel', (event) => {
        if (!overviewOpen || flight) return;

        event.preventDefault();
        overviewZoom = Math.max(0.6, Math.min(1.8, overviewZoom * Math.exp(event.deltaY * 0.001)));
        Object.assign(pose, overviewPose());
        wake();
    }, { passive: false });

    document.addEventListener('keydown', (event) => {
        if (!overviewOpen) return;

        if (event.key === 'Escape') {
            event.preventDefault();
            closeOverview();
        } else if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
            event.preventDefault();

            const way = event.key === 'ArrowRight' ? 1 : -1;
            const next = (Math.max(0, highlighted) + way + places.length) % places.length;

            highlight(next);
            flyTo(overviewPose(next * world.step), null);
        } else if (event.key === 'Enter') {
            event.preventDefault();
            choose(highlighted);
        }
    });

    if (button) {
        button.hidden = false;
        button.addEventListener('click', () => (overviewOpen ? closeOverview() : openOverview()));
    }

    /* ---------- turning, without a list of places ---------- */

    function sayWhere() {
        if (turner && places[here]) turner.here.textContent = title(places[here].panel) || '';
    }

    /*
     * One panel round, either way.
     *
     * At rest that is going there, through the page, like any other way of
     * going somewhere. Stepped back it only moves which panel is picked out,
     * the way the arrow keys do, so turning to look is not the same as
     * choosing.
     */
    function turn(way) {
        // From the panel the page shows, which is ahead of the stage's own
        // idea of it by one step when two turns come faster than a frame.
        const showing = places.findIndex((p) => !p.panel.hidden);
        const next = ((showing < 0 ? here : showing) + way + places.length) % places.length;

        if (overviewOpen) {
            const from = highlighted >= 0 ? highlighted : here;
            const picked = (from + way + places.length) % places.length;

            highlight(picked);
            flyTo(overviewPose(picked * world.step), null);

            return;
        }

        if (go) go(places[next].panel);
    }

    if (turner) {
        turner.root.hidden = false;
        turner.prev.addEventListener('click', () => turn(-1));
        turner.next.addEventListener('click', () => turn(1));
        turner.here.addEventListener('click', () => (overviewOpen ? closeOverview() : openOverview()));
        sayWhere();
    }

    // Alt and an arrow turns from anywhere but a box being typed in, where the
    // arrows belong to the text.
    document.addEventListener('keydown', (event) => {
        if (!event.altKey || overviewOpen || (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight')) return;

        const target = event.target;

        if (target && (target.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName))) return;

        event.preventDefault();
        turn(event.key === 'ArrowRight' ? 1 : -1);
    });

    /* ---------- the window changing under it ---------- */

    function rebuild() {
        if (lost) return;

        flight = null;

        if (overviewOpen) leaveOverview();

        flatten();
        build();
        Object.assign(pose, restPose(here));
        draw();
    }

    let resizing = 0;

    function soon() {
        clearTimeout(resizing);
        resizing = setTimeout(rebuild, 120);
    }

    window.addEventListener('resize', soon);

    // A page that scrolls carries its frame with it, and the world has to go
    // along or it jumps back the moment a step turns. Only setup's page
    // scrolls; the program's frame is a fixed part of the window.
    let scrolled = false;

    window.addEventListener('scroll', () => {
        if (moving || scrolled || lost) return;

        scrolled = true;
        requestAnimationFrame(() => {
            scrolled = false;

            if (moving) return;

            measureFrame();

            for (const place of places) sizePlace(place);

            draw();
        });
    }, { passive: true });

    // The frame can change size with the window standing still — a banner
    // arriving above it, a column giving way. Only at rest: while panels are
    // out in the world the frame is holding still on purpose.
    new ResizeObserver((entries) => {
        if (moving) return;

        const size = entries[entries.length - 1].contentRect;
        const r = frame.getBoundingClientRect();
        const f = world.frame;

        restHeight = size.height;

        // A frame that grows with what is in it — a step whose list has just
        // arrived — has not changed the world; only its outline needs to know.
        if (!fill && Math.abs(r.width - f.width) <= 1 && Math.abs(r.left - f.left) <= 1) {
            measureFrame();

            if (places[here]) places[here].height = places[here].panel.offsetHeight;

            return;
        }

        if (Math.abs(r.width - f.width) > 1 || Math.abs(r.height - f.height) > 1 || Math.abs(r.left - f.left) > 1) {
            soon();
        }
    }).observe(frame);

    canvas.addEventListener('webglcontextlost', (event) => {
        event.preventDefault();
        lost = true;
        flight = null;

        if (overviewOpen) leaveOverview();

        flatten();
        root.classList.remove('has-stage');
    });

    canvas.addEventListener('webglcontextrestored', () => {
        lost = false;
        root.classList.add('has-stage');
        rebuild();
    });

    build();
    Object.assign(pose, restPose(here));
    lightUp(0);
    draw();

    return {
        get overview() {
            return overviewOpen;
        },
        openOverview,
        closeOverview,
    };
}
