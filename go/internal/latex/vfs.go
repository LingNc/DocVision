package latex

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Mount is one root of a session's virtual workspace. The model never
// sees absolute host paths: it addresses files through mount names.
type Mount struct {
	Name     string // "work" (the session's own directory), "project", ...
	Dir      string // real directory
	Writable bool
}

// VFS is the session's virtual workspace: a small mount table that
// turns model-supplied paths into real paths and enforces read/write
// rights. Every session gets ONE namespace built from its mounts.
//
// Path syntax accepted from the model:
//
//	chapters/ch1.tex      -> default mount (first writable, else first)
//	project:chapters/a.md -> the mount named "project"
//	/project/chapters/a.md-> same
//	.  /  (empty)         -> the default mount's directory
//
// Traversal outside a mount ("../") is rejected.
type VFS struct {
	Mounts []Mount
}

// DefaultMount returns the mount used for plain relative paths.
func (v *VFS) DefaultMount(write bool) (Mount, error) {
	for _, m := range v.Mounts {
		if m.Dir == "" {
			continue
		}
		if write && !m.Writable {
			continue
		}
		return m, nil
	}
	if write {
		return Mount{}, fmt.Errorf("没有可写挂载点")
	}
	return Mount{}, fmt.Errorf("没有挂载点")
}

// Lookup finds a mount by name.
func (v *VFS) Lookup(name string) (Mount, bool) {
	for _, m := range v.Mounts {
		if m.Name == name && m.Dir != "" {
			return m, true
		}
	}
	return Mount{}, false
}

// Resolve maps a model path to a real path. write=true requires the
// target mount to be writable. The returned label is the mount name.
func (v *VFS) Resolve(p string, write bool) (full, label string, err error) {
	p = strings.TrimSpace(p)
	explicit := ""
	if strings.HasPrefix(p, "/") {
		p = strings.TrimPrefix(p, "/")
		if i := strings.Index(p, "/"); i >= 0 {
			explicit, p = p[:i], p[i+1:]
		} else {
			explicit, p = p, "."
		}
	} else if i := strings.Index(p, ":"); i > 0 {
		explicit, p = p[:i], p[i+1:]
	}
	var m Mount
	if explicit != "" {
		mm, ok := v.Lookup(explicit)
		if !ok {
			return "", "", fmt.Errorf("未知挂载点 %q（可用: %s）", explicit, v.Names())
		}
		m = mm
	} else {
		mm, derr := v.DefaultMount(write)
		if derr != nil {
			return "", "", derr
		}
		m = mm
	}
	if write && !m.Writable {
		return "", "", fmt.Errorf("挂载点 %q 是只读的", m.Name)
	}
	// Tolerate a REDUNDANT mount name inside the path: models mix the two
	// syntaxes and write "source:/source/book.pdf" (mount prefix + the bash
	// style absolute path). Without this it fell through to resolveInside,
	// which rejected the absolute remainder with a confusing
	// "路径越界（绝对路径）" instead of simply finding the file.
	if explicit != "" {
		rest := strings.TrimPrefix(p, "/")
		if trimmed := strings.TrimPrefix(rest, explicit); trimmed != rest {
			if trimmed == "" || strings.HasPrefix(trimmed, "/") {
				p = strings.TrimPrefix(trimmed, "/")
			}
		}
	}
	if p == "" {
		p = "."
	}
	full, rerr := resolveInside(m.Dir, p)
	if rerr != nil {
		return "", "", rerr
	}
	return full, m.Name, nil
}

// Names lists the mount names (for error messages / tool descriptions).
func (v *VFS) Names() string {
	var names []string
	for _, m := range v.Mounts {
		if m.Dir == "" {
			continue
		}
		n := m.Name
		if m.Writable {
			n += "(rw)"
		} else {
			n += "(ro)"
		}
		names = append(names, n)
	}
	return strings.Join(names, ", ")
}

// stripMount removes the explicit mount prefix from a path.
func stripMount(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "/") {
		rest := strings.TrimPrefix(p, "/")
		if i := strings.Index(rest, "/"); i > 0 {
			return rest[i+1:]
		}
		return "."
	}
	if i := strings.Index(p, ":"); i > 0 {
		return p[i+1:]
	}
	return p
}

// suggestInDir lists up to limit entries of dir as a "（可用: a, b, c）"
// suffix for "file not found" errors. Sessions used to get a bare
// "文件不存在: source:book_part1.pdf" and then burned several rounds
// guessing names (the real name differed by a suffix/part number); naming
// the neighbours turns that into a single round.
func suggestInDir(dir string, limit int) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, ".") {
			continue
		}
		if e.IsDir() {
			n += "/"
		}
		names = append(names, n)
		if len(names) >= limit {
			break
		}
	}
	if len(names) == 0 {
		return "（该目录为空）"
	}
	sort.Strings(names)
	return "（该目录可用: " + strings.Join(names, ", ") + "）"
}

// suggestNear is suggestInDir for a path that may not exist: it falls back
// to the deepest existing ancestor directory, so a missing file still gets
// a useful listing.
func suggestNear(full string, limit int) string {
	dir := filepath.Dir(full)
	for i := 0; i < 4; i++ {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return suggestInDir(dir, limit)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// Describe renders the mount table for a tool description.
func (v *VFS) Describe() string {
	if len(v.Mounts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Mounts: ")
	b.WriteString(v.Names())
	b.WriteString(". Plain paths are relative to ")
	if d, err := v.DefaultMount(false); err == nil {
		b.WriteString(d.Name)
	} else {
		b.WriteString("the first mount")
	}
	b.WriteString("; use \"name:path\" or \"/name/path\" for another mount.")
	return b.String()
}

// vfsFrom builds a VFS from a primary workspace root plus read-only
// extra roots (the historic ReadFileTool shape).
func vfsFrom(root string, alt []AltRoot) *VFS {
	v := &VFS{}
	if root != "" {
		v.Mounts = append(v.Mounts, Mount{Name: "work", Dir: root, Writable: true})
	}
	for _, a := range alt {
		if a.Dir == "" {
			continue
		}
		name := a.Label
		if name == "" {
			name = filepath.Base(a.Dir)
		}
		v.Mounts = append(v.Mounts, Mount{Name: name, Dir: a.Dir})
	}
	return v
}
