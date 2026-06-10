package glamour

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"charm.land/glamour/v2/styles"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

const markdown = "testdata/readme.markdown.in"

func TestTermRendererWriter(t *testing.T) {
	r, err := NewTermRenderer(
		WithStandardStyle(styles.DarkStyle),
	)
	if err != nil {
		t.Fatal(err)
	}

	in, err := os.ReadFile(markdown)
	if err != nil {
		t.Fatal(err)
	}

	_, err = r.Write(in)
	if err != nil {
		t.Fatal(err)
	}
	err = r.Close()
	if err != nil {
		t.Fatal(err)
	}

	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	golden.RequireEqual(t, b)
}

func TestTermRenderer(t *testing.T) {
	r, err := NewTermRenderer(
		WithStandardStyle("dark"),
	)
	if err != nil {
		t.Fatal(err)
	}

	in, err := os.ReadFile(markdown)
	if err != nil {
		t.Fatal(err)
	}

	b, err := r.Render(string(in))
	if err != nil {
		t.Fatal(err)
	}

	golden.RequireEqual(t, []byte(b))
}

func TestWithEmoji(t *testing.T) {
	r, err := NewTermRenderer(
		WithEmoji(),
	)
	if err != nil {
		t.Fatal(err)
	}

	b, err := r.Render(":+1:")
	if err != nil {
		t.Fatal(err)
	}
	b = strings.TrimSpace(b)

	// Thumbs up unicode character
	td := "\U0001f44d"

	if td != b {
		t.Errorf("Rendered output doesn't match!\nExpected: `\n%s`\nGot: `\n%s`\n", td, b)
	}
}

func TestWithPreservedNewLines(t *testing.T) {
	r, err := NewTermRenderer(
		WithPreservedNewLines(),
	)
	if err != nil {
		t.Fatal(err)
	}

	in, err := os.ReadFile("testdata/preserved_newline.in")
	if err != nil {
		t.Fatal(err)
	}

	b, err := r.Render(string(in))
	if err != nil {
		t.Fatal(err)
	}

	golden.RequireEqual(t, []byte(b))
}

func TestStyles(t *testing.T) {
	_, err := NewTermRenderer()
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTermRenderer(
		WithStandardStyle(styles.DarkStyle),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTermRenderer(
		WithEnvironmentConfig(),
	)
	if err != nil {
		t.Fatal(err)
	}
}

// TestCustomStyle checks the expected errors with custom styling. We need to
// support built-in styles and custom style sheets.
func TestCustomStyle(t *testing.T) {
	md := "testdata/example.md"
	tests := []struct {
		name      string
		stylePath string
		err       error
		expected  string
	}{
		{name: "style exists", stylePath: "testdata/custom.style", err: nil, expected: "testdata/custom.style"},
		{name: "style doesn't exist", stylePath: "testdata/notfound.style", err: os.ErrNotExist, expected: styles.DarkStyle},
		{name: "style is empty", stylePath: "", err: nil, expected: styles.DarkStyle},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GLAMOUR_STYLE", tc.stylePath)
			g, err := NewTermRenderer(
				WithEnvironmentConfig(),
			)
			if !errors.Is(err, tc.err) {
				t.Fatal(err)
			}
			if !errors.Is(tc.err, os.ErrNotExist) {
				w, err := NewTermRenderer(WithStylePath(tc.expected))
				if err != nil {
					t.Fatal(err)
				}
				text, _ := os.ReadFile(md)
				want, err := w.RenderBytes(text)
				got, err := g.RenderBytes(text)
				if !bytes.Equal(want, got) {
					t.Error("Wrong style used")
				}
			}
		})
	}
}

func TestRenderHelpers(t *testing.T) {
	in, err := os.ReadFile(markdown)
	if err != nil {
		t.Fatal(err)
	}

	b, err := Render(string(in), "dark")
	if err != nil {
		t.Error(err)
	}

	golden.RequireEqual(t, []byte(b))
}

