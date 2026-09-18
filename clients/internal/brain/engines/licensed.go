package engines

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/provision"
)

/*
 * Unity and Unreal: engines its owner installs and licenses.
 *
 * This program never installs either and never accepts their terms — see the
 * provision recipes. Everything else is the same contract: a project is found
 * and read, a new one can be laid out, the editor is driven from its command
 * line in batch mode with no window, and what it printed is read back into
 * errors with a file and a line.
 */

// HowLongALicensedBuildMayTake bounds a Unity or Unreal build: they are the
// slowest things this program will run, and still must end.
const HowLongALicensedBuildMayTake = 90 * time.Minute

// Unity is the Unity adapter.
type Unity struct {
	Find func() (path, version string, ok bool)

	// Licence says whether the editor may work unattended; provision's probe
	// unless a test says. See provision.UnityLicence.
	Licence func(path string) (state, why string)
}

func (Unity) ID() string     { return "unity" }
func (Unity) Title() string  { return "Unity" }
func (Unity) Recipe() string { return "unity" }
func (Unity) Licensed() bool { return true }

func (u Unity) Engine() (Installed, bool) {
	find := u.Find
	if find == nil {
		find = provision.FindUnity
	}

	path, version, ok := find()

	return Installed{Path: path, Version: version}, ok
}

var unityVersion = regexp.MustCompile(`(?m)^m_EditorVersion:\s*(\S+)`)

func (Unity) Detect(dir string) (Project, bool) {
	stamp := read(filepath.Join(dir, "ProjectSettings", "ProjectVersion.txt"))

	if stamp == "" || !exists(filepath.Join(dir, "Assets")) {
		return Project{}, false
	}

	p := Project{Engine: "unity", Dir: dir, Name: filepath.Base(dir)}

	if m := unityVersion.FindStringSubmatch(stamp); m != nil {
		p.Targets = m[1]
	}

	return p, true
}

func (Unity) Docs(version, topic string) []Doc {
	v := majorMinor(version)
	if v == "" {
		v = "Manual"
	}

	base := "https://docs.unity3d.com/" + v + "/Documentation/ScriptReference/"

	var out []Doc

	if topic != "" {
		out = append(out, Doc{Title: topic + " in the Unity " + v + " scripting reference", URL: base + topic + ".html"})
	}

	return append(out, Doc{Title: "Unity " + v + " scripting reference", URL: base + "index.html"})
}

// Scaffold lays a project out the way the editor expects, stamped with the
// installed editor's version. The editor makes everything else on first open.
func (u Unity) Scaffold(s Scaffold) ([]File, error) {
	version := s.Engine
	if version == "" {
		if engine, ok := u.Engine(); ok {
			version = engine.Version
		}
	}

	if version == "" {
		return nil, fmt.Errorf("a Unity project is stamped with the editor that opens it, and no editor is installed")
	}

	name := className(s.Name)

	game := fmt.Sprintf(`using UnityEngine;

// %[1]s starts here.
public class %[1]s : MonoBehaviour
{
    void Start()
    {
        Debug.Log("%[1]s started");
    }

    void Update()
    {
    }
}
`, name)

	build := `using System.IO;
using UnityEditor;
using UnityEditor.SceneManagement;
using UnityEngine;

// Called from the command line: -executeMethod Build.Linux -buildOutput <path>
public static class Build
{
    // The main scene, made when there is none: a camera, a light, and every
    // behaviour this project defines on one object, so the game's own code
    // runs when the player starts.
    public static void EnsureScene()
    {
        if (EditorBuildSettings.scenes.Length > 0) return;

        var scene = EditorSceneManager.NewScene(NewSceneSetup.DefaultGameObjects, NewSceneMode.Single);
        var game = new GameObject("Game");

        foreach (var type in TypeCache.GetTypesDerivedFrom<MonoBehaviour>())
        {
            if (!type.IsAbstract && type.Assembly.GetName().Name == "Assembly-CSharp")
            {
                game.AddComponent(type);
            }
        }

        Directory.CreateDirectory("Assets/Scenes");
        EditorSceneManager.SaveScene(scene, "Assets/Scenes/Main.unity");
        EditorBuildSettings.scenes = new[] { new EditorBuildSettingsScene("Assets/Scenes/Main.unity", true) };
    }

    public static void Linux()
    {
        EnsureScene();

        var args = System.Environment.GetCommandLineArgs();
        var output = "Builds/Linux/game.x86_64";

        for (int i = 0; i < args.Length - 1; i++)
        {
            if (args[i] == "-buildOutput") output = args[i + 1];
        }

        var report = BuildPipeline.BuildPlayer(EditorBuildSettings.scenes, output,
            BuildTarget.StandaloneLinux64, BuildOptions.None);

        if (report.summary.result != UnityEditor.Build.Reporting.BuildResult.Succeeded)
        {
            EditorApplication.Exit(1);
        }
    }
}
`

	return []File{
		{Path: "ProjectSettings/ProjectVersion.txt", Content: []byte("m_EditorVersion: " + version + "\n")},
		{Path: "Packages/manifest.json", Content: []byte(unityManifest)},
		{Path: "Assets/Scripts/" + name + ".cs", Content: []byte(game)},
		{Path: "Assets/Editor/Build.cs", Content: []byte(build)},
		{Path: ".gitignore", Content: []byte("Library/\nTemp/\nObj/\nLogs/\nUserSettings/\nBuilds/\n*.csproj\n*.sln\n")},
	}, nil
}

