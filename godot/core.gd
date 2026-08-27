extends Node3D

## The core: a turning globe, built on the graphics card.
##
## This replaces a hand-drawn 2D canvas. The canvas version was measured at
## roughly a fifth of a processor core, on a machine whose processor is also the
## thing running the language model — so every frame of it came out of how long
## an answer takes. The graphics card on this machine reports OpenGL 4.6 with
## direct rendering and had almost nothing to do.
##
## What is drawn is ornament and does not pretend otherwise. What it reads from
## the brain is real: how loud the voice or the microphone is at this moment,
## and how much was recalled by the last reply. When the globe is bright,
## something is happening; when it is quiet, nothing is.

## How long the globe takes to turn once, in seconds.
##
## Written as a period rather than as an angular speed. The 2D version expressed
## this as radians per millisecond and the number was wrong by a factor of about
## two hundred and fifty for a while, unnoticed through two readings, because a
## figure like 0.000045 does not mean anything to look at. A period does.
const SECONDS_PER_TURN := 150.0

const RADIUS := 1.0
const SURFACE_POINTS := 900
const LATITUDES := 9
const LONGITUDES := 16

## Where the brain answers. Loopback only, and only ever read from.
const API := "http://127.0.0.1:8790"

## How often the level is asked for. The brain measures it continuously; this is
## simply how often the picture catches up.
const LEVEL_INTERVAL := 0.09

var _globe: Node3D
var _points: MultiMeshInstance3D
var _wire: MeshInstance3D
var _orbits: Node3D
var _http: HTTPRequest
var _timer := 0.0

## The measured audio level, eased. Nothing here is driven by a timer.
var _level := 0.0
var _target := 0.0
var _speaking := false

## Rises when memories are recalled and falls away afterwards.
var _recall := 0.0


func _ready() -> void:
	_build_camera()
	_build_environment()

	_globe = Node3D.new()
	add_child(_globe)

	_build_halo()
	_build_surface()
	_build_wireframe()
	_build_orbits()
	_build_floor()

	_http = HTTPRequest.new()
	add_child(_http)
	_http.request_completed.connect(_on_level)


func _build_camera() -> void:
	var camera := Camera3D.new()

	camera.fov = 45.0
	add_child(camera)

	# Aimed after it is in the tree: look_at needs a global transform, and a
	# node that has not been added yet does not have one.
	camera.look_at_from_position(Vector3(0.0, 0.42, 3.75), Vector3.ZERO, Vector3.UP)


func _build_environment() -> void:
	var world := WorldEnvironment.new()
	var env := Environment.new()

	# The window is transparent so the page behind shows through: this sits over
	# a card in the dashboard, not on a background of its own.
	env.background_mode = Environment.BG_COLOR
	env.background_color = Color(0, 0, 0, 0)

	# Glow is the whole reason for doing this on the card. The 2D version faked
	# it with layered strokes, because a real blur cost more than everything
	# else on the page put together.
	env.glow_enabled = true
	env.glow_intensity = 1.5
	env.glow_bloom = 0.5
	env.glow_blend_mode = Environment.GLOW_BLEND_MODE_ADDITIVE
	env.glow_hdr_threshold = 0.4

	# Which blur levels contribute. Without any of these set, glow is enabled
	# and does nothing, which is how it looked on the first run.
	env.set_glow_level(1, 0.9)
	env.set_glow_level(2, 1.0)
	env.set_glow_level(3, 0.7)
	env.set_glow_level(4, 0.4)

	world.environment = env
	add_child(world)


