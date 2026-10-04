package orchestrator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/catalogue"
	"pn-scripts-assistant/internal/brain/models"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Local models, judged by what they say about themselves.
 *
 * Ollama answers /api/show with a model's capabilities — tools, vision,
 * thinking, fill-in-the-middle — its size in parameters, its context and its
 * licence. That, not the name, is what decides whether a model can do a
 * piece of work: "insert" is what code models are built for, and a model
 * without "tools" cannot work on files through this program's tools however
 * it is called.
 */

var httpClient = &http.Client{Timeout: 30 * time.Second}

type shown struct {
	Capabilities []string `json:"capabilities"`
	License      string   `json:"license"`
	Details      struct {
		Family        string `json:"family"`
		ParameterSize string `json:"parameter_size"`
		Quantization  string `json:"quantization_level"`
	} `json:"details"`
	ModelInfo map[string]any `json:"model_info"`
}

// LocalModels is every model Ollama has, as resources.
func LocalModels(ctx context.Context, baseURL string, machine Machine) ([]Resource, error) {
	list, err := models.New(baseURL).List(ctx)
	if err != nil {
		return nil, err
	}

	var out []Resource

	for _, m := range list {
		info, err := show(ctx, baseURL, m.Name)
		if err != nil {
			continue
		}

		out = append(out, modelResource(m.Name, m.Bytes, info, machine))
	}

	return out, nil
}

func show(ctx context.Context, baseURL, name string) (shown, error) {
	var s shown

	body, _ := json.Marshal(map[string]string{"model": name})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/show", bytes.NewReader(body))
	if err != nil {
		return s, err
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return s, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return s, fmt.Errorf("ollama said %s about %s", res.Status, name)
	}

	return s, json.NewDecoder(res.Body).Decode(&s)
}

var billions = regexp.MustCompile(`([\d.]+)\s*([BbMm])`)

// params is a model's size in billions of parameters, from "7.6B" or "137M".
func params(size string) float64 {
	m := billions.FindStringSubmatch(size)
	if m == nil {
		return 0
	}

	n, _ := strconv.ParseFloat(m[1], 64)

	if strings.EqualFold(m[2], "m") {
		return n / 1000
	}

	return n
}

// modelResource is a model as a resource, with how well it does each kind
// of work worked out from what it is.
func modelResource(name string, bytes int64, info shown, machine Machine) Resource {
	has := func(c string) bool {
		for _, v := range info.Capabilities {
			if v == c {
				return true
			}
		}

		return false
	}

	b := params(info.Details.ParameterSize)

	size := 0

	switch {
	case b >= 30:
		size = 4
	case b >= 13:
		size = 3
	case b >= 6:
		size = 2
	case b >= 3:
		size = 1
	}

	code := has("insert") // fill-in-the-middle is what a code model is built for

	can := map[string]int{}

	if has("embedding") && !has("completion") {
		can["embed"] = 5
	} else {
		bonus := func(yes bool, n int) int {
			if yes {
				return n
			}

			return 0
		}

		can[Chat] = min(7, 3+size)
		can[Write] = min(7, 2+size+bonus(!code, 1))
		can[Plan] = min(7, 1+size+bonus(has("thinking"), 2))
		can[Reason] = can[Plan]

		if has("tools") {
			can["tools"] = 1
			can[Code] = min(7, 1+size+bonus(code, 2))
			can[Edit] = min(7, can[Code]+1)
		}

		if has("vision") {
			can[Vision] = min(7, 3+size)
		}
	}

	r := Resource{ID: "ollama:" + name, Kind: Model, Title: name + " (local)", Model: name, Local: true,
		Tier: Local, State: Available, Evidence: Reported, Observed: time.Now(), Can: can, Size: bytes,
		Version: info.Details.Quantization, Licence: licenceName(info.License), Executes: true}

	r.Notes = append(r.Notes, fmt.Sprintf("%s parameters, %s, capabilities %s", orElse(info.Details.ParameterSize, "?"),
		orElse(info.Details.Family, "unknown family"), strings.Join(info.Capabilities, ", ")))

	if len(machine.GPU) == 0 || !strings.Contains(strings.ToLower(strings.Join(machine.GPU, " ")), "nvidia") {
		r.Notes = append(r.Notes, "runs on the processor here: expect a few words a second")
	}

	if machine.RAM > 0 && uint64(float64(bytes)*1.3) > machine.RAM {
		r.State, r.Why = NotSupported, "it needs more memory than this machine has"
	}

	return r
}