var monoBehaviour = regexp.MustCompile(`class\s+(\w+)\s*:\s*MonoBehaviour`)

func (u Unity) Validate(p Project) []Diagnostic {
	var found []Diagnostic

	if engine, ok := u.Engine(); ok && p.Targets != "" && engine.Version != "" && engine.Version != p.Targets {
		severity := "warning"

		if provision.Newer(majorMinor(engine.Version), majorMinor(p.Targets)) {
			severity = "error"
		}

		found = append(found, Diagnostic{Severity: severity, File: "ProjectSettings/ProjectVersion.txt",
			Message: fmt.Sprintf("made with Unity %s, and %s is installed", p.Targets, engine.Version)})
	}

	// Unity only finds a behaviour whose class is named as its file.
	filepath.WalkDir(filepath.Join(p.Dir, "Assets"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".cs" {
			return nil
		}

		if m := monoBehaviour.FindStringSubmatch(read(path)); m != nil &&
			m[1] != strings.TrimSuffix(filepath.Base(path), ".cs") {
			found = append(found, Diagnostic{Severity: "error", File: relative(p.Dir, path),
				Message: "the behaviour " + m[1] + " is in a file with another name, so Unity cannot attach it"})
		}

		return nil
	})

	return found
}

/*
 * unlicensed is the Run for an editor whose licence will not let it work
 * unattended — said before starting it for the work, rather than after a
 * minute of it failing, and in the same words for a check, a build and a run.
 */
func (u Unity) unlicensed(engine Installed, what string) (Run, bool) {
	if u.Licence == nil {
		return Run{}, false
	}

	state, why := u.Licence(engine.Path)
	if state != provision.LicenceInactive {
		return Run{}, false
	}

	return Run{What: what, Engine: "unity", Version: engine.Version,
		Problem: "the Unity editor is installed but not licensed to work unattended: " + why}, true
}

// licenceProblem is what a run's output says about the licence, if anything.
func licenceProblem(out string) string {
	for _, mark := range []string{"No valid Unity Editor license", "'com.unity.editor.headless' was not found"} {
		if strings.Contains(out, mark) {
			return "the Unity editor has no licence to work unattended (" + mark + ") — " +
				"its owner signs in to Unity Hub, which renews it"
		}
	}

	return ""
}

// Check opens the project in batch mode and quits, which compiles its scripts.
func (u Unity) Check(ctx context.Context, p Project) Run {
	engine, ok := u.Engine()
	if !ok {
		return notInstalled(u, "check")
	}

	if r, refused := u.unlicensed(engine, "check"); refused {
		return r
	}

	r := Run{What: "check", Engine: "unity", Version: engine.Version}
	r.Diagnostics = u.Validate(p)

	out, exited := r.step(ctx, p.Dir, HowLongALicensedBuildMayTake, engine.Path,
		"-batchmode", "-nographics", "-quit", "-projectPath", p.Dir, "-logFile", "-")
	r.Diagnostics = append(r.Diagnostics, csharpDiagnostics(out, p.Dir)...)

	if why := licenceProblem(out); why != "" {
		r.Problem = why
	}

	r.withErrors(exited)

	return r
}

