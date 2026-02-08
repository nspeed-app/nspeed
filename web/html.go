// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

package web

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/net/html"
)

// ReadURI opens and read an uri and return its content
func readURI(uri string, fs fs.FS) ([]byte, error) {
	url, err := url.Parse(uri)
	if err != nil || url.Scheme == "" {
		data, err := fs.Open(uri) //os.Open(filepath.Join(path, uri))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(data)
	}
	resp, err := http.Get(uri)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http.Get %s returned : %s (%d)", uri, http.StatusText(resp.StatusCode), resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// find 'import ...;" pattern and remove them
// works onyl for single line import
// this will add a return carriage at the end if not present
func removeImport(js string) string {
	keyword := "import "
	lenK := len(keyword)

	scanner := bufio.NewScanner(strings.NewReader(js))
	out := ""
	for scanner.Scan() {
		line := scanner.Text()
		l := strings.TrimSpace(line)
		if len(l) > lenK {
			if l[0:lenK] == keyword && l[len(l)-1] == ';' {
				continue
			}
		}
		out += line + "\n"
	}
	return out
}

// Inline processes an HTML node tree, inlining external resources like scripts and stylesheets.
//
// It traverses the HTML node tree, searching for <script> and <link> elements.
//
// For <script> elements with a "src" attribute:
//   - It attempts to read the content of the script from the provided filesystem (fs) or via HTTP.
//   - If successful, it replaces the <script> element's content with the fetched script code.
//   - If the script tag has the attribute `removeimports="true"`, it will remove any `import ...;` statements from the script (this allows inlining of js modules)
//   - If the URL is remote and doRemote is false, the script tag is left untouched.
//
// For <link> elements with "rel" set to "stylesheet" and an "href" attribute:
//   - It attempts to read the content of the stylesheet from the provided filesystem (fs) or via HTTP (if doRemote is true).
//   - If successful, it transforms the <link> element into a <style> element and inlines the fetched CSS code.
//
// Parameters:
//   - node: The current HTML node being processed.
//   - fs: The filesystem to use for reading local resources.
//   - doRemote: A boolean flag indicating whether to fetch remote resources via HTTP.
//
// Returns:
//   - An error if any operation fails (e.g., reading a resource, parsing a URL).
//   - nil if the operation is successful.
func Inline(node *html.Node, fs fs.FS, doRemote bool) error {
	if node.Type == html.ElementNode {
		// <script src="..."></script>
		if node.Data == "script" {
			newAttr := make([]html.Attribute, 0, len(node.Attr))
			removeImports := false
			for _, s := range node.Attr {
				if s.Key == "removeimports" {
					removeImports = s.Val == "true"
				}
			}
			for _, s := range node.Attr {
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
						code := string(script)
						if removeImports {
							code = removeImport(code)
						}
						js := html.Node{
							Type: html.TextNode,
							Data: "\n" + code + "\n",
						}
						node.FirstChild = &js
					} else {
						newAttr = append(newAttr, s)
					}
				} else {
					newAttr = append(newAttr, s)
				}
			}
			node.Attr = newAttr
		}
		// <link rel="stylesheet" type="text/css" href="..." />
		if node.Data == "link" {
			isCSS := false
			href := ""
			for _, s := range node.Attr {
				if s.Key == "rel" && s.Val == "stylesheet" {
					isCSS = true
				}
				if s.Key == "href" {
					href = s.Val
				}
			}
			if isCSS && href != "" {
				node.Data = "style"
				node.Attr = nil

				css, err := readURI(href, fs)
				if err != nil {
					return err
				}

				node.FirstChild = &html.Node{
					Type: html.TextNode,
					Data: "\n" + string(css) + "\n",
				}
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		err := Inline(child, fs, doRemote)
		if err != nil {
			return err
		}
	}
	return nil
}

// InlineFromString processes an HTML string, inlining external resources like scripts and stylesheets.
//
// See Inline for more details
func InlineFromString(source string, fs fs.FS, doRemote bool) (string, error) {
	root, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return "", fmt.Errorf("html parse failed:%w", err)
	}

	err = Inline(root, fs, doRemote)
	if err != nil {
		return "", fmt.Errorf("inline html failed:%w", err)
	}

	buf := new(bytes.Buffer)
	if err = html.Render(buf, root); err == nil {
		return buf.String(), nil
	}
	return "", err
}

// OpenURI opens an url or a file
func OpenURI(uri string) (io.ReadCloser, error) {
	url, err := url.Parse(uri)
	if err != nil || url.Scheme == "" {
		return os.Open(uri)
	}
	resp, err := http.Get(uri)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// ReadURI opens and read an url and return its content
func ReadURI(uri string) ([]byte, error) {
	r, err := OpenURI(uri)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}
