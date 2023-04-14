package utils

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// InlineScriptHTML replaces <script src="..."></script> with <script>content of ...</script>
// parameter is parsed html to modify in place
// see InlineScript for a string to string version
func InlineScriptHTML(n *html.Node) error {
	if n.Type == html.ElementNode {
		if n.Data == "script" {
			for _, s := range n.Attr {
				if s.Key == "src" {
					script, err := ReadURI(s.Val)
					if err != nil {
						return err
					}
					js := html.Node{
						Type: html.TextNode,
						Data: "\n" + string(script) + "\n",
					}
					n.FirstChild = &js
				}
			}
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		err := InlineScriptHTML(child)
		if err != nil {
			return err
		}
	}
	return nil
}

// InlineScript takes parses source as HTML and replace all
// script elements <script src="some uri"></script>
// with <script>content of uri</script>
// and returns the corresponding HTML
func InlineScript(source string) (string, error) {
	root, err := html.Parse(strings.NewReader(source))
	if err != nil {
		panic(err)
	}

	err = InlineScriptHTML(root)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)
	if err = html.Render(buf, root); err == nil {
		return buf.String(), nil
	}
	return "", err
}