// Build makes a Linux player with the project's Build script.
func (u Unity) Build(ctx context.Context, p Project, into string) Run {
	engine, ok := u.Engine()
	if !ok {
		return notInstalled(u, "build")
	}

	if r, refused := u.unlicensed(engine, "build"); refused {
		return r
	}

	r := Run{What: "build", Engine: "unity", Version: engine.Version}

	if !exists(filepath.Join(p.Dir, "Assets", "Editor", "Build.cs")) {
		r.Problem = "the project has no Assets/Editor/Build.cs to build with"

		return r
	}

	target := filepath.Join(into, safeName(p.Name)+".x86_64")

	out, exited := r.step(ctx, p.Dir, HowLongALicensedBuildMayTake, engine.Path,
		"-batchmode", "-nographics", "-quit", "-projectPath", p.Dir,
		"-executeMethod", "Build.Linux", "-buildOutput", target, "-logFile", "-")
	r.Diagnostics = csharpDiagnostics(out, p.Dir)

	if why := licenceProblem(out); why != "" {
		r.Problem = why
	}

	if exists(target) {
		r.Artifacts = append(r.Artifacts, target)
	} else {
		exited = false
	}

	r.withErrors(exited)

	return r
}

// Smoke runs a built player with no graphics for a few seconds: surviving is
// the pass. A Unity player has no way to be photographed without a display.
func (u Unity) Smoke(ctx context.Context, p Project, pictures string) Run {
	r := Run{What: "smoke", Engine: "unity"}

	players, _ := filepath.Glob(filepath.Join(p.Dir, "Builds", "*", "*.x86_64"))
	if len(players) == 0 {
		players, _ = filepath.Glob(filepath.Join(p.Dir, "Builds", "*.x86_64"))
	}

	if len(players) == 0 {
		r.Problem = "there is no built player to run — build it first"

		return r
	}

	out, _ := r.step(ctx, p.Dir, 15*time.Second, players[0], "-batchmode", "-nographics", "-logFile", "-")

	// Still running when stopped is the pass: a game does not end by itself.
	survived := strings.HasPrefix(r.Problem, "still running")
	r.Problem = ""
	r.Diagnostics = append(csharpDiagnostics(out, p.Dir), unityExceptions(out, p.Dir)...)
	r.Notes = append(r.Notes, "no picture: the player ran with no graphics, which is how it runs unattended")
	r.withErrors(survived || r.Exit == 0)

	return r
}

// Unreal is the Unreal Engine adapter.
type Unreal struct {
	Find func() (path, version string, ok bool)
}

func (Unreal) ID() string     { return "unreal" }
func (Unreal) Title() string  { return "Unreal Engine" }
func (Unreal) Recipe() string { return "unreal" }
func (Unreal) Licensed() bool { return true }

func (u Unreal) Engine() (Installed, bool) {
	find := u.Find
	if find == nil {
		find = provision.FindUnreal
	}

	path, version, ok := find()

	return Installed{Path: path, Version: version}, ok
}

// engineRoot is the folder the engine lives in, from the editor's path.
func engineRoot(editor string) string {
	return filepath.Clean(filepath.Join(filepath.Dir(editor), "..", "..", ".."))
}

func (Unreal) Detect(dir string) (Project, bool) {
	matches, _ := filepath.Glob(filepath.Join(dir, "*.uproject"))
	if len(matches) == 0 {
		return Project{}, false
	}

	p := Project{Engine: "unreal", Dir: dir, Name: strings.TrimSuffix(filepath.Base(matches[0]), ".uproject"),
		Details: map[string]string{"uproject": matches[0]}}

	var manifest struct {
		EngineAssociation string
	}

	if json.Unmarshal([]byte(read(matches[0])), &manifest) == nil {
		p.Targets = manifest.EngineAssociation
	}

	return p, true
}

func (Unreal) Docs(version, topic string) []Doc {
	v := majorMinor(version)

	query := ""
	if v != "" {
		query = "?application_version=" + v
	}

	var out []Doc

	if topic != "" {
		out = append(out, Doc{Title: topic + " in the Unreal Engine API",
			URL: "https://dev.epicgames.com/community/search?query=" + topic})
	}

	return append(out, Doc{Title: "Unreal Engine " + v + " API",
		URL: "https://dev.epicgames.com/documentation/en-us/unreal-engine/API" + query})
}

