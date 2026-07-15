package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// WikiPublisher converts a task's extracted knowledge (decisions, learnings,
// gotchas) into human-readable wiki markdown pages. It closes the gap where
// adb accumulated knowledge in tickets/<id>/knowledge/decisions.yaml but
// never published it anywhere a human or a knowledge base could read.
//
// Output pages carry YAML frontmatter (title/created/updated/tags/source)
// so they drop cleanly into a markdown wiki. The publisher is a pure
// transform over the KnowledgeExtractor's output plus a filesystem writer;
// it does not know about any specific external wiki, keeping the in-tool
// state authoritative and the wiki a downstream consumer (a one-way EMIT —
// there is no read-back).
//
// Beyond per-ticket pages it emits a navigable, LLM-consumable corpus
// (issue #127): graph-edge cross-links on each page, an index/home page,
// per-tag and per-initiative index pages, org/initiative namespacing, and the
// machine-readable llms.txt + AGENTS.md entrypoints. The graph/classifier/
// indexer seams are all OPTIONAL and nil-safe: with none wired the output is
// the flat per-ticket pages plus a tag index + entrypoints.
type WikiPublisher struct {
	extractor *KnowledgeExtractor
	// now returns the timestamp stamped into frontmatter. Injectable so
	// tests are deterministic; defaults to time.Now().UTC().
	now func() time.Time

	graph      WikiGraph
	classifier WikiClassifier
	indexer    MemoryIndexer
}

// WikiGraph is the (optional) graph seam used to cross-link a page to its 1-hop
// neighbourhood. GraphManager satisfies it structurally.
type WikiGraph interface {
	Neighbors(id string) ([]models.GraphEdge, error)
}

// WikiClassifier maps a task to its org + initiative for namespacing and the
// per-initiative index. Either may be empty (→ the page lands ungrouped at the
// wiki root). Optional.
type WikiClassifier interface {
	Classify(taskID string) (org, initiative string)
}

// NewWikiPublisher builds a WikiPublisher rooted at basePath (the adb
// workspace whose tickets/ holds the per-task knowledge).
func NewWikiPublisher(basePath string) *WikiPublisher {
	return &WikiPublisher{
		extractor: NewKnowledgeExtractor(basePath),
		now:       func() time.Time { return time.Now().UTC() },
	}
}

// SetGraph wires the graph seam so pages get a "Related (graph)" cross-link
// section. Nil-safe: unset → no cross-links.
func (p *WikiPublisher) SetGraph(g WikiGraph) { p.graph = g }

// SetClassifier wires org/initiative namespacing + the per-initiative index.
// Nil-safe: unset → flat, ungrouped pages.
func (p *WikiPublisher) SetClassifier(c WikiClassifier) { p.classifier = c }

// SetIndexer wires the vector store so published pages are indexed for semantic
// search (ns wiki/<task-id>). Nil-safe: unset → no indexing.
func (p *WikiPublisher) SetIndexer(m MemoryIndexer) { p.indexer = m }

// PublishResult summarises a publish run.
type PublishResult struct {
	OutDir       string
	TasksScanned int
	PagesWritten []string // relative ticket-knowledge pages written under OutDir
	IndexPages   []string // navigation artifacts: index.md, tags/*, initiatives/*, llms.txt, AGENTS.md
	Skipped      []string // task IDs skipped (no knowledge / unreadable)
	Indexed      int      // pages indexed into the vector store (0 unless an indexer is wired)
}

// pageMeta is the metadata collected per published ticket page, used to build
// the index / tag / initiative pages and the machine entrypoints.
type pageMeta struct {
	taskID     string
	title      string
	relPath    string // forward-slash path under OutDir
	tags       []string
	org        string
	initiative string
}

