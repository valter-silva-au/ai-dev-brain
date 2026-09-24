package profile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	texttemplate "text/template"
	"text/template/parse"
)

const (
	maxTemplateBytes         = 1 << 20
	maxRenderDataBytes       = 1 << 20
	maxRenderedArtifactBytes = 4 << 20
	maxTotalRenderedBytes    = 16 << 20
	maxRenderedArtifacts     = 256
)

type Template struct {
	ID          string `json:"id" yaml:"id"`
	Version     string `json:"version" yaml:"version"`
	SourceScope string `json:"source_scope" yaml:"source_scope"`
	Content     []byte `json:"-" yaml:"-"`
}

type TemplateResolver interface {
	ResolveTemplate(TemplateReference) (Template, error)
}

type TemplateCatalog struct {
	templates map[string]Template
}

type RenderRequest struct {
	Profile  Profile
	Resolver TemplateResolver
	Data     map[string]string
}

type RenderedFile struct {
	Role    string
	Path    string
	Content []byte
}

type RenderResult struct {
	Files    []RenderedFile
	Manifest RenderManifest
}

func Render(request RenderRequest) (RenderResult, error) {
	if err := validateResolvedProfile(request.Profile); err != nil {
		return RenderResult{}, fmt.Errorf("validate render profile: %w", err)
	}
	if request.Resolver == nil {
		return RenderResult{}, errors.New("template resolver is required")
	}
	if err := validateRenderData(request.Data); err != nil {
		return RenderResult{}, err
	}

	artifacts := make([]Artifact, 0, len(request.Profile.Artifacts))
	for _, artifact := range request.Profile.Artifacts {
		if artifact.Template != nil {
			artifacts = append(artifacts, cloneArtifact(artifact))
		}
	}
	sort.Slice(artifacts, func(left, right int) bool {
		return artifacts[left].Role < artifacts[right].Role
	})
	if len(artifacts) > maxRenderedArtifacts {
		return RenderResult{}, errors.New(
			"profile declares too many rendered artifacts",
		)
	}

	result := RenderResult{
		Files: make([]RenderedFile, 0, len(artifacts)),
		Manifest: RenderManifest{
			SchemaVersion: RenderManifestSchemaVersion,
			Profile:       profileReferenceOf(request.Profile),
			Artifacts:     make([]RenderedRecord, 0, len(artifacts)),
		},
	}
	totalRenderedBytes := 0
	for _, artifact := range artifacts {
		reference := *artifact.Template
		candidate, err := request.Resolver.ResolveTemplate(reference)
		if err != nil {
			return RenderResult{}, fmt.Errorf(
				"resolve template %s@%s required by %s",
				reference.ID,
				reference.Version,
				artifact.Role,
			)
		}
		if err := validateTemplate(candidate); err != nil {
			return RenderResult{}, err
		}
		if candidate.ID != reference.ID ||
			candidate.Version != reference.Version {
			return RenderResult{}, fmt.Errorf(
				"template resolver returned mismatched identity for %s@%s",
				reference.ID,
				reference.Version,
			)
		}
		templateText := strings.ReplaceAll(
			string(candidate.Content),
			"\r\n",
			"\n",
		)
		templateText = strings.ReplaceAll(templateText, "\r", "\n")
		parsed, err := texttemplate.New(candidate.ID).
			Option("missingkey=error").
			Parse(templateText)
		if err != nil {
			return RenderResult{}, fmt.Errorf(
				"parse template %s@%s",
				candidate.ID,
				candidate.Version,
			)
		}
		if err := validateTemplateTree(parsed.Tree.Root); err != nil {
			return RenderResult{}, fmt.Errorf(
				"template %s@%s uses an unsupported action",
				candidate.ID,
				candidate.Version,
			)
		}
		output := boundedBuffer{limit: maxRenderedArtifactBytes}
		if err := parsed.Execute(&output, request.Data); err != nil {
			return RenderResult{}, fmt.Errorf(
				"render template %s@%s",
				candidate.ID,
				candidate.Version,
			)
		}
		renderedText := strings.ReplaceAll(output.String(), "\r\n", "\n")
		renderedText = strings.ReplaceAll(renderedText, "\r", "\n")
		content := []byte(renderedText)
		totalRenderedBytes += len(content)
		if totalRenderedBytes > maxTotalRenderedBytes {
			return RenderResult{}, errors.New(
				"rendered artifacts exceed the aggregate size limit",
			)
		}
		hash := HashContent(content)
		result.Files = append(result.Files, RenderedFile{
			Role:    artifact.Role,
			Path:    artifact.Path,
			Content: content,
		})
		result.Manifest.Artifacts = append(
			result.Manifest.Artifacts,
			RenderedRecord{
				Role: artifact.Role,
				Path: artifact.Path,
				Template: TemplateProvenance{
					ID:          candidate.ID,
					Version:     candidate.Version,
					SourceScope: candidate.SourceScope,
				},
				GeneratedHash: hash,
				ObservedHash:  hash,
			},
		)
	}
	if err := ValidateRenderManifest(result.Manifest); err != nil {
		return RenderResult{}, fmt.Errorf("validate render result: %w", err)
	}
	return cloneRenderResult(result), nil
}

