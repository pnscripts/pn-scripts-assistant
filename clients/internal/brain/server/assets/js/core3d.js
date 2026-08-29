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
    function triangle(reach) {
        const points = [];

        for (let i = 0; i < 3; i++) {
            const a = -Math.PI / 2 + (i / 3) * Math.PI * 2;

            points.push(new THREE.Vector3(Math.cos(a) * reach, Math.sin(a) * reach, 0));
        }

        points.push(points[0].clone());

        return new THREE.BufferGeometry().setFromPoints(points);
    }

    const frameMaterial = new THREE.LineBasicMaterial({
        color: new THREE.Color(1, 0.32, 0.16),
        transparent: true,
        opacity: 0.9,
        depthWrite: false,
        blending: THREE.AdditiveBlending,
    });

    // Two of them, one just inside the other. A single line reads as a shape
    // drawn on the screen; a pair reads as something built.
    face.add(new THREE.Line(triangle(REACH), frameMaterial));
    face.add(new THREE.Line(triangle(REACH * 0.9), frameMaterial));

    /*
     * The iris.
     *
     * Rings of decreasing radius around a dark centre, each a little brighter
     * than the last, turning slowly and not together. The dark middle is the
     * part that makes it an eye: without it this is a target, and with it the
     * light has somewhere to be coming from.
     */
    const irisMaterial = new THREE.ShaderMaterial({
        uniforms: {
            tint: { value: new THREE.Color(1, 0.35, 0.12) },
            lit: { value: 1 },
        },
        vertexShader: `
            varying float across;
            void main() {
                across = uv.y;
                gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
            }`,
        fragmentShader: `
            uniform vec3 tint;
            uniform float lit;
            varying float across;
            void main() {
                // Brightest along the middle of the band, so each ring has an
                // edge that fades rather than a hard rim.
                float edge = sin(across * 3.14159);

                gl_FragColor = vec4(tint * lit, edge * 0.9);
            }`,
        transparent: true,
        depthWrite: false,
        side: THREE.DoubleSide,
        blending: THREE.AdditiveBlending,
    });

    const iris = new THREE.Group();

    face.add(iris);

    const rings = [];

    for (let i = 0; i < 5; i++) {
        const radius = 0.74 - i * 0.115;
        const band = new THREE.Mesh(
            new THREE.TorusGeometry(radius, 0.028 + i * 0.006, 8, 128),
            irisMaterial);

        iris.add(band);
        rings.push({ band, turn: (i % 2 ? -1 : 1) * (14 + i * 5) });
    }

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

    const rim = new THREE.Mesh(new THREE.TorusGeometry(0.235, 0.02, 8, 96), irisMaterial);

    rim.position.z = 0.03;
    face.add(rim);

    /* ---------- the glow behind it ---------- */

    const halo = new THREE.Sprite(new THREE.SpriteMaterial({
        map: haloTexture(),
        transparent: true,
        depthWrite: false,
        blending: THREE.AdditiveBlending,
        opacity: 0.85,
    }));

    halo.scale.set(5.0, 5.0, 1);
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

    function frame(now) {
        requestAnimationFrame(frame);

        const delta = Math.min(0.1, (now - last) / 1000);

        last = now;

        if (!resize()) return;

        easeSignals(delta);

        const seconds = now / 1000;

        // The form faces the viewer and does not turn. What moves is inside it.

        updateArcs(delta);

        // Brightness from sound that is really there, lifted while working so
        // the whole body reads as busy and not only the band crossing it.
        const working = signals.work && signals.work.busy;
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
        const tint = new THREE.Color(
            working ? signals.look.thinking_core
                : signals.state === 'speaking' ? signals.look.speaking
                    : signals.state === 'listening' && signals.smooth > 0.02
                        ? signals.look.listening
                        : signals.look.idle);

        irisMaterial.uniforms.tint.value.copy(tint);
        irisMaterial.uniforms.lit.value = Math.min(1.7, lit);

        frameMaterial.color.copy(tint);
        frameMaterial.opacity = 0.7 + signals.smooth * 0.3;

        halo.material.color.copy(tint);
        halo.material.opacity = 0.5 + signals.smooth * 0.55 + signals.recall * 0.3;

        // The rings turn, slowly and not together. This is the only motion in
        // the form, and it is what keeps an eye from looking painted on.
        for (const ring of rings) {
            ring.band.rotation.z = seconds / ring.turn;
        }

        // Brighter while working, so a busy core spills more light than a
        // quiet one without anything being drawn differently.
        bloom.strength = 0.45 + signals.smooth * 0.5 + (busy ? 0.25 : 0) + signals.recall * 0.3;

        composer.render();
    }

    requestAnimationFrame(frame);
}