func TestCapitalization(t *testing.T) {
	p := true
	style := styles.DarkStyleConfig
	style.H1.Upper = &p
	style.H2.Title = &p
	style.H3.Lower = &p

	r, err := NewTermRenderer(
		WithStyles(style),
	)
	if err != nil {
		t.Fatal(err)
	}

	b, err := r.Render("# everything is uppercase\n## everything is titled\n### everything is lowercase")
	if err != nil {
		t.Fatal(err)
	}

	golden.RequireEqual(t, []byte(b))
}

func FuzzData(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		func() int {
			_, err := RenderBytes(data, styles.DarkStyle)
			if err != nil {
				return 0
			}
			return 1
		}()
	})
}

func TestTableAscii(t *testing.T) {
	markdown := strings.TrimSpace(`
| Header A  | Header B  |
| --------- | --------- |
| Cell 1    | Cell 2    |
| Cell 3    | Cell 4    |
| Cell 5    | Cell 6    |
`)

	renderer, err := NewTermRenderer(
		WithStyles(styles.ASCIIStyleConfig),
		WithWordWrap(80),
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := renderer.Render(markdown)
	if err != nil {
		t.Fatal(err)
	}

	nonAsciiRegexp := regexp.MustCompile(`[^\x00-\x7f]+`)
	nonAsciiChars := nonAsciiRegexp.FindAllString(result, -1)
	if len(nonAsciiChars) > 0 {
		t.Errorf("Non-ASCII characters found in output: %v", nonAsciiChars)
	}
}

func ExampleASCIIStyleConfig() {
	markdown := strings.TrimSpace(`
| Header A  | Header B  |
| --------- | --------- |
| Cell 1    | Cell 2    |
| Cell 3    | Cell 4    |
| Cell 5    | Cell 6    |
`)

	renderer, err := NewTermRenderer(
		WithStyles(styles.ASCIIStyleConfig),
		WithWordWrap(80),
	)
	if err != nil {
		return
	}

	result, err := renderer.Render(markdown)
	if err != nil {
		return
	}
	result = strings.ReplaceAll(result, " ", ".")
	fmt.Println(result)

	// Output:
	// ..............................................................................
	// ...Header.A.............................|.Header.B............................
	// ..--------------------------------------|-------------------------------------
	// ...Cell.1...............................|.Cell.2..............................
	// ...Cell.3...............................|.Cell.4..............................
	// ...Cell.5...............................|.Cell.6..............................
}

func TestWithChromaFormatterDefault(t *testing.T) {
	r, err := NewTermRenderer(
		WithStandardStyle(styles.DarkStyle),
	)
	if err != nil {
		t.Fatal(err)
	}

	in, err := os.ReadFile("testdata/TestWithChromaFormatter.md")
	if err != nil {
		t.Fatal(err)
	}

	b, err := r.Render(string(in))
	if err != nil {
		t.Fatal(err)
	}

	golden.RequireEqual(t, []byte(b))
}

func TestWithChromaFormatterCustom(t *testing.T) {
	r, err := NewTermRenderer(
		WithStandardStyle(styles.DarkStyle),
		WithChromaFormatter("terminal16"),
	)
	if err != nil {
		t.Fatal(err)
	}

	in, err := os.ReadFile("testdata/TestWithChromaFormatter.md")
	if err != nil {
		t.Fatal(err)
	}

	b, err := r.Render(string(in))
	if err != nil {
		t.Fatal(err)
	}

	golden.RequireEqual(t, []byte(b))
}

type testLinkRenderer struct {
	called bool
}

func (r *testLinkRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindLink, r.renderLink)
}

