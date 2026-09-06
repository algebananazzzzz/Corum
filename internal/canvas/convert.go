// Package canvas captures configured Canvas material as deterministic raw files.
package canvas

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/dslipak/pdf"
	"golang.org/x/net/html"
	"gopkg.in/yaml.v3"
)

// StripVerifier removes Canvas's user-specific verifier query parameter.
func StripVerifier(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.RawQuery == "" {
		return raw
	}
	kept := make([]string, 0)
	for _, part := range strings.Split(u.RawQuery, "&") {
		key := part
		if index := strings.IndexByte(key, '='); index >= 0 {
			key = key[:index]
		}
		decoded, decodeErr := url.QueryUnescape(key)
		if decodeErr == nil && decoded == "verifier" {
			continue
		}
		kept = append(kept, part)
	}
	u.RawQuery = strings.Join(kept, "&")
	return u.String()
}

// LocalTime renders a Canvas RFC3339 instant in the configured IANA timezone.
func LocalTime(raw, zone string) (string, error) {
	if raw == "" {
		return "", nil
	}
	moment, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return "", err
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return "", err
	}
	return moment.In(location).Format(time.RFC3339), nil
}

// Slug produces a bounded, stable filename component while keeping lesson dots.
func Slug(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '.' || unicode.IsSpace(r) || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	value = strings.Trim(b.String(), "-._ ")
	value = regexp.MustCompile(`[\s_-]+`).ReplaceAllString(value, "-")
	if value == "" {
		return "untitled"
	}
	if len(value) <= 60 {
		return value
	}
	cut := value[:60]
	if value[60] != '-' {
		if i := strings.LastIndexByte(cut, '-'); i > 0 {
			cut = cut[:i]
		}
	}
	return strings.Trim(cut, "-.")
}

func scrubHTML(source string) (string, error) {
	node, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return "", err
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for child := n.FirstChild; child != nil; {
			next := child.NextSibling
			walk(child)
			child = next
		}
		if n.Type != html.ElementNode {
			return
		}
		if n.Data == "script" && attribute(n, "src") != "" && strings.Contains(attribute(n, "src"), "BlueCanvasMobileConfig.js") {
			n.Parent.RemoveChild(n)
			return
		}
		classes := strings.Fields(attribute(n, "class"))
		for _, class := range classes {
			if class == "external_link_icon" || class == "screenreader-only" {
				n.Parent.RemoveChild(n)
				return
			}
		}
		attrs := n.Attr[:0]
		for _, a := range n.Attr {
			key := strings.ToLower(a.Key)
			if strings.HasPrefix(key, "data-") || key == "class" {
				continue
			}
			if key == "href" || key == "src" {
				a.Val = StripVerifier(a.Val)
			}
			attrs = append(attrs, a)
		}
		n.Attr = attrs
	}
	walk(node)
	var out bytes.Buffer
	if err := html.Render(&out, node); err != nil {
		return "", err
	}
	return out.String(), nil
}
func attribute(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

// ToMarkdown converts scrubbed Canvas HTML with the pure-Go converter.
func ToMarkdown(source string) (string, error) {
	return ToMarkdownImages(source, nil)
}

// ToMarkdownImages also rewrites successfully captured inline images to local paths.
func ToMarkdownImages(source string, imagePaths map[string]string) (string, error) {
	clean, err := scrubHTML(source)
	if err != nil {
		return "", err
	}
	for remote, local := range imagePaths {
		clean = strings.ReplaceAll(clean, remote, local)
	}
	value, err := htmltomarkdown.ConvertString(clean)
	if err != nil {
		return "", err
	}
	value = regexp.MustCompile(`[ \t]+\n`).ReplaceAllString(value, "\n")
	value = regexp.MustCompile(`\n{3,}`).ReplaceAllString(value, "\n\n")
	return strings.TrimSpace(value) + "\n", nil
}

// Images returns inline image sources with Canvas verifiers removed.
func Images(source string) ([]string, error) {
	clean, err := scrubHTML(source)
	if err != nil {
		return nil, err
	}
	node, err := html.Parse(strings.NewReader(clean))
	if err != nil {
		return nil, err
	}
	images := []string{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" && attribute(n, "src") != "" {
			images = append(images, attribute(n, "src"))
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return images, nil
}

type Link struct {
	Type string `yaml:"type" json:"type"`
	Text string `yaml:"text" json:"text"`
	URL  string `yaml:"url" json:"url"`
}

func Links(source, origin string) ([]Link, error) {
	clean, err := scrubHTML(source)
	if err != nil {
		return nil, err
	}
	node, err := html.Parse(strings.NewReader(clean))
	if err != nil {
		return nil, err
	}
	result, seen := []Link{}, map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			href := attribute(n, "href")
			if href != "" && !strings.HasPrefix(href, "#") && !seen[href] {
				kind := "external"
				if strings.Contains(href, "/files/") {
					kind = "file"
				} else if origin != "" && strings.HasPrefix(href, strings.TrimRight(origin, "/")) {
					kind = "internal"
				}
				text := strings.TrimSpace(nodeText(n))
				if text == "" {
					text = href
				}
				result = append(result, Link{kind, text, href})
				seen[href] = true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(node)
	return result, nil
}
func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
			b.WriteByte(' ')
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// Frontmatter produces stable YAML metadata for captured raw material.
func Frontmatter(fields map[string]any, links []Link) (string, error) {
	payload := map[string]any{}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if fields[k] != nil {
			payload[k] = fields[k]
		}
	}
	if len(links) > 0 {
		payload["links"] = links
	}
	data, err := yaml.Marshal(payload)
	if err != nil {
		return "", err
	}
	return "---\n" + string(data) + "---\n\n", nil
}

// PDFText extracts plain text without CGO and normalizes whitespace.
func PDFText(reader io.ReaderAt, size int64) (string, error) {
	p, err := pdf.NewReader(reader, size)
	if err != nil {
		return "", err
	}
	text, err := p.GetPlainText()
	if err != nil {
		return "", err
	}
	data, err := io.ReadAll(text)
	if err != nil {
		return "", err
	}
	normalized := strings.Join(strings.Fields(string(data)), " ")
	if normalized == "" {
		return "", fmt.Errorf("PDF contains no extractable text")
	}
	return normalized + "\n", nil
}