## The halo.
##
## A billboard behind the sphere with a radial fall-off painted into it. The
## glow pass alone does not produce this — it brightens what is already drawn,
## and the space around the sphere has nothing in it to brighten.
func _build_halo() -> void:
	var gradient := Gradient.new()

	# Hollow in the middle. A halo that is brightest at its centre sits over the
	# sphere and washes the mesh out; what is wanted is light around it.
	gradient.offsets = PackedFloat32Array([0.0, 0.34, 0.52, 0.72, 1.0])
	gradient.colors = PackedColorArray([
		Color(0.10, 0.35, 0.7, 0.0),
		Color(0.14, 0.42, 0.8, 0.10),
		Color(0.28, 0.72, 1.0, 0.42),
		Color(0.18, 0.5, 0.9, 0.14),
		Color(0.05, 0.15, 0.4, 0.0),
	])

	var texture := GradientTexture2D.new()

	texture.gradient = gradient
	texture.fill = GradientTexture2D.FILL_RADIAL
	texture.fill_from = Vector2(0.5, 0.5)
	texture.fill_to = Vector2(1.0, 0.5)
	texture.width = 256
	texture.height = 256

	var halo := Sprite3D.new()

	halo.texture = texture
	halo.billboard = BaseMaterial3D.BILLBOARD_ENABLED
	halo.shaded = false
	halo.transparent = true
	halo.render_priority = -1
	halo.pixel_size = 0.026
	halo.modulate = Color(1, 1, 1, 0.9)
	halo.position = Vector3(0, 0, -1.6)

	add_child(halo)


## The floor: dotted ellipses under the sphere.
##
## Ornament, and the piece that stops the sphere floating in nothing. Dots
## rather than lines, because a solid ellipse under a wireframe reads as a
## shadow and a dotted one reads as a grid.
func _build_floor() -> void:
	var dot := SphereMesh.new()

	dot.radius = 0.011
	dot.height = 0.022
	dot.radial_segments = 5
	dot.rings = 2

	var material := StandardMaterial3D.new()

	material.shading_mode = BaseMaterial3D.SHADING_MODE_UNSHADED
	material.transparency = BaseMaterial3D.TRANSPARENCY_ALPHA
	material.albedo_color = Color(0.4, 0.85, 1.0, 0.55)
	material.emission_enabled = true
	material.emission = Color(0.35, 0.8, 1.0)
	material.emission_energy_multiplier = 2.0
	dot.material = material

	var rings := 5
	var per_ring := 96
	var multi := MultiMesh.new()

	multi.transform_format = MultiMesh.TRANSFORM_3D
	multi.mesh = dot
	multi.instance_count = rings * per_ring

	var index := 0

	for ring in rings:
		var scale_out := 0.95 + ring * 0.22

		for step in per_ring:
			var a := (float(step) / per_ring) * TAU

			multi.set_instance_transform(index, Transform3D(Basis(), Vector3(
				cos(a) * scale_out * 1.45,
				-1.12,
				sin(a) * scale_out * 0.9)))
			index += 1

	var floor_dots := MultiMeshInstance3D.new()

	floor_dots.multimesh = multi
	add_child(floor_dots)


## The surface: points spread evenly over a sphere.
##
## Placed by the golden angle in longitude against an even spread in the sine of
## latitude, which is what distributes points evenly instead of crowding them at
## the poles. One MultiMesh, so nine hundred points cost one draw call.
func _build_surface() -> void:
	var mesh := SphereMesh.new()

	mesh.radius = 0.008
	mesh.height = 0.016
	mesh.radial_segments = 6
	mesh.rings = 3

	var material := StandardMaterial3D.new()

	material.shading_mode = BaseMaterial3D.SHADING_MODE_UNSHADED
	material.albedo_color = Color(0.72, 0.95, 1.0)
	material.emission_enabled = true
	material.emission = Color(0.38, 0.85, 1.0)
	material.emission_energy_multiplier = 3.2
	mesh.material = material

	var multi := MultiMesh.new()

	multi.transform_format = MultiMesh.TRANSFORM_3D
	multi.use_colors = true
	multi.mesh = mesh
	multi.instance_count = SURFACE_POINTS

	var golden := PI * (3.0 - sqrt(5.0))

	for i in SURFACE_POINTS:
		var y := 1.0 - (float(i) / float(SURFACE_POINTS - 1)) * 2.0
		var ring := sqrt(max(0.0, 1.0 - y * y))
		var angle := golden * i

		var at := Vector3(cos(angle) * ring, y, sin(angle) * ring) * RADIUS

		multi.set_instance_transform(i, Transform3D(Basis(), at))
		multi.set_instance_color(i, Color(1, 1, 1, 1))

	_points = MultiMeshInstance3D.new()
	_points.multimesh = multi
	_globe.add_child(_points)


