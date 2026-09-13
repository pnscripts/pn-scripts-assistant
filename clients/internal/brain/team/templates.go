package team

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

/*
 * Templates are agents with no name yet.
 *
 * The specification asks for a starting set of sixteen — an executive, an
 * architect, engineers, a designer, a writer, a finance and a legal
 * specialist and the rest — and it asks for them as templates rather than as
 * a roster, which is the right call and for this program's own reason: the
 * roster is shown to the planner, and sixteen people who mostly differ in
 * name make its choice harder for nothing. So the six that ship stay six, and
 * these are a hire away.
 *
 * Each names a job from the taxonomy, a tool list arrived at the way the
 * built-in ones were — by what the work needs, and nothing it could be talked
 * into — a model size, and what it must never do. The job supplies what it
 * knows; the template supplies what it may touch.
 *
 * And its owner's own, from a templates folder beside the agents one, in the
 * same file format. A template file naming a built-in one replaces it.
 */

// TemplatesFolderName is where somebody's own templates live.
const TemplatesFolderName = "templates"

// Template is the shape of an agent before it has a name. Name is the
// template's own identifier, not the agent's.
type Template = Agent

var (
	codeTools = []string{"read_file", "write_file", "edit_file", "search_files",
		"list_directory", "run_command", "read_document"}

	readingTools = []string{"read_file", "search_files", "list_directory", "read_document"}

	desk = []string{"read_document", "write_document", "write_file", "read_file",
		"list_directory", "what_you_know"}

	web = []string{"web_search", "fetch_url", "read_a_page"}
)

func joined(lists ...[]string) []string {
	out := []string{}
	seen := map[string]bool{}

	for _, list := range lists {
		for _, one := range list {
			if !seen[one] {
				seen[one] = true
				out = append(out, one)
			}
		}
	}

	return out
}