// licenceName is the name a licence text starts with.
func licenceName(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			if len(line) > 60 {
				line = line[:60] + "…"
			}

			return line
		}
	}

	return ""
}

// ModelPlan is a local model to install, checked before it is fetched.
type ModelPlan struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Digest  string `json:"digest"`
	Licence string `json:"licence"`
	Source  string `json:"source"`
	Why     string `json:"why"`

	// Auto is whether its owner's policy lets it be installed without
	// asking; Ask is what would be asked when not.
	Auto bool   `json:"auto"`
	Ask  string `json:"ask,omitempty"`
}

// Registry is Ollama's model registry, which a plan is checked against.
var Registry = "https://registry.ollama.ai"

/*
 * Manifest is a model's size, identity and licence, read from the registry
 * before anything is downloaded: the sum of its layers, the digest of its
 * manifest — which is what Ollama calls the model's id afterwards, so the
 * installed model can be checked to be the one that was planned — and the
 * first line of the licence it ships with.
 */
func Manifest(ctx context.Context, name string) (size int64, digest, licence string, err error) {
	model, tag, _ := strings.Cut(name, ":")
	if tag == "" {
		tag = "latest"
	}

	if !strings.Contains(model, "/") {
		model = "library/" + model
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Registry+"/v2/"+model+"/manifests/"+tag, nil)
	if err != nil {
		return 0, "", "", err
	}

	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")

	res, err := httpClient.Do(req)
	if err != nil {
		return 0, "", "", err
	}

	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return 0, "", "", err
	}

	if res.StatusCode != http.StatusOK {
		return 0, "", "", fmt.Errorf("the registry has no %s (%s)", name, res.Status)
	}

	var manifest struct {
		Config struct {
			Size int64 `json:"size"`
		} `json:"config"`
		Layers []struct {
			MediaType string `json:"mediaType"`
			Digest    string `json:"digest"`
			Size      int64  `json:"size"`
		} `json:"layers"`
	}

	if err := json.Unmarshal(raw, &manifest); err != nil {
		return 0, "", "", err
	}

	sum := sha256.Sum256(raw)
	digest = hex.EncodeToString(sum[:])
	size = manifest.Config.Size

	for _, l := range manifest.Layers {
		size += l.Size

		if l.MediaType == "application/vnd.ollama.image.license" && licence == "" {
			licence = blobText(ctx, model, l.Digest)
		}
	}

	return size, digest, licenceName(licence), nil
}

func blobText(ctx context.Context, model, digest string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Registry+"/v2/"+model+"/blobs/"+digest, nil)
	if err != nil {
		return ""
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return ""
	}

	defer res.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4096))

	return string(raw)
}

/*
 * PlanModel is the local model to install when nothing installed will do a
 * piece of work: from the library, the largest one that can do it and that
 * this machine and its owner's budget allow — on a processor without a
 * graphics card, no bigger than about eight billion parameters, because a
 * larger one is minutes a sentence.
 */