## Latitude and longitude lines, as a single line mesh.
func _build_wireframe() -> void:
	var mesh := ImmediateMesh.new()
	var material := StandardMaterial3D.new()

	material.shading_mode = BaseMaterial3D.SHADING_MODE_UNSHADED
	material.transparency = BaseMaterial3D.TRANSPARENCY_ALPHA
	material.albedo_color = Color(0.42, 0.88, 1.0, 0.42)
	material.emission_enabled = true
	material.emission = Color(0.3, 0.75, 0.95)
	material.emission_energy_multiplier = 1.8

	mesh.surface_begin(Mesh.PRIMITIVE_LINES, material)

	var steps := 72

	for lat_index in range(1, LATITUDES):
		var lat: float = (float(lat_index) / LATITUDES) * PI - PI / 2.0
		var y := sin(lat) * RADIUS
		var ring := cos(lat) * RADIUS

		for step in steps:
			var a := (float(step) / steps) * TAU
			var b := (float(step + 1) / steps) * TAU

			mesh.surface_add_vertex(Vector3(cos(a) * ring, y, sin(a) * ring))
			mesh.surface_add_vertex(Vector3(cos(b) * ring, y, sin(b) * ring))

	for lon_index in LONGITUDES:
		var lon: float = (float(lon_index) / LONGITUDES) * PI

		for step in steps:
			var a := (float(step) / steps) * TAU
			var b := (float(step + 1) / steps) * TAU

			mesh.surface_add_vertex(Vector3(cos(a) * sin(lon), sin(a), cos(a) * cos(lon)) * RADIUS)
			mesh.surface_add_vertex(Vector3(cos(b) * sin(lon), sin(b), cos(b) * cos(lon)) * RADIUS)

	mesh.surface_end()

	_wire = MeshInstance3D.new()
	_wire.mesh = mesh
	_globe.add_child(_wire)


## Four orbits, wide and nearly flat, each leaning its own way.
##
## These do not turn with the globe: they are around it, not on it.
func _build_orbits() -> void:
	_orbits = Node3D.new()
	add_child(_orbits)

	# The phases are set apart deliberately. Without them all four points begin
	# at the same angle and, because the periods are minutes long, spend the
	# first several minutes bunched in one corner — which is exactly what it
	# looked like.
	var shapes := [
		{"rx": 2.05, "rz": 1.35, "lean": -0.26, "seconds": 300.0, "phase": 0.0},
		{"rx": 1.80, "rz": 1.62, "lean": 0.34, "seconds": -220.0, "phase": TAU * 0.25},
		{"rx": 2.20, "rz": 1.10, "lean": 0.12, "seconds": 380.0, "phase": TAU * 0.5},
		{"rx": 1.95, "rz": 1.48, "lean": -0.55, "seconds": -460.0, "phase": TAU * 0.75},
		{"rx": 2.30, "rz": 0.95, "lean": 0.52, "seconds": 520.0, "phase": TAU * 0.13},
		{"rx": 1.70, "rz": 1.70, "lean": -0.12, "seconds": -340.0, "phase": TAU * 0.62},
	]

	for shape in shapes:
		var ring := Node3D.new()

		ring.rotation = Vector3(PI / 2.0 + shape["lean"], 0.0, 0.0)
		ring.set_meta("seconds", shape["seconds"])
		# Each orbit's own radii, so the point running it follows that ellipse
		# rather than an average of all of them — which had all four points
		# bunched in the same corner.
		ring.set_meta("rx", shape["rx"])
		ring.set_meta("rz", shape["rz"])
		ring.set_meta("phase", shape["phase"])
		_orbits.add_child(ring)

		var mesh := ImmediateMesh.new()
		var material := StandardMaterial3D.new()

		material.shading_mode = BaseMaterial3D.SHADING_MODE_UNSHADED
		material.transparency = BaseMaterial3D.TRANSPARENCY_ALPHA
		material.albedo_color = Color(0.45, 0.88, 1.0, 0.42)
		material.emission_enabled = true
		material.emission = Color(0.35, 0.85, 1.0)

		mesh.surface_begin(Mesh.PRIMITIVE_LINES, material)

		var steps := 128

		for step in steps:
			var a := (float(step) / steps) * TAU
			var b := (float(step + 1) / steps) * TAU

			mesh.surface_add_vertex(Vector3(cos(a) * shape["rx"], 0.0, sin(a) * shape["rz"]))
			mesh.surface_add_vertex(Vector3(cos(b) * shape["rx"], 0.0, sin(b) * shape["rz"]))

		mesh.surface_end()

		var line := MeshInstance3D.new()

		line.mesh = mesh
		ring.add_child(line)

		# One bright point running each orbit.
		var spark := SphereMesh.new()

		spark.radius = 0.026
		spark.height = 0.052

		var glow := StandardMaterial3D.new()

		glow.shading_mode = BaseMaterial3D.SHADING_MODE_UNSHADED
		glow.albedo_color = Color(1, 1, 1)
		glow.emission_enabled = true
		glow.emission = Color(0.6, 0.95, 1.0)
		# Well above the glow threshold, which is what makes it bloom rather
		# than simply be a bright dot.
		glow.emission_energy_multiplier = 14.0
		spark.material = glow

		var point := MeshInstance3D.new()

		point.mesh = spark
		point.name = "Spark"
		ring.add_child(point)


