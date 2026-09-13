/*
 * The core: a turning globe, on the graphics card, inside the page.
 *
 * This was a separate Godot program for a while, in a window of its own that
 * was made a child of the brain's and moved to sit over this panel. It worked,
 * and it was the wrong shape for the problem: a second process with a second
 * window that had to be told where to be, could drift out of place, could not
 * respect the card's rounded corners, and added seventy megabytes to a program
 * that was seven. WebGL is on the graphics card too, and it is inside the page,
 * where a panel is simply a panel.
 *
 * The rings and the sphere are ornament and do not pretend otherwise. What is
 * real is the brightness: it follows the measured audio level and how much the
 * last reply actually recalled. A still globe means nothing is happening.
 */

import * as THREE from 'three';
import { EffectComposer } from 'three/addons/postprocessing/EffectComposer.js';
import { RenderPass } from 'three/addons/postprocessing/RenderPass.js';
import { UnrealBloomPass } from 'three/addons/postprocessing/UnrealBloomPass.js';
import { signals, easeSignals, onRecall } from './signals.js';

const canvas = document.getElementById('brainmap');

const RADIUS = 1;

/** The halo behind the sphere. A gradient, because glow needs something to be. */
function haloTexture(size = 256) {
    const c = document.createElement('canvas');

    c.width = c.height = size;

    const ctx = c.getContext('2d');
    const g = ctx.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2);

    // Hollow in the middle: a halo brightest at its centre sits over the sphere
    // and washes the mesh out. What is wanted is light around it.
    g.addColorStop(0, 'rgba(40,120,200,0)');
    g.addColorStop(0.42, 'rgba(50,150,230,0.10)');
    g.addColorStop(0.56, 'rgba(70,190,255,0.34)');
    g.addColorStop(0.74, 'rgba(40,120,210,0.10)');
    g.addColorStop(1, 'rgba(10,40,90,0)');

    ctx.fillStyle = g;
    ctx.fillRect(0, 0, size, size);

    return new THREE.CanvasTexture(c);
}

/*
 * cssColour reads a colour out of the stylesheet.
 *
 * So that the one panel painted by a shader cannot drift away from the ones
 * painted by the cascade. The fallback is the same value the stylesheet holds,
 * for the case where this runs before the variables are set.
 */
function cssValue(name, fallback) {
    const raw = getComputedStyle(document.documentElement)
        .getPropertyValue(name).trim();

    return raw || fallback;
}

function cssColour(name, fallback) {
    const raw = getComputedStyle(document.documentElement)
        .getPropertyValue(name).trim();

    if (!raw) return new THREE.Color(fallback);

    try {
        return new THREE.Color(raw);
    } catch (err) {
        return new THREE.Color(fallback);
    }
}