func PlanModel(ctx context.Context, need Need, library []catalogue.Model, machine Machine, pol Policy) (*ModelPlan, error) {
	type candidate struct {
		name  string
		b     float64
		score int
	}

	var found []candidate

	cpuOnly := !strings.Contains(strings.ToLower(strings.Join(machine.GPU, " ")), "nvidia")

	for _, m := range library {
		if m.Embedding() || !hasCan(m.Can, "tools") && (need.Files || need.Work == Code || need.Work == Edit) {
			continue
		}

		if need.Work == Vision && !hasCan(m.Can, "vision") {
			continue
		}

		if len(pol.Families) > 0 && !listed(pol.Families, familyOf(m.Name)) {
			continue
		}

		/*
		 * Made for code, or merely able to write some: a model named for code
		 * — qwen2.5-coder, codellama, deepseek-coder — is built for it, and
		 * one whose description mentions code among ten other things is not.
		 */
		name := strings.ToLower(m.Name)
		madeForCode := strings.Contains(name, "code") || strings.Contains(name, "coder")
		mentionsCode := strings.Contains(strings.ToLower(m.What), "code")

		for _, size := range m.Sizes {
			// The catalogue's own reading of its sizes: "8x7b" is every
			// expert, not the 7b at the end of it, and an "e2b" has no
			// number to go on and is passed over, as it always was.
			b := catalogue.Billions(size)
			if b <= 0 {
				continue
			}

			// What it would download — about 0.6 GB a billion parameters at
			// Ollama's usual four bits — against the download limit, and what
			// it would take to run against the memory here. The registry's
			// real size is checked before anything is fetched.
			download := int64(b * 0.62 * float64(gb))
			memory := int64(catalogue.MemoryFor(size) * float64(gb))

			if download > pol.Budget.MaxModelSize || download > pol.Budget.MaxDownload ||
				(machine.RAM > 0 && uint64(memory) > machine.RAM/2) {
				continue
			}

			if cpuOnly && b > 8.5 {
				continue
			}

			score := int(b * 10)
			if need.Work == Code || need.Work == Edit {
				switch {
				case madeForCode:
					score += 300
				case mentionsCode:
					score += 100
				}
			}

			found = append(found, candidate{name: m.Name + ":" + strings.ToLower(size), b: b, score: score})
		}
	}

	if len(found) == 0 {
		return nil, fmt.Errorf("no model in the library can do %s within this machine and the budget", workWords(need.Work))
	}

	sort.SliceStable(found, func(i, j int) bool { return found[i].score > found[j].score })

	var lastErr error

	for _, c := range found {
		size, digest, licence, err := Manifest(ctx, c.name)
		if err != nil {
			lastErr = err

			continue
		}

		p := &ModelPlan{Name: c.name, Size: size, Digest: digest, Licence: licence, Source: Registry,
			Why: fmt.Sprintf("the largest model this machine runs well for %s", workWords(need.Work))}

		free := machine.FreeFor("home")

		switch {
		case size > pol.Budget.MaxDownload:
			p.Ask = fmt.Sprintf("it is %s, more than the %s allowed without asking", sizeWords(size), sizeWords(pol.Budget.MaxDownload))
		case free >= 0 && free-size < pol.Budget.MinFreeDisk:
			p.Ask = fmt.Sprintf("it would leave %s free, less than the %s kept free", sizeWords(free-size), sizeWords(pol.Budget.MinFreeDisk))
		case !pol.Autonomy.InstallLocalModels:
			p.Ask = "installing models is asked about, by your policy"
		default:
			p.Auto = true
		}

		return p, nil
	}

	return nil, fmt.Errorf("the registry could not be read for any candidate: %v", lastErr)
}

func hasCan(list []string, c string) bool {
	for _, v := range list {
		if v == c {
			return true
		}
	}

	return false
}

func familyOf(name string) string {
	name, _, _ = strings.Cut(name, ":")

	return strings.TrimRight(name, "0123456789.-")
}

/*
 * InstallModel puts a planned model on this machine and proves it: the free
 * space before and after, the pull through Ollama (which checks every layer
 * against its digest), the installed model's id against the manifest's
 * digest, its capabilities, and one short answer from it. Every one of those
 * is evidence; the credentials that do not exist here are not.
 */
