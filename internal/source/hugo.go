package source

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

var (
	// hugoRefLink is a Markdown link whose target is a ref or relref shortcode. The link text may
	// hold one level of brackets, as in [`spec.kubelet`].
	hugoRefLink = regexp.MustCompile(`\[((?:[^\[\]]|\[[^\[\]]*\])*)\]\(\s*(\{\{[<%][^}]*[>%]\}\})\s*\)`)
	hugoRef     = regexp.MustCompile(`^\{\{[<%]\s*(?:rel)?ref\s+(?:path\s*=\s*)?"([^"]*)"[^}]*[>%]\}\}$`)
	// hugoBlock is a line that holds only an opening or closing block shortcode, such as
	// {{% alert title="Warning" %}} or {{< /tabpane >}}.
	hugoBlock = regexp.MustCompile(`^\s*\{\{[<%]\s*/?\s*([A-Za-z][\w-]*)(?:\s[^}]*)?[>%]\}\}\s*$`)
	langDir   = regexp.MustCompile(`^[a-z]{2}(?:-[A-Za-z]{2,4})?$`)
)

// hugoContentExts are the content file extensions Hugo drops from a page's path.
func hugoContentExts() []string { return []string{".md", ".markdown", ".html"} }

// hugoLinks rewrites the Hugo shortcodes of md, a section of the content file at file. A link to
// a ref or relref page points at that page on siteURL, or keeps only its text when siteURL is
// empty; a same-page ref keeps its #anchor. Lines that hold only a block shortcode become blank,
// so line numbers stay. dir is the site's content directory; empty means [contentDir].
func hugoLinks(md, file, siteURL, dir string) string {
	if !strings.Contains(md, "{{") {
		return md
	}
	ls := strings.Split(md, "\n")
	for i, l := range ls {
		if m := hugoBlock.FindStringSubmatch(l); m != nil && m[1] != "ref" && m[1] != "relref" {
			ls[i] = ""
		}
	}
	md = strings.Join(ls, "\n")
	if dir == "" {
		dir, _ = contentDir(file)
	}
	return hugoRefLink.ReplaceAllStringFunc(md, func(link string) string {
		m := hugoRefLink.FindStringSubmatch(link)
		ref := hugoRef.FindStringSubmatch(m[2])
		if ref == nil {
			return link
		}
		target := hugoTarget(ref[1], file, siteURL, dir)
		if target == "" {
			return m[1]
		}
		return "[" + m[1] + "](" + target + ")"
	})
}

// contentDir returns the Hugo content directory that holds file: the path up to the last content
// directory, plus the language directory after it, as in website/content/en.
func contentDir(file string) (string, bool) {
	dirs := strings.Split(path.Dir(file), "/")
	for i, d := range slices.Backward(dirs) {
		if d != "content" {
			continue
		}
		end := i + 1
		if end < len(dirs) && langDir.MatchString(dirs[end]) {
			end++
		}
		return strings.Join(dirs[:end], "/"), true
	}
	return "", false
}

// hugoTarget returns the URL a ref path points to, following Hugo's path resolution: an absolute
// path starts at the content directory; a relative one at the page's parent, or for a ../ path
// with an extension at the file's directory. It returns "" when the URL can't be known.
func hugoTarget(ref, file, siteURL, dir string) string {
	p, frag, _ := strings.Cut(ref, "#")
	anchor := ""
	if frag != "" {
		anchor = "#" + frag
	}
	if p == "" {
		return anchor
	}
	if siteURL == "" || dir == "" {
		return ""
	}
	rel, ok := strings.CutPrefix(file, strings.TrimSuffix(dir, "/")+"/")
	if !ok {
		return ""
	}
	logical := path.Clean("/" + p)
	if !strings.HasPrefix(p, "/") {
		logical = path.Join("/", pageBase(rel, p), p)
	}
	for _, ext := range hugoContentExts() {
		logical = strings.TrimSuffix(logical, ext)
	}
	for _, index := range []string{"_index", "index"} {
		if path.Base(logical) == index {
			logical = path.Dir(logical)
		}
	}
	page := strings.Trim(strings.ToLower(logical), "/")
	base := strings.TrimSuffix(siteURL, "/") + "/"
	if page == "" {
		return base + anchor
	}
	return base + page + "/" + anchor
}

// pageBase returns the directory a relative ref resolves from. A leaf bundle's index.md is the
// page of its directory, so its parent is one level up.
func pageBase(rel, ref string) string {
	dir := path.Dir(rel)
	if path.Base(rel) == "index.md" && (!strings.HasPrefix(ref, "../") || path.Ext(ref) == "") {
		return path.Dir(dir)
	}
	return dir
}
