package org

import "sort"

/*
 * The organisation the program ships with.
 *
 * Small, and that is the discipline rather than a shortcut. An organisation
 * that looks busy routes badly: every seat is another thing to choose
 * between, and two that differ only in name make the choice harder for
 * nothing. Five departments and ten seats, six of which somebody already
 * holds — and four deliberately empty, because a vacancy is how this program
 * says "there is a place for somebody who does SEO here" instead of being
 * silent about everything it has not got.
 *
 * Nobody is named here. A seat is a job held with a certain responsibility;
 * who sits in it is the roster's business, and keeping the two apart is what
 * lets two backend engineers with different models hold the same position.
 */
func BuiltIn() []Unit {
	return []Unit{
		{
			Name: "practice", Title: "The practice", Kind: Company, BuiltIn: true,
			Purpose: "Everything done here, and everyone who does it. The person " +
				"it works for is at the top of this chart, not in it.",
		},
		{
			Name: "engineering", Title: "Engineering", Kind: Department,
			Parent: "practice", BuiltIn: true,
			Purpose: "Building software, and working out why something does not " +
				"build or run.",
			Seats: []Position{
				{
					Name: "lead_developer", Title: "Lead developer",
					Job: "software.software_engineer", Seniority: Lead,
				},
				{
					Name: "backend_engineer", Title: "Backend engineer",
					Job: "software.backend_engineer", Seniority: Senior,
					ReportsTo: "lead_developer",
					Needs:     []string{"postgresql", "php", "laravel"},
				},
				{
					Name: "game_developer", Title: "Game developer",
					Job: "software.game_developer", Seniority: Mid,
					ReportsTo: "lead_developer",
				},
			},
		},
		{
			Name: "studio", Title: "Studio", Kind: Department,
			Parent: "practice", BuiltIn: true,
			Purpose: "Anything to be looked at or read: pictures, films, " +
				"documents, the words on a page.",
			Seats: []Position{
				{
					Name: "designer", Title: "Designer",
					Job: "design.graphic_designer", Seniority: Senior,
				},
				{
					Name: "writer", Title: "Writer",
					Job: "media.writer", Seniority: Senior,
				},
			},
		},
		{
			Name: "market", Title: "Market", Kind: Department,
			Parent: "practice", BuiltIn: true,
			Purpose: "Being found, and being understood once found.",
			Seats: []Position{
				{
					Name: "seo_specialist", Title: "SEO specialist",
					Job: "marketing.seo_specialist", Seniority: Mid,
				},
				{
					Name: "marketer", Title: "Marketer",
					Job: "marketing.marketing_manager", Seniority: Senior,
				},
			},
		},
		{
			Name: "office", Title: "Office", Kind: Department,
			Parent: "practice", BuiltIn: true,
			Purpose: "The diary, the post, the machine itself, and the books.",
			Seats: []Position{
				{
					Name: "assistant", Title: "Assistant",
					Job: "general.personal_assistant", Seniority: Senior,
				},
				{
					/*
					 * Empty, and the one to think hardest about filling.
					 *
					 * Bookkeeping is filed as high risk and asks for a person
					 * to stay in the loop, so an agent in this seat would stop
					 * and ask far more than the others. That is correct rather
					 * than inconvenient: the cost of a wrong entry here is not
					 * a wrong answer, it is a wrong filing.
					 */
					Name: "bookkeeper", Title: "Bookkeeper",
					Job: "finance.bookkeeper", Seniority: Mid,
					Never: []string{"send_email"},
				},
			},
		},
		{
			Name: "knowledge", Title: "Knowledge", Kind: Department,
			Parent: "practice", BuiltIn: true,
			Purpose: "Finding things out, and saying where each thing came from.",
			Seats: []Position{
				{
					Name: "researcher", Title: "Researcher",
					Job: "general.researcher", Seniority: Senior,
				},
			},
		},
	}
}

/*
 * Shape is the chart in reading order: every unit after the one it sits in.
 *
 * Anything that cannot be reached from a top-level unit is appended at the
 * end rather than dropped — a unit whose parent was renamed, or two that name
 * each other. Dropping it would be the worse failure by far: the file is
 * still on disk, the seats in it still exist, and the only thing missing
 * would be any sign of them.
 */