// PublishAll renders one wiki page per task that has extracted knowledge, then
// the navigation corpus (index/home, per-tag + per-initiative indexes, llms.txt,
// AGENTS.md). A task with no knowledge entries is recorded in Skipped rather than
// producing an empty page. Output is deterministic (stable ordering).
func (p *WikiPublisher) PublishAll(outDir string) (*PublishResult, error) {
	if outDir == "" {
		return nil, fmt.Errorf("outDir cannot be empty")
	}
	taskIDs, err := p.extractor.ListAllKnowledge()
	if err != nil {
		return nil, fmt.Errorf("listing knowledge: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}

	result := &PublishResult{OutDir: outDir, TasksScanned: len(taskIDs)}
	sort.Strings(taskIDs) // stable, deterministic output

	var metas []pageMeta
	for _, taskID := range taskIDs {
		knowledge, err := p.extractor.LoadKnowledge(taskID)
		if err != nil || knowledgeIsEmpty(knowledge) {
			result.Skipped = append(result.Skipped, taskID)
			continue
		}

		org, initiative := "", ""
		if p.classifier != nil {
			org, initiative = p.classifier.Classify(taskID)
		}
		relPath := strings.ToLower(taskID) + "-knowledge.md"
		if ns := namespaceDir(org, initiative); ns != "" {
			relPath = ns + "/" + relPath // forward-slash; OS separator applied at write time
		}

		page := p.renderPage(knowledge, org, initiative)
		abs := filepath.Join(outDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return result, fmt.Errorf("creating page dir for %s: %w", taskID, err)
		}
		if err := os.WriteFile(abs, []byte(page), 0o644); err != nil {
			return result, fmt.Errorf("writing %s: %w", relPath, err)
		}
		result.PagesWritten = append(result.PagesWritten, relPath)

		metas = append(metas, pageMeta{
			taskID: taskID, title: taskID + " Knowledge", relPath: relPath,
			tags: pageTags(knowledge), org: org, initiative: initiative,
		})

		if p.indexer != nil {
			// Indexing is best-effort and secondary to publishing: a memory hiccup
			// must not fail the wiki emit, but it is surfaced (not silently swallowed).
			if err := p.indexer.Upsert(context.Background(), "wiki/"+taskID, "page", page, map[string]string{"source": "wiki", "task_id": taskID}); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: wiki semantic-index %s: %v\n", taskID, err)
			} else {
				result.Indexed++
			}
		}
	}

	// Navigation corpus only when there is something to navigate.
	if len(metas) > 0 {
		idx, err := p.writeNavigation(outDir, metas)
		if err != nil {
			return result, err
		}
		result.IndexPages = idx
	}
	return result, nil
}

// namespaceDir returns the forward-slash sub-directory a page lives under given
// its org + initiative (both optional). Empty → the wiki root. It returns
// forward slashes directly (relPath is always forward-slash; the OS separator is
// only applied when touching the filesystem via filepath.FromSlash).
func namespaceDir(org, initiative string) string {
	switch {
	case org != "" && initiative != "":
		return org + "/" + initiative
	case org != "":
		return org
	case initiative != "":
		return initiative
	default:
		return ""
	}
}

// pageTags is the union of a page's frontmatter tags and every per-item tag,
// deduped + sorted — the set a tag index buckets the page under.
func pageTags(k *models.ExtractedKnowledge) []string {
	set := map[string]struct{}{"adb": {}, "knowledge": {}, "task-knowledge": {}}
	for _, d := range k.Decisions {
		for _, t := range d.Tags {
			set[t] = struct{}{}
		}
	}
	for _, l := range k.Learnings {
		for _, t := range l.Tags {
			set[t] = struct{}{}
		}
	}
	for _, g := range k.Gotchas {
		for _, t := range g.Tags {
			set[t] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// knowledgeIsEmpty reports whether there is nothing worth publishing.
func knowledgeIsEmpty(k *models.ExtractedKnowledge) bool {
	return k == nil || (len(k.Decisions) == 0 && len(k.Learnings) == 0 && len(k.Gotchas) == 0)
}

// renderPage renders a single task's knowledge as a wiki markdown page,
// appending a graph cross-link section when a graph is wired.
func (p *WikiPublisher) renderPage(k *models.ExtractedKnowledge, org, initiative string) string {
	ts := p.now().Format("2006-01-02")
	var b strings.Builder

	// Frontmatter — matches the common title/created/updated/tags/source
	// shape used by file-based wikis.
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: %s Knowledge\n", k.TaskID)
	fmt.Fprintf(&b, "created: %s\n", ts)
	fmt.Fprintf(&b, "updated: %s\n", ts)
	b.WriteString("tags: [adb, knowledge, task-knowledge]\n")
	if org != "" {
		fmt.Fprintf(&b, "org: %s\n", org)
	}
	if initiative != "" {
		fmt.Fprintf(&b, "initiative: %s\n", initiative)
	}
	fmt.Fprintf(&b, "source: tickets/%s/knowledge/decisions.yaml\n", k.TaskID)
	b.WriteString("---\n\n")

	fmt.Fprintf(&b, "# %s — Extracted Knowledge\n\n", k.TaskID)
	if k.Summary != "" {
		fmt.Fprintf(&b, "%s\n\n", k.Summary)
	}

	if len(k.Decisions) > 0 {
		b.WriteString("## Decisions\n\n")
		for _, d := range k.Decisions {
			renderDecision(&b, d)
		}
	}
	if len(k.Learnings) > 0 {
		b.WriteString("## Learnings\n\n")
		for _, l := range k.Learnings {
			renderLearning(&b, l)
		}
	}
	if len(k.Gotchas) > 0 {
		b.WriteString("## Gotchas\n\n")
		for _, g := range k.Gotchas {
			renderGotcha(&b, g)
		}
	}
	p.renderRelated(&b, k.TaskID)
	return b.String()
}

// renderRelated appends the ticket's 1-hop graph neighbourhood as a cross-link
// section. No-op when no graph is wired or the ticket has no incident edges.
func (p *WikiPublisher) renderRelated(b *strings.Builder, taskID string) {
	if p.graph == nil {
		return
	}
	edges, err := p.graph.Neighbors(taskID)
	if err != nil || len(edges) == 0 {
		return
	}
	// Sort locally so the rendered corpus is deterministic regardless of the
	// WikiGraph implementation's return order.
	sort.Slice(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.To < b.To
	})
	b.WriteString("## Related (graph)\n\n")
	for _, e := range edges {
		fmt.Fprintf(b, "- `%s` --%s--> `%s`\n", e.From, e.Type, e.To)
	}
	b.WriteString("\n")
}

// writeNavigation writes the index/home page, per-tag + per-initiative index
// pages, and the llms.txt + AGENTS.md entrypoints. Returns their relative paths.
func (p *WikiPublisher) writeNavigation(outDir string, metas []pageMeta) ([]string, error) {
	var written []string
	write := func(rel, content string) error {
		abs := filepath.Join(outDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return fmt.Errorf("creating dir for %s: %w", rel, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", rel, err)
		}
		written = append(written, rel)
		return nil
	}

	// Buckets: tag -> pages, initiative -> pages.
	byTag := map[string][]pageMeta{}
	byInitiative := map[string][]pageMeta{}
	for _, m := range metas {
		for _, t := range m.tags {
			byTag[t] = append(byTag[t], m)
		}
		if m.initiative != "" {
			byInitiative[m.initiative] = append(byInitiative[m.initiative], m)
		}
	}

	tagKeys := sortedBucketKeys(byTag)
	initKeys := sortedBucketKeys(byInitiative)

	if err := write("index.md", renderIndex(metas, tagKeys, initKeys, p.now())); err != nil {
		return written, err
	}
	for _, tag := range tagKeys {
		if err := write(filepath.ToSlash(filepath.Join("tags", tag+".md")), renderListPage("Tag: "+tag, byTag[tag], "tags")); err != nil {
			return written, err
		}
	}
	for _, init := range initKeys {
		if err := write(filepath.ToSlash(filepath.Join("initiatives", init+".md")), renderListPage("Initiative: "+init, byInitiative[init], "initiatives")); err != nil {
			return written, err
		}
	}
	if err := write("llms.txt", renderLLMsTxt(metas)); err != nil {
		return written, err
	}
	if err := write("AGENTS.md", renderAgentsMd(metas, tagKeys, initKeys)); err != nil {
		return written, err
	}
	return written, nil
}

// sortedBucketKeys returns the sorted keys of a tag/initiative → pages bucket.
func sortedBucketKeys(m map[string][]pageMeta) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func renderIndex(metas []pageMeta, tags, initiatives []string, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Knowledge wiki\n\n_Generated %s from adb ticket knowledge._\n\n", now.Format("2006-01-02"))
	if len(initiatives) > 0 {
		b.WriteString("## By initiative\n\n")
		for _, in := range initiatives {
			fmt.Fprintf(&b, "- [%s](initiatives/%s.md)\n", in, in)
		}
		b.WriteString("\n")
	}
	b.WriteString("## By tag\n\n")
	for _, t := range tags {
		fmt.Fprintf(&b, "- [%s](tags/%s.md)\n", t, t)
	}
	b.WriteString("\n## All pages\n\n")
	for _, m := range sortMetas(metas) {
		fmt.Fprintf(&b, "- [%s](%s)\n", m.title, m.relPath)
	}
	return b.String()
}

// renderListPage renders a tag / initiative index page linking to its member
// pages. relFrom is the sub-directory the page lives in, so links step back up.
func renderListPage(heading string, metas []pageMeta, relFrom string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", heading)
	for _, m := range sortMetas(metas) {
		fmt.Fprintf(&b, "- [%s](%s)\n", m.title, relLink(relFrom, m.relPath))
	}
	return b.String()
}

func renderLLMsTxt(metas []pageMeta) string {
	// llms.txt convention: a titled, sectioned link list an LLM reads to navigate.
	var b strings.Builder
	b.WriteString("# adb knowledge wiki\n\n")
	b.WriteString("> Extracted decisions, learnings, and gotchas from adb tickets. Each link is a markdown page.\n\n")
	b.WriteString("## Pages\n\n")
	for _, m := range sortMetas(metas) {
		fmt.Fprintf(&b, "- [%s](%s)\n", m.title, m.relPath)
	}
	return b.String()
}

func renderAgentsMd(metas []pageMeta, tags, initiatives []string) string {
	var b strings.Builder
	b.WriteString("# AGENTS.md — navigating this knowledge wiki\n\n")
	b.WriteString("This wiki is generated by `adb sync wiki` from ticket knowledge (one-way emit; the in-tool state is authoritative). To find something:\n\n")
	b.WriteString("- Start at `index.md` for the full page list.\n")
	if len(initiatives) > 0 {
		b.WriteString("- Browse by initiative under `initiatives/` (a page per initiative).\n")
	}
	b.WriteString("- Browse by tag under `tags/` (a page per tag).\n")
	b.WriteString("- Each page ends with a `Related (graph)` section linking its graph neighbours.\n\n")
	fmt.Fprintf(&b, "Corpus: %d page(s), %d tag(s), %d initiative(s).\n", len(metas), len(tags), len(initiatives))
	return b.String()
}

// relLink builds a link from an index page living in the sub-directory relFrom
// (forward-slash, relative to the wiki root) to a target whose path is relative
// to the wiki root. It steps up one level per relFrom segment, so it stays
// correct if index pages ever nest deeper than the current one-level tags/ and
// initiatives/ dirs (the target's own depth is irrelevant — it descends from root).
func relLink(relFrom, targetRel string) string {
	up := strings.Count(strings.Trim(relFrom, "/"), "/") + 1
	return strings.Repeat("../", up) + targetRel
}

func sortMetas(metas []pageMeta) []pageMeta {
	out := make([]pageMeta, len(metas))
	copy(out, metas)
	sort.Slice(out, func(i, j int) bool { return out[i].relPath < out[j].relPath })
	return out
}

func renderDecision(b *strings.Builder, d models.Decision) {
	title := d.Title
	if d.Status != "" {
		title = fmt.Sprintf("%s `%s`", d.Title, d.Status)
	}
	fmt.Fprintf(b, "### %s\n\n", title)
	if d.Description != "" {
		fmt.Fprintf(b, "%s\n\n", d.Description)
	}
	if d.Rationale != "" {
		fmt.Fprintf(b, "**Rationale:** %s\n\n", d.Rationale)
	}
	if len(d.Alternatives) > 0 {
		b.WriteString("**Alternatives considered:**\n")
		for _, a := range d.Alternatives {
			fmt.Fprintf(b, "- %s\n", a)
		}
		b.WriteString("\n")
	}
	if len(d.Consequences) > 0 {
		b.WriteString("**Consequences:**\n")
		for _, c := range d.Consequences {
			fmt.Fprintf(b, "- %s\n", c)
		}
		b.WriteString("\n")
	}
}

func renderLearning(b *strings.Builder, l models.Learning) {
	title := l.Title
	if l.Category != "" {
		title = fmt.Sprintf("%s `%s`", l.Title, l.Category)
	}
	fmt.Fprintf(b, "### %s\n\n", title)
	if l.Description != "" {
		fmt.Fprintf(b, "%s\n\n", l.Description)
	}
}

func renderGotcha(b *strings.Builder, g models.Gotcha) {
	title := g.Title
	if g.Severity != "" {
		title = fmt.Sprintf("%s `%s`", g.Title, g.Severity)
	}
	fmt.Fprintf(b, "### %s\n\n", title)
	if g.Description != "" {
		fmt.Fprintf(b, "%s\n\n", g.Description)
	}
	if g.Solution != "" {
		fmt.Fprintf(b, "**Solution:** %s\n\n", g.Solution)
	}
	if g.Prevention != "" {
		fmt.Fprintf(b, "**Prevention:** %s\n\n", g.Prevention)
	}
}