// Scaffold lays out a C++ project: the .uproject, one game module and its two
// build targets. The engine generates everything else when it first builds.
func (u Unreal) Scaffold(s Scaffold) ([]File, error) {
	version := majorMinor(s.Engine)
	if version == "" {
		if engine, ok := u.Engine(); ok {
			version = majorMinor(engine.Version)
		}
	}

	if version == "" {
		version = "5.4"
	}

	name := className(s.Name)

	uproject := fmt.Sprintf(`{
	"FileVersion": 3,
	"EngineAssociation": %q,
	"Category": "",
	"Description": "",
	"Modules": [
		{
			"Name": %q,
			"Type": "Runtime",
			"LoadingPhase": "Default"
		}
	]
}
`, version, name)

	target := func(suffix, kind string) string {
		return fmt.Sprintf(`using UnrealBuildTool;

public class %[1]s%[2]sTarget : TargetRules
{
	public %[1]s%[2]sTarget(TargetInfo Target) : base(Target)
	{
		Type = TargetType.%[3]s;
		DefaultBuildSettings = BuildSettingsVersion.Latest;
		IncludeOrderVersion = EngineIncludeOrderVersion.Latest;
		ExtraModuleNames.Add("%[1]s");
	}
}
`, name, suffix, kind)
	}

	build := fmt.Sprintf(`using UnrealBuildTool;

public class %[1]s : ModuleRules
{
	public %[1]s(ReadOnlyTargetRules Target) : base(Target)
	{
		PCHUsage = PCHUsageMode.UseExplicitOrSharedPCHs;
		PublicDependencyModuleNames.AddRange(new string[] { "Core", "CoreUObject", "Engine", "InputCore" });
	}
}
`, name)

	header := fmt.Sprintf("#pragma once\n\n#include \"CoreMinimal.h\"\n")
	source := fmt.Sprintf("#include \"%[1]s.h\"\n#include \"Modules/ModuleManager.h\"\n\nIMPLEMENT_PRIMARY_GAME_MODULE(FDefaultGameModuleImpl, %[1]s, \"%[1]s\");\n", name)

	return []File{
		{Path: name + ".uproject", Content: []byte(uproject)},
		{Path: "Source/" + name + ".Target.cs", Content: []byte(target("", "Game"))},
		{Path: "Source/" + name + "Editor.Target.cs", Content: []byte(target("Editor", "Editor"))},
		{Path: "Source/" + name + "/" + name + ".Build.cs", Content: []byte(build)},
		{Path: "Source/" + name + "/" + name + ".h", Content: []byte(header)},
		{Path: "Source/" + name + "/" + name + ".cpp", Content: []byte(source)},
		{Path: "Config/DefaultEngine.ini", Content: []byte("[/Script/EngineSettings.GameMapsSettings]\n")},
		{Path: ".gitignore", Content: []byte("Binaries/\nIntermediate/\nSaved/\nDerivedDataCache/\nPackaged/\n")},
	}, nil
}

func (u Unreal) Validate(p Project) []Diagnostic {
	var found []Diagnostic

	if engine, ok := u.Engine(); ok && p.Targets != "" && engine.Version != "" &&
		majorMinor(engine.Version) != majorMinor(p.Targets) {
		found = append(found, Diagnostic{Severity: "warning", File: filepath.Base(p.Details["uproject"]),
			Message: fmt.Sprintf("made for Unreal %s, and %s is installed", p.Targets, engine.Version)})
	}

	var manifest struct {
		Modules []struct{ Name string }
	}

	if json.Unmarshal([]byte(read(p.Details["uproject"])), &manifest) == nil {
		for _, m := range manifest.Modules {
			if !exists(filepath.Join(p.Dir, "Source", m.Name)) {
				found = append(found, Diagnostic{Severity: "error", File: filepath.Base(p.Details["uproject"]),
					Message: "the module " + m.Name + " has no folder under Source"})
			}
		}
	}

	return found
}