func Shape(chart []Unit) []Unit {
	children := map[string][]Unit{}
	known := map[string]bool{}

	for _, u := range chart {
		known[u.Name] = true
	}

	roots := []Unit{}

	for _, u := range chart {
		if u.Parent == "" || !known[u.Parent] || u.Parent == u.Name {
			roots = append(roots, u)

			continue
		}

		children[u.Parent] = append(children[u.Parent], u)
	}

	sort.SliceStable(roots, byKind(roots))

	for parent := range children {
		sort.SliceStable(children[parent], byKind(children[parent]))
	}

	out := make([]Unit, 0, len(chart))
	seen := map[string]bool{}

	var walk func(u Unit)

	walk = func(u Unit) {
		if seen[u.Name] {
			return
		}

		seen[u.Name] = true
		out = append(out, u)

		for _, child := range children[u.Name] {
			walk(child)
		}
	}

	for _, root := range roots {
		walk(root)
	}

	// Whatever a cycle swallowed. Visible, at the end, rather than gone.
	for _, u := range chart {
		if !seen[u.Name] {
			seen[u.Name] = true
			out = append(out, u)
		}
	}

	return out
}

// Bigger things first, then alphabetically, so a chart reads the way an
// organisation is usually drawn rather than the way a folder is listed.
func byKind(units []Unit) func(i, j int) bool {
	rank := map[string]int{Company: 0, Division: 1, Department: 2, Team: 3}

	return func(i, j int) bool {
		a, b := rank[units[i].Kind], rank[units[j].Kind]

		if a != b {
			return a < b
		}

		return units[i].Title < units[j].Title
	}
}

// DepthOf is how far in each unit sits, for drawing the chart. Anything
// unreachable is at the top level, which is where Shape puts it.
func DepthOf(chart []Unit) map[string]int {
	known := map[string]bool{}

	for _, u := range chart {
		known[u.Name] = true
	}

	parent := map[string]string{}

	for _, u := range chart {
		if u.Parent != u.Name && known[u.Parent] {
			parent[u.Name] = u.Parent
		}
	}

	out := map[string]int{}

	for _, u := range chart {
		depth, at := 0, u.Name

		// Bounded by the number of units, so a cycle costs a walk rather than
		// a hang. Nothing on this machine should be able to freeze the
		// interface because two files name each other.
		for range chart {
			up, ok := parent[at]
			if !ok {
				break
			}

			depth++
			at = up
		}

		out[u.Name] = depth
	}

	return out
}

// Under is the units directly inside one.
func Under(chart []Unit, parent string) []Unit {
	out := []Unit{}

	for _, u := range chart {
		if u.Parent == parent && u.Name != parent {
			out = append(out, u)
		}
	}

	sort.SliceStable(out, byKind(out))

	return out
}

/*
 * Manages is the seats that answer to one.
 *
 * Used for escalation rather than for permission: a seat's manager is who a
 * problem goes to, and never a way to borrow what that manager may do. That
 * distinction is the whole of why delegation intersects authority instead of
 * inheriting it.
 */
func Manages(chart []Unit, position string) []Position {
	out := []Position{}

	for _, seat := range Seats(chart) {
		if seat.ReportsTo == position && seat.Name != position {
			out = append(out, seat)
		}
	}

	return out
}

/*
 * AnswersTo walks up the reporting line, nearest first.
 *
 * Stops on a seat that answers to nobody, which is the ordinary case here:
 * the person at the top of this chart is a person. Bounded like the depth
 * walk, so two seats naming each other cost a loop and not the program.
 */
func AnswersTo(chart []Unit, position string) []Position {
	seats := Seats(chart)

	byName := map[string]Position{}

	for _, seat := range seats {
		byName[seat.Name] = seat
	}

	out := []Position{}
	seen := map[string]bool{position: true}

	at, ok := byName[position]
	if !ok {
		return out
	}

	for range seats {
		next, ok := byName[at.ReportsTo]

		if !ok || seen[next.Name] {
			break
		}

		seen[next.Name] = true
		out = append(out, next)
		at = next
	}

	return out
}