func InstallModel(ctx context.Context, baseURL string, p ModelPlan, progress func(string), note func(store.Evidence)) error {
	if note == nil {
		note = func(store.Evidence) {}
	}

	home, _ := os.UserHomeDir()
	where := filepath.Join(home, ".ollama")

	if env := os.Getenv("OLLAMA_MODELS"); env != "" {
		where = env
	}

	before, _ := freeAt(where)

	// Each as it happens, so the plan is on record before the download and
	// a failure part-way leaves what was done before it.
	note(store.Evidence{Kind: store.EvidenceInstall, Subject: p.Name,
		Detail: fmt.Sprintf("planned: %s from %s, manifest sha256 %s, licence %q, %s free before",
			sizeWords(p.Size), p.Source, p.Digest, p.Licence, sizeWords(before)), OK: true})

	client := models.New(baseURL)

	if err := client.Pull(ctx, p.Name, progress); err != nil {
		note(store.Evidence{Kind: store.EvidenceInstall, Subject: p.Name, Detail: "ollama pull " + p.Name + " failed: " + err.Error()})

		return err
	}

	after, _ := freeAt(where)

	note(store.Evidence{Kind: store.EvidenceCommand, Subject: "ollama pull " + p.Name,
		Detail: fmt.Sprintf("pulled; %s free after", sizeWords(after)), OK: true})

	// The id Ollama gives an installed model is its manifest's digest.
	id, err := installedID(ctx, baseURL, p.Name)
	switch {
	case err != nil:
		note(store.Evidence{Kind: store.EvidenceVersion, Subject: p.Name, Detail: "could not read its id: " + err.Error()})
	case p.Digest != "" && !strings.HasPrefix(p.Digest, id):
		note(store.Evidence{Kind: store.EvidenceVersion, Subject: p.Name,
			Detail: "its id " + id + " is not the planned manifest " + p.Digest[:12]})

		return fmt.Errorf("the installed %s is not the one that was planned", p.Name)
	default:
		note(store.Evidence{Kind: store.EvidenceVersion, Subject: p.Name, Detail: "id " + id + " matches the planned manifest", OK: true})
	}

	info, err := show(ctx, baseURL, p.Name)
	if err != nil {
		return err
	}

	note(store.Evidence{Kind: store.EvidenceCheck, Subject: p.Name + " capabilities",
		Detail: strings.Join(info.Capabilities, ", "), OK: len(info.Capabilities) > 0})

	answer, err := generate(ctx, baseURL, p.Name, "Reply with the single word: ready")
	ok := err == nil && strings.Contains(strings.ToLower(answer), "ready")

	note(store.Evidence{Kind: store.EvidenceSmoke, Subject: p.Name + " answers",
		Detail: firstLine(orElse(answer, fmt.Sprint(err))), OK: ok})

	if !ok {
		return fmt.Errorf("%s was installed and did not answer", p.Name)
	}

	return nil
}

func installedID(ctx context.Context, baseURL, name string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/tags", nil)
	if err != nil {
		return "", err
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()

	var tags struct {
		Models []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"models"`
	}

	if err := json.NewDecoder(res.Body).Decode(&tags); err != nil {
		return "", err
	}

	for _, m := range tags.Models {
		if m.Name == name || m.Name == name+":latest" {
			return m.Digest[:min(12, len(m.Digest))], nil
		}
	}

	return "", fmt.Errorf("%s is not listed", name)
}

func generate(ctx context.Context, baseURL, model, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]any{"model": model, "prompt": prompt, "stream": false,
		"options": map[string]any{"num_predict": 8}})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 5 * time.Minute}

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()

	var out struct {
		Response string `json:"response"`
	}

	return out.Response, json.NewDecoder(res.Body).Decode(&out)
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")

	return line
}