func (r *testLinkRenderer) renderLink(
	w util.BufWriter, _ []byte, node ast.Node, entering bool,
) (ast.WalkStatus, error) {
	r.called = true
	if entering {
		n := node.(*ast.Link)
		_, _ = w.WriteString("[CUSTOM:" + string(n.Destination) + "]")
	} else {
		_, _ = w.WriteString("[/CUSTOM]")
	}
	return ast.WalkContinue, nil
}

func TestWithNodeRenderers(t *testing.T) {
	lr := &testLinkRenderer{}
	r, err := NewTermRenderer(
		WithStandardStyle(styles.DarkStyle),
		WithNodeRenderers(util.Prioritized(lr, 500)),
	)
	if err != nil {
		t.Fatal(err)
	}

	out, err := r.Render("[click here](https://example.com)")
	if err != nil {
		t.Fatal(err)
	}

	if !lr.called {
		t.Fatal("custom node renderer was not called")
	}
	if !strings.Contains(out, "[CUSTOM:https://example.com]") {
		t.Errorf("expected custom link output, got: %s", out)
	}
}

func TestWithNodeRenderers_LowerPriorityWins(t *testing.T) {
	first := &testLinkRenderer{}
	second := &testLinkRenderer{}
	r, err := NewTermRenderer(
		WithStandardStyle(styles.DarkStyle),
		WithNodeRenderers(
			util.Prioritized(first, 100),
			util.Prioritized(second, 200),
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = r.Render("[link](https://example.com)")
	if err != nil {
		t.Fatal(err)
	}

	if !first.called {
		t.Fatal("higher-priority (lower value) renderer was not called")
	}
	if second.called {
		t.Fatal("lower-priority renderer should not have been called")
	}
}

type testCodeBlockRenderer struct {
	called bool
}

func (r *testCodeBlockRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.renderCodeBlock)
}

func (r *testCodeBlockRenderer) renderCodeBlock(
	w util.BufWriter, _ []byte, _ ast.Node, entering bool,
) (ast.WalkStatus, error) {
	r.called = true
	if entering {
		_, _ = w.WriteString("[CUSTOM_CODE]")
		return ast.WalkSkipChildren, nil
	}
	return ast.WalkContinue, nil
}

func TestWithNodeRenderers_BlockElement(t *testing.T) {
	cr := &testCodeBlockRenderer{}
	r, err := NewTermRenderer(
		WithStandardStyle(styles.DarkStyle),
		WithNodeRenderers(util.Prioritized(cr, 500)),
	)
	if err != nil {
		t.Fatal(err)
	}

	out, err := r.Render("before\n\n```\ncode\n```\n\nafter")
	if err != nil {
		t.Fatal(err)
	}

	if !cr.called {
		t.Fatal("custom code block renderer was not called")
	}

	stripped := regexp.MustCompile(`\x1b\[[^m]*m`).ReplaceAllString(out, "")
	beforeIdx := strings.Index(stripped, "before")
	customIdx := strings.Index(stripped, "[CUSTOM_CODE]")
	afterIdx := strings.Index(stripped, "after")

	if beforeIdx == -1 || customIdx == -1 || afterIdx == -1 {
		t.Fatalf("expected all content in output, got: %q", stripped)
	}
	if beforeIdx > customIdx || customIdx > afterIdx {
		t.Errorf("content out of order: before=%d [CUSTOM_CODE]=%d after=%d in: %q",
			beforeIdx, customIdx, afterIdx, stripped)
	}
}

func TestWithNodeRenderers_Empty(t *testing.T) {
	r, err := NewTermRenderer(
		WithStandardStyle(styles.DarkStyle),
		WithNodeRenderers(),
	)
	if err != nil {
		t.Fatal(err)
	}

	out, err := r.Render("hello world")
	if err != nil {
		t.Fatal(err)
	}

	stripped := regexp.MustCompile(`\x1b\[[^m]*m`).ReplaceAllString(out, "")
	if !strings.Contains(stripped, "hello world") {
		t.Errorf("expected default rendering, got: %s", out)
	}
}
