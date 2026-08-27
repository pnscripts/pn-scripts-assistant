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

/** Seconds for the sphere to come round once. */
const SECONDS_PER_TURN = 150;

/** Written as periods rather than as angular speeds, for a reason recorded in
 *  the project's history: expressed the other way, one of these numbers was
 *  wrong by a factor of two hundred and fifty and survived two readings. */
const ORBITS = [
    { rx: 2.05, rz: 1.35, lean: -0.55, tip: 0.30, seconds: 300 },
    { rx: 1.80, rz: 1.62, lean: 0.42, tip: -0.22, seconds: -220 },
    { rx: 2.20, rz: 1.10, lean: 0.12, tip: 0.62, seconds: 380 },
    { rx: 1.95, rz: 1.48, lean: -0.34, tip: -0.50, seconds: -460 },
    { rx: 2.30, rz: 0.95, lean: 0.62, tip: 0.10, seconds: 520 },
    { rx: 1.70, rz: 1.70, lean: -0.10, tip: -0.68, seconds: -340 },
];

const SURFACE_POINTS = 900;
const LATITUDES = 9;
const LONGITUDES = 18;
const RADIUS = 1;

/** A soft round dot, drawn once and used by every point sprite. */
function dotTexture(size = 64) {
    const c = document.createElement('canvas');

    c.width = c.height = size;

    const ctx = c.getContext('2d');
    const g = ctx.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2);

    g.addColorStop(0, 'rgba(255,255,255,1)');
    g.addColorStop(0.35, 'rgba(210,245,255,0.85)');
    g.addColorStop(1, 'rgba(120,200,255,0)');

    ctx.fillStyle = g;
    ctx.fillRect(0, 0, size, size);

    return new THREE.CanvasTexture(c);
}

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

    camera.position.set(0, 1.05, 4.25);
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
     */
    const ground = new THREE.Mesh(
        new THREE.PlaneGeometry(40, 40),
        new THREE.ShaderMaterial({
            uniforms: { middle: { value: new THREE.Color(0x14324c) }, edge: { value: new THREE.Color(0x070d16) } },
            vertexShader: `
                varying vec2 spot;
                void main() {
                    spot = uv;
                    gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
                }`,
            fragmentShader: `
                uniform vec3 middle;
                uniform vec3 edge;
                varying vec2 spot;
                void main() {
                    float away = distance(spot, vec2(0.5, 0.52)) * 2.2;
                    gl_FragColor = vec4(mix(middle, edge, clamp(away, 0.0, 1.0)), 1.0);
                }`,
            depthWrite: false,
        }));

    ground.position.z = -14;
    scene.add(ground);

    const globe = new THREE.Group();

    scene.add(globe);

    const dots = dotTexture();

    /* ---------- the surface ---------- */

    const surface = new Float32Array(SURFACE_POINTS * 3);
    const golden = Math.PI * (3 - Math.sqrt(5));

    for (let i = 0; i < SURFACE_POINTS; i++) {
        // The golden angle against an even spread in the sine of latitude, which
        // is what distributes points evenly over a sphere rather than crowding
        // them at the poles.
        const y = 1 - (i / (SURFACE_POINTS - 1)) * 2;
        const ring = Math.sqrt(Math.max(0, 1 - y * y));
        const angle = golden * i;

        surface[i * 3] = Math.cos(angle) * ring * RADIUS;
        surface[i * 3 + 1] = y * RADIUS;
        surface[i * 3 + 2] = Math.sin(angle) * ring * RADIUS;
    }

    const surfaceGeometry = new THREE.BufferGeometry();

    surfaceGeometry.setAttribute('position', new THREE.BufferAttribute(surface, 3));

    /*
     * A shader rather than PointsMaterial, for two things it cannot do.
     *
     * Points on the far side of the sphere are dimmed by their depth, which is
     * what stops a wireframe globe reading as a flat disc of confetti. And each
     * point breathes on its own slow cycle, so the surface is never quite
     * still — the difference between a photograph of a globe and a globe.
     */
    const surfaceMaterial = new THREE.ShaderMaterial({
        uniforms: {
            map: { value: dots },
            tint: { value: new THREE.Color(0.55, 0.9, 1) },
            lit: { value: 1 },
            time: { value: 0 },
        },
        vertexShader: `
            uniform float time;
            varying float depth;
            varying float twinkle;
            void main() {
                vec4 seen = modelViewMatrix * vec4(position, 1.0);
                depth = clamp((seen.z + 1.6) / 3.2, 0.0, 1.0);
                twinkle = 0.75 + 0.25 * sin(time * 1.4 + position.x * 9.0 + position.y * 7.0);
                gl_PointSize = (2.2 + 2.6 * depth) * twinkle * 5.0 / -seen.z;
                gl_Position = projectionMatrix * seen;
            }`,
        fragmentShader: `
            uniform sampler2D map;
            uniform vec3 tint;
            uniform float lit;
            varying float depth;
            varying float twinkle;
            void main() {
                vec4 dot = texture2D(map, gl_PointCoord);
                if (dot.a < 0.03) discard;
                float front = 0.3 + 0.7 * depth;
                gl_FragColor = vec4(tint * lit * front * twinkle, dot.a * front);
            }`,
        transparent: true,
        depthWrite: false,
        blending: THREE.AdditiveBlending,
    });

    globe.add(new THREE.Points(surfaceGeometry, surfaceMaterial));

    /* ---------- the mesh ---------- */

    const mesh = [];
    const steps = 64;

    for (let i = 1; i < LATITUDES; i++) {
        const lat = (i / LATITUDES) * Math.PI - Math.PI / 2;
        const y = Math.sin(lat) * RADIUS;
        const ring = Math.cos(lat) * RADIUS;

        for (let step = 0; step < steps; step++) {
            const a = (step / steps) * Math.PI * 2;
            const b = ((step + 1) / steps) * Math.PI * 2;

            mesh.push(Math.cos(a) * ring, y, Math.sin(a) * ring);
            mesh.push(Math.cos(b) * ring, y, Math.sin(b) * ring);
        }
    }

    for (let i = 0; i < LONGITUDES; i++) {
        const lon = (i / LONGITUDES) * Math.PI;

        for (let step = 0; step < steps; step++) {
            const a = (step / steps) * Math.PI * 2;
            const b = ((step + 1) / steps) * Math.PI * 2;

            mesh.push(Math.cos(a) * Math.sin(lon) * RADIUS, Math.sin(a) * RADIUS, Math.cos(a) * Math.cos(lon) * RADIUS);
            mesh.push(Math.cos(b) * Math.sin(lon) * RADIUS, Math.sin(b) * RADIUS, Math.cos(b) * Math.cos(lon) * RADIUS);
        }
    }

    const meshGeometry = new THREE.BufferGeometry();

    meshGeometry.setAttribute('position', new THREE.Float32BufferAttribute(mesh, 3));

    const meshMaterial = new THREE.LineBasicMaterial({
        color: new THREE.Color(0.35, 0.8, 1),
        transparent: true,
        opacity: 0.24,
        depthWrite: false,
    });

    globe.add(new THREE.LineSegments(meshGeometry, meshMaterial));

    /* ---------- the shell ---------- */

    /*
     * A skin over the sphere that glows at the edge and is clear through the
     * middle.
     *
     * The wireframe alone reads as a cage: you see straight through it and
     * nothing says there is a surface there. This is the effect that makes it
     * a body — bright where you are looking along the surface, invisible where
     * you are looking at it head on, which is what light does at a glancing
     * angle on anything.
     */
    const shellMaterial = new THREE.ShaderMaterial({
        uniforms: {
            tint: { value: new THREE.Color(0.35, 0.8, 1) },
            lit: { value: 1 },
        },
        vertexShader: `
            varying vec3 normalOut;
            varying vec3 towardsEye;
            void main() {
                vec4 seen = modelViewMatrix * vec4(position, 1.0);
                normalOut = normalize(normalMatrix * normal);
                towardsEye = normalize(-seen.xyz);
                gl_Position = projectionMatrix * seen;
            }`,
        fragmentShader: `
            uniform vec3 tint;
            uniform float lit;
            varying vec3 normalOut;
            varying vec3 towardsEye;
            void main() {
                float facing = abs(dot(normalize(normalOut), normalize(towardsEye)));
                float edge = pow(1.0 - facing, 3.4);
                gl_FragColor = vec4(tint * lit, edge * 0.5);
            }`,
        transparent: true,
        depthWrite: false,
        side: THREE.FrontSide,
        blending: THREE.AdditiveBlending,
    });

    globe.add(new THREE.Mesh(new THREE.SphereGeometry(RADIUS * 1.005, 64, 48), shellMaterial));

    /* ---------- the halo ---------- */

    const halo = new THREE.Sprite(new THREE.SpriteMaterial({
        map: haloTexture(),
        transparent: true,
        depthWrite: false,
        blending: THREE.AdditiveBlending,
        opacity: 0.85,
    }));

    halo.scale.set(3.4, 3.4, 1);
    halo.position.z = -0.3;
    scene.add(halo);

    /* ---------- orbits, and a point running each ---------- */

    const travellers = [];

    ORBITS.forEach((orbit, index) => {
        const path = [];

        for (let step = 0; step <= 160; step++) {
            const a = (step / 160) * Math.PI * 2;

            path.push(Math.cos(a) * orbit.rx, 0, Math.sin(a) * orbit.rz);
        }

        const geometry = new THREE.BufferGeometry();

        geometry.setAttribute('position', new THREE.Float32BufferAttribute(path, 3));

        const line = new THREE.Line(geometry, new THREE.LineBasicMaterial({
            color: new THREE.Color(0.4, 0.85, 1),
            transparent: true,
            opacity: 0.22,
            depthWrite: false,
        }));

        line.rotation.set(orbit.lean, 0, orbit.tip);
        scene.add(line);

        const spark = new THREE.Sprite(new THREE.SpriteMaterial({
            map: dots,
            transparent: true,
            depthWrite: false,
            blending: THREE.AdditiveBlending,
            color: new THREE.Color(0.75, 0.97, 1),
        }));

        spark.scale.set(0.13, 0.13, 1);
        line.add(spark);

        // Set apart, or six points that all begin at the same angle spend the
        // first several minutes bunched in one corner.
        travellers.push({ orbit, spark, phase: (index / ORBITS.length) * Math.PI * 2 });
    });

    /* ---------- the floor ---------- */

    const floor = [];

    for (let ring = 0; ring < 5; ring++) {
        const spread = 0.95 + ring * 0.22;
        const count = 96 + ring * 16;

        for (let step = 0; step < count; step++) {
            const a = (step / count) * Math.PI * 2;

            floor.push(Math.cos(a) * spread * 1.45, -1.12, Math.sin(a) * spread * 0.95);
        }
    }

    const floorGeometry = new THREE.BufferGeometry();

    floorGeometry.setAttribute('position', new THREE.Float32BufferAttribute(floor, 3));

    scene.add(new THREE.Points(floorGeometry, new THREE.PointsMaterial({
        size: 0.022,
        map: dots,
        transparent: true,
        depthWrite: false,
        blending: THREE.AdditiveBlending,
        opacity: 0.5,
        color: new THREE.Color(0.4, 0.85, 1),
    })));

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

    /* ---------- the scanner ---------- */

    /*
     * A band that sweeps the sphere while the brain is working.
     *
     * This is the core's whole job when nothing is being said. A reply here can
     * take minutes, and for all of that the globe turned at exactly the rate it
     * turns when idle — so the one thing on screen that ought to say "this is
     * alive" said nothing at all, and the honest question, is it working or is
     * it stuck, had to be answered by a line of text elsewhere.
     *
     * It moves only while there is real work. A band that swept on a timer
     * would look identical and mean nothing, and once one thing on a display
     * means nothing the rest stops being believed.
     */
    const scanner = new THREE.Mesh(
        new THREE.TorusGeometry(1, 0.012, 6, 96),
        new THREE.MeshBasicMaterial({
            color: 0x8fe9f2,
            transparent: true,
            opacity: 0,
            depthWrite: false,
            blending: THREE.AdditiveBlending,
        }));

    scanner.rotation.x = Math.PI / 2;
    globe.add(scanner);

    /** What each kind of work looks like. Colour carries the kind; the sweep
     *  carries the fact that there is any. */
    const WORK = {
        thinking: { colour: 0x8fe9f2, seconds: 2.6 },
        tool: { colour: 0xf0b26b, seconds: 1.5 },
        learning: { colour: 0xb39ddb, seconds: 2.0 },
        model: { colour: 0x7bc47f, seconds: 3.2 },
        embedding: { colour: 0xb39ddb, seconds: 2.0 },
        waiting: { colour: 0xf0b26b, seconds: 4.0 },
    };

    let sweep = 0;

    function updateScanner(delta, now) {
        const work = signals.work || {};
        const style = WORK[work.kind] || WORK.thinking;

        if (!work.busy) {
            // Fades rather than switching off, so the end of a turn is a
            // settling rather than a blink.
            scanner.material.opacity = Math.max(0, scanner.material.opacity - delta * 2);
            sweep = 0;

            return;
        }

        sweep = (sweep + delta / style.seconds) % 1;

        // Pole to pole. The band is widest at the equator because that is what
        // a circle on a sphere does, which is also what makes it read as
        // passing through the body rather than sliding across a picture of one.
        const y = Math.cos(sweep * Math.PI);
        const radius = Math.sqrt(Math.max(0.0001, 1 - y * y));

        scanner.position.y = y * RADIUS;
        scanner.scale.set(radius, radius, 1);
        scanner.material.color.setHex(style.colour);

        // Brightest crossing the middle, faint at the poles, so the sweep has a
        // shape rather than a hard start and stop.
        scanner.material.opacity = 0.35 + Math.sin(sweep * Math.PI) * 0.5;
    }

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

    function frame(now) {
        requestAnimationFrame(frame);

        const delta = Math.min(0.1, (now - last) / 1000);

        last = now;

        if (!resize()) return;

        easeSignals(delta);

        const seconds = now / 1000;

        spin += delta * (signals.work && signals.work.busy ? 2.6 : 1);
        globe.rotation.y = (spin * Math.PI * 2) / SECONDS_PER_TURN;

        for (const t of travellers) {
            const a = (seconds * Math.PI * 2) / t.orbit.seconds + t.phase;

            t.spark.position.set(Math.cos(a) * t.orbit.rx, 0, Math.sin(a) * t.orbit.rz);
        }

        updateScanner(delta, now);
        updateArcs(delta);

        // Brightness from sound that is really there, lifted while working so
        // the whole body reads as busy and not only the band crossing it.
        const busy = signals.work && signals.work.busy ? 0.5 : 0;
        const lit = 0.7 + signals.smooth * 2.2 + signals.recall * 0.8 + busy;
        const speaking = signals.state === 'speaking';
        const tint = speaking
            ? new THREE.Color(0.45, 1, 0.62)
            : new THREE.Color(0.55, 0.9, 1);

        surfaceMaterial.uniforms.tint.value.copy(tint);
        surfaceMaterial.uniforms.lit.value = Math.min(1.55, lit);
        surfaceMaterial.uniforms.time.value = seconds;
        meshMaterial.color.copy(tint);
        shellMaterial.uniforms.tint.value.copy(tint);
        shellMaterial.uniforms.lit.value = Math.min(1.5, lit * 0.8);
        meshMaterial.opacity = 0.18 + signals.smooth * 0.3;
        halo.material.opacity = 0.6 + signals.smooth * 0.5;

        // Brighter while working, so a busy core spills more light than a
        // quiet one without anything being drawn differently.
        bloom.strength = 0.45 + signals.smooth * 0.5 + (busy ? 0.25 : 0) + signals.recall * 0.3;

        composer.render();
    }

    requestAnimationFrame(frame);
}
