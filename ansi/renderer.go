package ansi

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	east "github.com/yuin/goldmark-emoji/ast"
	"github.com/yuin/goldmark/ast"
	astext "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// Options is used to configure an ANSIRenderer.
type Options struct {
	BaseURL          string
	WordWrap         int
	TableWrap        *bool
	InlineTableLinks bool
	PreserveNewLines bool
	Styles           StyleConfig
	ChromaFormatter  string
}

// ANSIRenderer renders markdown content as ANSI escaped sequences.
type ANSIRenderer struct { //nolint: revive
	context         RenderContext
	customRenderers []util.PrioritizedValue
}

// NewRenderer returns a new ANSIRenderer with style and options set.
func NewRenderer(options Options) *ANSIRenderer {
	return &ANSIRenderer{
		context: NewRenderContext(options),
	}
}

// NewRendererWithCustom returns a new ANSIRenderer with custom node renderers
// that write to glamour's block stack buffer instead of the final output.
func NewRendererWithCustom(options Options, customRenderers []util.PrioritizedValue) *ANSIRenderer {
	return &ANSIRenderer{
		context:         NewRenderContext(options),
		customRenderers: customRenderers,
	}
}

// wrappingRegisterer wraps a NodeRendererFuncRegisterer and redirects the
// writer passed to registered functions to glamour's current block stack
// buffer, so custom renderer output lands in the correct position.
type wrappingRegisterer struct {
	inner   renderer.NodeRendererFuncRegisterer
	context *RenderContext
}

func (r *wrappingRegisterer) Register(kind ast.NodeKind, fn renderer.NodeRendererFunc) {
	r.inner.Register(kind, func(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
		bs := r.context.blockStack
		if bs.Len() > 0 {
			return fn(&blockBufWriter{bs.Current().Block}, src, node, entering)
		}
		return fn(w, src, node, entering)
	})
}

// blockBufWriter adapts *bytes.Buffer to util.BufWriter. bytes.Buffer writes
// are unbuffered so Buffered always returns 0.
type blockBufWriter struct {
	*bytes.Buffer
}

func (b *blockBufWriter) Buffered() int { return 0 }
func (b *blockBufWriter) Flush() error  { return nil }

// RegisterFuncs implements NodeRenderer.RegisterFuncs.
func (r *ANSIRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	// blocks
	reg.Register(ast.KindDocument, r.renderNode)
	reg.Register(ast.KindHeading, r.renderNode)
	reg.Register(ast.KindBlockquote, r.renderNode)
	reg.Register(ast.KindCodeBlock, r.renderNode)
	reg.Register(ast.KindFencedCodeBlock, r.renderNode)
	reg.Register(ast.KindHTMLBlock, r.renderNode)
	reg.Register(ast.KindList, r.renderNode)
	reg.Register(ast.KindListItem, r.renderNode)
	reg.Register(ast.KindParagraph, r.renderNode)
	reg.Register(ast.KindTextBlock, r.renderNode)
	reg.Register(ast.KindThematicBreak, r.renderNode)

	// inlines
	reg.Register(ast.KindAutoLink, r.renderNode)
	reg.Register(ast.KindCodeSpan, r.renderNode)
	reg.Register(ast.KindEmphasis, r.renderNode)
	reg.Register(ast.KindImage, r.renderNode)
	reg.Register(ast.KindLink, r.renderNode)
	reg.Register(ast.KindRawHTML, r.renderNode)
	reg.Register(ast.KindText, r.renderNode)
	reg.Register(ast.KindString, r.renderNode)

	// tables
	reg.Register(astext.KindTable, r.renderNode)
	reg.Register(astext.KindTableHeader, r.renderNode)
	reg.Register(astext.KindTableRow, r.renderNode)
	reg.Register(astext.KindTableCell, r.renderNode)

	// definitions
	reg.Register(astext.KindDefinitionList, r.renderNode)
	reg.Register(astext.KindDefinitionTerm, r.renderNode)
	reg.Register(astext.KindDefinitionDescription, r.renderNode)

	// footnotes
	reg.Register(astext.KindFootnote, r.renderNode)
	reg.Register(astext.KindFootnoteList, r.renderNode)
	reg.Register(astext.KindFootnoteLink, r.renderNode)
	reg.Register(astext.KindFootnoteBacklink, r.renderNode)

	// checkboxes
	reg.Register(astext.KindTaskCheckBox, r.renderNode)

	// strikethrough
	reg.Register(astext.KindStrikethrough, r.renderNode)

	// emoji
	reg.Register(east.KindEmoji, r.renderNode)

	// Custom renderers are registered after glamour's defaults so they override
	// them for the same node kinds. Sort descending by priority value so that
	// lower-value (higher-precedence) renderers are registered last and win.
	if len(r.customRenderers) > 0 {
		wr := &wrappingRegisterer{inner: reg, context: &r.context}
		sorted := make([]util.PrioritizedValue, len(r.customRenderers))
		copy(sorted, r.customRenderers)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Priority > sorted[j].Priority
		})
		for _, pv := range sorted {
			if cr, ok := pv.Value.(renderer.NodeRenderer); ok {
				cr.RegisterFuncs(wr)
			}
		}
	}
}

func (r *ANSIRenderer) renderNode(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	writeTo := io.Writer(w)
	bs := r.context.blockStack

	// children get rendered by their parent
	if isChild(node) {
		return ast.WalkContinue, nil
	}

	e := r.NewElement(node, source)
	if entering { //nolint: nestif
		// everything below the Document element gets rendered into a block buffer
		if bs.Len() > 0 {
			writeTo = io.Writer(bs.Current().Block)
		}

		_, _ = io.WriteString(writeTo, e.Entering)
		if e.Renderer != nil {
			err := e.Renderer.Render(writeTo, r.context)
			if err != nil {
				return ast.WalkStop, fmt.Errorf("glamour: error rendering: %w", err)
			}
		}
	} else {
		// everything below the Document element gets rendered into a block buffer
		if bs.Len() > 0 {
			writeTo = io.Writer(bs.Parent().Block)
		}

		// if we're finished rendering the entire document,
		// flush to the real writer
		if node.Type() == ast.TypeDocument {
			writeTo = w
		}

		if e.Finisher != nil {
			err := e.Finisher.Finish(writeTo, r.context)
			if err != nil {
				return ast.WalkStop, fmt.Errorf("glamour: error finishing render: %w", err)
			}
		}

		_, _ = io.WriteString(bs.Current().Block, e.Exiting)
	}

	return ast.WalkContinue, nil
}

func isChild(node ast.Node) bool {
	for n := node.Parent(); n != nil; n = n.Parent() {
		// These types are already rendered by their parent
		switch n.Kind() {
		case ast.KindCodeSpan, ast.KindAutoLink, ast.KindLink, ast.KindImage, ast.KindEmphasis, astext.KindStrikethrough, astext.KindTableCell:
			return true
		}
	}

	return false
}

func resolveRelativeURL(baseURL string, rel string) string {
	u, err := url.Parse(rel)
	if err != nil {
		return rel
	}
	if u.IsAbs() {
		return rel
	}
	u.Path = strings.TrimPrefix(u.Path, "/")

	base, err := url.Parse(baseURL)
	if err != nil {
		return rel
	}
	return base.ResolveReference(u).String()
}
