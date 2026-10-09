// Package critique runs a fresh-context adversarial critic over a plan and
// checks that the plan answers every item it raised.
package critique

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/waygatetech/vet/internal/findings"
)

// File is a named input passed to the critic verbatim.
type File struct {
	Name    string
	Content string
}

const instructions = `You are an adversarial reviewer of an implementation plan. You have not seen
the planning conversation; judge the plan only on what is written below.

Find:
- failure-mode: ways the planned change could break, regress, or fail in production.
- acceptance: acceptance criteria that are missing, vague, or untestable.
- overlap: capability that duplicates or should extend an existing concept in concepts.yaml.
- contract: schema, public API, event, config, or CLI changes the plan makes but does not declare in contracts_changed.

Raise only concrete, specific issues. Use severity "blocking" for issues that
must be resolved before implementation, "warning" for real risks, and "info"
for minor notes. An empty findings list is a valid answer.

Reply with only this JSON and nothing else:
{"findings": [{"check": "failure-mode|acceptance|overlap|contract", "severity": "blocking|warning|info", "message": "..."}]}
`

// Prompt builds the critic's entire input. The critic sees nothing else.
func Prompt(ticket, plan, concepts string, context []File) string {
	var b strings.Builder
	b.WriteString(instructions)
	section := func(name, body string) {
		fmt.Fprintf(&b, "\n===== %s =====\n%s\n", name, strings.TrimRight(body, "\n"))
	}
	section("TICKET", ticket)
	section("PLAN", plan)
	section("concepts.yaml", concepts)
	for _, f := range context {
		section("CONTEXT: "+f.Name, f.Content)
	}
	return b.String()
}

// Render formats the report as the critique file, one "## C<n>" item per finding.
func Render(r findings.Report) []byte {
	var b bytes.Buffer
	b.WriteString("# Critique\n\nAnswer every item in the plan frontmatter under critique_responses, keyed by ID.\n")
	if len(r.Findings) == 0 {
		b.WriteString("\nNo items.\n")
	}
	for i, f := range r.Findings {
		fmt.Fprintf(&b, "\n## C%d [%s] %s\n\n%s\n", i+1, f.Severity, f.Check, strings.TrimSpace(f.Message))
	}
	return b.Bytes()
}

var itemID = regexp.MustCompile(`(?m)^## (C\d+) `)

// Check returns the critique item IDs that the plan's critique_responses
// frontmatter leaves unanswered or answers with an empty string.
func Check(critique, plan []byte) ([]string, error) {
	fm, err := Frontmatter(plan)
	if err != nil {
		return nil, err
	}
	var meta struct {
		CritiqueResponses map[string]string `yaml:"critique_responses"`
	}
	if err := yaml.Unmarshal(fm, &meta); err != nil {
		return nil, fmt.Errorf("parsing plan frontmatter: %w", err)
	}
	var missing []string
	for _, m := range itemID.FindAllSubmatch(critique, -1) {
		id := string(m[1])
		if strings.TrimSpace(meta.CritiqueResponses[id]) == "" {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

// Frontmatter returns the YAML between the leading "---" fences, or nil if
// the plan has none.
func Frontmatter(plan []byte) ([]byte, error) {
	plan = bytes.ReplaceAll(plan, []byte("\r\n"), []byte("\n"))
	rest, ok := bytes.CutPrefix(plan, []byte("---\n"))
	if !ok {
		return nil, nil
	}
	fm, _, ok := bytes.Cut(rest, []byte("\n---\n"))
	if !ok {
		return nil, errors.New("plan frontmatter is not closed with ---")
	}
	return fm, nil
}