// BuiltInTemplates is the starting set the specification names.
func BuiltInTemplates() []Template {
	return []Template{
		{Name: "executive", Title: "Executive", Job: "leadership.chief_executive", Uses: UsesBest,
			For:   "deciding between options, setting priorities and saying what matters most",
			Tools: []string{"read_document", "write_document", "what_you_know"},
			Never: []string{"send_email", "run_command"},
			Brief: "Decide, and say why in two sentences. Name what would change your mind."},
		{Name: "software_architect", Title: "Software architect", Job: "software.software_architect",
			Uses: UsesBest, For: "how a system should be put together, and reviewing whether it is",
			Tools: joined(readingTools, []string{"write_document", "write_file"}),
			Brief: "Draw the boundaries before the details. Say what each choice costs."},
		{Name: "backend_engineer", Title: "Backend engineer", Job: "software.backend_engineer",
			Uses: UsesWork, For: "servers, APIs, databases and the code behind them", Tools: codeTools},
		{Name: "frontend_engineer", Title: "Frontend engineer", Job: "software.frontend_engineer",
			Uses: UsesWork, For: "what runs in the browser: pages, components, styles and scripts",
			Tools: codeTools},
		{Name: "devops_engineer", Title: "DevOps engineer", Job: "software.devops_engineer",
			Uses: UsesWork, For: "building, deploying and keeping services running", Tools: codeTools,
			Brief: "Change staging before anything live, and say which one a command touches."},
		{Name: "security_engineer", Title: "Security engineer", Job: "security.security_engineer",
			Uses: UsesBest, For: "finding what could be attacked and how to close it",
			Tools: joined(readingTools, []string{"fetch_url", "web_search"}),
			Brief: "Report what you find; do not exploit it. Rank by how bad, then how likely."},
		{Name: "qa_engineer", Title: "QA engineer", Job: "software.qa_engineer", Uses: UsesWork,
			For:   "testing whether something works, and finding how it breaks",
			Tools: joined(readingTools, []string{"run_command"})},
		{Name: "ai_engineer", Title: "AI engineer", Job: "ai.ai_engineer", Uses: UsesWork,
			For:   "models, prompts, embeddings and the code that uses them",
			Tools: joined(codeTools, []string{"list_models"})},
		{Name: "product_manager", Title: "Product manager", Job: "product.product_manager",
			Uses: UsesBest, For: "what to build, for whom, and in what order", Tools: joined(desk, web)},
		{Name: "ux_designer", Title: "UX designer", Job: "design.ux_designer", Uses: UsesBest,
			For:   "how something is used: flows, layouts and what a person sees first",
			Tools: []string{"make_a_picture", "read_document", "write_document", "read_file", "list_directory"}},
		{Name: "business_analyst", Title: "Business analyst", Job: "product.business_analyst",
			Uses: UsesWork, For: "turning what somebody wants into requirements that can be checked",
			Tools: joined(desk, web)},
		{Name: "technical_writer", Title: "Technical writer", Job: "media.technical_writer",
			Uses: UsesBest, For: "documentation: guides, references and how-tos",
			Tools: joined(desk, []string{"search_files"})},
		{Name: "research_specialist", Title: "Researcher", Job: "general.researcher", Uses: UsesQuick,
			For:   "finding something out and saying where it came from",
			Tools: joined(web, []string{"read_document", "read_file", "search_files", "what_you_know"})},
		{Name: "finance_specialist", Title: "Finance specialist", Job: "finance.financial_analyst",
			Uses: UsesBest, For: "budgets, forecasts, costs and what the numbers say",
			Tools: []string{"read_document", "write_document", "read_file", "list_directory", "what_you_know"},
			Never: []string{"send_email"},
			Brief: "Show the working. A number without where it came from is not an answer."},
		{Name: "legal_specialist", Title: "Legal specialist", Job: "legal.corporate_lawyer",
			Uses: UsesBest, For: "contracts, obligations and what an agreement actually commits to",
			Tools: []string{"read_document", "write_document", "read_file", "web_search", "fetch_url"},
			Never: []string{"send_email"},
			Brief: "Say what the text commits to, and flag what needs a qualified lawyer. " +
				"Never present an opinion as legal advice."},
		{Name: "marketing_specialist", Title: "Marketing specialist", Job: "marketing.marketing_manager",
			Uses: UsesBest, For: "who something is for, how they find it, and what to tell them",
			Tools: joined(web, []string{"write_document", "write_file", "make_a_picture"})},
	}
}

// Templates is the built-in set with its owner's own added or replacing.
func Templates(root string) []Template {
	byName := map[string]Template{}
	order := []string{}

	for _, t := range BuiltInTemplates() {
		t.BuiltIn = true
		byName[t.Name] = t
		order = append(order, t.Name)
	}

	entries, _ := os.ReadDir(filepath.Join(root, TemplatesFolderName))

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(root, TemplatesFolderName, entry.Name()))
		if err != nil {
			continue
		}

		t := parse(string(raw), strings.TrimSuffix(entry.Name(), ".md"))

		if !validName.MatchString(t.Name) || t.For == "" {
			continue
		}

		if _, known := byName[t.Name]; !known {
			order = append(order, t.Name)
		}

		byName[t.Name] = t
	}

	out := make([]Template, 0, len(order))

	for _, name := range order {
		out = append(out, byName[name])
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].BuiltIn && !out[j].BuiltIn })

	return out
}

/*
 * FromTemplate is an agent made from a template, under its own name.
 *
 * Copied, not linked: a template changed next month does not change the
 * people already hired from it, any more than rewriting a job advert changes
 * the person who answered the last one.
 */
func FromTemplate(t Template, name string) Agent {
	agent := t

	agent.Name = name
	agent.BuiltIn = false
	agent.State = Active
	agent.HiredFor = 0
	agent.Tools = append([]string{}, t.Tools...)
	agent.Never = append([]string{}, t.Never...)
	agent.Can = append([]string{}, t.Can...)

	return agent
}

// TemplateFor is the template for a job, when there is one.
func TemplateFor(templates []Template, job string) (Template, bool) {
	for _, t := range templates {
		if t.Job == job {
			return t, true
		}
	}

	return Template{}, false
}