func NewTemplateCatalog(templates []Template) (*TemplateCatalog, error) {
	result := &TemplateCatalog{
		templates: make(map[string]Template, len(templates)),
	}
	for _, candidate := range templates {
		if err := validateTemplate(candidate); err != nil {
			return nil, err
		}
		key := templateKey(TemplateReference{
			ID:      candidate.ID,
			Version: candidate.Version,
		})
		if _, exists := result.templates[key]; exists {
			return nil, fmt.Errorf(
				"duplicate template identity %s@%s",
				candidate.ID,
				candidate.Version,
			)
		}
		candidate.Content = append([]byte(nil), candidate.Content...)
		result.templates[key] = candidate
	}
	return result, nil
}

func (catalog *TemplateCatalog) ResolveTemplate(
	reference TemplateReference,
) (Template, error) {
	if catalog == nil {
		return Template{}, errors.New("template catalog is nil")
	}
	candidate, exists := catalog.templates[templateKey(reference)]
	if !exists {
		return Template{}, fmt.Errorf(
			"template %s@%s was not found",
			reference.ID,
			reference.Version,
		)
	}
	candidate.Content = append([]byte(nil), candidate.Content...)
	return candidate, nil
}

func HashContent(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func validateTemplate(value Template) error {
	if err := validateTemplateReference(TemplateReference{
		ID:      value.ID,
		Version: value.Version,
	}); err != nil {
		return err
	}
	if strings.TrimSpace(value.SourceScope) == "" ||
		value.SourceScope != strings.TrimSpace(value.SourceScope) {
		return errors.New("template source scope is required")
	}
	if !validTemplateSourceScope(value.SourceScope) {
		return errors.New("template source scope is not recognized")
	}
	if len(value.Content) == 0 {
		return errors.New("template content is required")
	}
	if len(value.Content) > maxTemplateBytes {
		return errors.New("template exceeds the supported size limit")
	}
	return nil
}

func validateRenderData(data map[string]string) error {
	total := 0
	for key, value := range data {
		total += len(key) + len(value)
		if total > maxRenderDataBytes {
			return errors.New("render data exceeds the supported size limit")
		}
	}
	return nil
}

func validateTemplateTree(node parse.Node) error {
	if node == nil {
		return nil
	}
	switch value := node.(type) {
	case *parse.ListNode:
		for _, child := range value.Nodes {
			if err := validateTemplateTree(child); err != nil {
				return err
			}
		}
	case *parse.TextNode, *parse.CommentNode, *parse.DotNode,
		*parse.FieldNode, *parse.StringNode, *parse.NumberNode,
		*parse.BoolNode, *parse.NilNode:
		return nil
	case *parse.ActionNode:
		return validateTemplateTree(value.Pipe)
	case *parse.PipeNode:
		if len(value.Decl) != 0 {
			return errors.New("template variables are not supported")
		}
		for _, command := range value.Cmds {
			if err := validateTemplateTree(command); err != nil {
				return err
			}
		}
	case *parse.CommandNode:
		if len(value.Args) != 1 {
			return errors.New("template functions are not supported")
		}
		return validateTemplateTree(value.Args[0])
	case *parse.IfNode:
		if err := validateTemplateTree(value.Pipe); err != nil {
			return err
		}
		if err := validateTemplateTree(value.List); err != nil {
			return err
		}
		return validateTemplateTree(value.ElseList)
	case *parse.RangeNode:
		if err := validateTemplateTree(value.Pipe); err != nil {
			return err
		}
		if err := validateTemplateTree(value.List); err != nil {
			return err
		}
		return validateTemplateTree(value.ElseList)
	case *parse.WithNode:
		if err := validateTemplateTree(value.Pipe); err != nil {
			return err
		}
		if err := validateTemplateTree(value.List); err != nil {
			return err
		}
		return validateTemplateTree(value.ElseList)
	default:
		return errors.New("template action is not supported")
	}
	return nil
}

type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (buffer *boundedBuffer) Write(content []byte) (int, error) {
	if len(content) > buffer.limit-buffer.Len() {
		return 0, errors.New("rendered artifact exceeds the supported size limit")
	}
	return buffer.Buffer.Write(content)
}

var _ io.Writer = (*boundedBuffer)(nil)

func templateKey(reference TemplateReference) string {
	return reference.ID + "\x00" + reference.Version
}

func validTemplateSourceScope(value string) bool {
	switch value {
	case "builtin",
		"user_global",
		"workspace",
		"organization",
		"host",
		"owner",
		"repository",
		"ticket":
		return true
	default:
		return false
	}
}

func cloneRenderResult(value RenderResult) RenderResult {
	result := value
	result.Files = make([]RenderedFile, 0, len(value.Files))
	for _, file := range value.Files {
		file.Content = append([]byte(nil), file.Content...)
		result.Files = append(result.Files, file)
	}
	result.Manifest = cloneRenderManifest(value.Manifest)
	return result
}