// Check compiles the editor target, which is what opening the project needs.
func (u Unreal) Check(ctx context.Context, p Project) Run {
	engine, ok := u.Engine()
	if !ok {
		return notInstalled(u, "check")
	}

	r := Run{What: "check", Engine: "unreal", Version: engine.Version}
	r.Diagnostics = u.Validate(p)

	script := filepath.Join(engineRoot(engine.Path), "Engine", "Build", "BatchFiles", "Linux", "Build.sh")

	out, exited := r.step(ctx, p.Dir, HowLongALicensedBuildMayTake, script, p.Name+"Editor", "Linux",
		"Development", "-Project="+p.Details["uproject"], "-WaitMutex", "-NoHotReload")
	r.Diagnostics = append(r.Diagnostics, clangDiagnostics(out, p.Dir)...)
	r.withErrors(exited)

	return r
}

// Build cooks and packages a Linux game into the folder given.
func (u Unreal) Build(ctx context.Context, p Project, into string) Run {
	engine, ok := u.Engine()
	if !ok {
		return notInstalled(u, "build")
	}

	r := Run{What: "build", Engine: "unreal", Version: engine.Version}

	uat := filepath.Join(engineRoot(engine.Path), "Engine", "Build", "BatchFiles", "RunUAT.sh")

	out, exited := r.step(ctx, p.Dir, HowLongALicensedBuildMayTake, uat, "BuildCookRun",
		"-project="+p.Details["uproject"], "-platform=Linux", "-clientconfig=Shipping",
		"-build", "-cook", "-stage", "-pak", "-archive", "-archivedirectory="+into,
		"-unattended", "-utf8output", "-nop4")
	r.Diagnostics = clangDiagnostics(out, p.Dir)

	if exists(filepath.Join(into, "Linux")) {
		r.Artifacts = append(r.Artifacts, filepath.Join(into, "Linux"))
	} else {
		exited = false
	}

	r.withErrors(exited)

	return r
}

// Smoke runs the packaged game with no renderer for a few seconds.
func (u Unreal) Smoke(ctx context.Context, p Project, pictures string) Run {
	r := Run{What: "smoke", Engine: "unreal"}

	games, _ := filepath.Glob(filepath.Join(p.Dir, "Packaged", "Linux", "*.sh"))
	if len(games) == 0 {
		r.Problem = "there is no packaged game to run — build it first"

		return r
	}

	r.step(ctx, p.Dir, 20*time.Second, games[0], "-nullrhi", "-unattended", "-nosplash", "-nosound")

	survived := strings.HasPrefix(r.Problem, "still running")
	r.Problem = ""
	r.Notes = append(r.Notes, "no picture: the game ran without a renderer, which is what running it unattended means")
	r.withErrors(survived || r.Exit == 0)

	return r
}

/*
 * unityManifest is the built-in modules a small game uses — physics, 2D
 * physics, audio, UI, animation — each of which ships inside the editor, so
 * nothing is downloaded to open the project.
 */
const unityManifest = `{
  "dependencies": {
    "com.unity.modules.animation": "1.0.0",
    "com.unity.modules.audio": "1.0.0",
    "com.unity.modules.imgui": "1.0.0",
    "com.unity.modules.jsonserialize": "1.0.0",
    "com.unity.modules.physics": "1.0.0",
    "com.unity.modules.physics2d": "1.0.0",
    "com.unity.modules.ui": "1.0.0",
    "com.unity.modules.uielements": "1.0.0"
  }
}
`

var (
	unityException = regexp.MustCompile(`(?m)^(\w*Exception): (.+)$`)
	unityAt        = regexp.MustCompile(`in (.+?\.cs):(\d+)`)
)

// unityExceptions is what a player threw while it ran, each with the script
// and line it came from when the log says.
func unityExceptions(out, root string) []Diagnostic {
	var found []Diagnostic

	seen := map[string]bool{}

	for _, loc := range unityException.FindAllStringSubmatchIndex(out, -1) {
		m := out[loc[0]:loc[1]]
		parts := unityException.FindStringSubmatch(m)

		d := Diagnostic{Severity: "error", Message: parts[1] + ": " + strings.TrimSpace(parts[2])}

		following := out[loc[1]:min(len(out), loc[1]+600)]
		if at := unityAt.FindStringSubmatch(following); at != nil {
			d.File = relative(root, at[1])
			d.Line, _ = strconv.Atoi(at[2])
		}

		if key := d.String(); !seen[key] {
			seen[key] = true
			found = append(found, d)
		}
	}

	return found
}