export function startCore() {
    if (!canvas) return;

    const renderer = new THREE.WebGLRenderer({
        canvas,
        alpha: true,
        antialias: true,
        powerPreference: 'high-performance',
    });

    renderer.setClearAlpha(0);

    const scene = new THREE.Scene();
    const camera = new THREE.PerspectiveCamera(45, 1, 0.1, 100);

    camera.position.set(0, 1.15, 5.15);
    camera.lookAt(0, 0, 0);

    /*
     * The panel's own background, painted in the scene.
     *
     * The canvas was transparent and the card's gradient showed through it.
     * Once the bloom pass was added that stopped working — the composer renders
     * into its own target and hands back an opaque frame, so the panel went
     * black. Rather than fight that, the scene paints the same colours itself,
     * which has the side benefit the 2D version needed too: bloom has to have
     * something to bloom into, and against flat black a glow reads as a
     * wireframe with a halo stuck on it.
     *
     * The colours come from the stylesheet rather than from here. Painted by
     * hand they drifted, and the core ended up several shades darker than
     * every panel beside it — the one panel on the screen that is not a panel,
     * looking like a hole in the row.
     */
    /*
     * The panel's background is the scene's sky.
     *
     * It used to be a plane parked behind everything, which is a way of
     * pretending to have a background rather than having one — and it showed:
     * the plane is far bigger than the visible area, so the panel's corners
     * landed at arbitrary points of its gradient, and no amount of adjusting
     * made the core match the cards beside it.
     *
     * A sky is stretched to exactly the viewport, so what is painted is what
     * is seen. And it is painted by the browser's own gradient code, from the
     * same two colours the stylesheet gives every other card, at the same
     * angle — so this is not a rendering that resembles the panel background.
     * It is the panel background, drawn by the thing that draws the others.
     */
    function panelSky() {
        const c = document.createElement('canvas');

        // Small on purpose: it is a two-stop gradient stretched over a panel,
        // and a larger texture would carry no more information.
        c.width = c.height = 64;

        const ctx = c.getContext('2d');

        /*
         * CSS reckons a gradient angle clockwise from straight up and runs its
         * line through the centre, long enough that the box's corners land at
         * its ends. 160 degrees is what .card uses.
         */
        const angle = (160 * Math.PI) / 180;
        const sin = Math.sin(angle);
        const cos = Math.cos(angle);
        const length = Math.abs(c.width * sin) + Math.abs(c.height * cos);

        const grad = ctx.createLinearGradient(
            c.width / 2 - (sin * length) / 2, c.height / 2 + (cos * length) / 2,
            c.width / 2 + (sin * length) / 2, c.height / 2 - (cos * length) / 2);

        grad.addColorStop(0, cssValue('--card-top', '#0d1c2b'));
        grad.addColorStop(1, cssValue('--card-bottom', '#080e17'));

        ctx.fillStyle = grad;
        ctx.fillRect(0, 0, c.width, c.height);

        const texture = new THREE.CanvasTexture(c);

        texture.colorSpace = THREE.SRGBColorSpace;

        return texture;
    }

    scene.background = panelSky();


    const globe = new THREE.Group();

    scene.add(globe);

    /*
     * The form: a triangle pointing down, around an eye.
     *
     * Taken from the film this was asked to look like. Two things carry it and
     * both matter — the inverted triangle, which is the silhouette you
     * recognise from across a room, and the iris inside it, which is where the
     * light comes from and the thing that reads as looking back at you.
     *
     * It faces the viewer rather than turning in space. A sphere had to rotate
     * to show it was alive; this does not, because an eye is already the most
     * alive thing a shape can be, and turning it would only make it a wheel.
     */
    const face = new THREE.Group();

    globe.add(face);

    /** How far the triangle's points sit from the middle. */
    const REACH = 1.9;

    /** Where the corners are, with one at the bottom. */
    function corners(reach) {
        const out = [];

        for (let i = 0; i < 3; i++) {
            const a = -Math.PI / 2 + (i / 3) * Math.PI * 2;

            out.push(new THREE.Vector2(Math.cos(a) * reach, Math.sin(a) * reach));
        }

        return out;
    }

    /** How thick the frame is drawn, in the same units as REACH. */
    const EDGE = 0.012;

    /*
     * The triangle is built out of thin quads rather than drawn with lines.
     *
     * A THREE.Line is a GL line, and a GL line is one pixel wide whatever is
     * asked of it, aliased along its length, and rendered slightly differently
     * by every driver. On a diagonal at the size this sits on screen that
     * produces a staircase: the edge steps across a pixel every few pixels and
     * the frame reads as a wobble rather than a straight line, which is the
     * one thing a triangle has to be.
     *
     * Two triangles of geometry per edge, six per frame. It costs nothing, it
     * is the same on every machine, and the thickness is a number rather than
     * a wish.
     *
     * Named frameEdges rather than frame, which is not fussiness: the render
     * loop below is also a function declaration called frame, in this same
     * scope, and the later declaration wins for the whole of it. Calling
     * frame(REACH, EDGE) therefore called the render loop, which returned
     * undefined, which became an empty geometry — and the triangle simply was
     * not there, with nothing logged anywhere to say why.
     */
    function frameEdges(reach, thickness) {
        const points = corners(reach);
        const vertices = [];

        for (let i = 0; i < 3; i++) {
            const from = points[i];
            const to = points[(i + 1) % 3];

            // The edge's own sideways direction, so the quad has square ends
            // and the corners meet without a gap.
            const along = new THREE.Vector2().subVectors(to, from).normalize();
            const across = new THREE.Vector2(-along.y, along.x).multiplyScalar(thickness);

            // Extended by half a thickness at each end, which is what closes
            // the corner: two rectangles meeting at a point leave a notch.
            const back = along.clone().multiplyScalar(thickness);

            const a = from.clone().sub(back);
            const b = to.clone().add(back);

            const corners4 = [
                a.clone().add(across), b.clone().add(across),
                b.clone().sub(across), a.clone().sub(across),
            ];

            for (const [x, y, z] of [[0, 1, 2], [0, 2, 3]]) {
                for (const at of [x, y, z]) {
                    vertices.push(corners4[at].x, corners4[at].y, 0);
                }
            }
        }

        const geometry = new THREE.BufferGeometry();

        geometry.setAttribute('position',
            new THREE.Float32BufferAttribute(vertices, 3));

        return geometry;
    }

    const frameMaterial = new THREE.MeshBasicMaterial({
        color: new THREE.Color(1, 0.32, 0.16),
        transparent: true,
        opacity: 0.9,
        depthWrite: false,
        side: THREE.DoubleSide,
        blending: THREE.AdditiveBlending,
    });

    // Two of them, one just inside the other. A single line reads as a shape
    // drawn on the screen; a pair reads as something built. The inner one is
    // thinner, so the pair has a front and a back rather than reading as one
    // fat smudge.
    face.add(new THREE.Mesh(frameEdges(REACH, EDGE), frameMaterial));
    face.add(new THREE.Mesh(frameEdges(REACH * 0.9, EDGE * 0.6), frameMaterial));

    /*
     * The iris.
     *
     * Five concentric rings, which is what this was, read as a target. An eye
     * is not made of rings — it is made of fibres running out from the pupil,
     * a hot corona where the light comes from, and an aperture that is never
     * quite even. So it is drawn rather than assembled: one disc, and a
     * fragment shader that works in polar coordinates.
     *
     * Cheaper as well as better, which is not a coincidence. Seven meshes and
     * seven draw calls became one, on a page that is already the most
     * expensive thing on this machine.
     */
    const irisMaterial = new THREE.ShaderMaterial({
        uniforms: {
            tint: { value: new THREE.Color(1, 0.35, 0.12) },
            lit: { value: 1 },
            turning: { value: 0 },
            open: { value: 0.5 },
        },
        vertexShader: `
            varying vec2 place;
            void main() {
                place = uv * 2.0 - 1.0;
                gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
            }`,
        fragmentShader: `
            uniform vec3 tint;
            uniform float lit;
            uniform float turning;
            uniform float open;
            varying vec2 place;

            #define PUPIL 0.30
            #define EDGE  0.98

            void main() {
                float d = length(place);

                if (d > EDGE || d < PUPIL) discard;

                float a = atan(place.y, place.x);

                // Where this point sits across the iris, pupil to rim.
                float across = (d - PUPIL) / (EDGE - PUPIL);

                /*
                 * The fibres.
                 *
                 * Two sets at different counts turning opposite ways, which is
                 * what stops it reading as a cog. They are thinner near the
                 * pupil and fan out, so the eye has a direction.
                 */
                float fine = sin(a * 64.0 + turning * 0.55);
                float coarse = sin(a * 24.0 - turning * 0.30 + across * 2.4);

                float fibres = pow(abs(fine), 2.5) * 0.30 + pow(abs(coarse), 1.6) * 0.55;

                /*
                 * Thickest just outside the pupil and gone by the rim.
                 *
                 * A real iris is densest where it meets the pupil, and drawing
                 * the fibres evenly across the whole disc is what made the old
                 * version look like a woven mat rather than an eye.
                 */
                fibres *= smoothstep(0.95, 0.05, across) * (0.45 + 0.55 * open);

                /*
                 * The corona: a hot band just outside the pupil.
                 *
                 * This is the part that makes it an eye rather than a hole. The
                 * light has to look like it comes from somewhere, and it comes
                 * from here — so it is the brightest thing in the shape by a
                 * distance, and everything else is what it falls on.
                 */
                /*
                 * Breathing, rather than turning.
                 *
                 * Something has to move or the shape reads as a picture, and
                 * the only motion that does not become a hand is one that
                 * happens everywhere at once. So the corona swells and settles
                 * where it meets the pupil — the way a pupil actually behaves,
                 * and unreadable as a direction.
                 */
                float breath = 0.30 + 0.045 * sin(turning * 1.1);

                float corona = smoothstep(breath, 0.0, across) * (1.7 + 0.25 * sin(turning * 0.8));

                /*
                 * A dark band across the middle of the iris.
                 *
                 * Nothing here occludes — it is all additive — so depth has to
                 * be made by leaving somewhere empty. Without this the corona
                 * and the rim run into each other and the whole thing flattens
                 * into a disc.
                 */
                float shadow = 1.0 - 0.55 * smoothstep(0.18, 0.55, across) *
                    smoothstep(0.95, 0.62, across);

                // And a thin rim, to close the shape off.
                float rim = smoothstep(0.88, 0.995, across) *
                    smoothstep(1.0, 0.955, across) * 9.0;

                /*
                 * The aperture: two soft blades, and they stay where they are.
                 *
                 * They used to turn, and a bright thing going round the middle
                 * of a circle is a clock hand however slowly it moves and
                 * whatever it is made of. An eye does not sweep. What is left
                 * is asymmetry, which is what they were for: an iris that is
                 * exactly even is a machine part.
                 */
                float blade = smoothstep(0.45, 1.0, sin(a * 2.0 + 0.7)) *
                    smoothstep(0.06, 0.7, across) * 0.35 * open;

                float light = ((fibres + blade) * shadow + corona + rim) * lit;

                /*
                 * The brightness must not eat the colour.
                 *
                 * Multiplying the tint by a number above one clips each
                 * channel in turn — red first, then green — so the palette's
                 * amber came out as fire: red at the edge, orange, yellow, and
                 * white in the middle. The core then agreed with nothing else
                 * on the screen, which is the one thing it is supposed to do.
                 *
                 * So the hue is carried by the colour and the intensity by the
                 * alpha, which under additive blending is what makes it
                 * bright. Only the very hottest part is allowed any white at
                 * all, and only a little.
                 */
                vec3 colour = tint + vec3(0.30) * smoothstep(1.5, 3.2, light);

                gl_FragColor = vec4(colour, clamp(light, 0.0, 0.95));
            }`,
        transparent: true,
        depthWrite: false,
        side: THREE.DoubleSide,
        blending: THREE.AdditiveBlending,
    });

    const iris = new THREE.Mesh(new THREE.CircleGeometry(0.78, 96), irisMaterial);

    iris.position.z = 0.01;
    face.add(iris);

    /*
     * The pupil: a dark disc with a hot rim.
     *
     * Drawn opaque and in front, because everything else here is additive and
     * additive light cannot make anything darker. Without something that
     * actually occludes, the middle fills in and the eye closes.
     */
    const pupil = new THREE.Mesh(
        new THREE.CircleGeometry(0.235, 64),
        new THREE.MeshBasicMaterial({ color: 0x08060a, transparent: true, opacity: 0.92 }));

    pupil.position.z = 0.02;
    face.add(pupil);

    /*
     * The pupil's own rim, on its own material.
     *
     * The iris is now drawn in polar coordinates across a flat disc, and that
     * shader put on a torus would produce nothing recognisable. This is a
     * plain bright ring, tinted with everything else.
     */
    const rimMaterial = new THREE.MeshBasicMaterial({
        color: 0xff5a1e,
        transparent: true,
        opacity: 0.95,
        depthWrite: false,
        blending: THREE.AdditiveBlending,
    });

    const rim = new THREE.Mesh(new THREE.TorusGeometry(0.235, 0.018, 6, 48), rimMaterial);

    rim.position.z = 0.03;
    face.add(rim);

    /* ---------- the glow behind it ---------- */

    /*
     * The glow lights the eye, not the panel.
     *
     * It was five units across against a view about four units tall — larger
     * than the whole panel — so whatever the background underneath was painted
     * as, what anybody actually saw was a blue wash filling the card. The
     * gradient below it could be made to match the other panels exactly, and
     * the core would still be the one that looked different, because the
     * matching part was covered up.
     *
     * Three units keeps it inside the triangle, where a glow around an eye
     * belongs, and leaves the corners of the panel as plain card background.
     */
    const halo = new THREE.Sprite(new THREE.SpriteMaterial({
        map: haloTexture(),
        transparent: true,
        depthWrite: false,
        blending: THREE.AdditiveBlending,
        opacity: 0.5,
    }));

    halo.scale.set(3.0, 3.0, 1);
    halo.position.z = -0.5;
    scene.add(halo);

    /* ---------- arcs, when memories are used ---------- */

    /*
     * One arc leaps across the globe for every memory a reply actually used.
     *
     * Ornament in how it looks and information in when it appears: if the globe
     * is quiet, nothing was recalled. That distinction is the whole reason any
     * of this is allowed to be here — a flourish that fired on a schedule would
     * look the same and mean nothing.
     */
    const arcs = [];
    const ARC_LIFE = 2.4;

    function spawnArc(seed) {
        const from = new THREE.Vector3().setFromSphericalCoords(
            RADIUS, Math.acos(1 - 2 * ((seed * 0.37) % 1)), seed * 2.4);
        const to = new THREE.Vector3().setFromSphericalCoords(
            RADIUS, Math.acos(1 - 2 * ((seed * 0.71 + 0.3) % 1)), seed * 3.1 + 2);

        // Lifted off the surface at the middle, so it goes over the sphere
        // rather than through it.
        const middle = from.clone().add(to).multiplyScalar(0.5).setLength(RADIUS * 1.28);
        const curve = new THREE.QuadraticBezierCurve3(from, middle, to);

        const line = new THREE.Line(
            new THREE.BufferGeometry().setFromPoints(curve.getPoints(28)),
            new THREE.LineBasicMaterial({
                color: 0xdff8fd,
                transparent: true,
                opacity: 0,
                depthWrite: false,
                blending: THREE.AdditiveBlending,
            }));

        globe.add(line);
        arcs.push({ line, age: 0 });
    }

    onRecall((ids) => {
        const many = Math.min(ids.length, 9);

        for (let i = 0; i < many; i++) spawnArc(performance.now() * 0.001 + i * 7.13);
    });

    function updateArcs(delta) {
        for (let i = arcs.length - 1; i >= 0; i--) {
            const arc = arcs[i];

            arc.age += delta;

            if (arc.age >= ARC_LIFE) {
                globe.remove(arc.line);
                arc.line.geometry.dispose();
                arc.line.material.dispose();
                arcs.splice(i, 1);

                continue;
            }

            // Rises and falls, so an arc arrives rather than switching on.
            arc.line.material.opacity = Math.sin((arc.age / ARC_LIFE) * Math.PI) * 0.9;
        }
    }

    /*
     * No sweeping band.
     *
     * There was one, going pole to pole while the brain worked, and it was
     * removed on the owner's judgement: sitting behind a conversation, a thing
     * that keeps travelling across the field of view reads as a metronome. It
     * pulls the eye on a rhythm that has nothing to do with what is happening.
     *
     * The colour says it instead, which is the whole of what needed saying:
     * orange while it is working, green while it is speaking, and its ordinary
     * blue the rest of the time. A colour change is seen at a glance and then
     * stops asking for attention, which is what a status light should do.
     */

    /* ---------- bloom ---------- */

    /*
     * Real bloom, rather than the fake one.
     *
     * Everything before this drew its own glow — a sprite behind the sphere, a
     * gradient in a dot texture, arcs stroked twice at different widths. Those
     * are imitations of what light does, each hand-placed, and they cannot
     * respond to what is actually bright: a scanner crossing the equator or a
     * cluster of arcs firing at once should light the whole panel, and painted
     * glow never will because it does not know they are there.
     *
     * The threshold matters. Only what is brighter than the body blooms, so the
     * mesh stays a mesh and the bright things — points, sparks, the scanner —
     * are what spill light.
     */
    const composer = new EffectComposer(renderer);

    composer.addPass(new RenderPass(scene, camera));

    const bloom = new UnrealBloomPass(new THREE.Vector2(1, 1), 0.85, 0.55, 0.55);

    composer.addPass(bloom);

    /* ---------- the loop ---------- */

    let last = performance.now();

    // Turned by accumulated time rather than the clock, so that changing the
    // rate speeds it up from where it is instead of jumping to wherever the
    // clock says the faster rate would have put it by now.
    let spin = 0;

    function resize() {
        const rect = canvas.getBoundingClientRect();

        if (rect.width < 2 || rect.height < 2) return false;

        const ratio = Math.min(window.devicePixelRatio || 1, 2);

        renderer.setPixelRatio(ratio);
        renderer.setSize(rect.width, rect.height, false);

        // The bloom pass renders at its own size, and it is the expensive one:
        // three quarters of the width is a quarter less work for a blur nobody
        // can see the resolution of.
        composer.setPixelRatio(ratio * 0.75);
        composer.setSize(rect.width, rect.height);

        camera.aspect = rect.width / rect.height;
        camera.updateProjectionMatrix();

        return true;
    }

    /*
     * How often this is worth drawing.
     *
     * The browser offers sixty frames a second and there is no graphics card
     * in this machine, so every one of them is drawn by the same processor
     * that is running the language model — measured at more than half a core,
     * to animate a slowly turning ring.
     *
     * Nothing here moves fast enough to need sixty. Thirty is
     * indistinguishable while the brain is working, and while it is sitting
     * idle the picture barely changes at all, so it drops to eight — which is
     * still motion, and an eighth of the work.
     *
     * Paced rather than throttled: the animation is driven by elapsed time, so
     * a frame skipped is not a frame of movement lost. It arrives at the same
     * place, less often.
     */
    const WHILE_WORKING = 1000 / 30;
    const WHILE_RESTING = 1000 / 8;

    // The states in which nothing is happening that anybody is watching for.
    const QUIET = new Set(['idle', 'listening', 'waiting', 'stopping']);

    let drawnAt = 0;

    // Measured over the first few dozen frames; see FRAME_BUDGET_MS.
    let glowIsAffordable = true;
    let drawnFrames = 0;
    let drawnTime = 0;

/*
 * Whether the glow is affordable here, measured rather than guessed.
 *
 * The bloom pass renders the scene again into a buffer and blurs it — nothing
 * at all on a graphics card, and the most expensive thing on this page without
 * one. Which of those a machine is cannot be asked directly: WebGL reports a
 * renderer name that on this webview says nothing useful, and the program's own
 * idea of "has a GPU" means one that can run a language model, which is a
 * different question.
 *
 * So it is timed. The first two seconds are drawn with the glow while the cost
 * of a frame is measured; past the budget, the glow goes and does not come
 * back. A machine that can afford it keeps it and never notices this existed.
 */
const FRAME_BUDGET_MS = 9;
const JUDGE_AFTER = 40;

    function frame(now) {
        requestAnimationFrame(frame);

        /*
         * Nothing is drawn while nobody is looking at it.
         *
         * The core lives on the command centre, and the browser goes on calling
         * this sixty times a second while its owner is reading the System page
         * — rendering a scene inside a hidden div, on a processor that is
         * already the reason answers take as long as they do.
         *
         * offsetParent is null exactly when an ancestor is display:none, which
         * is what a hidden view is. Cheap enough to ask every frame, and it
         * asks nothing of the layout that was not already computed.
         */
        if (canvas.offsetParent === null || document.hidden) return;

        /*
         * And not more often than it is worth drawing.
         *
         * Which of the two rates applies comes from the same signal that
         * drives the picture: at rest the scene is a slow pulse, and while
         * something is happening it has to keep up with what it is showing.
         */
        /*
         * Listening is resting.
         *
         * The microphone is open from the moment the program starts and stays
         * open, because that is what a thing you can talk to does — so
         * treating "listening" as work meant the higher rate was the only rate
         * this ever ran at. What is worth thirty frames a second is the brain
         * actually doing something: thinking, working, speaking, learning.
         */
        const gap = QUIET.has(signals.state || 'idle') ? WHILE_RESTING : WHILE_WORKING;

        if (now - drawnAt < gap) return;

        const delta = Math.min(0.25, (now - (last || now)) / 1000);

        last = now;
        drawnAt = now;

        // Counted so that "it stops when nobody is looking" is a thing that can
        // be checked rather than believed. Costs one addition per frame.
        window.brainCoreFrames = (window.brainCoreFrames || 0) + 1;

        if (!resize()) return;

        easeSignals(delta);

        const seconds = now / 1000;

        // The form faces the viewer and does not turn. What moves is inside it.

        updateArcs(delta);

        // Brightness from sound that is really there, lifted while working so
        // the whole body reads as busy and not only the band crossing it.
        /*
         * Only work somebody is waiting on colours the core.
         *
         * Learning from the last conversation runs a minute after every
         * exchange, and colouring the core for it means opening the program and
         * finding it already amber and apparently thinking about a question
         * nobody asked. The working line still says what it is doing; the core
         * is reserved for the turn its owner is waiting for.
         */
        const working = signals.work && signals.work.busy && !signals.work.background;
        const busy = working ? 0.5 : 0;
        const lit = 0.7 + signals.smooth * 2.2 + signals.recall * 0.8 + busy;

        /*
         * What the sphere is coloured, in the order that answers "what is it
         * doing" soonest. Working comes first: a turn that is thinking while
         * the microphone happens to be open is thinking, and that is the thing
         * worth saying.
         */
        /*
         * Three states, said with colour alone.
         *
         * Working first: a turn that is thinking while the microphone happens
         * to be open is thinking, and that is the thing worth saying.
         */
        /*
         * Four states, in the order that answers soonest.
         *
         * Working first: a turn that is thinking while the microphone happens
         * to be open is thinking, and that is the thing worth saying. Then its
         * own voice, then yours, then rest.
         */
        /*
         * Coloured by the same status as everything else on the page.
         *
         * The core used to decide this for itself from four colours, while the
         * feed had five tones of its own and the talk button a third set in
         * CSS — so a single moment was amber in the middle of the screen,
         * cyan in the panel beside it and violet on the button below. Now all
         * three ask the same question and get the same answer.
         *
         * The core still leads with work over listening: a turn that is
         * thinking while the microphone happens to be open is thinking, and
         * that is the thing worth saying.
         */
        const status = window.brainStatusOf
            ? window.brainStatusOf(signals.work)
            : 'idle';

        const resting = status === 'idle' &&
            signals.state === 'listening' && signals.smooth > 0.02
            ? 'listening'
            : status;

        const tint = new THREE.Color(statusColour(resting, signals.look));

        irisMaterial.uniforms.tint.value.copy(tint);
        irisMaterial.uniforms.lit.value = Math.min(1.7, lit);
        irisMaterial.uniforms.turning.value = seconds;

        /*
         * The aperture opens while it is doing something.
         *
         * The one piece of information the shape carries beyond its colour: a
         * working core is wider open than a resting one, which is legible from
         * across a room in a way that a hue is not.
         */
        irisMaterial.uniforms.open.value = 0.35 + signals.smooth * 0.5 + (busy ? 0.3 : 0);

        rimMaterial.color.copy(tint);

        frameMaterial.color.copy(tint);
        frameMaterial.opacity = 0.7 + signals.smooth * 0.3;

        halo.material.color.copy(tint);
        halo.material.opacity = 0.26 + signals.smooth * 0.34 + signals.recall * 0.2;

        // Brighter while working, so a busy core spills more light than a
        // quiet one without anything being drawn differently.
        bloom.strength = 0.45 + signals.smooth * 0.5 + (busy ? 0.25 : 0) + signals.recall * 0.3;

        /*
         * The glow is skipped where it has to be drawn by the processor.
         *
         * The bloom pass renders the whole scene again into a buffer and blurs
         * it, twice — cheap on a graphics card and the single most expensive
         * thing on this page without one, which is the machine this was built
         * on. What is lost is a soft halo; what is bought is most of a core
         * that the language model is waiting for.
         *
         * Asked of the renderer rather than assumed: a machine with
         * acceleration keeps the glow.
         */
        if (glowIsAffordable) {
            const startedDrawing = performance.now();

            composer.render();

            /*
             * Judged once, on the average of the first few dozen frames.
             *
             * Not on one frame: the first is always slow, and a single reading
             * during a burst of other work would drop the glow on a machine
             * that can afford it.
             */
            drawnFrames += 1;
            drawnTime += performance.now() - startedDrawing;

            if (drawnFrames >= JUDGE_AFTER) {
                const each = drawnTime / drawnFrames;

                glowIsAffordable = each <= FRAME_BUDGET_MS;
                window.brainGlowCost = Math.round(each * 10) / 10;

                if (!glowIsAffordable) {
                    // Said once, because it is a visible change to the picture
                    // and somebody may wonder where the halo went.
                    console.info(
                        `PN Scripts Assistant: the glow costs ${each.toFixed(1)}ms a frame on this ` +
                        'machine, which the processor is needed for. Drawing it plainly.'
                    );
                }
            }
        } else {
            renderer.render(scene, camera);
        }

        /*
         * The loading screen is waiting on this one.
         *
         * Reported after the first frame is on the screen rather than when the
         * scene is built: a reactor that exists and has not drawn is exactly
         * as blank as one that does not, and it is the drawing that somebody
         * is waiting to see. See boot.js.
         */
        if (window.brainBooted) window.brainBooted('drawn');
    }

    requestAnimationFrame(frame);
}

/*
 * statusColour is the one place the core turns a status into a colour.
 *
 * Reads the same palette the rest of the page uses, so that changing a colour
 * by voice changes it here too rather than in the stylesheet alone.
 */
function statusColour(status, look) {
    const palette = window.brainLook || look || {};

    const named = {
        idle: palette.idle,
        listening: palette.listening,
        hearing: palette.hearing,
        thinking: palette.thinking_core,
        tool: palette.tool,
        speaking: palette.speaking,
        waiting: palette.waiting,
        learning: palette.learning,
    };

    // Falls back to thinking rather than to nothing: a core with no colour is
    // invisible, and an unknown status is more likely work than rest.
    return named[status] || palette.thinking_core || '#f0b26b';
}