func _process(delta: float) -> void:
	_timer += delta

	if _timer >= LEVEL_INTERVAL:
		_timer = 0.0
		_ask_level()

	# Eased, so a change of state is a swell rather than a jump.
	_level += (_target - _level) * min(1.0, delta * 9.0)
	_recall = max(0.0, _recall - delta * 0.4)

	var seconds := Time.get_ticks_msec() / 1000.0

	_globe.rotation.y = seconds * TAU / SECONDS_PER_TURN

	for ring in _orbits.get_children():
		var period: float = ring.get_meta("seconds")
		var angle: float = seconds * TAU / period + ring.get_meta("phase")
		var spark: Node3D = ring.get_node("Spark")

		spark.position = Vector3(
			cos(angle) * ring.get_meta("rx"),
			0.0,
			sin(angle) * ring.get_meta("rz"))

	_apply_brightness()


## Brightness follows sound that is really there.
func _apply_brightness() -> void:
	var lit := 0.75 + _level * 2.4 + _recall * 0.8

	var tint := Color(0.45, 1.0, 0.62) if _speaking else Color(0.55, 0.9, 1.0)

	var surface := _points.multimesh.mesh.material as StandardMaterial3D

	surface.emission = tint
	surface.emission_energy_multiplier = 2.0 + lit * 1.8

	var wire := _wire.mesh.surface_get_material(0) as StandardMaterial3D

	if wire != null:
		wire.emission = tint
		wire.albedo_color = Color(tint.r, tint.g, tint.b, 0.22 + _level * 0.3)


func _ask_level() -> void:
	if _http.get_http_client_status() != HTTPClient.STATUS_DISCONNECTED:
		return

	_http.request(API + "/api/level")


func _on_level(_result: int, code: int, _headers: PackedStringArray, body: PackedByteArray) -> void:
	if code != 200:
		return

	var parsed = JSON.parse_string(body.get_string_from_utf8())

	if typeof(parsed) != TYPE_DICTIONARY:
		return

	var source: String = parsed.get("source", "")
	var level: float = parsed.get("level", 0.0)
	var floor_level: float = parsed.get("floor", 0.0)

	# The room's own noise is subtracted, using the floor the brain measured for
	# this room rather than a guessed one. Without it the globe would react to a
	# fan as though somebody were talking.
	var audible := 0.0

	if source != "" and level > floor_level:
		audible = (level - floor_level) / max(0.05, 1.0 - floor_level)

	_speaking = source == "voice"
	_target = audible
