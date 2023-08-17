package utils

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// ReadURI opens and read an url and return its content
func readURI(uri string, fs fs.FS) ([]byte, error) {
	url, err := url.Parse(uri)
	if err != nil || url.Scheme == "" {
		data, err := fs.Open(uri) //os.Open(filepath.Join(path, uri))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(data)
	}
	if err != nil {
		return nil, err
	}

	resp, err := http.Get(uri)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(resp.Body)
}

// InlineHTML replaces:
//
// <script src="..."></script>
// with
// <script>content of ...</script>
//
// and
//
// <link rel="stylesheet" href="..." />
// with
// <style>content of ...</style>
//
// parameter is parsed html to modify in place
// see InlineAssets for a string to string version
func InlineHTML(n *html.Node, fs fs.FS, doRemote bool) error {
	if n.Type == html.ElementNode {
		// <script src="..."></script>
		if n.Data == "script" {
			newAttr := make([]html.Attribute, 0, len(n.Attr))
			for _, s := range n.Attr {
				if s.Key == "src" {
					// check the source
					url, err := url.Parse(s.Val)
					if err != nil {
						return err
					}
					if (url.Host != "" && doRemote) || url.Host == "" {
						script, err := readURI(s.Val, fs)
						if err != nil {
							return err
						}
						js := html.Node{
							Type: html.TextNode,
							Data: "\n" + string(script) + "\n",
						}
						n.FirstChild = &js
					} else {
						newAttr = append(newAttr, s)
					}
				} else {
					newAttr = append(newAttr, s)
				}
			}
			n.Attr = newAttr
		}
		// <link rel="stylesheet" type="text/css" href="..." />
		if n.Data == "link" {
			isCSS := false
			href := ""
			for _, s := range n.Attr {
				if s.Key == "rel" && s.Val == "stylesheet" {
					isCSS = true
				}
				if s.Key == "href" {
					href = s.Val
				}
			}
			if isCSS && href != "" {
				n.Data = "style"
				n.Attr = nil

				css, err := readURI(href, fs)
				if err != nil {
					return err
				}

				n.FirstChild = &html.Node{
					Type: html.TextNode,
					Data: "\n" + string(css) + "\n",
				}
			}
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		err := InlineHTML(child, fs, doRemote)
		if err != nil {
			return err
		}
	}
	return nil
}

// InlineAssets takes parses source as HTML and replace all
// script & style import elements with their content
// and returns the corresponding HTML.
func InlineAssets(source string, fs fs.FS, doRemote bool) (string, error) {
	root, err := html.Parse(strings.NewReader(source))
	if err != nil {
		panic(err)
	}

	err = InlineHTML(root, fs, doRemote)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)
	if err = html.Render(buf, root); err == nil {
		return buf.String(), nil
	}
	return "", err
}
